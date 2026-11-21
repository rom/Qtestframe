// Package tracker talks to external issue trackers (Jira Cloud/Server REST
// API v2, GitHub Issues and GitLab REST API v4) to create issues for
// defects and requirements, to update them and to read their status back.
//
// Requests honour the server's outbound proxy and TLS settings through the
// http.Client they are given (by default one with the environment's proxy,
// HTTPS_PROXY / NO_PROXY, and the system's trusted CAs). Every request has
// a timeout; a tracker that limits the rate (429, or 403 with no requests
// remaining) is retried after its Retry-After when that is short, and
// reported with the wait otherwise. Tokens are only ever sent in request
// headers: no error message contains them.
package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Issue is what the tool needs to know about an external issue.
type Issue struct {
	Key    string // PROJ-12, owner/repo#12 or group/project#12
	URL    string // web URL
	Title  string
	Status string // the tracker's own status name
	// Mapped is the matching defect status (open, in_progress, resolved, closed, rejected).
	Mapped string
	// Closed: the issue is done (closed, resolved or rejected) in the tracker.
	Closed bool
	// Description as the tracker holds it.
	Description string
}

// NewIssue is an issue to create, or the new content of an issue to update.
type NewIssue struct {
	Title, Description, Severity, Priority string
	// Labels are added to the tracker's default labels.
	Labels []string
	// IssueType overrides the configured Jira issue type.
	IssueType string
}

// Client is a connection to one project or repository.
type Client interface {
	Create(ctx context.Context, in NewIssue) (*Issue, error)
	Get(ctx context.Context, key string) (*Issue, error)
	// Update replaces the title and description of an issue and adds
	// in.Labels to its labels.
	Update(ctx context.Context, key string, in NewIssue) (*Issue, error)
	// Comment adds a comment to an issue.
	Comment(ctx context.Context, key, body string) error
	// Check verifies the URL, credentials and project/repository.
	Check(ctx context.Context) error
	// Markup is the format of descriptions and comments: MarkupMarkdown
	// (GitHub, GitLab) or MarkupJira (Jira wiki markup).
	Markup() string
}

// Description formats.
const (
	MarkupMarkdown = "markdown"
	MarkupJira     = "jira"
)

// New returns the client for a tracker configuration.
func New(t *model.DefectTracker, hc *http.Client) (Client, error) {
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	switch t.Type {
	case "jira":
		return &jira{cfg: t, hc: hc}, nil
	case "github":
		return &github{cfg: t, hc: hc}, nil
	case "gitlab":
		return newGitLab(t, hc), nil
	}
	return nil, fmt.Errorf("no defect tracker is configured for the project")
}

var jiraIssueRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)

// ErrBadKey: an issue key that cannot belong to the tracker (a mistake of
// the caller, not of the tracker).
var ErrBadKey = errors.New("invalid issue key")

// ErrConfig: a setting of the tracker (milestone, assignee) the tracker
// does not know.
var ErrConfig = errors.New("tracker setting")

// Error is a failed request to the tracker.
type Error struct {
	Status int
	Body   string
	// RetryAfter is the wait the tracker asked for (429), if any.
	RetryAfter time.Duration
}

// hints explain the status codes users meet most.
var hints = map[int]string{
	http.StatusUnauthorized:        "the token was rejected: check that it is correct and has not expired or been revoked",
	http.StatusForbidden:           "permission denied: the token lacks a scope or role the request needs",
	http.StatusNotFound:            "not found: check the project or repository and that the token can see it",
	http.StatusUnprocessableEntity: "the tracker rejected the request",
	http.StatusBadRequest:          "the tracker rejected the request",
	http.StatusTooManyRequests:     "rate limited",
}

func (e *Error) Error() string {
	s := fmt.Sprintf("the tracker answered %d", e.Status)
	if h := hints[e.Status]; h != "" {
		s += " (" + h + ")"
	}
	if e.RetryAfter > 0 {
		s += fmt.Sprintf("; try again in %s", e.RetryAfter.Round(time.Second))
	}
	if m := errorMessage(e.Body); m != "" {
		s += ": " + m
	}
	return s
}

