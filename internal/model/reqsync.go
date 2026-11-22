package model

import "time"

// Two-way sync of requirements with Jira and Azure DevOps (migration 0060,
// internal/api/reqsync.go). The connections and their write-only tokens are
// the import sources (ImportSources); a sync configuration says, per
// source, which fields go which way, which items take part and what
// happens when both sides changed.

// Directions of a synced field.
const (
	SyncPull = "pull" // tracker → Qtestframe
	SyncPush = "push" // Qtestframe → tracker
	SyncBoth = "both"
	SyncNone = "none"
)

// SyncDirections are the directions of a field.
var SyncDirections = []string{SyncPull, SyncPush, SyncBoth, SyncNone}

// Conflict policies: what a sync does with a field changed on both sides.
const (
	SyncTrackerWins = "tracker"
	SyncLocalWins   = "qtestframe"
	SyncManual      = "manual"
)

// SyncConflictPolicies are the conflict policies.
var SyncConflictPolicies = []string{SyncTrackerWins, SyncLocalWins, SyncManual}

// SyncFields are the requirement fields a sync can map.
var SyncFields = []string{"title", "description", "status", "priority", "tags", "criteria", "rationale"}

// ReqSyncField maps a requirement field to a tracker field.
type ReqSyncField struct {
	// Field is the requirement field (SyncFields).
	Field string `json:"field"`
	// Remote is the tracker's field ID (summary, customfield_10050,
	// System.Title, Microsoft.VSTS.Common.Priority, …).
	Remote    string `json:"remote"`
	Direction string `json:"direction"`
	// Values maps tracker values to requirement values (pull), PushValues
	// requirement values to tracker values (push); both case-insensitive.
	// Used for status and priority; a status without an entry falls back
	// to the usual workflow names.
	Values     map[string]string `json:"values,omitempty"`
	PushValues map[string]string `json:"push_values,omitempty"`
}

// ReqSyncPush says which requirements made in Qtestframe are pushed to
// the tracker as new issues or work items.
type ReqSyncPush struct {
	// Create: requirements in the scope without a tracker item get one.
	Create bool `json:"create"`
	// The scope: requirements in the folder (and its subfolders; "" all)
	// with the tag ("" any) that match the query (Qtestframe query
	// language, "" all).
	FolderID string `json:"folder_id"`
	Tag      string `json:"tag"`
	Query    string `json:"query"`
	// ItemType: the Jira issue type or Azure DevOps work item type of new
	// items (default Story / User Story); ProjectKey the Jira project
	// (default the defect tracker's project key).
	ItemType   string `json:"item_type"`
	ProjectKey string `json:"project_key"`
}

// ReqSyncSettings is the configuration of the sync with one source.
type ReqSyncSettings struct {
	Fields []ReqSyncField `json:"fields"`
	// Query (JQL or WIQL) selects the tracker items that take part; ""
	// uses the import source's default query. Items linked to
	// requirements take part also when the query no longer finds them.
	Query string `json:"query"`
	// ImportNew creates requirements (in FolderID) for items the query
	// finds that are not linked yet.
	ImportNew bool        `json:"import_new"`
	FolderID  string      `json:"folder_id"`
	Push      ReqSyncPush `json:"push"`
	// ConflictPolicy: SyncTrackerWins, SyncLocalWins or SyncManual.
	ConflictPolicy string `json:"conflict_policy"`
	// Impact is what a change pulled from the tracker does to the linked
	// items (ImpactChoices: suspect, notify, skip).
	Impact string `json:"impact"`
	// Comments syncs the discussions with the comments; Links the
	// relations between synced requirements with the tracker's links.
	Comments bool `json:"comments"`
	Links    bool `json:"links"`
	// MaxWritesPerMinute bounds the writes to the tracker (default 60).
	MaxWritesPerMinute int `json:"max_writes_per_minute"`
	// CommentsSince: comments older than this (the time comment sync was
	// turned on) are not synced. Set by the server.
	CommentsSince *time.Time `json:"comments_since,omitempty"`
}

