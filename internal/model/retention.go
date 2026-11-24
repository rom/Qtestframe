package model

import "time"

// Retention categories (Administration → System settings → Retention and
// backups). Each is kept for a number of days; 0 keeps it forever.
const (
	RetainRuns          = "runs"          // test runs with their results and attachments
	RetainLogs          = "logs"          // sign-in, job, CI and sync logs; web test evidence
	RetainNotifications = "notifications" // in-app notifications
	RetainSessions      = "sessions"      // sign-in sessions and expired API tokens
	RetainChat          = "chat"          // chat delivery history and input suggestion history
	RetainExploratory   = "exploratory"   // notes of finished exploratory sessions
	RetainAudit         = "audit"         // the audit trail: never purged (hash chain)
)

// MaxRetentionDays is the longest retention period (100 years).
const MaxRetentionDays = 36500

// RetentionCategories lists the purgeable categories in display order.
var RetentionCategories = []string{RetainRuns, RetainLogs, RetainNotifications, RetainSessions, RetainChat, RetainExploratory}

// RetentionOverrideKeys are the keys of a per-organisation override
// (TenantPolicy.Retention): "<category>_days" and runs_keep_latest.
var RetentionOverrideKeys = []string{"runs_days", "runs_keep_latest", "logs_days", "notifications_days", "sessions_days", "chat_days", "exploratory_days"}

// RetentionPolicy is the effective retention of an organisation: days per
// category (0 = forever) and the number of latest runs of each test that
// are always kept.
type RetentionPolicy struct {
	Days map[string]int `json:"days"`
	// RunsKeepLatest: runs holding one of the latest N results of a test
	// are never purged (0 = no exception).
	RunsKeepLatest int `json:"runs_keep_latest"`
	// Overridden names the keys set by the organisation's override.
	Overridden []string `json:"overridden,omitempty"`
}

// RetentionCount is the outcome of a preview or a purge for one category.
type RetentionCount struct {
	Category string `json:"category"`
	Days     int    `json:"days"`
	// Count is what would be (preview) or was (purge) deleted.
	Count int `json:"count"`
	// Held is what is older than the period but kept by a legal hold or an
	// electronic signature.
	Held int `json:"held"`
}

// RetentionReport is the preview or the result of a purge.
type RetentionReport struct {
	TenantID   string           `json:"tenant_id"`
	TenantName string           `json:"tenant_name,omitempty"`
	DryRun     bool             `json:"dry_run"`
	Policy     RetentionPolicy  `json:"policy"`
	Counts     []RetentionCount `json:"counts"`
	At         time.Time        `json:"at"`
}

// Total returns the number of deleted (or to be deleted) records.
func (r *RetentionReport) Total() int {
	n := 0
	for _, c := range r.Counts {
		n += c.Count
	}
	return n
}

// LegalHold is the legal hold of a project or a release: while it is on,
// nothing of it is purged by the retention job and it cannot be deleted.
type LegalHold struct {
	On     bool       `json:"on"`
	Reason string     `json:"reason"`
	By     string     `json:"by,omitempty"`
	At     *time.Time `json:"at,omitempty"`
}

// HeldItem is a project or release under legal hold (the list of the
// retention page).
type HeldItem struct {
	Kind        string    `json:"kind"` // project | release
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	TenantName  string    `json:"tenant_name"`
	ProjectID   string    `json:"project_id"`
	ProjectKey  string    `json:"project_key"`
	ProjectName string    `json:"project_name"`
	Name        string    `json:"name"`
	Hold        LegalHold `json:"hold"`
}

// ReleaseHold is a release of a project with its legal hold.
type ReleaseHold struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Hold    LegalHold `json:"hold"`
}

// ErasedEmailDomain is the domain of the pseudonymous address an erased
// account gets ("former-user-<n>@erased.invalid").
const ErasedEmailDomain = "erased.invalid"