// errorMessage extracts the message of an error answer: the message,
// error or errorMessages of a JSON body, else the body itself (shortened).
func errorMessage(body string) string {
	b := strings.TrimSpace(body)
	var m map[string]any
	if json.Unmarshal([]byte(b), &m) == nil && m != nil {
		var parts []string
		for _, k := range []string{"message", "error", "error_description", "errorMessages", "errors"} {
			if v, ok := m[k]; ok {
				if s := flatten(v); s != "" {
					parts = append(parts, s)
				}
			}
		}
		if len(parts) > 0 {
			b = strings.Join(parts, "; ")
		}
	}
	if r := []rune(b); len(r) > 300 {
		b = string(r[:300]) + "…"
	}
	return b
}

// flatten renders a JSON error value: "x", ["a","b"], {"title":["is too long"]}.
func flatten(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var out []string
		for _, e := range x {
			if s := flatten(e); s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, ", ")
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var out []string
		for _, k := range keys {
			if s := flatten(x[k]); s != "" {
				out = append(out, k+" "+s)
			}
		}
		return strings.Join(out, "; ")
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// requestTimeout bounds each request to the tracker.
const requestTimeout = 30 * time.Second

// Rate limits: a wait the tracker asks for of at most maxRetryWait is
// waited for, up to maxAttempts requests in all.
var (
	maxRetryWait = 10 * time.Second
	maxAttempts  = 3
	// sleep waits (replaced in tests).
	sleep = func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
	now = time.Now
)

// retryAfter is the wait a rate-limited answer asks for: Retry-After in
// seconds or as a date, else the reset time of the rate limit headers of
// GitLab (RateLimit-Reset) and GitHub (X-RateLimit-Reset); 0 when the
// answer is not rate limited.
func retryAfter(res *http.Response) (time.Duration, bool) {
	limited := res.StatusCode == http.StatusTooManyRequests
	if res.StatusCode == http.StatusForbidden {
		for _, h := range []string{"RateLimit-Remaining", "X-RateLimit-Remaining"} {
			if res.Header.Get(h) == "0" {
				limited = true
			}
		}
	}
	if !limited {
		return 0, false
	}
	if v := strings.TrimSpace(res.Header.Get("Retry-After")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second, true
		}
		if t, err := http.ParseTime(v); err == nil {
			return max(t.Sub(now()), 0), true
		}
	}
	for _, h := range []string{"RateLimit-Reset", "X-RateLimit-Reset"} {
		if n, err := strconv.ParseInt(res.Header.Get(h), 10, 64); err == nil && n > 0 {
			return max(time.Unix(n, 0).Sub(now()), 0), true
		}
	}
	return time.Second, true
}

func do(ctx context.Context, hc *http.Client, method, url string, body any, auth func(*http.Request), out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = b
	}
	for attempt := 1; ; attempt++ {
		wait, err := doOnce(ctx, hc, method, url, payload, body != nil, auth, out)
		var te *Error
		if wait < 0 || !errors.As(err, &te) {
			return err
		}
		if attempt >= maxAttempts || wait > maxRetryWait {
			te.RetryAfter = max(wait, time.Second)
			return err
		}
		if err := sleep(ctx, wait); err != nil {
			return fmt.Errorf("cannot reach the tracker: %w", err)
		}
	}
}

