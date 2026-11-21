package tracker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- GitLab
//
// GitLab REST API v4 on gitlab.com or a self-managed instance. The token
// (personal, project or group access token with the api scope) is sent in
// the PRIVATE-TOKEN header. The project is its numeric ID or its path
// (group/subgroup/project), which the API takes URL-encoded.

// GitLabURL is the default instance.
const GitLabURL = "https://gitlab.com"

type gitlab struct {
	cfg *model.DefectTracker
	hc  *http.Client
	// root: …/api/v4; base: root/projects/<encoded id or path>
	root, base string
}

func newGitLab(t *model.DefectTracker, hc *http.Client) *gitlab {
	u := strings.TrimRight(t.URL, "/")
	if u == "" {
		u = GitLabURL
	}
	root := strings.TrimSuffix(u, "/api/v4") + "/api/v4"
	return &gitlab{cfg: t, hc: hc, root: root, base: root + "/projects/" + url.PathEscape(strings.Trim(t.Project, "/"))}
}

func (g *gitlab) Markup() string { return MarkupMarkdown }

func (g *gitlab) auth(r *http.Request) {
	if g.cfg.Token != "" {
		r.Header.Set("PRIVATE-TOKEN", g.cfg.Token)
	}
}

type glIssue struct {
	IID         int      `json:"iid"`
	WebURL      string   `json:"web_url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Labels      []string `json:"labels"`
	References  struct {
		Full string `json:"full"`
	} `json:"references"`
}

func (g *gitlab) issue(gi *glIssue) *Issue {
	key := gi.References.Full
	if key == "" {
		key = strings.Trim(g.cfg.Project, "/") + "#" + strconv.Itoa(gi.IID)
	}
	closed := gi.State == "closed"
	mapped := "open"
	if closed {
		mapped = "closed"
	}
	return &Issue{Key: key, URL: gi.WebURL, Title: gi.Title, Status: gi.State, Mapped: mapped, Closed: closed, Description: gi.Description}
}

const glKeyHint = "a GitLab issue is referred to as group/project#123 or #123"

// milestoneID resolves the configured milestone to the ID the API takes:
// the number shown in GitLab (%3, the milestone's iid) or its title,
// looked up among the milestones of the project and its groups.
func (g *gitlab) milestoneID(ctx context.Context) (int, error) {
	m := strings.TrimPrefix(strings.TrimSpace(g.cfg.Milestone), "%")
	q := url.Values{"include_ancestors": {"true"}}
	if _, err := strconv.Atoi(m); err == nil {
		q.Set("iids[]", m)
	} else {
		q.Set("title", m)
	}
	var list []struct {
		ID    int    `json:"id"`
		IID   int    `json:"iid"`
		Title string `json:"title"`
	}
	if err := do(ctx, g.hc, http.MethodGet, g.base+"/milestones?"+q.Encode(), nil, g.auth, &list); err != nil {
		return 0, err
	}
	for _, x := range list {
		if strconv.Itoa(x.IID) == m || strings.EqualFold(x.Title, m) {
			return x.ID, nil
		}
	}
	return 0, fmt.Errorf("%w: GitLab has no milestone %q in project %s", ErrConfig, g.cfg.Milestone, g.cfg.Project)
}

// assigneeID resolves the configured assignee: a numeric user ID or a username.
func (g *gitlab) assigneeID(ctx context.Context) (int, error) {
	a := strings.TrimPrefix(strings.TrimSpace(g.cfg.Assignee), "@")
	if n, err := strconv.Atoi(a); err == nil {
		return n, nil
	}
	var list []struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	}
	if err := do(ctx, g.hc, http.MethodGet, g.root+"/users?username="+url.QueryEscape(a), nil, g.auth, &list); err != nil {
		return 0, err
	}
	for _, u := range list {
		if strings.EqualFold(u.Username, a) {
			return u.ID, nil
		}
	}
	return 0, fmt.Errorf("%w: GitLab has no user %q", ErrConfig, a)
}

func (g *gitlab) Create(ctx context.Context, in NewIssue) (*Issue, error) {
	desc := in.Description
	if in.Severity != "" {
		desc = strings.TrimSpace(desc + "\n\nSeverity: " + in.Severity)
	}
	body := map[string]any{"title": in.Title, "description": desc}
	if labels := mergeLabels(splitLabels(g.cfg.Labels), in.Labels); len(labels) > 0 {
		body["labels"] = strings.Join(labels, ",")
	}
	if strings.TrimSpace(g.cfg.Milestone) != "" {
		id, err := g.milestoneID(ctx)
		if err != nil {
			return nil, err
		}
		body["milestone_id"] = id
	}
	if strings.TrimSpace(g.cfg.Assignee) != "" {
		id, err := g.assigneeID(ctx)
		if err != nil {
			return nil, err
		}
		body["assignee_ids"] = []int{id}
	}
	var gi glIssue
	if err := do(ctx, g.hc, http.MethodPost, g.base+"/issues", body, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *gitlab) Get(ctx context.Context, key string) (*Issue, error) {
	n, err := issueNumber(key, g.cfg.Project, glKeyHint)
	if err != nil {
		return nil, err
	}
	var gi glIssue
	if err := do(ctx, g.hc, http.MethodGet, g.base+"/issues/"+n, nil, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *gitlab) Update(ctx context.Context, key string, in NewIssue) (*Issue, error) {
	n, err := issueNumber(key, g.cfg.Project, glKeyHint)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"title": in.Title, "description": in.Description}
	if l := mergeLabels(in.Labels); len(l) > 0 {
		body["add_labels"] = strings.Join(l, ",")
	}
	var gi glIssue
	if err := do(ctx, g.hc, http.MethodPut, g.base+"/issues/"+n, body, g.auth, &gi); err != nil {
		return nil, err
	}
	return g.issue(&gi), nil
}

func (g *gitlab) Comment(ctx context.Context, key, text string) error {
	n, err := issueNumber(key, g.cfg.Project, glKeyHint)
	if err != nil {
		return err
	}
	return do(ctx, g.hc, http.MethodPost, g.base+"/issues/"+n+"/notes", map[string]string{"body": text}, g.auth, nil)
}

func (g *gitlab) Check(ctx context.Context) error {
	return do(ctx, g.hc, http.MethodGet, g.base, nil, g.auth, nil)
}
