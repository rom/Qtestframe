package model

import (
	"encoding/json"
	"time"
)

// Change requests (formal change control): a change request proposes new
// field values for requirements and tests (a diff against the version
// they were proposed on) with a reason, is assessed (the transitive
// change impact is computed and stored on it), decided by the project's
// change control board with electronic signatures, applied atomically
// (normal revisions, history and the chosen impact action), verified and
// closed, optionally with a new baseline.
//
// With change control enforced, items whose status is protected (default
// approved, implemented, verified, ready) or that are in a signed baseline
// cannot be edited directly: only by applying an approved change request,
// or with an audited emergency override.

// KindChangeRequest names change requests (keys PROJ-CR-n).
const KindChangeRequest = "change_request"

// Change request states, categories, decisions and change control modes.
var (
	// CRStatuses: draft → submitted → assessed → in_review → approved |
	// rejected | deferred; approved → implemented → verified → closed.
	// Rejected and deferred change requests can be withdrawn to draft or
	// closed.
	CRStatuses = []string{"draft", "submitted", "assessed", "in_review", "approved", "rejected", "deferred", "implemented", "verified", "closed"}
	// CROpenStatuses are the states of a change request that is not
	// finished: its items show it as an open change request.
	CROpenStatuses = []string{"draft", "submitted", "assessed", "in_review", "approved", "deferred"}
	CRCategories   = []string{"correction", "enhancement", "regulatory", "defect_driven", "risk_driven", "other"}
	// CRDecisions are the decisions of a board member's signature.
	CRDecisions = []string{"approve", "reject", "defer"}
	// ChangeControlModes: off (default, items are edited directly), warn
	// (direct edits of protected items are allowed but flagged), enforce
	// (protected items change only through change requests).
	ChangeControlModes = []string{"off", "warn", "enforce"}
	// DefaultProtectedStatuses are the statuses under change control of a
	// project that chose none.
	DefaultProtectedStatuses = []string{"approved", "implemented", "verified", "ready"}
)

