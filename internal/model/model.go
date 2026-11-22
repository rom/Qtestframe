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
var ScriptLanguages = []string{"sh", "bash", "python", "javascript", "playwright"}

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

// TenantSettings are organisation-wide settings, edited by holders of
// settings.manage (admins and test leads by default). Every field's zero
// value is the strict default.
type TenantSettings struct {
	// AllowUnreadyTests permits executing tests whose status is not "ready".
	AllowUnreadyTests bool `json:"allow_unready_tests"`
	// AllowUnreadyRequirements permits executing tests that verify
	// requirements whose status is not in ReadyRequirementStatuses.
	AllowUnreadyRequirements bool `json:"allow_unready_requirements"`
	// ReadyRequirementStatuses are the requirement statuses that count as
	// ready for testing (default DefaultReadyRequirementStatuses).
	ReadyRequirementStatuses []string `json:"ready_requirement_statuses"`
	// AllowUnmetDependencies permits starting a test before the tests it
	// depends on have passed.
	AllowUnmetDependencies bool `json:"allow_unmet_dependencies"`
	// ManualVerification turns off "status follows evidence": by default an
	// implemented requirement becomes verified when all its linked tests
	// pass and no link is suspect, and a verified one goes back to
	// implemented when that no longer holds.
	ManualVerification bool `json:"manual_verification"`
}

// DefaultReadyRequirementStatuses are the requirement statuses ready for
// testing unless a tenant chooses others.
var DefaultReadyRequirementStatuses = []string{"approved", "implemented", "verified"}

// ReadyTestStatus is the test status that allows execution.
const ReadyTestStatus = "ready"

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
	// MFAEnabled: the user has a confirmed second factor (filled by the
	// user lists and /auth/me, not stored in the users table).
	MFAEnabled bool `json:"mfa_enabled"`
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
	EntityType string `json:"entity_type"` // requirement | test | run | execution | step
	EntityID   string `json:"entity_id"`
	Key        string `json:"key"` // item key or run name
	// RunID is set for execution and step links (the run containing the execution).
	RunID string `json:"run_id,omitempty"`
	// ExecutionID is set for step links (the execution of the step result).
	ExecutionID string `json:"execution_id,omitempty"`
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
	// EmailNotifications: off (default) | immediate | digest (see EmailModes).
	EmailNotifications string `json:"email_notifications,omitempty"`

	// --- obligation keyword highlighting ---
	// HideObligationHighlight turns off highlighting obligation keywords
	// (MUST, SKA INTE …) in requirement descriptions; highlighting is on
	// by default.
	HideObligationHighlight bool `json:"hide_obligation_highlight,omitempty"`
	// Autocomplete: suggestions in free-text fields (see AutocompleteModes):
	// "" = on (my history and project values) | history | project | off.
	// Only "" and history record what the user saves.
	Autocomplete string `json:"autocomplete,omitempty"`
}

