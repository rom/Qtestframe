package app

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedVerification shows verification correlations in the demo project:
// a product breakdown (system elements) with allocated requirements and
// tests, links mapped to criteria and test steps, verification records
// (inspection, analysis, demonstration) with a sign-off and a document of
// the archive, and a refines hierarchy that rolls up.
func seedVerification(ctx context.Context, st *store.Store, tenantID string, p *model.Project, reqs []*model.Requirement, testIDs []string) error {
	if len(reqs) < 8 || len(testIDs) < 5 {
		return nil
	}
	// system elements
	el := func(name, kind string, parent *string, desc string) (*model.SystemElement, error) {
		e := &model.SystemElement{ProjectID: p.ID, ParentID: parent, Name: name, Kind: kind, Description: desc, CreatedBy: "seed"}
		return e, st.CreateElement(ctx, tenantID, e)
	}
	portal, err := el("Online banking portal", "system", nil, "The web banking portal as delivered to customers.")
	if err != nil {
		return err
	}
	auth, err := el("Authentication service", "subsystem", &portal.ID, "Sign-in, lockout and single sign-on.")
	if err != nil {
		return err
	}
	idp, err := el("Identity provider", "supplier", &auth.ID, "The corporate OpenID Connect / SAML identity provider (external supplier).")
	if err != nil {
		return err
	}
	pay, err := el("Payment engine", "subsystem", &portal.ID, "Transfers, limits and statements.")
	if err != nil {
		return err
	}
	ops, err := el("Operations platform", "subsystem", &portal.ID, "Hosting, logging and monitoring.")
	if err != nil {
		return err
	}
	for _, a := range []struct {
		e     *model.SystemElement
		reqs  []int
		tests []int
	}{{auth, []int{0, 1, 2}, []int{0, 1, 2}}, {idp, []int{2}, nil}, {pay, []int{3, 4, 5}, []int{3, 4}}, {ops, []int{6, 7}, nil}} {
		var rids, tids []string
		for _, i := range a.reqs {
			rids = append(rids, reqs[i].ID)
		}
		for _, i := range a.tests {
			tids = append(tids, testIDs[i])
		}
		if err := st.Allocate(ctx, tenantID, a.e.ID, model.KindRequirement, rids, nil); err != nil {
			return err
		}
		if err := st.Allocate(ctx, tenantID, a.e.ID, model.KindTest, tids, nil); err != nil {
			return err
		}
	}

	// map the links of "Password login" and "Transfer limits" to their criteria
	links, err := st.ListLinks(ctx, tenantID, p.ID, "", "")
	if err != nil {
		return err
	}
	tests, err := st.ListTests(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	stepOf := func(testID string, pos int) string {
		for _, t := range tests {
			if t.ID == testID && pos < len(t.Steps) {
				return t.Steps[pos].ID
			}
		}
		return ""
	}
	mapLink := func(req *model.Requirement, testID string, crit, step int) error {
		for _, l := range links {
			if l.RequirementID == req.ID && l.TestCaseID == testID && crit < len(req.Criteria) {
				lc := model.LinkCriterion{CriterionID: req.Criteria[crit].ID}
				if step >= 0 {
					lc.StepID = stepOf(testID, step)
				}
				_, err := st.SetLinkCriteria(ctx, tenantID, l.ID, []model.LinkCriterion{lc})
				return err
			}
		}
		return nil
	}
	if err := mapLink(reqs[0], testIDs[0], 0, 2); err != nil { // "Press Sign in" shows the dashboard
		return err
	}
	if err := mapLink(reqs[0], testIDs[1], 1, -1); err != nil {
		return err
	}
	if err := mapLink(reqs[3], testIDs[3], 0, 1); err != nil { // "Transfer 1500" is rejected
		return err
	}

	// a document of the archive and verification records
	doc := &model.FileInfo{ProjectID: &p.ID, Kind: model.FileDocument, Name: "Limit configuration review.txt", ContentType: "text/plain",
		Description: "Review of the daily transfer limit configuration", Tags: []string{"verification"}, UploadedBy: "seed"}
	if err := st.CreateFile(ctx, tenantID, doc, []byte("Daily limit configuration reviewed against the product sheet: default 1000, max 50000, change needs four-eyes approval.\n")); err != nil {
		return err
	}
	today := st.Now().UTC()
	date := func(days int) string { return today.AddDate(0, 0, days).Format("2006-01-02") }
	crit := func(r *model.Requirement, i int) *string {
		if i < len(r.Criteria) {
			return &r.Criteria[i].ID
		}
		return nil
	}
	records := []*model.VerificationRecord{
		{RequirementID: reqs[3].ID, CriterionID: crit(reqs[3], 1), Method: "inspection", Result: model.RecordPass, PerformedBy: "Maria Berg",
			PerformedOn: date(-3), Note: "Limit table and approval rule checked against the product sheet.", FileIDs: []string{doc.ID}, UntilChange: true},
		{RequirementID: reqs[4].ID, CriterionID: crit(reqs[4], 0), Method: "analysis", Result: model.RecordConditional, PerformedBy: "Performance team",
			PerformedOn: date(-10), ValidUntil: date(170), Note: "p95 1.4 s in the load model; valid while the hosting stays on the current cluster size."},
		{RequirementID: reqs[2].ID, CriterionID: crit(reqs[2], 0), Method: "demonstration", Result: model.RecordFail, PerformedBy: "Jonas Ek",
			PerformedOn: date(-1), Note: "SAML sign-in works; the OpenID Connect connection to the identity provider is not configured yet."},
		{RequirementID: reqs[6].ID, CriterionID: crit(reqs[6], 0), Method: "demonstration", Result: model.RecordPass, PerformedBy: "Operations",
			PerformedOn: date(-5), Note: "Audit events shown on the central syslog server during the operations review."},
	}
	for _, r := range records {
		r.CreatedBy = "seed"
		if err := st.CreateRecord(ctx, tenantID, r); err != nil {
			return err
		}
	}
	if _, err := st.SignRecord(ctx, tenantID, records[0].ID, "lead@example.com", "Test lead", "Checked the review document."); err != nil {
		return err
	}

	// hierarchy: single sign-on refines password login (a decomposition)
	if err := st.CreateRelation(ctx, tenantID, &model.Relation{SourceID: reqs[2].ID, TargetID: reqs[0].ID, Type: "refines", CreatedBy: "seed"}); err != nil {
		return err
	}
	_, err = st.SyncEvidenceStatus(ctx, tenantID, p.ID, nil, "seed")
	return err
}
