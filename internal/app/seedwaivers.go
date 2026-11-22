package app

import (
	"context"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Demo data of waivers, residual risk and the Statement of Applicability:
// risks with a residual assessment (one verified, one entered by hand),
// an approved waiver for release 1.4, a deviation draft, and SoA
// decisions for ISO/IEC 27001:2022 Annex A.

// seedSources are the ISO/IEC 27001 controls the demo requirements cite
// (by requirement title), so the Statement of Applicability has data.
var seedSources = map[string]string{
	"Password login":  "Customer RFP v1.2; ISO/IEC 27001:2022 A.8.5",
	"Account lockout": "Customer RFP v1.2; ISO/IEC 27001:2022 A.8.5",
	"Single sign-on":  "Customer RFP v1.2; ISO/IEC 27001:2022 A.5.17",
	"Central logging": "Customer RFP v1.2; ISO/IEC 27001:2022 A.8.15",
	"Transfer limits": "Customer RFP v1.2; ISO/IEC 27001:2022 A.8.3",
}

// seedSource is the source field of a demo requirement.
func seedSource(title string) string {
	if s, ok := seedSources[title]; ok {
		return s
	}
	return "Customer RFP v1.2"
}

func seedWaivers(ctx context.Context, st *store.Store, tenantID string, p *model.Project, reqs []*model.Requirement) error {
	if len(reqs) < 7 {
		return nil
	}
	rels, err := st.ListReleases(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	var rel *string
	for i := range rels {
		if rels[i].Name == "1.4" {
			rel = &rels[i].ID
		}
	}
	// risks before and after mitigation
	risks := []*model.Risk{
		{ProjectID: p.ID, Title: "Account takeover by password guessing", Category: "Security", Likelihood: 4, Impact: 5, Status: "mitigating", Owner: "Security team",
			Mitigation: "Lock accounts after repeated failures; offer single sign-on.", RequirementIDs: []string{reqs[0].ID, reqs[1].ID},
			ResidualLikelihood: 2, ResidualImpact: 3, ReviewDate: time.Now().UTC().AddDate(0, 1, 0).Format(time.DateOnly), ReviewOwner: "CISO", CreatedBy: "seed"},
		{ProjectID: p.ID, Title: "Audit trail lost on log server outage", Category: "Operations", Likelihood: 3, Impact: 4, Status: "mitigating", Owner: "Operations",
			Mitigation: "Buffer audit events locally and forward them when the log server is back.", RequirementIDs: []string{reqs[6].ID},
			ResidualLikelihood: 2, ResidualImpact: 4, ResidualManual: true, ResidualJustification: "The local buffer holds 7 days of events; outages have lasted hours.", CreatedBy: "seed"},
		{ProjectID: p.ID, Title: "Transfers above the limit slip through", Category: "Fraud", Likelihood: 2, Impact: 5, Status: "identified", Owner: "Payments",
			Mitigation: "Server-side limit check.", RequirementIDs: []string{reqs[3].ID}, ResidualLikelihood: 1, ResidualImpact: 5, CreatedBy: "seed"},
	}
	for _, k := range risks {
		if err := st.CreateRisk(ctx, tenantID, k); err != nil {
			return err
		}
	}
	// an approved waiver for the release: single sign-on is not ready
	w := &model.Waiver{ProjectID: p.ID, Type: "waiver", Title: "Release 1.4 without single sign-on", ReleaseID: rel,
		RequirementIDs: []string{reqs[2].ID}, Justification: "The corporate identity provider is not available to customers before the autumn.",
		Compensating: "Password login with account lockout stays the only sign-in method.", Conditions: "Only for release 1.4.",
		RiskID: &risks[0].ID, RequestedBy: "seed@example.com", Approvers: []string{"ciso@example.com"}, Status: "requested",
		ExpiresOn: time.Now().UTC().AddDate(0, 3, 0).Format(time.DateOnly), CreatedBy: "seed"}
	if err := st.CreateWaiver(ctx, tenantID, w); err != nil {
		return err
	}
	if err := st.SignWaiver(ctx, tenantID, &model.WaiverApproval{WaiverID: w.ID, UserID: "seed", UserEmail: "ciso@example.com", UserName: "CISO (demo)", Decision: "approve", Comment: "Accepted for 1.4."}); err != nil {
		return err
	}
	if err := st.SetWaiverStatus(ctx, tenantID, w.ID, []string{"requested"}, "approved", "ciso@example.com", "Accepted for 1.4."); err != nil {
		return err
	}
	// a deviation still being written
	d := &model.Waiver{ProjectID: p.ID, Type: "deviation", Title: "Statements as HTML instead of PDF", RequirementIDs: []string{reqs[5].ID},
		Justification: "Customers asked for statements they can read on phones.", Compensating: "Responsive HTML statements that can be printed to PDF.",
		RequestedBy: "seed@example.com", CreatedBy: "seed"}
	if err := st.CreateWaiver(ctx, tenantID, d); err != nil {
		return err
	}
	// Statement of Applicability decisions
	yes, no := true, false
	_, err = st.SetSoADecisions(ctx, tenantID, p.ID, "iso-27001", "seed", []model.SoADecision{
		{ControlID: "A.5.15", Applicable: &yes, Justification: "Customers and staff need role-based access.", Owner: "Security team"},
		{ControlID: "A.5.17", Applicable: &yes, Justification: "Customer passwords and single sign-on.", Owner: "Security team"},
		{ControlID: "A.8.3", Applicable: &yes, Justification: "Account data is restricted to its owner.", Owner: "Payments"},
		{ControlID: "A.8.5", Applicable: &yes, Justification: "Online banking requires secure sign-in.", Owner: "Security team"},
		{ControlID: "A.8.15", Applicable: &yes, Justification: "Audit logging is a regulatory duty.", Owner: "Operations"},
		{ControlID: "A.7.1", Applicable: &no, Justification: "The portal runs in a certified cloud data centre; there are no own premises.", Owner: "Operations"},
		{ControlID: "A.7.4", Applicable: &no, Justification: "No own premises (see A.7.1).", Owner: "Operations"},
	})
	return err
}