// Preference value sets.
var (
	// Themes: light/dark/system plus decorative colour themes.
	Themes = []string{"light", "dark", "system", "spring", "autumn", "desert", "camouflage",
		"city_camouflage", "arctic_camouflage", "navy", "midnight", "green_console", "amber_terminal", "crazy",
		"ocean", "forest", "sunset", "nordic", "lavender", "rose", "high_contrast", "high_contrast_light"}
	// Languages of the web client ("" = browser language).
	Languages   = []string{"en", "sv"}
	StartPages  = []string{"projects", "last_project", "default_project", "search"}
	PageSizes   = []int{10, 25, 50, 100}
	DateFormats = []string{"locale", "iso", "relative"}
	// AutocompleteModes: the values of Preferences.Autocomplete besides ""
	// (on: both sources).
	AutocompleteModes = []string{"history", "project", "off"}
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
	// SCIM is set when the identity provider manages the membership
	// (SCIM provisioning); its role follows the provider's groups.
	SCIM bool `json:"scim,omitempty"`
	// MFAEnabled: the member has a confirmed second factor (member lists).
	MFAEnabled bool `json:"mfa_enabled"`
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
	// MFAEnrolRequired: the MFA policy requires a second factor the user
	// has not set up; the session can only reach /api/v1/auth/ until then.
	MFAEnrolRequired bool
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
	Icon string `json:"icon"`
	// Color is "" (theme colour) or "#rrggbb"; used for the icon and accents.
	Color string `json:"color"`
	// RequirementSources are offered for a requirement's source field (the
	// defaults when the project has none of its own).
	RequirementSources []string `json:"requirement_sources"`
	// CustomSources reports whether RequirementSources are the project's own.
	CustomSources bool `json:"custom_sources"`
	// Type is one of ProjectTypes; "software" enables prompt generation.
	Type string `json:"type"`
	// TechContext describes the technology of a software project (languages,
	// frameworks, architecture, conventions); generated prompts include it.
	TechContext string `json:"tech_context"`
	// Access is ProjectAccessOrganisation (every member of the
	// organisation) or ProjectAccessRestricted (project members and
	// organisation administrators only).
	Access    string    `json:"access"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// MyAccess is the caller's access to the project (set by the API).
	MyAccess *ProjectAccessInfo `json:"my_access,omitempty"`
}

// Project access modes.
const (
	ProjectAccessOrganisation = "organisation"
	ProjectAccessRestricted   = "restricted"
)

// ProjectAccessInfo is a caller's access to a project.
type ProjectAccessInfo struct {
	// Source is "admin" (organisation administrator), "member" (a project
	// member: Role is the project role), "group" (through a SCIM group:
	// Role lists the groups' roles) or "organisation" (the organisation
	// role of an organisation-wide project).
	Source string `json:"source"`
	// Role is the project role ("" for admin and organisation).
	Role string `json:"role,omitempty"`
	// Permissions are the effective permissions in the project.
	Permissions []string `json:"permissions"`
	// CanShare reports whether the caller may change the project's access
	// and members.
	CanShare bool `json:"can_share"`
}

// ProjectMember is a user or SCIM group with a role in a project.
type ProjectMember struct {
	ProjectID string `json:"project_id"`
	// Type is "user" or "group".
	Type string `json:"type"`
	// ID is the user id or the SCIM group id.
	ID     string `json:"id"`
	Role   string `json:"role"`
	Email  string `json:"email,omitempty"`
	Name   string `json:"display_name,omitempty"`
	Avatar string `json:"avatar,omitempty"`
	// OrgRole is the user's organisation role ("" for groups).
	OrgRole   string    `json:"org_role,omitempty"`
	AddedBy   string    `json:"added_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectTypes are the kinds of project. Software projects get extra
// features (prompt generation for requirements and tests).
var ProjectTypes = []string{"general", "software", "hardware", "system", "business"}

// ProjectTypeSoftware is the project type with software features.
const ProjectTypeSoftware = "software"

// DefaultRequirementSources are offered when a project has no own list.
var DefaultRequirementSources = []string{"Customer", "Contract", "Standard", "Regulation", "Stakeholder", "Product management", "Architecture", "Security analysis", "Internal", "Previous release"}

// Folder is a node in the requirement or test hierarchy ("group").
type Folder struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	ParentID    *string `json:"parent_id"`
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	// Color is '' or #rrggbb; items in the folder (and in subfolders
	// without a colour of their own) are shown in it.
	Color     string    `json:"color"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
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
	ID          string   `json:"id"`
	ProjectID   string   `json:"project_id"`
	FolderID    *string  `json:"folder_id"`
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Obligation  string   `json:"obligation"`
	Type        string   `json:"type"`
	Priority    string   `json:"priority"`
	Status      string   `json:"status"`
	Source      string   `json:"source"`
	Rationale   string   `json:"rationale"`
	Owner       string   `json:"owner"`
	Tags        []string `json:"tags"`
	// CatalogRef is the catalogue id the requirement was copied from
	// (internal/catalog), "" otherwise.
	CatalogRef  string                `json:"catalog_ref"`
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
	// SuspectLinks: links to tests that are suspect.
	SuspectLinks int `json:"suspect_links"`
	// RiskLevel: the highest level of the open risks it addresses ("" none).
	RiskLevel string `json:"risk_level,omitempty"`
	// OpenDefects: open defects linked to it or to its tests.
	OpenDefects int `json:"open_defects"`
	// RelationRefs lists the related items in both directions: requirement
	// relations and the tests that verify it ("verifies", incoming).
	RelationRefs  []ItemRef `json:"relations,omitempty"`
	CriteriaCount int       `json:"criteria_count,omitempty"`
}

// ItemRef is a derived reference from a requirement or test to a related
// item, as shown in the list columns.
type ItemRef struct {
	// Type is a RelationTypes value, "verifies" (test → requirement) or
	// "calls" (test → test whose steps it calls).
	Type string `json:"type"`
	// Direction is "outgoing" when this item is the source, else "incoming".
	Direction string `json:"direction"`
	// Kind of the item at the other end: requirement | test.
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
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
	ID             string   `json:"id"`
	ProjectID      string   `json:"project_id"`
	FolderID       *string  `json:"folder_id"`
	Key            string   `json:"key"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Preconditions  string   `json:"preconditions"`
	ExecutionType  string   `json:"execution_type"`
	ScriptLanguage string   `json:"script_language"`
	Script         string   `json:"script"`
	ExpectedResult string   `json:"expected_result"`
	Priority       string   `json:"priority"`
	Status         string   `json:"status"`
	EstimatedMin   int      `json:"estimated_minutes"`
	Tags           []string `json:"tags"`
	// CatalogRef is the catalogue id the test was copied from, "" otherwise.
	CatalogRef  string     `json:"catalog_ref"`
	Position    int        `json:"position"`
	CategoryIDs []string   `json:"category_ids"`
	Steps       []TestStep `json:"steps"`
	// Parameters and DataRows make the test data-driven: it is executed
	// once per row (an iteration), with {{name}} placeholders in step texts
	// replaced by the row's values.
	Parameters []string  `json:"parameters"`
	DataRows   []DataRow `json:"data_rows"`
	Version    int       `json:"version"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Derived.
	FolderPath   string    `json:"folder_path,omitempty"`
	LinkedReqs   int       `json:"linked_requirements"`
	LastResult   string    `json:"last_result,omitempty"`
	CommentCount int       `json:"comment_count"`
	CalledBy     []TestRef `json:"called_by,omitempty"`
	// RelationRefs: requirements it verifies, tests it calls and tests that call it.
	RelationRefs []ItemRef `json:"relations,omitempty"`
	StepCount    int       `json:"step_count,omitempty"`
	// SuspectLinks: links to requirements that are suspect.
	SuspectLinks int      `json:"suspect_links"`
	Defects      []string `json:"defects,omitempty"`
	// Flakiness: pass/fail flips among the last results; Flaky when it
	// reaches the threshold (see store.FlakyWindow, store.FlakyMinFlips).
	Flakiness int  `json:"flakiness,omitempty"`
	Flaky     bool `json:"flaky,omitempty"`
}

// TraceLink connects a requirement (optionally a specific criterion) to a test.
type TraceLink struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	RequirementID string    `json:"requirement_id"`
	TestCaseID    string    `json:"test_case_id"`
	CriterionID   *string   `json:"criterion_id"`
	CreatedAt     time.Time `json:"created_at"`
	// SuspectSince is set when the requirement or test changed after the
	// link was made or last confirmed; SuspectReason says what changed.
	SuspectSince  *time.Time `json:"suspect_since"`
	SuspectReason string     `json:"suspect_reason,omitempty"`
	// Derived.
	RequirementKey string `json:"requirement_key,omitempty"`
	TestCaseKey    string `json:"test_case_key,omitempty"`
}

// DataRow is one row of a data-driven test: values by parameter name and an
// optional name (e.g. "admin user").
type DataRow struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

// DimensionValue is one coordinate of a configuration, e.g. Browser = Chrome.
type DimensionValue struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

// Configuration is a named combination of dimension values (environment,
// browser, OS, hardware, firmware ...) that tests are executed against.
type Configuration struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"project_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Values      []DimensionValue `json:"values"`
	Position    int              `json:"position"`
	CreatedBy   string           `json:"created_by"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	// Derived: number of runs that target it.
	RunCount int `json:"run_count"`
}

