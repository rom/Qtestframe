package model

import "time"

// Waivers and deviations: a formal record that accepts requirements that
// are failed, untested or not fulfilled (a waiver: accept not meeting them
// now) or met by a different solution (a deviation), for one release or for
// the whole project. An approved waiver that has not expired makes release
// readiness and the exit criteria treat its requirements as accepted, shown
// as waived.

// KindWaiver names waivers (keys PROJ-WVR-n).
const KindWaiver = "waiver"

// Waiver types and states.
var (
	WaiverTypes = []string{"waiver", "deviation"}
	// WaiverStatuses: draft → requested → approved | rejected; approved →
	// expired (past its expiry date) | revoked.
	WaiverStatuses = []string{"draft", "requested", "approved", "rejected", "expired", "revoked"}
	// WaiverDecisions are the decisions of an approver's signature.
	WaiverDecisions = []string{"approve", "reject"}
)

// Waiver accepts requirements that are not (or differently) met.
type Waiver struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Key       string `json:"key"`
	// Type: waiver | deviation.
	Type  string `json:"type"`
	Title string `json:"title"`
	// ReleaseID: the release it applies to; nil = the whole project.
	ReleaseID      *string  `json:"release_id"`
	RequirementIDs []string `json:"requirement_ids"`
	Justification  string   `json:"justification"`
	// Compensating measures that reduce the effect of not meeting the
	// requirements.
	Compensating string `json:"compensating"`
	// Conditions under which the acceptance holds.
	Conditions string `json:"conditions"`
	// RiskID: the risk that records what not meeting the requirements means.
	RiskID      *string `json:"risk_id"`
	RequestedBy string  `json:"requested_by"`
	// Approvers (e-mail addresses) who must all approve; empty: one
	// approval by anyone holding waiver.approve suffices.
	Approvers []string `json:"approvers"`
	Status    string   `json:"status"`
	// ExpiresOn (YYYY-MM-DD, "" none): from the day after, the waiver is
	// expired and its requirements are no longer accepted.
	ExpiresOn       string     `json:"expires_on"`
	DecidedAt       *time.Time `json:"decided_at"`
	DecidedBy       string     `json:"decided_by"`
	DecisionComment string     `json:"decision_comment"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	// Derived.
	ReleaseName  string           `json:"release_name,omitempty"`
	RiskKey      string           `json:"risk_key,omitempty"`
	Requirements []ItemRefLite    `json:"requirements"`
	Approvals    []WaiverApproval `json:"approvals"`
	// Active: approved and not expired today.
	Active bool `json:"active"`
	// ContentHash is what approvals sign.
	ContentHash string `json:"content_hash"`
}

// ItemRefLite is the key and title of an item.
type ItemRefLite struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

// WaiverApproval is an approver's electronic signature on a waiver.
type WaiverApproval struct {
	ID          string    `json:"id"`
	WaiverID    string    `json:"waiver_id"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	UserName    string    `json:"user_name"`
	Decision    string    `json:"decision"`
	Comment     string    `json:"comment"`
	ContentHash string    `json:"content_hash"`
	SignedAt    time.Time `json:"signed_at"`
	// Valid: the waiver still has the content that was signed.
	Valid bool `json:"valid"`
}

// RiskThresholdDefault is the risk acceptance threshold of a project that
// set none: residual levels above it need an accepted sign-off.
const RiskThresholdDefault = "medium"

// Residual sources: where a risk's residual score comes from.
const (
	ResidualInherent = "inherent" // no residual assessment, or its mitigation is not verified yet
	ResidualVerified = "verified" // the residual assessment applies: the mitigation is verified
	ResidualManual   = "manual"   // entered by hand with a justification
)

// SoA implementation states of a control.
var SoAStatuses = []string{"implemented", "partial", "planned", "not_implemented"}

// SoADecision is a project's decision on one control of a reference source.
type SoADecision struct {
	ControlID string `json:"control_id"`
	// Applicable: nil = not decided.
	Applicable    *bool     `json:"applicable"`
	Justification string    `json:"justification"`
	Owner         string    `json:"owner"`
	UpdatedBy     string    `json:"updated_by"`
	UpdatedAt     time.Time `json:"updated_at"`
}
