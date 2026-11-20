// Package tracker talks to external issue trackers (Jira Cloud/Server REST
// API v2/v3 and GitHub Issues) to create issues for defects and to read
// their status back.
package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Issue is what the tool needs to know about an external issue.
type Issue struct {
	Key    string // PROJ-12 or owner/repo#12
	URL    string // web URL
	Title  string
	Status string // the tracker's own status name
	// Mapped is the matching defect status (open, in_progress, resolved, closed, rejected).
	Mapped string
}

// NewIssue is an issue to create.
type NewIssue struct {
	Title, Description, Severity, Priority string
}

// Client is a connection to one project or repository.
type Client interface {
	Create(ctx context.Context, in NewIssue) (*Issue, error)
	Get(ctx context.Context, key string) (*Issue, error)
	// Check verifies the URL, credentials and project/repository.
	Check(ctx context.Context) error
}

// New returns the client for a tracker configuration.
func New(t *model.DefectTracker, hc *http.Client) (Client, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	switch t.Type {
	case "jira":
		return &jira{cfg: t, hc: hc}, nil
	case "github":
		return &github{cfg: t, hc: hc}, nil
	}
	return nil, fmt.Errorf("no defect tracker is configured for the project")
}

// Error is a failed request to the tracker.
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string {
	b := strings.TrimSpace(e.Body)
	if len(b) > 300 {
		b = b[:300] + "…"
	}
	return fmt.Sprintf("the tracker answered %d: %s", e.Status, b)
}

func do(ctx context.Context, hc *http.Client, method, url string, body any, auth func(*http.Request), out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "qtestframe")
	auth(req)
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach the tracker: %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		return &Error{Status: res.StatusCode, Body: string(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("unexpected answer from the tracker: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- Jira

type jira struct {
	cfg *model.DefectTracker
	hc  *http.Client
}

func (j *jira) auth(r *http.Request) {
	switch {
	case j.cfg.User != "" && j.cfg.Token != "":
		r.SetBasicAuth(j.cfg.User, j.cfg.Token)
	case j.cfg.Token != "":
		r.Header.Set("Authorization", "Bearer "+j.cfg.Token)
	}
}

type jiraIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Resolution *struct {
			Name string `json:"name"`
		} `json:"resolution"`
	} `json:"fields"`
}

// jiraMapped maps a Jira status category and resolution to a defect status.
func jiraMapped(category, resolution string) string {
	switch category {
	case "new":
		return "open"
	case "indeterminate":
		return "in_progress"
	case "done":
		r := strings.ToLower(resolution)
		for _, w := range []string{"won't", "wont", "duplicate", "reject", "declin", "invalid", "cannot reproduce", "not a bug"} {
			if strings.Contains(r, w) {
				return "rejected"
			}
		}
		return "closed"
	}
	return ""
}

func (j *jira) issue(ji *jiraIssue) *Issue {
	res := ""
	if ji.Fields.Resolution != nil {
		res = ji.Fields.Resolution.Name
	}
	return &Issue{Key: ji.Key, URL: j.cfg.URL + "/browse/" + ji.Key, Title: ji.Fields.Summary, Status: ji.Fields.Status.Name,
		Mapped: jiraMapped(ji.Fields.Status.StatusCategory.Key, res)}
}

func (j *jira) Create(ctx context.Context, in NewIssue) (*Issue, error) {
	desc := in.Description
	if in.Severity != "" {
		desc = strings.TrimSpace(desc + "\n\nSeverity: " + in.Severity)
	}
	// API v2 takes plain text descriptions (v3 needs the document format).
	body := map[string]any{"fields": map[string]any{
		"project":     map[string]string{"key": j.cfg.ProjectKey},
		"summary":     in.Title,
		"description": desc,
		"issuetype":   map[string]string{"name": j.cfg.IssueType},
	}}
	var created struct {
		Key string `json:"key"`
	}
	if err := do(ctx, j.hc, http.MethodPost, j.cfg.URL+"/rest/api/2/issue", body, j.auth, &created); err != nil {
		return nil, err
	}
	return j.Get(ctx, created.Key)
}

func (j *jira) Get(ctx context.Context, key string) (*Issue, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	var ji jiraIssue
	if err := do(ctx, j.hc, http.MethodGet, j.cfg.URL+"/rest/api/2/issue/"+key+"?fields=summary,status,resolution", nil, j.auth, &ji); err != nil {
		return nil, err
	}
	return j.issue(&ji), nil
}

func (j *jira) Check(ctx context.Context) error {
	return do(ctx, j.hc, http.MethodGet, j.cfg.URL+"/rest/api/2/project/"+j.cfg.ProjectKey, nil, j.auth, nil)
}

// ---------------------------------------------------------------- GitHub

type github struct {
	cfg *model.DefectTracker
	hc  *http.Client
}

func (g *github) auth(r *http.Request) {
	r.Header.Set("Accept", "application/vnd.github+json")
	if g.cfg.Token != "" {
		r.Header.Set("Authorization", "Bearer "+g.cfg.Token)
	}
}

type ghIssue struct {
	Number      int    `json:"number"`
	HTMLURL     string `json:"html_url"`
	Title       string `json:"title"`
	State       string `json:"state"`
	StateReason string `json:"state_reason"`
}

func ghMapped(state, reason string) string {
	if state == "closed" {
		if reason == "not_planned" {
			return "rejected"
		}
		return "closed"
	}
	return "open"
}

func (g *github) issue(gi *ghIssue) *Issue {
	st := gi.State
	if gi.StateReason != "" && gi.State == "closed" {
		st += " (" + gi.StateReason + ")"
	}
	return &Issue{Key: g.cfg.Repo + "#" + strconv.Itoa(gi.Number), URL: gi.HTMLURL, Title: gi.Title, Status: st, Mapped: ghMapped(gi.State, gi.StateReason)}
}

// number extracts the issue number from "owner/repo#12", "#12" or "12".
func number(key string) (string, error) {
	k := strings.TrimSpace(key)
	if i := strings.LastIndex(k, "#"); i >= 0 {
		k = k[i+1:]
	}
	if _, err := strconv.Atoi(k); err != nil {
		return "", fmt.Errorf("a GitHub issue is referred to as owner/repo#123 or #123")
	}
	return k, nil
}

func (g *github) Create(ctx context.Context, in NewIssue) (*Issue, error) {
	body := map[string]any{"title": in.Title, "body": in.Description}
	var labels []string
	for _, l := range strings.Split(g.cfg.Labels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			labels = append(labels, l)
		}
	}
	if len(labels) > 0 {
		body["labels"] = labels
	}
	var gi ghIssue
	if err := do(ctx, g.hc, http.MethodPost, g.cfg.URL+"/repos/"+g.cfg.Repo+"/issues", body, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *github) Get(ctx context.Context, key string) (*Issue, error) {
	n, err := number(key)
	if err != nil {
		return nil, err
	}
	var gi ghIssue
	if err := do(ctx, g.hc, http.MethodGet, g.cfg.URL+"/repos/"+g.cfg.Repo+"/issues/"+n, nil, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *github) Check(ctx context.Context) error {
	return do(ctx, g.hc, http.MethodGet, g.cfg.URL+"/repos/"+g.cfg.Repo, nil, g.auth, nil)
}
