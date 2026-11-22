package uploader

import (
	"context"
	"net/http"
	"net/url"

	"github.com/rom/qtestframe/internal/ci"
)

// GateRequest is the body of POST /projects/{pid}/gate.
type GateRequest struct {
	Release      string   `json:"release,omitempty"`
	Variant      string   `json:"variant,omitempty"`
	Run          string   `json:"run,omitempty"`
	Build        string   `json:"build,omitempty"`
	Require      []string `json:"require,omitempty"`
	Scope        string   `json:"scope,omitempty"`
	Commit       string   `json:"commit,omitempty"`
	MergeRequest string   `json:"merge_request,omitempty"`
	SummaryOnly  bool     `json:"summary_only,omitempty"`
}

// Gate evaluates the quality gate of a project.
func (c *Client) Gate(ctx context.Context, projectID string, in GateRequest) (*ci.Report, error) {
	var rep ci.Report
	if err := c.sendJSON(ctx, http.MethodPost, "/projects/"+url.PathEscape(projectID)+"/gate", in, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

// SelectRequest is the body of POST /projects/{pid}/ci/select.
type SelectRequest struct {
	RequirementIDs []string `json:"requirement_ids,omitempty"`
	SinceBuild     string   `json:"since_build,omitempty"`
	Variant        string   `json:"variant,omitempty"`
	IncludeRelated bool     `json:"include_related,omitempty"`
	Commits        []string `json:"commits,omitempty"`
	MergeRequest   string   `json:"merge_request,omitempty"`
	Keys           []string `json:"keys,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	All            bool     `json:"all,omitempty"`
	IncludeManual  bool     `json:"include_manual,omitempty"`
	CreateRun      bool     `json:"create_run,omitempty"`
	Run            string   `json:"run,omitempty"`
	Build          string   `json:"build,omitempty"`
	Environment    string   `json:"environment,omitempty"`
	ReleaseID      *string  `json:"release_id,omitempty"`
}

// Select returns the tests to run for a change.
func (c *Client) Select(ctx context.Context, projectID string, in SelectRequest) (*ci.Selection, error) {
	var sel ci.Selection
	if err := c.sendJSON(ctx, http.MethodPost, "/projects/"+url.PathEscape(projectID)+"/ci/select", in, &sel); err != nil {
		return nil, err
	}
	return &sel, nil
}
