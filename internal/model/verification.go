package model

import "time"

// Verification correlations: criterion-level verification, verification
// records (evidence other than test results), the roll-up of verification
// along the requirement hierarchy (refines), and allocation of
// requirements and tests to system elements. See internal/verification.

// Verification states of a criterion, of a requirement's own evidence and
// of a requirement rolled up with its children.
const (
	// VerVerified: every criterion (and every piece of evidence that
	// counts) has passed; for a parent also every child is verified.
	VerVerified = "verified"
	// VerPartial: some, but not all, of it is verified.
	VerPartial = "partial"
	// VerFailed: some evidence failed (a test failed or was blocked, a
	// record's result is fail).
	VerFailed = "failed"
	// VerPending: there is evidence, but none of it has passed yet (not
	// run, a suspect link, a step not executed).
	VerPending = "pending"
	// VerNone: no evidence at all.
	VerNone = "none"
)

// VerificationStates lists the states (query language, analytics).
var VerificationStates = []string{VerVerified, VerPartial, VerFailed, VerPending, VerNone}

// Results of a verification record.
const (
	RecordPass        = "pass"
	RecordFail        = "fail"
	RecordConditional = "conditional"
)

// RecordResults are the valid verification record results.
var RecordResults = []string{RecordPass, RecordFail, RecordConditional}

// Evidence states (Evidence.State).
const (
	EvidencePassed  = "passed"
	EvidenceFailed  = "failed"
	EvidencePending = "pending"
	// EvidenceVoid: the evidence does not count (an expired or invalidated
	// record, a record superseded by a newer one for the same criterion).
	EvidenceVoid = "void"
)

// VerificationRecord is evidence other than a test result: an inspection,
// analysis, demonstration or review (or a test done outside the tool) of
// a criterion of a requirement.
type VerificationRecord struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	RequirementID string `json:"requirement_id"`
	// CriterionID: the criterion verified; nil for the requirement as a
	// whole (counts like a plain test link).
	CriterionID *string `json:"criterion_id"`
	// Method is an EvaluationMethods value.
	Method string `json:"method"`
	// Result is a RecordResults value.
	Result string `json:"result"`
	// PerformedBy names who did it (free text, default the author);
	// PerformedOn is the date (YYYY-MM-DD).
	PerformedBy string `json:"performed_by"`
	PerformedOn string `json:"performed_on"`
	Note        string `json:"note"`
	// FileIDs are documents of the project's archive attached as evidence.
	FileIDs []string  `json:"file_ids"`
	Files   []FileRef `json:"files"`
	// ValidUntil (YYYY-MM-DD, "" = no end date): the record stops counting
	// after this day. UntilChange: it stops counting when the requirement's
	// content changes (InvalidatedAt / InvalidatedReason are then set).
	ValidUntil        string     `json:"valid_until"`
	UntilChange       bool       `json:"until_change"`
	InvalidatedAt     *time.Time `json:"invalidated_at"`
	InvalidatedReason string     `json:"invalidated_reason"`
	// Signoff by a second person (not the author nor the performer).
	Signoff   *RecordSignoff `json:"signoff"`
	CreatedBy string         `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	// Derived.
	RequirementKey string `json:"requirement_key,omitempty"`
	// CriterionText: the description of the criterion.
	CriterionText string `json:"criterion_text,omitempty"`
	// Validity: valid | expired | invalidated (at the time of reading).
	Validity string `json:"validity"`
}

// RecordSignoff is the second person's confirmation of a record.
type RecordSignoff struct {
	By      string    `json:"by"` // e-mail
	Name    string    `json:"name"`
	Comment string    `json:"comment"`
	At      time.Time `json:"at"`
}

// FileRef is a short reference to a stored file.
type FileRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// LinkCriterion maps a trace link to a criterion it verifies, optionally
// through one step of the test ("" = the whole test).
type LinkCriterion struct {
	CriterionID string `json:"criterion_id"`
	StepID      string `json:"step_id,omitempty"`
}

// VerificationSettings are a project's verification settings.
type VerificationSettings struct {
	// StrictCriteria: every criterion needs its own evidence (a test mapped
	// to it, or a verification record). Off (default): a plain test link
	// without criterion mapping counts for every criterion that has no
	// evidence of its own, as before criterion-level verification.
	StrictCriteria bool       `json:"strict_criteria"`
	UpdatedBy      string     `json:"updated_by,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

