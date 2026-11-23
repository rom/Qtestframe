package model

import "time"

// Test environments (migration 0057): the systems tests run against, their
// components and versions, their bookings, and the test data sets tests
// use. See docs/api.md "Test environments" and "Test data sets".

// KindEnvironment, KindBooking and KindDataSet name environments, their
// bookings and test data sets (entity types of revisions, attachments,
// notifications and the audit log).
const (
	KindEnvironment = "environment"
	KindBooking     = "environment_booking"
	KindDataSet     = "test_data_set"
)

// Environment types and states.
var (
	// EnvironmentTypes: dev, test, staging, pre-production, a
	// production-like system, a lab, a hardware-in-the-loop rig, a device.
	EnvironmentTypes = []string{"dev", "test", "staging", "pre_prod", "prod_like", "lab", "hil_rig", "device"}
	// EnvironmentStatuses: available (can be booked and used), in_use,
	// maintenance, broken and retired (kept for the history, not bookable).
	EnvironmentStatuses = []string{"available", "in_use", "maintenance", "broken", "retired"}
	// BookingStatuses: pending (waits for approval), approved, rejected,
	// cancelled. BookingActiveStatuses take up capacity.
	BookingStatuses       = []string{"pending", "approved", "rejected", "cancelled"}
	BookingActiveStatuses = []string{"pending", "approved"}
)

// Component is one part of an environment and its version ("app" 2.3.1,
// "DB" 16).
type Component struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Environment is a system tests run against.
type Environment struct {
	ID string `json:"id"`
	// ProjectID: the project it belongs to; "" for an environment shared
	// by the organisation (Shared).
	ProjectID   string      `json:"project_id"`
	Shared      bool        `json:"shared"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	URLs        []string    `json:"urls"`
	Description string      `json:"description"`
	Components  []Component `json:"components"`
	// OwnerID: the user responsible (approves bookings with the
	// environment managers).
	OwnerID string `json:"owner_id"`
	Status  string `json:"status"`
	// StatusNote: why (maintenance window, what is broken); StatusByHealth:
	// the health check set the status (broken), so a passing check sets it
	// back to available.
	StatusNote     string   `json:"status_note"`
	StatusByHealth bool     `json:"status_by_health"`
	Tags           []string `json:"tags"`
	// ConfigurationIDs and VariantIDs: the configurations and variants the
	// environment provides (project environments only).
	ConfigurationIDs []string `json:"configuration_ids"`
	VariantIDs       []string `json:"variant_ids"`
	// Capacity: how many bookings may overlap (1: exclusive).
	Capacity         int  `json:"capacity"`
	RequiresApproval bool `json:"requires_approval"`
	// CredentialsRef says where the credentials are (a vault path, a note);
	// never the secret itself.
	CredentialsRef string `json:"credentials_ref"`
	// Secret is write-only: it is accepted on create and update ("" keeps
	// it, ClearSecret removes it) and never returned; HasSecret says
	// whether one is stored. POST /environments/{id}/secret reveals it to
	// environment managers (audited).
	Secret      string `json:"secret,omitempty"`
	HasSecret   bool   `json:"has_secret"`
	ClearSecret bool   `json:"clear_secret,omitempty"`
	// Health check: HealthURL is fetched every HealthIntervalMinutes;
	// HealthStatus up | down ("" not checked yet).
	HealthURL             string     `json:"health_url"`
	HealthIntervalMinutes int        `json:"health_interval_minutes"`
	HealthStatus          string     `json:"health_status"`
	HealthCheckedAt       *time.Time `json:"health_checked_at"`
	HealthError           string     `json:"health_error"`
	Version               int        `json:"version"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	// Derived: the bookings now (pending or approved) and the next
	// approved booking to come; the runs that selected it.
	BookedNow   int        `json:"booked_now"`
	NextBooking *time.Time `json:"next_booking,omitempty"`
	RunCount    int        `json:"run_count"`
	ProjectKey  string     `json:"project_key,omitempty"`
}

// EnvironmentRef is a short reference to an environment.
type EnvironmentRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Booking reserves an environment for a time.
type Booking struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environment_id"`
	// ProjectID: the project it was booked for (its runs, cycles and
	// sessions can be linked).
	ProjectID string    `json:"project_id"`
	UserID    string    `json:"user_id"`
	BookedBy  string    `json:"booked_by"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Purpose   string    `json:"purpose"`
	RunID     *string   `json:"run_id"`
	CycleID   *string   `json:"cycle_id"`
	SessionID *string   `json:"session_id"`
	Status    string    `json:"status"`
	// The decision on a booking that needed approval.
	DecidedBy    string     `json:"decided_by"`
	DecidedAt    *time.Time `json:"decided_at"`
	DecisionNote string     `json:"decision_note"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// Derived.
	EnvironmentName string `json:"environment_name,omitempty"`
	ProjectKey      string `json:"project_key,omitempty"`
	RunName         string `json:"run_name,omitempty"`
	CycleName       string `json:"cycle_name,omitempty"`
	SessionKey      string `json:"session_key,omitempty"`
	// Hidden: the booking is for a project the caller does not see (only
	// the time and the status are shown).
	Hidden bool `json:"hidden,omitempty"`
}

