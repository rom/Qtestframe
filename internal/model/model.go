// Package model contains the domain types shared by the store, API and
// export layers. JSON tags define the public REST contract (see docs/api.md).
package model

import (
	"encoding/json"
	"time"
)

// Item kinds used by folders and categories.
const (
	KindRequirement = "requirement"
	KindTest        = "test"
)

// Obligation levels (requirement language). Based on RFC 2119 / RFC 8174
// with additional descriptive levels commonly used in specifications.
const (
	ObligationMust      = "must"
	ObligationMustNot   = "must_not"
	ObligationShall     = "shall"
	ObligationShallNot  = "shall_not"
	ObligationShould    = "should"
	ObligationShouldNot = "should_not"
	ObligationMay       = "may"
	ObligationOptional  = "optional"
	ObligationWill      = "will"
	ObligationDescribe  = "describe"
)

// Obligations lists every valid obligation level in descending strength.
var Obligations = []string{
	ObligationMust, ObligationMustNot, ObligationShall, ObligationShallNot,
	ObligationShould, ObligationShouldNot, ObligationMay, ObligationOptional,
	ObligationWill, ObligationDescribe,
}

// Requirement statuses.
var RequirementStatuses = []string{"draft", "review", "approved", "implemented", "verified", "rejected", "obsolete"}

// Requirement types.
var RequirementTypes = []string{"functional", "non_functional", "security", "performance", "usability", "interface", "constraint", "business", "regulatory"}

// Priorities shared by requirements and tests.
var Priorities = []string{"critical", "high", "medium", "low"}

// Test case statuses.
var TestStatuses = []string{"draft", "ready", "review", "deprecated"}

// Execution types.
const (
	ExecManual    = "manual"
	ExecAutomated = "automated"
)

// Script languages supported by the runner.
var ScriptLanguages = []string{"sh", "bash", "python", "javascript"}

// Evaluation (verification) methods – the classic IADT set.
var EvaluationMethods = []string{"test", "inspection", "analysis", "demonstration", "review"}

// Run statuses.
var RunStatuses = []string{"planned", "in_progress", "completed", "aborted"}

// Execution / step result statuses.
const (
	ResultNotRun     = "not_run"
	ResultInProgress = "in_progress"
	ResultPassed     = "passed"
	ResultFailed     = "failed"
	ResultBlocked    = "blocked"
	ResultSkipped    = "skipped"
)

// ResultStatuses lists all valid execution results.
var ResultStatuses = []string{ResultNotRun, ResultInProgress, ResultPassed, ResultFailed, ResultBlocked, ResultSkipped}

