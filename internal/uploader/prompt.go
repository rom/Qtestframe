package uploader

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// PromptRequest is the body of POST /projects/{pid}/prompts.
type PromptRequest struct {
	Kind      string   `json:"kind"`
	Lang      string   `json:"lang,omitempty"`
	TestType  string   `json:"test_type,omitempty"`
	Framework string   `json:"framework,omitempty"`
	Notes     string   `json:"notes,omitempty"`
	Query     string   `json:"q,omitempty"`
	FolderID  string   `json:"folder_id,omitempty"`
	ReleaseID string   `json:"release_id,omitempty"`
	Variant   string   `json:"variant,omitempty"`
	Compact   bool     `json:"compact,omitempty"`
	Split     string   `json:"split,omitempty"`
	MaxTokens int      `json:"max_tokens,omitempty"`
	Formats   []string `json:"formats"` // nil: the kind's defaults
}

// PromptPart is one part of a (split) prompt.
type PromptPart struct {
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Label  string `json:"label"`
	Text   string `json:"prompt"`
	Items  int    `json:"items"`
	Tokens int    `json:"tokens"`
}

// Prompt is a built prompt.
type Prompt struct {
	Kind   string       `json:"kind"`
	Lang   string       `json:"lang"`
	Title  string       `json:"title"`
	Text   string       `json:"prompt"`
	Items  int          `json:"items"`
	Tokens int          `json:"tokens"`
	Parts  []PromptPart `json:"parts"`
}

// Prompt builds a prompt for an AI assistant (nothing is sent to an AI
// service).
func (c *Client) Prompt(ctx context.Context, projectID string, in PromptRequest) (*Prompt, error) {
	var out Prompt
	if err := c.sendJSON(ctx, http.MethodPost, "/projects/"+url.PathEscape(projectID)+"/prompts", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Named is an item with an id and a name (a folder, release or variant).
type Named struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Version  string  `json:"version"`
	ParentID *string `json:"parent_id"`
}

func (c *Client) list(ctx context.Context, path string) ([]Named, error) {
	var out struct {
		Items []Named `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, "", &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// FindFolder finds a requirement folder by id, name or path ("A/B" or
// "A / B"); "" when there is none.
func (c *Client) FindFolder(ctx context.Context, projectID, ref string) (string, error) {
	return c.FindFolderOf(ctx, projectID, "requirement", ref)
}

// FindFolderOf finds a folder of kind (requirement or test) by id, name
// or path; "" when there is none.
func (c *Client) FindFolderOf(ctx context.Context, projectID, kind, ref string) (string, error) {
	fs, err := c.list(ctx, "/projects/"+url.PathEscape(projectID)+"/folders?kind="+url.QueryEscape(kind))
	if err != nil {
		return "", err
	}
	byID := map[string]Named{}
	for _, f := range fs {
		byID[f.ID] = f
	}
	path := func(f Named) string {
		p := f.Name
		for depth := 0; f.ParentID != nil && depth < 64; depth++ {
			parent, ok := byID[*f.ParentID]
			if !ok {
				break
			}
			p, f = parent.Name+"/"+p, parent
		}
		return p
	}
	norm := func(s string) string {
		parts := strings.Split(s, "/")
		for i := range parts {
			parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
		}
		return strings.Join(parts, "/")
	}
	want := norm(ref)
	for _, f := range fs {
		if f.ID == ref || norm(path(f)) == want {
			return f.ID, nil
		}
	}
	for _, f := range fs {
		if strings.EqualFold(f.Name, strings.TrimSpace(ref)) {
			return f.ID, nil
		}
	}
	return "", nil
}

// FindRelease finds a release by id, name or version; "" when there is none.
func (c *Client) FindRelease(ctx context.Context, projectID, ref string) (string, error) {
	rs, err := c.list(ctx, "/projects/"+url.PathEscape(projectID)+"/releases")
	if err != nil {
		return "", err
	}
	for _, r := range rs {
		if r.ID == ref || strings.EqualFold(r.Name, ref) || (r.Version != "" && strings.EqualFold(r.Version, ref)) {
			return r.ID, nil
		}
	}
	return "", nil
}