// BookingConflict is the answer to a booking that does not fit: the
// overlapping bookings (pending and approved) and the capacity.
type BookingConflict struct {
	Capacity    int       `json:"capacity"`
	Overlapping []Booking `json:"overlapping"`
}

// ReleaseEnvironment is one environment (with the component versions at
// execution time) the results of a release's runs were obtained in.
type ReleaseEnvironment struct {
	EnvironmentID string      `json:"environment_id"`
	Name          string      `json:"name"`
	Type          string      `json:"type"`
	Components    []Component `json:"components"`
	Executions    int         `json:"executions"`
	Passed        int         `json:"passed"`
	Failed        int         `json:"failed"`
	Blocked       int         `json:"blocked"`
	LastAt        *time.Time  `json:"last_at"`
}

// Test data sets.
var (
	// DataSetKinds: table (parameters and rows, usable by data-driven
	// tests) or files (attachments of the set).
	DataSetKinds = []string{"table", "files"}
	// DataClassifications: synthetic, anonymised or a production copy
	// (shown with a warning: it may hold personal or confidential data).
	DataClassifications = []string{"synthetic", "anonymised", "production_copy"}
	DataSetStatuses     = []string{"active", "archived"}
	// DataLinkTypes are the items a set can be linked to; DataLinkUsages:
	// source (a data-driven test takes its rows from the set) or reference.
	DataLinkTypes     = []string{KindTest, "configuration", "run"}
	DataLinkUsages    = []string{"source", "reference"}
	RefreshStatuses   = []string{"open", "done", "rejected"}
	DataSetExpiryDays = 14 // a set expiring within this many days is "expiring"
)

// DataSet is a named, versioned set of test data.
type DataSet struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"`
	Parameters  []string  `json:"parameters"`
	DataRows    []DataRow `json:"data_rows"`
	// Classification: synthetic | anonymised | production_copy.
	Classification string `json:"classification"`
	OwnerID        string `json:"owner_id"`
	// ExpiresOn: YYYY-MM-DD ("" never).
	ExpiresOn string   `json:"expires_on"`
	Tags      []string `json:"tags"`
	Status    string   `json:"status"`
	// Version counts the changes; usage records the version a run used.
	Version   int       `json:"version"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Derived.
	Links         []DataLink `json:"links"`
	UsageCount    int        `json:"usage_count"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	OpenRefreshes int        `json:"open_refreshes"`
	FileCount     int        `json:"file_count"`
	Expired       bool       `json:"expired"`
	Expiring      bool       `json:"expiring"`
}

// DataLink links a data set to a test, configuration or run.
type DataLink struct {
	ID        string    `json:"id"`
	DataSetID string    `json:"data_set_id"`
	ItemType  string    `json:"item_type"`
	ItemID    string    `json:"item_id"`
	Usage     string    `json:"usage"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	// Derived: the item's key or name.
	ItemKey   string `json:"item_key"`
	ItemTitle string `json:"item_title"`
	// DataSetName (lists of an item's sets).
	DataSetName string `json:"data_set_name,omitempty"`
}

// DataUsage is one use of a data set by a run.
type DataUsage struct {
	ID          string    `json:"id"`
	DataSetID   string    `json:"data_set_id"`
	DataVersion int       `json:"data_version"`
	RunID       *string   `json:"run_id"`
	TestCaseID  *string   `json:"test_case_id"`
	UsedBy      string    `json:"used_by"`
	UsedAt      time.Time `json:"used_at"`
	RunName     string    `json:"run_name,omitempty"`
	TestKey     string    `json:"test_key,omitempty"`
}

// RefreshRequest asks the owner of a data set to refresh it.
type RefreshRequest struct {
	ID             string     `json:"id"`
	DataSetID      string     `json:"data_set_id"`
	RequesterID    string     `json:"requester_id"`
	RequestedBy    string     `json:"requested_by"`
	Reason         string     `json:"reason"`
	NeededBy       string     `json:"needed_by"`
	Status         string     `json:"status"`
	ResolvedBy     string     `json:"resolved_by"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	ResolutionNote string     `json:"resolution_note"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DataSetName    string     `json:"data_set_name,omitempty"`
	ProjectID      string     `json:"project_id,omitempty"`
}
