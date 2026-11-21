package model

import "time"

// WebTestSettings are a project's settings for web tests (script language
// "playwright"): where they run against and the variables passed to every
// automated script.
type WebTestSettings struct {
	// BaseURL is Playwright's baseURL (page.goto('/login') is relative to
	// it) unless an environment URL or a configuration overrides it.
	BaseURL string `json:"base_url"`
	// EnvironmentURLs: a run whose environment matches (case-insensitive)
	// uses that URL instead.
	EnvironmentURLs []EnvironmentURL `json:"environment_urls"`
	// Variables are passed to automated scripts as QTF_VAR_<NAME>.
	Variables []TestVariable `json:"variables"`
}

// EnvironmentURL is the base URL of a run environment.
type EnvironmentURL struct {
	Environment string `json:"environment"`
	BaseURL     string `json:"base_url"`
}

// TestVariable is a project-level variable of automated tests. The value
// of a secret variable is write-only: the API never returns it (HasValue
// says whether one is stored) and it is masked in stored output.
type TestVariable struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Secret   bool   `json:"secret"`
	HasValue bool   `json:"has_value"`
}

// Script job states.
const (
	JobQueued      = "queued"
	JobRunning     = "running"
	JobPassed      = "passed"
	JobFailed      = "failed"
	JobSkipped     = "skipped"
	JobCancelled   = "cancelled"
	JobError       = "error"
	JobInterrupted = "interrupted"
)

// JobActive reports whether a job state is queued or running.
func JobActive(state string) bool { return state == JobQueued || state == JobRunning }

// ScriptJob is an automated test (script or web test) run in the
// background.
type ScriptJob struct {
	ID          string `json:"id"`
	TenantID    string `json:"-"`
	ProjectID   string `json:"project_id"`
	RunID       string `json:"run_id"`
	ExecutionID string `json:"execution_id"`
	BatchID     string `json:"batch_id"`
	Seq         int    `json:"seq"`
	State       string `json:"state"`
	// After lists jobs (prerequisites) that must finish first.
	After       []string `json:"after"`
	RequestedBy string   `json:"requested_by"`
	UserID      string   `json:"-"`
	Instance    string   `json:"-"`
	// Message explains the outcome (failure, skip or cancel reason).
	Message    string     `json:"message"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	// Previous is the execution's result before the job.
	Previous ExecutionState `json:"-"`
	// Set in lists: the execution's test and configuration.
	TestKey           string `json:"test_key,omitempty"`
	TestTitle         string `json:"test_title,omitempty"`
	ConfigurationName string `json:"configuration_name,omitempty"`
}

// ExecutionState is the result part of an execution, saved by a job to
// restore it.
type ExecutionState struct {
	Status     string     `json:"status"`
	ExecutedBy string     `json:"executed_by"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMS int64      `json:"duration_ms"`
}