// Valid reports whether v is contained in set.
func Valid(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Tenant is an isolated organisation.
type Tenant struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// User is a global identity which can be member of many tenants.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	SuperAdmin   bool      `json:"super_admin"`
	AuthProvider string    `json:"auth_provider"`
	ExternalID   string    `json:"external_id,omitempty"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
	// Preferences are the user's personal settings (see Preferences).
	Preferences Preferences `json:"preferences"`
	// Avatar is "" (initials), "preset:<name>" or "file:<file id>".
	Avatar string `json:"avatar"`
	// MustChangePassword is set after an administrator reset the password.
	MustChangePassword bool `json:"must_change_password"`
}

// LoginEvent records a sign-in attempt.
type LoginEvent struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Success   bool      `json:"success"`
	Method    string    `json:"method"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

// File kinds.
const (
	FileAttachment = "attachment"
	FileDocument   = "document"
	FileReport     = "report"
	FileAvatar     = "avatar"
	FileIcon       = "icon"
)

// FileInfo is the metadata of a stored file.
type FileInfo struct {
	ID          string     `json:"id"`
	ProjectID   *string    `json:"project_id"`
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	SHA256      string     `json:"sha256"`
	Description string     `json:"description"`
	Tags        []string   `json:"tags"`
	UploadedBy  string     `json:"uploaded_by"`
	CreatedAt   time.Time  `json:"created_at"`
	URL         string     `json:"url"` // download URL
	Links       []FileLink `json:"links"`
}

// FileLink is an item a file is attached to.
type FileLink struct {
	EntityType string `json:"entity_type"` // requirement | test | run | execution
	EntityID   string `json:"entity_id"`
	Key        string `json:"key"` // item key or run name
	// RunID is set for execution links (the run containing the execution).
	RunID string `json:"run_id,omitempty"`
}

// ReportDefinition is a saved custom report.
type ReportDefinition struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Preferences are per-user settings edited on the profile page. Empty
// values mean "use the default".
type Preferences struct {
	Theme            string `json:"theme,omitempty"`      // see Themes
	Language         string `json:"language,omitempty"`   // en | sv; "" = browser
	StartPage        string `json:"start_page,omitempty"` // projects | last_project | default_project | search
	DefaultProject   string `json:"default_project_id,omitempty"`
	DefaultTenant    string `json:"default_tenant_id,omitempty"` // tenant selected at login
	PageSize         int    `json:"page_size,omitempty"`         // 10 | 25 | 50 | 100
	DateFormat       string `json:"date_format,omitempty"`       // locale | iso | relative
	Timezone         string `json:"timezone,omitempty"`          // IANA name, "" = browser
	CompactTables    bool   `json:"compact_tables,omitempty"`
	HideAnalysis     bool   `json:"hide_analysis,omitempty"` // hide live requirement language analysis
	CreateLinkedTest *bool  `json:"create_linked_test,omitempty"`
	IntroDone        bool   `json:"intro_done,omitempty"` // first-login intro tour finished or skipped // personal default for the editor checkbox
}

// Preference value sets.
var (
	// Themes: light/dark/system plus decorative colour themes.
	Themes = []string{"light", "dark", "system", "spring", "autumn", "desert", "camouflage",
		"city_camouflage", "navy", "green_console", "crazy"}
	// Languages of the web client ("" = browser language).
	Languages   = []string{"en", "sv"}
	StartPages  = []string{"projects", "last_project", "default_project", "search"}
	PageSizes   = []int{10, 25, 50, 100}
	DateFormats = []string{"locale", "iso", "relative"}
)

// SessionInfo describes an active login session (profile page).
type SessionInfo struct {
	ID        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	LastSeen  *time.Time `json:"last_seen"`
	IP        string     `json:"ip"`
	UserAgent string     `json:"user_agent"`
	TenantID  string     `json:"tenant_id"`
	Current   bool       `json:"current"`
}

// Membership binds a user to a tenant with a role.
type Membership struct {
	TenantID   string    `json:"tenant_id"`
	TenantSlug string    `json:"tenant_slug,omitempty"`
	TenantName string    `json:"tenant_name,omitempty"`
	UserID     string    `json:"user_id"`
	Email      string    `json:"email,omitempty"`
	Name       string    `json:"display_name,omitempty"`
	Avatar     string    `json:"avatar,omitempty"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"created_at"`
}

// Session is a server-side login session.
type Session struct {
	ID        string // SHA-256 of the cookie token
	UserID    string
	TenantID  string
	CSRF      string
	CreatedAt time.Time
	ExpiresAt time.Time
	IP        string
	UserAgent string
	LastSeen  *time.Time
}

// APIToken allows non-interactive (CI) access.
type APIToken struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenant_id"`
	UserID    string     `json:"user_id"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Hash      string     `json:"-"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
}

// Project groups requirements, tests and runs.
type Project struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// DefectURLTemplate turns defect IDs into links, e.g.
	// "https://jira.example.com/browse/{id}".
	DefectURLTemplate string `json:"defect_url_template"`
	// Icon is "" (initials), "preset:<name>" or "file:<file id>".
	Icon      string    `json:"icon"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Folder is a node in the requirement or test hierarchy ("group").
type Folder struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	ParentID    *string   `json:"parent_id"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
}