// doOnce sends one request; wait >= 0 when the tracker limited the rate.
func doOnce(ctx context.Context, hc *http.Client, method, url string, payload []byte, hasBody bool, auth func(*http.Request), out any) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var rd io.Reader
	if hasBody {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return -1, err
	}
	req.Header.Set("Accept", "application/json")
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "qtestframe")
	auth(req)
	res, err := hc.Do(req)
	if err != nil {
		return -1, fmt.Errorf("cannot reach the tracker: %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		wait, limited := retryAfter(res)
		if !limited {
			wait = -1
		}
		return wait, &Error{Status: res.StatusCode, Body: string(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return -1, fmt.Errorf("unexpected answer from the tracker: %w", err)
		}
	}
	return -1, nil
}

// splitLabels parses a comma-separated label list.
func splitLabels(s string) []string {
	var out []string
	for _, l := range strings.Split(s, ",") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// mergeLabels joins label lists without duplicates (case-insensitive), in order.
func mergeLabels(lists ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range lists {
		for _, x := range l {
			x = strings.TrimSpace(x)
			if k := strings.ToLower(x); x != "" && !seen[k] {
				seen[k] = true
				out = append(out, x)
			}
		}
	}
	return out
}

// issueNumber extracts the number from "owner/repo#12", "#12" or "12".
func issueNumber(key, example string) (string, error) {
	k := strings.TrimSpace(key)
	if i := strings.LastIndex(k, "#"); i >= 0 {
		k = k[i+1:]
	}
	if n, err := strconv.Atoi(k); err != nil || n <= 0 {
		return "", fmt.Errorf("%w: %s", ErrBadKey, example)
	}
	return k, nil
}

// ---------------------------------------------------------------- Jira

type jira struct {
	cfg *model.DefectTracker
	hc  *http.Client
}

func (j *jira) Markup() string { return MarkupJira }

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
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Status      struct {
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
		Mapped: jiraMapped(ji.Fields.Status.StatusCategory.Key, res), Closed: ji.Fields.Status.StatusCategory.Key == "done",
		Description: ji.Fields.Description}
}

// jiraLabels: Jira labels cannot contain spaces.
func jiraLabels(in []string) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		out = append(out, strings.Join(strings.Fields(l), "_"))
	}
	return mergeLabels(out)
}

func (j *jira) Create(ctx context.Context, in NewIssue) (*Issue, error) {
	desc := in.Description
	if in.Severity != "" {
		desc = strings.TrimSpace(desc + "\n\nSeverity: " + in.Severity)
	}
	typ := in.IssueType
	if typ == "" {
		typ = j.cfg.IssueType
	}
	// API v2 takes wiki markup descriptions (v3 needs the document format).
	fields := map[string]any{
		"project":     map[string]string{"key": j.cfg.ProjectKey},
		"summary":     in.Title,
		"description": desc,
		"issuetype":   map[string]string{"name": typ},
	}
	if l := jiraLabels(in.Labels); len(l) > 0 {
		fields["labels"] = l
	}
	var created struct {
		Key string `json:"key"`
	}
	if err := do(ctx, j.hc, http.MethodPost, j.cfg.URL+"/rest/api/2/issue", map[string]any{"fields": fields}, j.auth, &created); err != nil {
		return nil, err
	}
	return j.Get(ctx, created.Key)
}

func jiraKey(key string) (string, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if !jiraIssueRe.MatchString(key) {
		return "", fmt.Errorf("%w: a Jira issue is referred to by its key, e.g. SHOP-12", ErrBadKey)
	}
	return key, nil
}

func (j *jira) Get(ctx context.Context, key string) (*Issue, error) {
	key, err := jiraKey(key)
	if err != nil {
		return nil, err
	}
	var ji jiraIssue
	if err := do(ctx, j.hc, http.MethodGet, j.cfg.URL+"/rest/api/2/issue/"+key+"?fields=summary,status,resolution,description", nil, j.auth, &ji); err != nil {
		return nil, err
	}
	return j.issue(&ji), nil
}

func (j *jira) Update(ctx context.Context, key string, in NewIssue) (*Issue, error) {
	key, err := jiraKey(key)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"fields": map[string]any{"summary": in.Title, "description": in.Description}}
	if l := jiraLabels(in.Labels); len(l) > 0 {
		var ops []map[string]string
		for _, x := range l {
			ops = append(ops, map[string]string{"add": x})
		}
		body["update"] = map[string]any{"labels": ops}
	}
	if err := do(ctx, j.hc, http.MethodPut, j.cfg.URL+"/rest/api/2/issue/"+key, body, j.auth, nil); err != nil {
		return nil, err
	}
	return j.Get(ctx, key)
}

func (j *jira) Comment(ctx context.Context, key, text string) error {
	key, err := jiraKey(key)
	if err != nil {
		return err
	}
	return do(ctx, j.hc, http.MethodPost, j.cfg.URL+"/rest/api/2/issue/"+key+"/comment", map[string]string{"body": text}, j.auth, nil)
}