// ConfigurationRef is a short reference to a configuration.
type ConfigurationRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
	// ReleaseID is the release this run tests (optional).
	ReleaseID *string `json:"release_id"`
	// Configurations the run targets (each test is executed once per
	// configuration); empty for a run without configurations.
	Configurations []ConfigurationRef `json:"configurations"`
	// Derived.
	Summary     map[string]int `json:"summary,omitempty"`
	ReleaseName string         `json:"release_name,omitempty"`
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
	// Evidence is what proves the result (observations, values, log
	// excerpts, links); files are attachments of the step result.
	Evidence string `json:"evidence"`
	// AttachmentCount is the number of files attached as evidence.
	AttachmentCount int `json:"attachment_count"`
	// IterationID: the iteration (data row) of a data-driven execution.
	IterationID *string `json:"iteration_id"`
}

// Iteration is the execution of one data row of a data-driven test.
type Iteration struct {
	ID       string            `json:"id"`
	Position int               `json:"position"`
	Name     string            `json:"name"`
	Values   map[string]string `json:"values"`
	Status   string            `json:"status"`
	Notes    string            `json:"notes"`
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
	// ConfigurationID / ConfigurationName: the configuration this
	// execution runs in (nil without configuration).
	ConfigurationID   *string `json:"configuration_id"`
	ConfigurationName string  `json:"configuration_name,omitempty"`
	// Iterations of a data-driven test (GET /executions/{id}); IterationCount
	// is set in lists.
	Iterations     []Iteration `json:"iterations,omitempty"`
	IterationCount int         `json:"iteration_count"`
	// Set by GET /tests/{id}/executions (history across runs).
	RunName        string     `json:"run_name,omitempty"`
	RunStatus      string     `json:"run_status,omitempty"`
	RunEnvironment string     `json:"run_environment,omitempty"`
	RunBuild       string     `json:"run_build,omitempty"`
	RunCreatedAt   *time.Time `json:"run_created_at,omitempty"`
}