// Category is a classification value inside a dimension, e.g.
// dimension "Polarity" with values "Positive" and "Negative".
type Category struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Scope       string `json:"scope"` // requirement | test
	Dimension   string `json:"dimension"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// Requirement is a single specification statement.
type Requirement struct {
	ID          string                `json:"id"`
	ProjectID   string                `json:"project_id"`
	FolderID    *string               `json:"folder_id"`
	Key         string                `json:"key"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Obligation  string                `json:"obligation"`
	Type        string                `json:"type"`
	Priority    string                `json:"priority"`
	Status      string                `json:"status"`
	Source      string                `json:"source"`
	Rationale   string                `json:"rationale"`
	Owner       string                `json:"owner"`
	Tags        []string              `json:"tags"`
	Version     int                   `json:"version"`
	Position    int                   `json:"position"`
	CategoryIDs []string              `json:"category_ids"`
	Criteria    []EvaluationCriterion `json:"criteria,omitempty"`
	CreatedBy   string                `json:"created_by"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	// Derived fields (not stored).
	FolderPath   string `json:"folder_path,omitempty"`
	LinkedTests  int    `json:"linked_tests"`
	Coverage     string `json:"coverage,omitempty"` // none | not_run | passed | failed | partial
	CommentCount int    `json:"comment_count"`
	Relations    int    `json:"relation_count"`
}

// EvaluationCriterion describes how a requirement is judged fulfilled.
type EvaluationCriterion struct {
	ID            string `json:"id"`
	RequirementID string `json:"requirement_id"`
	Description   string `json:"description"`
	Method        string `json:"method"`
	Threshold     string `json:"threshold"`
	Position      int    `json:"position"`
}

// TestStep is a single manual step of a test case.
type TestStep struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	Action   string `json:"action"`
	Expected string `json:"expected"`
	Data     string `json:"data"`
	// CallTestID makes this a "call" step: the steps of the referenced test
	// (same project) are inserted here when a run is created.
	CallTestID    *string `json:"call_test_id"`
	CallTestKey   string  `json:"call_test_key,omitempty"`
	CallTestTitle string  `json:"call_test_title,omitempty"`
}

// TestRef is a short reference to a test case.
type TestRef struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

// TestCase describes a manual or automated test.
type TestCase struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	FolderID       *string    `json:"folder_id"`
	Key            string     `json:"key"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Preconditions  string     `json:"preconditions"`
	ExecutionType  string     `json:"execution_type"`
	ScriptLanguage string     `json:"script_language"`
	Script         string     `json:"script"`
	ExpectedResult string     `json:"expected_result"`
	Priority       string     `json:"priority"`
	Status         string     `json:"status"`
	EstimatedMin   int        `json:"estimated_minutes"`
	Tags           []string   `json:"tags"`
	Position       int        `json:"position"`
	CategoryIDs    []string   `json:"category_ids"`
	Steps          []TestStep `json:"steps"`
	Version        int        `json:"version"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// Derived.
	FolderPath   string    `json:"folder_path,omitempty"`
	LinkedReqs   int       `json:"linked_requirements"`
	LastResult   string    `json:"last_result,omitempty"`
	CommentCount int       `json:"comment_count"`
	CalledBy     []TestRef `json:"called_by,omitempty"`
}

// TraceLink connects a requirement (optionally a specific criterion) to a test.
type TraceLink struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	RequirementID string    `json:"requirement_id"`
	TestCaseID    string    `json:"test_case_id"`
	CriterionID   *string   `json:"criterion_id"`
	CreatedAt     time.Time `json:"created_at"`
	// Derived.
	RequirementKey string `json:"requirement_key,omitempty"`
	TestCaseKey    string `json:"test_case_key,omitempty"`
}

// TestRun is an execution campaign (cycle) over a set of tests.
type TestRun struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Environment string     `json:"environment"`
	Build       string     `json:"build"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	// Derived.
	Summary map[string]int `json:"summary,omitempty"`
}

// StepResult is the outcome of one manual step.
type StepResult struct {
	ID       string `json:"id"`
	StepID   string `json:"step_id"`
	Position int    `json:"position"`
	Action   string `json:"action"`
	Expected string `json:"expected"`
	Status   string `json:"status"`
	Actual   string `json:"actual"`
}

