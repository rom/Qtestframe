package model

import "time"

// ---------------------------------------------------------------- audit chain

// AuditCheckpoint is a signed statement that entry Seq of a tenant's audit
// chain had hash Hash (HMAC-SHA256 with the server key KeyID).
type AuditCheckpoint struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Seq       int64     `json:"seq"`
	Hash      string    `json:"hash"`
	KeyID     string    `json:"key_id"`
	Signature string    `json:"signature"`
	CreatedAt time.Time `json:"created_at"`
}

// Problems a verification of the audit chain reports (AuditBreak.Problem).
const (
	// AuditEntryChanged: the entry's fields no longer give its hash.
	AuditEntryChanged = "entry_changed"
	// AuditLinkBroken: the entry does not point to the hash of the entry
	// before it (an entry was removed, replaced or reordered).
	AuditLinkBroken = "link_broken"
	// AuditSeqGap: sequence numbers are missing (entries were removed).
	AuditSeqGap = "sequence_gap"
	// AuditTruncated: the chain ends before its recorded head or a signed
	// checkpoint (entries at the end were removed).
	AuditTruncated = "truncated"
	// AuditCheckpointMismatch: a signed checkpoint names another hash for
	// its entry (the chain was rewritten).
	AuditCheckpointMismatch = "checkpoint_mismatch"
	// AuditCheckpointForged: a checkpoint's signature is not valid.
	AuditCheckpointForged = "checkpoint_signature"
)

// AuditBreak is the first broken link of an audit chain.
type AuditBreak struct {
	Seq       int64      `json:"seq"`
	EntryID   string     `json:"entry_id"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	Problem   string     `json:"problem"`
	Message   string     `json:"message"`
}

// AuditVerification is the result of verifying a tenant's audit chain.
type AuditVerification struct {
	TenantID string `json:"tenant_id"`
	OK       bool   `json:"ok"`
	// Entries checked, the last sequence number and hash.
	Entries  int64  `json:"entries"`
	LastSeq  int64  `json:"last_seq"`
	LastHash string `json:"last_hash"`
	// Checkpoints checked; LastCheckpoint is the newest (written by this
	// verification).
	Checkpoints    int              `json:"checkpoints"`
	LastCheckpoint *AuditCheckpoint `json:"last_checkpoint,omitempty"`
	// KeyProtected: the checkpoint key is stored encrypted (without a
	// configured key provider it is plaintext and gives less protection).
	KeyProtected bool `json:"key_protected"`
	// Unchained: entries written during the verification, not chained yet.
	Unchained  int         `json:"unchained"`
	Broken     *AuditBreak `json:"broken,omitempty"`
	VerifiedAt time.Time   `json:"verified_at"`
	DurationMS int64       `json:"duration_ms"`
}

// ---------------------------------------------------------------- electronic records

// Actions that the electronic-records settings can make require an
// electronic signature (ERecordsSettings.SignatureActions), enforced by the
// server on every path that performs them.
const (
	// SignRequirementApprove: a requirement's status changes to approved.
	SignRequirementApprove = "requirement.approve"
	// SignTestApprove: a test's status changes to ready (approved for
	// execution).
	SignTestApprove = "test.approve"
	// SignReleaseGo: a release's status changes to released (the go
	// decision).
	SignReleaseGo = "release.go"
	// SignBaselineCreate: a baseline is created.
	SignBaselineCreate = "baseline.create"
)

// ERecordsSignatureActions lists the actions that can require a signature.
var ERecordsSignatureActions = []string{SignRequirementApprove, SignTestApprove, SignReleaseGo, SignBaselineCreate}

// Signatures that are electronic signatures in any case (their own
// dialogs); in electronic-records mode they need the full re-authentication
// and a meaning.
const (
	SignBaselineSignoff     = "baseline.signoff"
	SignWaiverDecision      = "waiver.decision"
	SignRiskAccept          = "risk.accept"
	SignVerificationSignoff = "verification.signoff"
	// SignChangeRequestDecision: a change control board decision.
	SignChangeRequestDecision = "change_request.decision"
)

// ERecordsReasonEdits are the kinds of change that can require a reason
// for change (ERecordsSettings.ReasonEdits).
var ERecordsReasonEdits = []string{
	"requirement.update", "requirement.delete", "test.update", "test.delete",
	"run.update", "run.delete", "execution.update", "release.update", "release.delete",
	"baseline.delete", "risk.update", "risk.delete", "waiver.update", "verification.update", "verification.delete",
}

// DefaultSignatureMeanings are the meanings offered until an organisation
// sets its own list.
var DefaultSignatureMeanings = []string{"authored", "reviewed", "approved", "verified", "released"}

// ERecordsSettings are an organisation's electronic-records settings (21
// CFR Part 11 / EU GMP Annex 11). Off by default.
type ERecordsSettings struct {
	Enabled bool `json:"enabled"`
	// Meanings a signer chooses from (default DefaultSignatureMeanings).
	Meanings []string `json:"meanings"`
	// SignatureActions require an electronic signature (see
	// ERecordsSignatureActions).
	SignatureActions []string `json:"signature_actions"`
	// ReasonEdits require a reason for change (see ERecordsReasonEdits).
	ReasonEdits []string `json:"reason_edits"`
	// IdleTimeoutMinutes ends a browser session after this long without
	// activity (1–480, default 15).
	IdleTimeoutMinutes int `json:"idle_timeout_minutes"`
	// SSOReauthMinutes: accounts that sign in with single sign-on (no
	// password here) sign within this many minutes of signing in (1–60,
	// default 5).
	SSOReauthMinutes int       `json:"sso_reauth_minutes"`
	UpdatedBy        string    `json:"updated_by"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ESignature is an electronic signature on a record: its manifestation
// (printed name, date and time, meaning) and the hash of the record
// version signed.
type ESignature struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	EntityKey  string `json:"entity_key"`
	Action     string `json:"action"`
	Meaning    string `json:"meaning"`
	Comment    string `json:"comment"`
	UserID     string `json:"user_id"`
	UserEmail  string `json:"user_email"`
	UserName   string `json:"user_name"`
	// Method: how the signer was re-authenticated (password, password+mfa,
	// sso, sso+mfa).
	Method string `json:"method"`
	// TimeZone is the signer's IANA time zone (the signing time is UTC).
	TimeZone    string    `json:"time_zone"`
	ContentHash string    `json:"content_hash"`
	SignedAt    time.Time `json:"signed_at"`
	// Valid (derived): the record still has the content that was signed;
	// CurrentHash is its hash now ("" unknown: the record is gone).
	Valid       bool   `json:"valid"`
	CurrentHash string `json:"current_hash,omitempty"`
}
