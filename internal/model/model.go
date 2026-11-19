// Package model contains the domain types shared by the store, API and
// export layers. JSON tags define the public REST contract (see docs/api.md).
package model

import "time"

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
}

// Membership binds a user to a tenant with a role.
type Membership struct {
	TenantID   string    `json:"tenant_id"`
	TenantSlug string    `json:"tenant_slug,omitempty"`
	TenantName string    `json:"tenant_name,omitempty"`
	UserID     string    `json:"user_id"`
	Email      string    `json:"email,omitempty"`
	Name       string    `json:"display_name,omitempty"`
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
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	FolderPath  string `json:"folder_path,omitempty"`
	LinkedTests int    `json:"linked_tests"`
	Coverage    string `json:"coverage,omitempty"` // none | not_run | passed | failed | partial
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
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// Derived.
	FolderPath string `json:"folder_path,omitempty"`
	LinkedReqs int    `json:"linked_requirements"`
	LastResult string `json:"last_result,omitempty"`
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
	StepResults []StepResult `json:"step_results,omitempty"`
}

// AuditEntry records a change.
type AuditEntry struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	UserID     string    `json:"user_id"`
	UserEmail  string    `json:"user_email"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Details    string    `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}
