package tracker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------- code hosts
//
// Commit statuses and merge / pull request notes on GitLab and GitHub: what
// a CI pipeline (the qtestframe report and gate commands) and the server
// (when results with a commit arrive) post back to the code host. They use
// the same HTTP helpers as the trackers (rate limits are waited for).

// Commit status states (GitLab's names; GitHub's are mapped).
const (
	StatusPending  = "pending"
	StatusRunning  = "running"
	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

// CommitStatus is the status of a commit for one check name.
type CommitStatus struct {
	SHA   string
	State string
	// Name: e.g. qtestframe/tests or qtestframe/gate.
	Name        string
	TargetURL   string
	Description string
	// Ref (branch or tag) and PipelineID (GitLab: attach the status to the
	// running pipeline instead of an external one); both optional.
	Ref        string
	PipelineID string
}

// CodeHost posts commit statuses and keeps one note of Qtestframe per
// merge / pull request up to date.
type CodeHost interface {
	// SetStatus creates or updates the status of a commit.
	SetStatus(ctx context.Context, st CommitStatus) error
	// UpsertNote finds the note of the merge / pull request whose body
	// contains marker and replaces its body, or adds a note (updated false).
	UpsertNote(ctx context.Context, number, marker, body string) (updated bool, err error)
	// Name of the code host (GitLab, GitHub).
	Name() string
}

// maxNotePages bounds the pages of notes searched for the marker.
const maxNotePages = 10

// GitLabAuth says how a GitLab request authenticates.
type GitLabAuth int

const (
	// GitLabPrivateToken: a personal, project or group access token (the
	// PRIVATE-TOKEN header).
	GitLabPrivateToken GitLabAuth = iota
	// GitLabJobToken: the CI_JOB_TOKEN of the running job (JOB-TOKEN).
	// GitLab accepts it only for some endpoints; see docs/user-guide
	// ci-cd.md.
	GitLabJobToken
)

type gitlabHost struct {
	hc    *http.Client
	base  string
	token string
	kind  GitLabAuth
}

// NewGitLabHost returns the code host for a GitLab project (numeric id or
// path) at apiURL (…/api/v4, or the instance URL; "" gitlab.com).
func NewGitLabHost(apiURL, project, token string, kind GitLabAuth, hc *http.Client) CodeHost {
	u := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if u == "" {
		u = GitLabURL
	}
	root := strings.TrimSuffix(u, "/api/v4") + "/api/v4"
	if hc == nil {
		hc = http.DefaultClient
	}
	return &gitlabHost{hc: hc, base: root + "/projects/" + url.PathEscape(strings.Trim(project, "/")), token: token, kind: kind}
}

func (g *gitlabHost) Name() string { return "GitLab" }

func (g *gitlabHost) auth(r *http.Request) {
	if g.token == "" {
		return
	}
	if g.kind == GitLabJobToken {
		r.Header.Set("JOB-TOKEN", g.token)
	} else {
		r.Header.Set("PRIVATE-TOKEN", g.token)
	}
}

func named(service string, err error) error {
	var te *Error
	if errors.As(err, &te) {
		te.Service = service
	}
	return err
}

// shorten cuts s to n runes.
func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func (g *gitlabHost) SetStatus(ctx context.Context, st CommitStatus) error {
	body := map[string]any{"state": st.State, "name": st.Name, "description": shorten(st.Description, 255)}
	if st.TargetURL != "" {
		body["target_url"] = st.TargetURL
	}
	if st.Ref != "" {
		body["ref"] = st.Ref
	}
	if n, err := strconv.Atoi(st.PipelineID); err == nil && n > 0 {
		body["pipeline_id"] = n
	}
	err := do(ctx, g.hc, http.MethodPost, g.base+"/statuses/"+url.PathEscape(st.SHA), body, g.auth, nil)
	// GitLab refuses to set the state a status already has ("Cannot
	// transition status via :succeed from :success"): nothing to do.
	var te *Error
	if errors.As(err, &te) && te.Status == http.StatusBadRequest && strings.Contains(te.Body, "Cannot transition status") {
		return nil
	}
	return named("GitLab", err)
}

type glNote struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

func (g *gitlabHost) UpsertNote(ctx context.Context, number, marker, body string) (bool, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(number), "!"))
	if err != nil || n <= 0 {
		return false, fmt.Errorf("invalid merge request IID %q", number)
	}
	mr := g.base + "/merge_requests/" + strconv.Itoa(n) + "/notes"
	for page := 1; page <= maxNotePages; page++ {
		var notes []glNote
		if err := do(ctx, g.hc, http.MethodGet, fmt.Sprintf("%s?per_page=100&page=%d&sort=asc&order_by=created_at", mr, page), nil, g.auth, &notes); err != nil {
			return false, named("GitLab", err)
		}
		for _, x := range notes {
			if strings.Contains(x.Body, marker) {
				err := do(ctx, g.hc, http.MethodPut, mr+"/"+strconv.Itoa(x.ID), map[string]string{"body": body}, g.auth, nil)
				return true, named("GitLab", err)
			}
		}
		if len(notes) < 100 {
			break
		}
	}
	return false, named("GitLab", do(ctx, g.hc, http.MethodPost, mr, map[string]string{"body": body}, g.auth, nil))
}