// ReqSyncConfig is a project's sync with one source (the API view).
type ReqSyncConfig struct {
	ID       string          `json:"id"`
	System   string          `json:"system"`
	Enabled  bool            `json:"enabled"`
	Settings ReqSyncSettings `json:"settings"`
	// IntervalMinutes between scheduled syncs (0 = only by hand and webhooks).
	IntervalMinutes int `json:"interval_minutes"`
	// The webhook receiver: its URL and whether a secret is set (the
	// secret is write-only).
	HasWebhookSecret bool   `json:"has_webhook_secret"`
	WebhookURL       string `json:"webhook_url"`
	// RunAs: the user whose permissions scheduled and webhook syncs use.
	RunAs      string     `json:"run_as"`
	NextRunAt  *time.Time `json:"next_run_at"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastStatus string     `json:"last_status"`
	LastError  string     `json:"last_error"`
	Failures   int        `json:"failures"`
	// OpenConflicts: conflicts waiting for a decision.
	OpenConflicts int       `json:"open_conflicts"`
	Linked        int       `json:"linked"`
	Version       int       `json:"version"`
	UpdatedBy     string    `json:"updated_by"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Connected: the import source of the system is configured.
	Connected bool `json:"connected"`
}

// ReqSyncFieldDiff is a field of a conflict.
type ReqSyncFieldDiff struct {
	Field string `json:"field"`
	// Base: the value both sides had after the last sync.
	Base   string `json:"base"`
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

// ReqSyncConflict is an item changed on both sides (policy manual).
type ReqSyncConflict struct {
	ID             string             `json:"id"`
	ProjectID      string             `json:"project_id"`
	System         string             `json:"system"`
	RequirementID  string             `json:"requirement_id"`
	RequirementKey string             `json:"requirement_key"`
	Title          string             `json:"title"`
	ExternalID     string             `json:"external_id"`
	ExternalURL    string             `json:"external_url,omitempty"`
	Fields         []ReqSyncFieldDiff `json:"fields"`
	LocalVersion   int                `json:"local_version"`
	RemoteVersion  string             `json:"remote_version"`
	Status         string             `json:"status"` // open | resolved | dismissed
	Resolution     string             `json:"resolution,omitempty"`
	ResolvedBy     string             `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time         `json:"resolved_at,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// ReqSyncEntry is what a sync did (or, in a dry run, would do) with an item.
type ReqSyncEntry struct {
	RequirementID  string `json:"requirement_id,omitempty"`
	RequirementKey string `json:"requirement_key,omitempty"`
	ExternalID     string `json:"external_id,omitempty"`
	Title          string `json:"title,omitempty"`
	// Action: pull, push, create_local, create_remote, conflict,
	// comment_pull, comment_push, link_pull, link_push, skip, error.
	Action string `json:"action"`
	// Fields changed (or in conflict).
	Fields []string `json:"fields,omitempty"`
	// Changes: field → "old → new" (dry run and log).
	Changes map[string]string `json:"changes,omitempty"`
	Message string            `json:"message,omitempty"`
}

// ReqSyncSummary counts what a run did.
type ReqSyncSummary struct {
	Pulled        int `json:"pulled"`
	Pushed        int `json:"pushed"`
	CreatedLocal  int `json:"created_local"`
	CreatedRemote int `json:"created_remote"`
	Conflicts     int `json:"conflicts"`
	Comments      int `json:"comments"`
	Links         int `json:"links"`
	Unchanged     int `json:"unchanged"`
	Errors        int `json:"errors"`
}

// ReqSyncRun is an entry of the sync log.
type ReqSyncRun struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"project_id"`
	System     string         `json:"system"`
	Trigger    string         `json:"trigger"` // manual | schedule | webhook | resolve
	DryRun     bool           `json:"dry_run"`
	Status     string         `json:"status"` // ok | partial | error | running
	Summary    ReqSyncSummary `json:"summary"`
	Entries    []ReqSyncEntry `json:"entries"`
	Error      string         `json:"error,omitempty"`
	StartedBy  string         `json:"started_by"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at"`
}