// Execution is one test case inside a run.
type Execution struct {
	ID          string       `json:"id"`
	RunID       string       `json:"run_id"`
	TestCaseID  string       `json:"test_case_id"`
	TestKey     string       `json:"test_key"`
	TestTitle   string       `json:"test_title"`
	Status      string       `json:"status"`
	Assignee    string       `json:"assignee"`
	ExecutedBy  string       `json:"executed_by"`
	StartedAt   *time.Time   `json:"started_at"`
	FinishedAt  *time.Time   `json:"finished_at"`
	DurationMS  int64        `json:"duration_ms"`
	Output      string       `json:"output"`
	Notes       string       `json:"notes"`
	Position    int          `json:"position"`
	Defects     []string     `json:"defects"`
	StepResults []StepResult `json:"step_results,omitempty"`
	// Set by GET /tests/{id}/executions (history across runs).
	RunName        string     `json:"run_name,omitempty"`
	RunStatus      string     `json:"run_status,omitempty"`
	RunEnvironment string     `json:"run_environment,omitempty"`
	RunBuild       string     `json:"run_build,omitempty"`
	RunCreatedAt   *time.Time `json:"run_created_at,omitempty"`
}

// Relation types between requirements. "refines" means the source is a
// child (decomposition) of the target.
var RelationTypes = []string{"refines", "depends_on", "derived_from", "conflicts_with", "duplicates", "relates_to"}

// Relation links two requirements of the same project.
type Relation struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	SourceID    string    `json:"source_id"`
	TargetID    string    `json:"target_id"`
	Type        string    `json:"type"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	SourceKey   string    `json:"source_key"`
	SourceTitle string    `json:"source_title"`
	TargetKey   string    `json:"target_key"`
	TargetTitle string    `json:"target_title"`
	// Direction relative to the requirement queried: outgoing | incoming.
	Direction string `json:"direction,omitempty"`
}

// Revision is a stored snapshot of a requirement or test case.
type Revision struct {
	ID            string          `json:"id"`
	EntityType    string          `json:"entity_type"` // requirement | test
	EntityID      string          `json:"entity_id"`
	Version       int             `json:"version"`
	Action        string          `json:"action"` // create | update | restore | move | bulk_update
	ChangedFields []string        `json:"changed_fields"`
	Snapshot      json.RawMessage `json:"snapshot"`
	UserEmail     string          `json:"user_email"`
	UserName      string          `json:"user_name"`
	UserAvatar    string          `json:"user_avatar"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Baseline is a named, frozen snapshot of a project's requirements and tests.
type Baseline struct {
	ID           string         `json:"id"`
	ProjectID    string         `json:"project_id"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	CreatedBy    string         `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	Requirements int            `json:"requirement_count"`
	Tests        int            `json:"test_count"`
	Items        []BaselineItem `json:"items,omitempty"`
}

// BaselineItem is one frozen item of a baseline.
type BaselineItem struct {
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Key        string          `json:"key"`
	Title      string          `json:"title"`
	Version    int             `json:"version"`
	Snapshot   json.RawMessage `json:"snapshot"`
}

// BaselineDiff compares a baseline with another baseline or the current state.
type BaselineDiff struct {
	From      string           `json:"from"`
	To        string           `json:"to"`
	Added     []BaselineChange `json:"added"`
	Removed   []BaselineChange `json:"removed"`
	Changed   []BaselineChange `json:"changed"`
	Unchanged int              `json:"unchanged"`
}

// BaselineChange describes one differing item.
type BaselineChange struct {
	EntityType    string   `json:"entity_type"`
	EntityID      string   `json:"entity_id"`
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	FromVersion   int      `json:"from_version,omitempty"`
	ToVersion     int      `json:"to_version,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// Comment is a discussion entry on a requirement or test.
type Comment struct {
	ID         string    `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	UserID     string    `json:"user_id"`
	UserEmail  string    `json:"user_email"`
	UserName   string    `json:"user_name"`
	UserAvatar string    `json:"user_avatar"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AuditEntry records a change.
type AuditEntry struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	UserID     string    `json:"user_id"`
	UserEmail  string    `json:"user_email"`
	UserName   string    `json:"user_name"`
	UserAvatar string    `json:"user_avatar"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Details    string    `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}