type githubHost struct {
	hc    *http.Client
	base  string
	token string
}

// NewGitHubHost returns the code host for a GitHub repository (owner/name)
// at apiURL ("" https://api.github.com).
func NewGitHubHost(apiURL, repo, token string, hc *http.Client) CodeHost {
	u := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if u == "" {
		u = GitHubAPIURL
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	return &githubHost{hc: hc, base: u + "/repos/" + strings.Trim(repo, "/"), token: token}
}

// GitHubAPIURL is the default GitHub API.
const GitHubAPIURL = "https://api.github.com"

func (g *githubHost) Name() string { return "GitHub" }

func (g *githubHost) auth(r *http.Request) {
	r.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		r.Header.Set("Authorization", "Bearer "+g.token)
	}
}

func (g *githubHost) SetStatus(ctx context.Context, st CommitStatus) error {
	state := st.State
	switch state {
	case StatusFailed:
		state = "failure"
	case StatusRunning:
		state = "pending"
	case StatusCanceled:
		state = "error"
	}
	body := map[string]any{"state": state, "context": st.Name, "description": shorten(st.Description, 140)}
	if st.TargetURL != "" {
		body["target_url"] = st.TargetURL
	}
	return named("GitHub", do(ctx, g.hc, http.MethodPost, g.base+"/statuses/"+url.PathEscape(st.SHA), body, g.auth, nil))
}

func (g *githubHost) UpsertNote(ctx context.Context, number, marker, body string) (bool, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(number), "#"))
	if err != nil || n <= 0 {
		return false, fmt.Errorf("invalid pull request number %q", number)
	}
	issue := g.base + "/issues/" + strconv.Itoa(n) + "/comments"
	for page := 1; page <= maxNotePages; page++ {
		var notes []glNote
		if err := do(ctx, g.hc, http.MethodGet, fmt.Sprintf("%s?per_page=100&page=%d", issue, page), nil, g.auth, &notes); err != nil {
			return false, named("GitHub", err)
		}
		for _, x := range notes {
			if strings.Contains(x.Body, marker) {
				err := do(ctx, g.hc, http.MethodPatch, g.base+"/issues/comments/"+strconv.Itoa(x.ID), map[string]string{"body": body}, g.auth, nil)
				return true, named("GitHub", err)
			}
		}
		if len(notes) < 100 {
			break
		}
	}
	return false, named("GitHub", do(ctx, g.hc, http.MethodPost, issue, map[string]string{"body": body}, g.auth, nil))
}