// Relation types between requirements. "refines" means the source is a
// child (decomposition) of the target; "provides" means the source supplies
// something (a capability, interface or data) that the target needs.
var RelationTypes = []string{"refines", "depends_on", "provides", "derived_from", "conflicts_with", "duplicates", "relates_to"}

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

// KindDefect and KindRisk name defects and risks (keys PROJ-BUG-n, PROJ-RISK-n).
const (
	KindDefect = "defect"
	KindRisk   = "risk"
)

// DefectStatuses are the states of a defect; DefectOpenStatuses count as open.
var (
	DefectStatuses     = []string{"open", "in_progress", "resolved", "closed", "rejected"}
	DefectOpenStatuses = []string{"open", "in_progress"}
	// DefectSeverities from most to least severe.
	DefectSeverities = []string{"critical", "major", "minor", "trivial"}
	// DefectLinkTypes are the items a defect can be linked to.
	DefectLinkTypes = []string{KindRequirement, KindTest, "execution", "run"}
)

// Defect is a problem found while testing, optionally mirrored in an
// external tracker (Jira, GitHub Issues).
type Defect struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	Key            string     `json:"key"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	Severity       string     `json:"severity"`
	Priority       string     `json:"priority"`
	Assignee       string     `json:"assignee"`
	ExternalSystem string     `json:"external_system"`
	ExternalKey    string     `json:"external_key"`
	ExternalURL    string     `json:"external_url"`
	ExternalStatus string     `json:"external_status"`
	SyncedAt       *time.Time `json:"synced_at"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	// Links to requirements, tests, executions and runs.
	Links []DefectLink `json:"links,omitempty"`
}

// DefectLink connects a defect to an item.
type DefectLink struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	// Derived: key (or run name) and title of the item.
	Key   string `json:"key,omitempty"`
	Title string `json:"title,omitempty"`
}

// DefectTracker is a project's connection to an external issue tracker. It
// is used for defects and for issues created from requirements.
type DefectTracker struct {
	// Type: "" (none), "jira", "github" or "gitlab".
	Type string `json:"type"`
	// URL: Jira base URL (https://example.atlassian.net), the GitHub API
	// URL (default https://api.github.com) or the GitLab instance URL
	// (default https://gitlab.com; https only).
	URL string `json:"url"`
	// ProjectKey: Jira project key; Repo: GitHub owner/repo; Project:
	// GitLab project ID or path (group/subgroup/project).
	ProjectKey string `json:"project_key"`
	Repo       string `json:"repo"`
	Project    string `json:"project"`
	// IssueType for Jira defects (default Bug); Labels: default labels
	// added to GitHub and GitLab issues (comma-separated).
	IssueType string `json:"issue_type"`
	Labels    string `json:"labels"`
	// Milestone (GitHub and GitLab: number/ID or title) and Assignee
	// (username or numeric ID) given to new issues; empty for none.
	Milestone string `json:"milestone"`
	Assignee  string `json:"assignee"`
	// RequirementLabels maps requirement fields to labels of the issues
	// created from requirements: comma-separated templates with the
	// placeholders {type}, {priority}, {obligation}, {status} and {tags}
	// (one label per tag); "" uses DefaultRequirementLabels, "none" adds no
	// mapped labels.
	RequirementLabels string `json:"requirement_labels"`
	// RequirementIssueType: Jira issue type of the issues created from
	// requirements (default Task).
	RequirementIssueType string `json:"requirement_issue_type"`
	// CommentOnPass posts a comment on a requirement's issue when a sync
	// finds that all its linked tests passed.
	CommentOnPass bool `json:"comment_on_pass"`
	// User: Jira account e-mail (with an API token) — empty for a bearer token.
	User string `json:"user"`
	// Token is write-only: the API reports only HasToken.
	Token    string `json:"token,omitempty"`
	HasToken bool   `json:"has_token"`
}