// CROpen reports whether a change request in status is still open.
func CROpen(status string) bool {
	for _, s := range CROpenStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// ChangeBoard is the change control board of a project: the members who
// decide change requests (by e-mail address and/or by role) and how many
// approvals are needed: "all" (every listed user, and one holder of each
// listed role), "any" (one) or a number.
type ChangeBoard struct {
	Users  []string `json:"users"`
	Roles  []string `json:"roles"`
	Quorum string   `json:"quorum"`
}

// ChangeControlSettings are the change control settings of a project.
type ChangeControlSettings struct {
	Mode string `json:"mode"`
	// ProtectedStatuses: the item statuses under change control (empty:
	// DefaultProtectedStatuses).
	ProtectedStatuses []string `json:"protected_statuses"`
	// LockSignedBaselines: the items of a baseline with a sign-off are
	// under change control whatever their status.
	LockSignedBaselines bool        `json:"lock_signed_baselines"`
	Board               ChangeBoard `json:"board"`
	// SignatureMeaning is what a board member's signature means (shown in
	// the signature dialog and recorded with it).
	SignatureMeaning string `json:"signature_meaning"`
	// BaselineOnClose: closing a change request creates a baseline unless
	// the closer chooses otherwise.
	BaselineOnClose bool       `json:"baseline_on_close"`
	UpdatedBy       string     `json:"updated_by"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

// DefaultSignatureMeaning is what a board signature means when the
// project set no text.
const DefaultSignatureMeaning = "I have assessed this change request and its impact, and I decide it as stated."

// ChangeRequestLinks are what a change request refers to.
type ChangeRequestLinks struct {
	DefectIDs []string `json:"defect_ids"`
	RiskIDs   []string `json:"risk_ids"`
	// Issues: tracker issues (keys or URLs).
	Issues []string `json:"issues"`
}

// ChangeRequest proposes changes to requirements and tests.
type ChangeRequest struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Key       string `json:"key"`
	Title     string `json:"title"`
	// Reason: why the change is needed (the justification).
	Reason   string `json:"reason"`
	Category string `json:"category"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
	// ImpactAction: what applying does with the impact (ImpactChoices).
	ImpactAction string `json:"impact_action"`
	// Impact: the change impact computed at the assessment (nil before).
	Impact      *ChangeImpact `json:"impact"`
	ImpactNotes string        `json:"impact_notes"`
	// Effort: the assessor's effort estimate (free text, e.g. "3 days").
	Effort          string             `json:"effort"`
	AssessedBy      string             `json:"assessed_by"`
	AssessedAt      *time.Time         `json:"assessed_at"`
	Links           ChangeRequestLinks `json:"links"`
	BaselineOnClose bool               `json:"baseline_on_close"`
	// BaselineID: the baseline created when it was closed ("" none).
	BaselineID      string     `json:"baseline_id"`
	DecidedBy       string     `json:"decided_by"`
	DecidedAt       *time.Time `json:"decided_at"`
	DecisionComment string     `json:"decision_comment"`
	AppliedBy       string     `json:"applied_by"`
	AppliedAt       *time.Time `json:"applied_at"`
	VerifiedBy      string     `json:"verified_by"`
	VerifiedAt      *time.Time `json:"verified_at"`
	ClosedBy        string     `json:"closed_by"`
	ClosedAt        *time.Time `json:"closed_at"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	// Derived.
	Items     []ChangeRequestItem    `json:"items"`
	Approvals []ChangeRequestSignoff `json:"approvals"`
	Events    []ChangeRequestEvent   `json:"events"`
	Defects   []ItemRefLite          `json:"defects"`
	Risks     []ItemRefLite          `json:"risks"`
	// ContentHash is what the board's signatures sign: the items with
	// their proposals and base versions, the reason and the impact action.
	ContentHash string `json:"content_hash"`
	// Outdated: an item changed since its change was proposed.
	Outdated bool `json:"outdated"`
}

// ChangeRequestItem is the proposed change of one requirement or test.
type ChangeRequestItem struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	// BaseVersion: the version the change was proposed on.
	BaseVersion int `json:"base_version"`
	// Proposed: the new field values (field → JSON value, as in a PATCH).
	Proposed       map[string]json.RawMessage `json:"proposed"`
	AppliedVersion int                        `json:"applied_version"`
	// Derived: the item's key, title, current version and current values
	// of the proposed fields; Outdated when the version moved on since the
	// proposal; Missing when the item no longer exists.
	Key            string                     `json:"key"`
	Title          string                     `json:"title"`
	Status         string                     `json:"status"`
	CurrentVersion int                        `json:"current_version"`
	Current        map[string]json.RawMessage `json:"current"`
	Outdated       bool                       `json:"outdated"`
	Missing        bool                       `json:"missing,omitempty"`
}

// ChangeRequestSignoff is a board member's signed decision.
type ChangeRequestSignoff struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	UserName    string    `json:"user_name"`
	Decision    string    `json:"decision"`
	Meaning     string    `json:"meaning"`
	Comment     string    `json:"comment"`
	ContentHash string    `json:"content_hash"`
	SignedAt    time.Time `json:"signed_at"`
	// Valid: the change request still has the content that was signed.
	Valid bool `json:"valid"`
}

// ChangeRequestEvent is one step of a change request's timeline.
type ChangeRequestEvent struct {
	ID      string    `json:"id"`
	Action  string    `json:"action"`
	Actor   string    `json:"actor"`
	Comment string    `json:"comment"`
	At      time.Time `json:"at"`
}

// CRProposalFields are the fields a change request can propose per kind.
var CRProposalFields = map[string][]string{
	KindRequirement: {"title", "description", "obligation", "type", "priority", "status", "source", "rationale", "owner", "tags", "custom_fields"},
	KindTest:        {"title", "description", "preconditions", "expected_result", "execution_type", "priority", "status", "estimated_minutes", "owner", "tags", "custom_fields"},
}
