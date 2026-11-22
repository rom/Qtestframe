package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// The "Part 11 / Annex 11 controls" report lists, per requirement of 21
// CFR Part 11 and EU GMP Annex 11, what the tool provides, how it is set up
// in the organisation (evidence) and what the customer's own procedures
// must cover. It claims nothing the tool does not do: validation, training,
// written policies and the identity checks before accounts are created are
// always the customer's.

// Who covers a control (ComplianceControl.Provider).
const (
	ctlTool     = "tool"     // the tool provides it
	ctlShared   = "shared"   // the tool provides means, procedures must use them
	ctlCustomer = "customer" // the customer's procedures only
)

// Status of a control in the organisation (ComplianceControl.Status).
const (
	ctlMet          = "met"            // provided and in effect
	ctlNotSet       = "not_configured" // the tool can, the settings do not
	ctlProcedural   = "procedural"     // for the customer's procedures
	ctlNeedsAttn    = "attention"      // in effect but with a finding
	
)

// ComplianceControl is one row of the report.
type ComplianceControl struct {
	ID         string   `json:"id"`
	References []string `json:"references"`
	Title      string   `json:"title"`
	Provider   string   `json:"provider"`
	Status     string   `json:"status"`
	Tool       string   `json:"tool"`
	Customer   string   `json:"customer"`
	Evidence   []string `json:"evidence"`
}

// ComplianceReport is the "Part 11 / Annex 11 controls" report.
type ComplianceReport struct {
	Tenant      string                   `json:"tenant"`
	GeneratedAt time.Time                `json:"generated_at"`
	GeneratedBy string                   `json:"generated_by"`
	Settings    *model.ERecordsSettings  `json:"settings"`
	Audit       *model.AuditVerification `json:"audit"`
	Signatures  map[string]int           `json:"signatures"`
	Controls    []ComplianceControl      `json:"controls"`
	Disclaimer  string                   `json:"disclaimer"`
}

const complianceDisclaimer = "This report lists technical controls of Qtestframe and how they are set up in this organisation. " +
	"Using the tool does not by itself make a system compliant with 21 CFR Part 11 or EU GMP Annex 11: the regulated company must validate it for its intended use, " +
	"and its procedures must cover the rows marked customer (and the customer part of shared rows)."