// RiskStatuses are the states of a risk; RiskOpenStatuses still need action.
var (
	RiskStatuses     = []string{"identified", "mitigating", "mitigated", "accepted", "closed"}
	RiskOpenStatuses = []string{"identified", "mitigating"}
	// RiskLevels from lowest to highest.
	RiskLevels = []string{"low", "medium", "high", "critical"}
)

// RiskLevel is the level of a likelihood × impact score (1–25).
func RiskLevel(likelihood, impact int) string {
	switch score := likelihood * impact; {
	case score >= 17:
		return "critical"
	case score >= 10:
		return "high"
	case score >= 5:
		return "medium"
	}
	return "low"
}

// Risk is something that could go wrong, scored by likelihood and impact,
// and addressed by requirements.
type Risk struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Key         string    `json:"key"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Likelihood  int       `json:"likelihood"`
	Impact      int       `json:"impact"`
	Status      string    `json:"status"`
	Mitigation  string    `json:"mitigation"`
	Owner       string    `json:"owner"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Derived.
	Score          int      `json:"score"`
	Level          string   `json:"level"`
	RequirementIDs []string `json:"requirement_ids"`
	// Requirements with tests, and with all tests passed.
	Covered  int `json:"covered"`
	Verified int `json:"verified"`

	// Residual risk after mitigation: likelihood and impact (0 = not
	// assessed). They apply once the mitigation is verified (every
	// requirement of the risk passed its tests), or at once when entered by
	// hand (ResidualManual) with a justification.
	ResidualLikelihood    int    `json:"residual_likelihood"`
	ResidualImpact        int    `json:"residual_impact"`
	ResidualManual        bool   `json:"residual_manual"`
	ResidualJustification string `json:"residual_justification"`
	// ReviewDate (YYYY-MM-DD, "" none) and ReviewOwner of the next review.
	ReviewDate  string `json:"review_date"`
	ReviewOwner string `json:"review_owner"`
	// Acceptance: the "accept risk" electronic signature over the residual
	// assessment (AcceptedBy "" = not accepted).
	AcceptedBy        string     `json:"accepted_by"`
	AcceptedName      string     `json:"accepted_name"`
	AcceptedAt        *time.Time `json:"accepted_at"`
	AcceptanceComment string     `json:"acceptance_comment"`
	AcceptanceHash    string     `json:"-"`
	// Derived: the residual score and level in effect, where they come
	// from (ResidualInherent, ResidualVerified, ResidualManual), the
	// project's acceptance threshold, whether the residual level is above
	// it, whether the acceptance still matches the assessment, and whether
	// the review date has passed.
	ResidualScore   int    `json:"residual_score"`
	ResidualLevel   string `json:"residual_level"`
	ResidualSource  string `json:"residual_source"`
	Threshold       string `json:"threshold"`
	AboveThreshold  bool   `json:"above_threshold"`
	AcceptanceValid bool   `json:"acceptance_valid"`
	ReviewDue       bool   `json:"review_due"`
}

// ResidualOf returns the residual likelihood, impact and source in effect
// for a risk whose mitigation is verified or not.
func (k *Risk) ResidualOf(mitigationVerified bool) (int, int, string) {
	assessed := k.ResidualLikelihood > 0 && k.ResidualImpact > 0
	switch {
	case assessed && k.ResidualManual:
		return k.ResidualLikelihood, k.ResidualImpact, ResidualManual
	case assessed && mitigationVerified:
		return k.ResidualLikelihood, k.ResidualImpact, ResidualVerified
	}
	return k.Likelihood, k.Impact, ResidualInherent
}

// Notification is an in-app message to one user.
type Notification struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	ProjectID  string     `json:"project_id"`
	Event      string     `json:"event"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	EntityType string     `json:"entity_type"`
	EntityID   string     `json:"entity_id"`
	Actor      string     `json:"actor"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// WebhookEvents are the events a webhook can subscribe to ("*" = all).