func (j *jira) Check(ctx context.Context) error {
	return do(ctx, j.hc, http.MethodGet, j.cfg.URL+"/rest/api/2/project/"+j.cfg.ProjectKey, nil, j.auth, nil)
}

// ---------------------------------------------------------------- GitHub

type github struct {
	cfg *model.DefectTracker
	hc  *http.Client
}

func (g *github) Markup() string { return MarkupMarkdown }

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
	Body        string `json:"body"`
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
	return &Issue{Key: g.cfg.Repo + "#" + strconv.Itoa(gi.Number), URL: gi.HTMLURL, Title: gi.Title, Status: st,
		Mapped: ghMapped(gi.State, gi.StateReason), Closed: gi.State == "closed", Description: gi.Body}
}

const ghKeyHint = "a GitHub issue is referred to as owner/repo#123 or #123"

// milestone resolves the configured milestone: a number, or the title of
// a milestone of the repository.
func (g *github) milestone(ctx context.Context) (int, error) {
	m := strings.TrimSpace(g.cfg.Milestone)
	if n, err := strconv.Atoi(m); err == nil {
		return n, nil
	}
	var list []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := do(ctx, g.hc, http.MethodGet, g.cfg.URL+"/repos/"+g.cfg.Repo+"/milestones?state=all&per_page=100", nil, g.auth, &list); err != nil {
		return 0, err
	}
	for _, x := range list {
		if strings.EqualFold(x.Title, m) {
			return x.Number, nil
		}
	}
	return 0, fmt.Errorf("%w: GitHub has no milestone %q in %s", ErrConfig, m, g.cfg.Repo)
}

func (g *github) Create(ctx context.Context, in NewIssue) (*Issue, error) {
	body := map[string]any{"title": in.Title, "body": in.Description}
	if labels := mergeLabels(splitLabels(g.cfg.Labels), in.Labels); len(labels) > 0 {
		body["labels"] = labels
	}
	if a := strings.TrimSpace(g.cfg.Assignee); a != "" {
		body["assignees"] = []string{a}
	}
	if strings.TrimSpace(g.cfg.Milestone) != "" {
		n, err := g.milestone(ctx)
		if err != nil {
			return nil, err
		}
		body["milestone"] = n
	}
	var gi ghIssue
	if err := do(ctx, g.hc, http.MethodPost, g.cfg.URL+"/repos/"+g.cfg.Repo+"/issues", body, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *github) Get(ctx context.Context, key string) (*Issue, error) {
	n, err := issueNumber(key, ghKeyHint)
	if err != nil {
		return nil, err
	}
	var gi ghIssue
	if err := do(ctx, g.hc, http.MethodGet, g.cfg.URL+"/repos/"+g.cfg.Repo+"/issues/"+n, nil, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *github) Update(ctx context.Context, key string, in NewIssue) (*Issue, error) {
	n, err := issueNumber(key, ghKeyHint)
	if err != nil {
		return nil, err
	}
	base := g.cfg.URL + "/repos/" + g.cfg.Repo + "/issues/" + n
	if l := mergeLabels(in.Labels); len(l) > 0 {
		if err := do(ctx, g.hc, http.MethodPost, base+"/labels", map[string]any{"labels": l}, g.auth, nil); err != nil {
			return nil, err
		}
	}
	var gi ghIssue
	if err := do(ctx, g.hc, http.MethodPatch, base, map[string]any{"title": in.Title, "body": in.Description}, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *github) Comment(ctx context.Context, key, text string) error {
	n, err := issueNumber(key, ghKeyHint)
	if err != nil {
		return err
	}
	return do(ctx, g.hc, http.MethodPost, g.cfg.URL+"/repos/"+g.cfg.Repo+"/issues/"+n+"/comments", map[string]string{"body": text}, g.auth, nil)
}

func (g *github) Check(ctx context.Context) error {
	return do(ctx, g.hc, http.MethodGet, g.cfg.URL+"/repos/"+g.cfg.Repo, nil, g.auth, nil)
}