// Evidence is one piece of evidence about a requirement or criterion.
type Evidence struct {
	// Kind: test | record.
	Kind string `json:"kind"`
	// ID of the test or the record; LinkID of the trace link (tests).
	ID     string `json:"id"`
	LinkID string `json:"link_id,omitempty"`
	// Key and Title of the test; for a record Key is its method and
	// Title the performer.
	Key   string `json:"key"`
	Title string `json:"title"`
	// StepID / StepPosition (1-based): the test step it is mapped to.
	StepID       string `json:"step_id,omitempty"`
	StepPosition int    `json:"step_position,omitempty"`
	// Result: the test's last result (step result with a step) or the
	// record's result.
	Result string `json:"result"`
	// State: passed | failed | pending | void.
	State string `json:"state"`
	// Plain: a test link without criterion mapping (or a record without
	// criterion) counted for this criterion (non-strict projects).
	Plain bool `json:"plain,omitempty"`
	// Note says why the evidence is pending or void: suspect, step_not_run,
	// not_run, expired, invalidated, superseded, conditional.
	Note string `json:"note,omitempty"`
	// Date of the record, or when the test last ran.
	Date string `json:"date,omitempty"`
}

// CriterionVerification is the verification of one criterion.
type CriterionVerification struct {
	CriterionID string     `json:"criterion_id"`
	Position    int        `json:"position"`
	Description string     `json:"description"`
	Method      string     `json:"method"`
	State       string     `json:"state"`
	Evidence    []Evidence `json:"evidence"`
}

// VerificationReason explains why a requirement is not verified: a code
// (no_evidence, criterion_missing, criterion_failed, criterion_pending,
// evidence_failed, evidence_pending, child_not_verified,
// status_inconsistent, child_status_unverified) with parameters, and the
// English text.
type VerificationReason struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
	Text   string            `json:"text"`
}

// ChildVerification is a child (a requirement that refines it) in a
// requirement's roll-up.
type ChildVerification struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"`
	State  string `json:"state"`
}

// RequirementVerification answers "is the requirement verified, and why".
type RequirementVerification struct {
	RequirementID string `json:"requirement_id"`
	Key           string `json:"key"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	// Strict: the project's strict criterion coverage setting.
	Strict bool `json:"strict"`
	// Own: the state from its own criteria and evidence; State: rolled up
	// with its children (refines). Verified is State == verified.
	Own      string `json:"own"`
	State    string `json:"state"`
	Verified bool   `json:"verified"`
	// Criteria with their evidence; Evidence: the evidence about the
	// requirement as a whole (plain test links, records without criterion).
	Criteria []CriterionVerification `json:"criteria"`
	Evidence []Evidence              `json:"evidence"`
	// CriteriaVerified of CriteriaTotal.
	CriteriaVerified int `json:"criteria_verified"`
	CriteriaTotal    int `json:"criteria_total"`
	// Children (direct) and their rolled-up states.
	Children         []ChildVerification `json:"children"`
	ChildrenVerified int                 `json:"children_verified"`
	// ParentIDs: the requirements it refines.
	ParentIDs []string `json:"parent_ids"`
	// Reasons it is not verified (empty when verified), and
	// inconsistencies of the status with the evidence or the hierarchy.
	Reasons         []VerificationReason `json:"reasons"`
	Inconsistencies []VerificationReason `json:"inconsistencies"`
}

// System element kinds.
var ElementKinds = []string{"system", "subsystem", "component", "product", "supplier", "other"}

// SystemElement is a node of a project's product breakdown: a system,
// subsystem, component, product or supplier.
type SystemElement struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	ParentID    *string   `json:"parent_id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Description string    `json:"description"`
	Position    int       `json:"position"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Derived: path from the root ("Vehicle / Power unit").
	Path string `json:"path,omitempty"`
	// Directly allocated requirements and tests.
	RequirementIDs []string `json:"requirement_ids"`
	TestIDs        []string `json:"test_ids"`
}

// ElementCoverage is the coverage and verification of the requirements
// allocated to an element or to any element below it.
type ElementCoverage struct {
	ElementID string  `json:"element_id"`
	ParentID  *string `json:"parent_id"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Path      string  `json:"path"`
	Depth     int     `json:"depth"`
	// Requirements (active: not rejected or obsolete) allocated to it or
	// below; Covered: with any evidence; Verified / Failed: rolled-up state.
	Requirements int `json:"requirements"`
	Covered      int `json:"covered"`
	Verified     int `json:"verified"`
	Failed       int `json:"failed"`
	// Tests allocated to it or below, and of those passed (last result).
	Tests       int `json:"tests"`
	TestsPassed int `json:"tests_passed"`
	// Percent verified of Requirements (0 when none).
	Percent float64 `json:"percent"`
}
