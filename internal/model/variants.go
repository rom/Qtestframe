package model

import "time"

// Variants, applicability and code links (migration 0036).

// VariantKinds are the kinds of variants ("" is allowed: unspecified).
var VariantKinds = []string{"product", "market", "customer", "other"}

// Variant is a product variant, market or customer configuration of a
// project. Variants form a hierarchy (ParentID); a child inherits the
// attributes of its ancestors (its own values win).
type Variant struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	ParentID    *string `json:"parent_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Kind        string  `json:"kind"`
	// Attributes are feature attributes such as {"region": "EU",
	// "voltage": "230V"}.
	Attributes map[string]string `json:"attributes"`
	// ConfigurationIDs are the test configurations the variant is tested
	// in (empty: every configuration).
	ConfigurationIDs []string  `json:"configuration_ids"`
	Position         int       `json:"position"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Derived: the path of names from the root ("EU / EU model DE").
	Path string `json:"path,omitempty"`
}

// VariantRef is a short reference to a variant.
type VariantRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Applicability says which variants a requirement or test applies to:
// the listed variants (each with its descendants) or the variants the
// expression matches. Both empty: all variants.
type Applicability struct {
	VariantIDs []string `json:"variant_ids"`
	// Expression in the query language, evaluated against each variant:
	// name / variant (the variant and its ancestors), kind and the
	// attributes as fields, e.g. `region:EU AND NOT voltage:110V`.
	Expression string `json:"expression"`
	// Derived: the names of VariantIDs.
	Variants  []VariantRef `json:"variants,omitempty"`
	UpdatedBy string       `json:"updated_by,omitempty"`
	UpdatedAt *time.Time   `json:"updated_at,omitempty"`
}

// All reports whether the applicability covers every variant.
func (a *Applicability) All() bool {
	return a == nil || (len(a.VariantIDs) == 0 && a.Expression == "")
}

// Build is a CI build that test results were uploaded for.
type Build struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	// ExternalID is the CI's build id (unique per project).
	ExternalID  string    `json:"external_id"`
	Number      string    `json:"number"`
	CommitSHA   string    `json:"commit_sha"`
	Branch      string    `json:"branch"`
	PipelineURL string    `json:"pipeline_url"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Derived: the runs of the build and their result summary.
	Runs    int            `json:"runs"`
	Summary map[string]int `json:"summary,omitempty"`
}

// Label is how a build is shown: its number, else its CI id.
func (b *Build) Label() string {
	if b.Number != "" {
		return b.Number
	}
	return b.ExternalID
}

// Code change kinds, providers and link sources.
const (
	CodeCommit       = "commit"
	CodeMergeRequest = "merge_request"
	ProviderGitLab   = "gitlab"
	ProviderGitHub   = "github"
	ProviderManual   = "manual"
	CodeLinkAuto     = "auto"
	CodeLinkManual   = "manual"
)

// CodeChangeStatuses are the states of a merge or pull request.
var CodeChangeStatuses = []string{"open", "merged", "closed"}

// CodeChange is a commit or a merge / pull request.
type CodeChange struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	// Kind: commit | merge_request; Provider: gitlab | github | manual.
	Kind       string `json:"kind"`
	Provider   string `json:"provider"`
	Repository string `json:"repository"`
	// Ref: the commit SHA or the MR / PR number.
	Ref          string `json:"ref"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Author       string `json:"author"`
	Branch       string `json:"branch"`
	TargetBranch string `json:"target_branch"`
	// Status of a merge request: open | merged | closed.
	Status     string    `json:"status"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Items the change refers to.
	Items []CodeChangeItem `json:"items"`
	// Derived: the builds of the commit (commit_sha = ref).
	Builds []Build `json:"builds,omitempty"`
}

// CodeChangeItem is a requirement or test a code change refers to.
type CodeChangeItem struct {
	ItemType string `json:"item_type"`
	ItemID   string `json:"item_id"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	// Source: auto (a key in the text) | manual.
	Source    string    `json:"source"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CodeSettings is a project's code integration (webhooks).
type CodeSettings struct {
	// Secret is write-only: the GitLab secret token and the GitHub webhook
	// secret of the project's hooks.
	Secret    string `json:"secret,omitempty"`
	HasSecret bool   `json:"has_secret"`
	// Hook URLs to enter in GitLab and GitHub (derived).
	GitLabURL string     `json:"gitlab_url,omitempty"`
	GitHubURL string     `json:"github_url,omitempty"`
	UpdatedBy string     `json:"updated_by,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}