var WebhookEvents = []string{
	"requirement.created", "requirement.updated", "test.created", "test.updated",
	"run.created", "run.updated", "run.completed", "execution.updated",
	"defect.created", "defect.updated", "defect.status", "defect.assigned",
	"risk.created", "risk.updated", "release.created", "release.updated", "release.released",
	"comment.created", "review.requested", "review.completed", "baseline.signed",
	"waiver.requested", "waiver.approved", "waiver.rejected", "waiver.revoked",
}

// Webhook posts events of a project to a URL, signed with HMAC-SHA256.
type Webhook struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Secret         string     `json:"secret,omitempty"` // only returned when created
	HasSecret      bool       `json:"has_secret"`
	Events         []string   `json:"events"`
	Enabled        bool       `json:"enabled"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
	LastStatus     int        `json:"last_status"`
	LastError      string     `json:"last_error"`
}

// Review request statuses and decisions.
var (
	ReviewStatuses  = []string{"pending", "approved", "changes_requested", "cancelled"}
	ReviewDecisions = []string{"approved", "changes_requested"}
	// SignoffMeanings say what a signature on a baseline means.
	SignoffMeanings = []string{"authored", "reviewed", "approved", "released"}
)

// DefaultReviewChecklists are the checklist items a new review gets per kind.
var DefaultReviewChecklists = map[string][]string{
	KindRequirement: {"Clear and unambiguous", "Verifiable (can be tested)", "Obligation level is right (MUST/SHOULD/MAY)", "No conflict or duplicate with other requirements", "Acceptance criteria are complete"},
	KindTest:        {"Preconditions and test data are stated", "Steps can be repeated by someone else", "Every step has an expected result", "Linked to the requirements it verifies"},
}

// ChecklistItem is one point of a review checklist.
type ChecklistItem struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
	Note    string `json:"note,omitempty"`
}

// ReviewRequest asks one person to review a requirement or test.
type ReviewRequest struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	EntityKey     string          `json:"entity_key"`
	EntityTitle   string          `json:"entity_title"`
	EntityVersion int             `json:"entity_version"`
	ReviewerID    string          `json:"reviewer_id"`
	ReviewerEmail string          `json:"reviewer_email"`
	ReviewerName  string          `json:"reviewer_name"`
	RequestedBy   string          `json:"requested_by"`
	Message       string          `json:"message"`
	Checklist     []ChecklistItem `json:"checklist"`
	Status        string          `json:"status"`
	Comment       string          `json:"comment"`
	DueDate       string          `json:"due_date"`
	DecidedAt     *time.Time      `json:"decided_at"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	// Outdated: the item changed after the review was requested.
	Outdated bool `json:"outdated"`
}

// Signoff is an electronic signature on a baseline.
type Signoff struct {
	ID          string    `json:"id"`
	BaselineID  string    `json:"baseline_id"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	UserName    string    `json:"user_name"`
	Meaning     string    `json:"meaning"`
	Comment     string    `json:"comment"`
	ContentHash string    `json:"content_hash"`
	SignedAt    time.Time `json:"signed_at"`
	// Valid: the baseline content still has the hash that was signed.
	Valid bool `json:"valid"`
}

// ReleaseStatuses are the states of a release.
var ReleaseStatuses = []string{"planned", "active", "released", "cancelled"}

// ExitCriterionTypes are the kinds of exit criteria of a release.
var ExitCriterionTypes = []string{
	"coverage",         // % of scope requirements with at least one test
	"passed",           // % of scope requirements whose tests all passed in the release's runs
	"mandatory_passed", // every MUST/SHALL requirement in scope passed
	"executed",         // % of the executions of the release's runs that have a result
	"no_failed",        // no failed or blocked test in the release's runs
	"approved",         // % of scope requirements approved, implemented or verified
	"no_suspect",       // no suspect links in scope
	"no_open_defects",  // no open defects (of at least the given severity) on the scope
	"risks_mitigated",  // every risk of at least the given level linked to scope requirements is mitigated or accepted
	"trace_rules",      // the enabled traceability rules pass on the scope (level error: errors only, warning: also warnings)
	"residual_risk",    // no risk linked to scope requirements has a residual level above the threshold (the level; default the project's) unless the risk is accepted
}

// ExitCriterion is one condition a release must meet.
type ExitCriterion struct {
	Type string `json:"type"`
	// Threshold in percent for the percentage criteria (default 100).
	Threshold float64 `json:"threshold,omitempty"`
	// Severity / level for no_open_defects and risks_mitigated; for
	// trace_rules the least rule severity that counts (error | warning);
	// for residual_risk the risk acceptance threshold of the release (""
	// = the project's).
	Level string `json:"level,omitempty"`
}

// Release is a version of the product with its scope and exit criteria.
type Release struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Description  string          `json:"description"`
	Status       string          `json:"status"`
	TargetDate   string          `json:"target_date"`
	ExitCriteria []ExitCriterion `json:"exit_criteria"`
	ReleasedAt   *time.Time      `json:"released_at"`
	ReleasedBy   string          `json:"released_by"`
	ReleaseNote  string          `json:"release_note"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	// Derived.
	RequirementIDs []string `json:"requirement_ids,omitempty"`
	Requirements   int      `json:"requirement_count"`
	Runs           int      `json:"run_count"`
}