func (s *Server) complianceReport(r *http.Request, p *Principal) (*ComplianceReport, error) {
	ctx := r.Context()
	tid := p.TenantID()
	st, err := s.Store.ERecordsSettings(ctx, tid)
	if err != nil {
		return nil, err
	}
	v, err := s.Store.VerifyAudit(ctx, tid)
	if err != nil {
		return nil, err
	}
	sigs, err := s.Store.CountESignatures(ctx, tid)
	if err != nil {
		return nil, err
	}
	members, err := s.Store.ListTenantMembers(ctx, tid)
	if err != nil {
		return nil, err
	}
	mfaUsers := 0
	for _, m := range members {
		if mm, err := s.userMFA(ctx, m.UserID); err == nil && mm != nil {
			mfaUsers++
		}
	}
	mode := "unknown"
	if pol, err := s.Store.MFAPolicy(ctx, tid); err == nil && pol != nil {
		mode = pol.Mode
		if len(pol.Roles) > 0 {
			mode += " (" + strings.Join(pol.Roles, ", ") + ")"
		}
	}
	on := st.Enabled
	ifOn := func(s string) string {
		if on {
			return ctlMet
		}
		return s
	}
	list := func(xs []string) string {
		if len(xs) == 0 {
			return "none"
		}
		return strings.Join(xs, ", ")
	}
	total := 0
	for _, n := range sigs {
		total += n
	}
	auditStatus, auditEvidence := ctlMet, fmt.Sprintf("Audit chain verified now: %d entries, %d signed checkpoints, intact", v.Entries, v.Checkpoints)
	if !v.OK {
		auditStatus, auditEvidence = ctlNeedsAttn, fmt.Sprintf("Audit chain verified now: BROKEN at entry %d (%s): %s", v.Broken.Seq, v.Broken.Problem, v.Broken.Message)
	}
	keyEvidence := "Checkpoint key stored encrypted with the server's key provider"
	if !v.KeyProtected {
		keyEvidence = "Checkpoint key stored WITHOUT encryption (no key provider configured): checkpoints protect less"
		if auditStatus == ctlMet {
			auditStatus = ctlNeedsAttn
		}
	}
	reasonStatus := ifOn(ctlNotSet)
	if on && len(st.ReasonEdits) == 0 {
		reasonStatus = ctlNotSet
	}
	sigActStatus := ifOn(ctlNotSet)
	if on && len(st.SignatureActions) == 0 {
		sigActStatus = ctlNotSet
	}
	rep := &ComplianceReport{Tenant: p.Tenant.Name, GeneratedAt: s.Store.Now(), GeneratedBy: p.User.Email, Settings: st, Audit: v, Signatures: sigs, Disclaimer: complianceDisclaimer}
	modeEvidence := "Electronic-records mode: off"
	if on {
		modeEvidence = "Electronic-records mode: on"
	}
	rep.Controls = []ComplianceControl{
		{ID: "validation", References: []string{"Part 11 §11.10(a)", "Annex 11 §4"}, Title: "Validation of the system", Provider: ctlCustomer, Status: ctlProcedural,
			Tool: "Automated test suites, release notes and this documentation support the customer's validation.",
			Customer: "Validate the installed version for the intended use (requirements, risk assessment, IQ/OQ/PQ) and re-validate after upgrades."},
		{ID: "copies", References: []string{"Part 11 §11.10(b)", "Annex 11 §8"}, Title: "Accurate and complete copies of records", Provider: ctlTool, Status: ctlMet,
			Tool:     "Exports of items, reports and generated documents (PDF, Word, CSV, ReqIF); the audit trail exports with sequence numbers and hashes (JSON, CSV).",
			Customer: "Define which copies are provided for inspection and how.", Evidence: []string{"GET /api/v1/audit/archive"}},
		{ID: "retention", References: []string{"Part 11 §11.10(c)", "Annex 11 §7, §17"}, Title: "Protection and retention of records", Provider: ctlShared, Status: ctlProcedural,
			Tool:     "Logical backups (database independent) that keep the audit chain valid; deleted items go to the trash; signatures are kept when a project is deleted.",
			Customer: "Back up regularly, test restores, keep backups and audit-trail exports for the retention period, archive before deleting a project."},
		{ID: "access", References: []string{"Part 11 §11.10(d), (g)", "Annex 11 §12"}, Title: "Access limited to authorised people; authority checks", Provider: ctlShared, Status: ctlMet,
			Tool:     "Role-based permissions per organisation and project, enforced by the server for every request; API tokens never exceed the user's roles.",
			Customer: "Grant and review access (periodic access reviews), remove leavers.", Evidence: []string{fmt.Sprintf("%d members", len(members))}},
		{ID: "audit_trail", References: []string{"Part 11 §11.10(e)", "Annex 11 §9"}, Title: "Secure, computer-generated, time-stamped audit trail", Provider: ctlTool, Status: auditStatus,
			Tool:     "Every change is recorded with user, UTC time, action and details in a hash chain per organisation (SHA-256, signed checkpoints); entries cannot be changed through the application; verification shows the first broken link.",
			Customer: "Review the audit trail as part of record review; verify the chain periodically and archive exports.",
			Evidence: []string{auditEvidence, keyEvidence}},
		{ID: "reason", References: []string{"Annex 11 §9"}, Title: "Reason for change", Provider: ctlTool, Status: reasonStatus,
			Tool:     "In electronic-records mode the chosen kinds of change need a reason, recorded in the audit trail.",
			Customer: "Decide which changes need a reason.", Evidence: []string{modeEvidence, "Edits requiring a reason: " + list(st.ReasonEdits)}},
		{ID: "sequencing", References: []string{"Part 11 §11.10(f)"}, Title: "Operational checks (sequence of steps)", Provider: ctlShared, Status: sigActStatus,
			Tool:     "Workflow states, readiness checks and, in electronic-records mode, signatures required by the server for the chosen actions.",
			Customer: "Define the workflow and which steps need a signature.", Evidence: []string{"Actions requiring a signature: " + list(st.SignatureActions)}},
		{ID: "devices", References: []string{"Part 11 §11.10(h)"}, Title: "Device checks", Provider: ctlCustomer, Status: ctlProcedural,
			Tool: "Not applicable to a web application beyond the recorded client address and browser of sign-ins.", Customer: "Define whether terminals must be restricted."},
		{ID: "training", References: []string{"Part 11 §11.10(i)", "Annex 11 §2"}, Title: "Qualified, trained people", Provider: ctlCustomer, Status: ctlProcedural,
			Customer: "Train users, administrators and developers and keep training records."},
		{ID: "policies", References: []string{"Part 11 §11.10(j), §11.100(c)"}, Title: "Accountability for signatures; certification to the agency", Provider: ctlCustomer, Status: ctlProcedural,
			Customer: "Written policy that electronic signatures are the legally binding equivalent of handwritten ones; certify this to the FDA before use."},
		{ID: "documentation", References: []string{"Part 11 §11.10(k)", "Annex 11 §10"}, Title: "Control of system documentation; change management", Provider: ctlShared, Status: ctlProcedural,
			Tool: "Versioned user and administrator guides delivered with each version.", Customer: "Control the documentation and changes to the installation (upgrades, configuration)."},
		{ID: "manifestation", References: []string{"Part 11 §11.50", "Annex 11 §14"}, Title: "Signature manifestation", Provider: ctlTool, Status: ifOn(ctlNotSet),
			Tool:     "Signed records show the printed name, date and time (UTC and the signer's time zone) and the meaning of the signature, also in exports.",
			Customer: "Define the meanings and when each applies.", Evidence: []string{"Meanings: " + list(st.Meanings), fmt.Sprintf("%d signatures recorded", total)}},
		{ID: "linking", References: []string{"Part 11 §11.70", "Annex 11 §14"}, Title: "Signature and record linking", Provider: ctlTool, Status: ctlMet,
			Tool:     "Each signature stores the hash of the record version signed; a later change shows the signature as no longer matching. Signatures cannot be copied or moved through the application.",
			Customer: "Re-sign records changed after signing when the procedure requires it."},
		{ID: "uniqueness", References: []string{"Part 11 §11.100(a), (b)"}, Title: "Unique signatures; identity verified", Provider: ctlShared, Status: ctlProcedural,
			Tool:     "One account per e-mail address; accounts are disabled, not reused.",
			Customer: "Verify a person's identity before giving an account; never reassign accounts."},
		{ID: "components", References: []string{"Part 11 §11.200(a)"}, Title: "Signing with two components, re-authentication at signing", Provider: ctlTool, Status: ifOn(ctlNotSet),
			Tool: "In electronic-records mode every signature needs the account's password (and a second-factor code when enrolled), at the time of signing; API tokens cannot sign. Single sign-on accounts sign within " +
				fmt.Sprint(st.SSOReauthMinutes) + " minutes of signing in at the identity provider.",
			Customer: "Decide whether single sign-on accounts may sign; require multi-factor authentication where the risk calls for it.", Evidence: []string{modeEvidence}},
		{ID: "passwords", References: []string{"Part 11 §11.300", "Annex 11 §12"}, Title: "Controls for identification codes and passwords", Provider: ctlShared, Status: ctlMet,
			Tool: fmt.Sprintf("Password rules, rate-limited sign-in, multi-factor authentication with policies, session expiry and, in electronic-records mode, an inactivity timeout of %d minutes.", st.IdleTimeoutMinutes),
			Customer: "Password ageing and loss management (or the identity provider's), periodic checks of accounts, reporting of misuse.",
			Evidence: []string{"Multi-factor policy: " + mode, fmt.Sprintf("%d of %d members use a second factor", mfaUsers, len(members))}},
		{ID: "continuity", References: []string{"Annex 11 §16"}, Title: "Business continuity", Provider: ctlCustomer, Status: ctlProcedural,
			Tool: "Backups and restores, health checks.", Customer: "Plan for outages and test the plan."},
		{ID: "periodic", References: []string{"Annex 11 §11"}, Title: "Periodic evaluation", Provider: ctlCustomer, Status: ctlProcedural,
			Tool: "This report and the audit verification as evidence.", Customer: "Evaluate the system periodically."},
	}
	return rep, nil
}

// handleERecordsReport: GET /erecords/report[?format=md] — the "Part 11 /
// Annex 11 controls" report (it verifies the audit chain to report on it).
func (s *Server) handleERecordsReport(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	rep, err := s.complianceReport(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "erecords.report", "tenant", p.TenantID(), "")
	if r.URL.Query().Get("format") != "md" {
		writeJSON(w, http.StatusOK, rep)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Part 11 / Annex 11 controls — %s\n\n", rep.Tenant)
	fmt.Fprintf(&b, "Generated %s UTC by %s.\n\n> %s\n\n", rep.GeneratedAt.UTC().Format("2006-01-02 15:04:05"), rep.GeneratedBy, rep.Disclaimer)
	fmt.Fprintf(&b, "| Control | References | Covered by | Status | Tool | Customer | Evidence |\n|---|---|---|---|---|---|---|\n")
	cell := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }
	for _, c := range rep.Controls {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", cell(c.Title), cell(strings.Join(c.References, "; ")), c.Provider, c.Status,
			cell(c.Tool), cell(c.Customer), cell(strings.Join(c.Evidence, "; ")))
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="part11-annex11-controls-`+p.Tenant.Slug+`.md"`)
	_, _ = w.Write([]byte(b.String()))
}