// DefaultExitCriteria are given to new releases.
var DefaultExitCriteria = []ExitCriterion{
	{Type: "coverage", Threshold: 100},
	{Type: "mandatory_passed"},
	{Type: "passed", Threshold: 95},
	{Type: "no_failed"},
	{Type: "no_suspect"},
}

// ItemLinkSourceTypes are the items that can carry links.
var ItemLinkSourceTypes = []string{KindRequirement, KindTest, "run"}

// ItemLinkTargetTypes are what a link can point to: an item of the same
// project, a file of its archive (document or saved report), a report
// definition, or an external URL.
var ItemLinkTargetTypes = []string{KindRequirement, KindTest, "run", "file", "report", "url"}

// ItemLink is a link from a requirement, test or run to another item of
// the project or to an external URL.
type ItemLink struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	SourceType string    `json:"source_type"`
	SourceID   string    `json:"source_id"`
	TargetType string    `json:"target_type"`
	TargetID   *string   `json:"target_id"`
	URL        string    `json:"url"`
	Label      string    `json:"label"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	// Derived: keys (or names) and titles of both ends; TargetKind is the
	// kind of a file (document, report, attachment).
	SourceKey   string `json:"source_key"`
	SourceTitle string `json:"source_title"`
	TargetKey   string `json:"target_key"`
	TargetTitle string `json:"target_title"`
	TargetKind  string `json:"target_kind,omitempty"`
	// Direction relative to the item queried: outgoing | incoming.
	Direction string `json:"direction,omitempty"`
	// Issue is set on a link to an issue created from a requirement in the
	// project's tracker.
	Issue *LinkedIssue `json:"issue,omitempty"`
}

// DefaultRequirementLabels is the label mapping of issues created from
// requirements when the tracker settings name none.
const DefaultRequirementLabels = "requirement, type::{type}, priority::{priority}, {tags}"

// LinkedIssue is the tracker issue an item link points to.
type LinkedIssue struct {
	// Tracker: gitlab | github | jira.
	Tracker string `json:"tracker"`
	// Key: group/project#12, owner/repo#12 or SHOP-12.
	Key string `json:"key"`
	// Status is the tracker's own status; State is open or closed.
	Status   string     `json:"status"`
	State    string     `json:"state"`
	SyncedAt *time.Time `json:"synced_at"`
	// PassedAt: when the "all tests passed" comment was posted.
	PassedAt *time.Time `json:"passed_at,omitempty"`
}

// TestDependency says that TestID may only be executed after DependsOnID
// has passed (same project).
type TestDependency struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	TestID      string    `json:"test_id"`
	DependsOnID string    `json:"depends_on_id"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	// Derived.
	TestKey        string `json:"test_key"`
	TestTitle      string `json:"test_title"`
	DependsOnKey   string `json:"depends_on_key"`
	DependsOnTitle string `json:"depends_on_title"`
	// Direction relative to the test queried: outgoing (this test depends
	// on the other) | incoming (the other test depends on this one).
	Direction string `json:"direction,omitempty"`
	// LastResult of the test at the other end (GET /tests/{id}/dependencies).
	OtherLastResult string `json:"other_last_result,omitempty"`
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

// Traceability rule kinds: what an item matching the rule's selector must
// have at least Min of.
const (
	// TraceVerifiedBy: a requirement is verified by tests (trace links).
	TraceVerifiedBy = "verified_by"
	// TraceVerifies: a test verifies requirements (trace links).
	TraceVerifies = "verifies"
	// TraceRelation: a requirement has relations to other requirements
	// (e.g. refines, incoming = refined by child requirements).
	TraceRelation = "relation"
	// TraceItemLink: a requirement or test has item links.
	TraceItemLink = "item_link"
)

// TraceRuleKinds lists the rule kinds; TraceRuleSeverities their severities.
var (
	TraceRuleKinds      = []string{TraceVerifiedBy, TraceVerifies, TraceRelation, TraceItemLink}
	TraceRuleSeverities = []string{"error", "warning"}
	// TraceRuleDirections of relations and item links ("" = any).
	TraceRuleDirections = []string{"", "incoming", "outgoing"}
)

// TraceRule says that the items of a project matching Selector must have
// at least Min of a trace kind.
type TraceRule struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Severity: error | warning.
	Severity string `json:"severity"`
	Enabled  bool   `json:"enabled"`
	// Scope: the items the rule applies to, requirement | test.
	Scope string `json:"scope"`
	// Selector is a query-language expression on Scope items ("" = all).
	Selector string `json:"selector"`
	// Kind is a TraceRuleKinds value.
	Kind string `json:"kind"`
	// Min is the least number of qualifying traces (default 1; 0 only with
	// Every, to check the conditions without requiring a trace).
	Min int `json:"min"`
	TraceRuleOptions
	Position  int       `json:"position"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TraceRuleOptions are the kind-specific settings of a rule.
type TraceRuleOptions struct {
	// NotSuspect: only links that are not suspect count (verified_by, verifies).
	NotSuspect bool `json:"not_suspect,omitempty"`
	// Passed: only tests whose last result passed count (verified_by).
	Passed bool `json:"passed,omitempty"`
	// RelationType of relation rules ("" = any type).
	RelationType string `json:"relation_type,omitempty"`
	// Direction of relations and item links: incoming | outgoing | "" (any).
	Direction string `json:"direction,omitempty"`
	// TargetType of item links: the type at the other end ("" = any).
	TargetType string `json:"target_type,omitempty"`
	// TargetQuery filters the requirement or test at the other end.
	TargetQuery string `json:"target_query,omitempty"`
	// Every: every trace of the kind must meet the conditions (e.g. no
	// suspect link at all), not only Min of them.
	Every bool `json:"every,omitempty"`
}

// MFA policy modes.
const (
	MFAOptional = "optional" // users may enrol
	MFAAll      = "all"      // required for everyone in the scope
	MFARoles    = "roles"    // required for the listed roles
)

// MFASuperAdminRole is the pseudo-role of super administrators in the
// server policy's role list.
const MFASuperAdminRole = "super_admin"

// MFAPolicy is the MFA policy of the server (Scope "") or of a tenant.
type MFAPolicy struct {
	Scope string   `json:"scope"`
	Mode  string   `json:"mode"`
	Roles []string `json:"roles"`
	// GraceDays: users the policy newly covers may keep signing in without
	// a second factor for this many days; afterwards they must enrol at
	// their next sign-in. 0 = at the next sign-in.
	GraceDays int `json:"grace_days"`
	// RememberDays: "remember this device" skips the second factor for
	// this many days (server policy only; 0 = off).
	RememberDays int `json:"remember_days"`
	// RequiredSince is when the policy started to require MFA (the grace
	// period counts from then, or from the user's creation if later).
	RequiredSince *time.Time `json:"required_since"`
	UpdatedBy     string     `json:"updated_by"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

// Requires reports whether the policy requires MFA for a role (or the
// super administrator pseudo-role).
func (p *MFAPolicy) Requires(role string) bool {
	switch p.Mode {
	case MFAAll:
		return true
	case MFARoles:
		for _, r := range p.Roles {
			if r == role {
				return true
			}
		}
	}
	return false
}

// UserMFA is a user's TOTP enrolment.
type UserMFA struct {
	UserID     string
	Secret     string
	EnabledAt  *time.Time
	LastStep   int64
	CreatedAt  time.Time
	CodesLeft  int
	CodesTotal int
}

// MFAChallenge is a sign-in waiting for its second factor.
type MFAChallenge struct {
	ID        string
	UserID    string
	TenantID  string
	Method    string
	Details   string
	Attempts  int
	CreatedAt time.Time
	ExpiresAt time.Time
}
