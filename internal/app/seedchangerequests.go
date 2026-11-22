package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// SeedDemoChangeRequests turns on change control (warn) in the demo
// project with the administrator and the managers as the change control
// board, and adds change requests in several states: one by Sara (the
// SME) waiting for the administrator's signature, a closed one whose
// changes were applied, a deferred one and a draft. It runs after the
// users exist (without an administrator account the board member is
// only an address).
func SeedDemoChangeRequests(ctx context.Context, st *store.Store, tenantID string, p *model.Project, adminEmail string) error {
	admin, err := st.GetUserByEmail(ctx, adminEmail)
	if errors.Is(err, store.ErrNotFound) {
		admin, err = &model.User{ID: "seed", Email: adminEmail, DisplayName: "Change board (demo)"}, nil
		if admin.Email == "" {
			admin.Email = "admin@example.com"
		}
	}
	if err != nil {
		return err
	}
	sara, err := demoUser(ctx, st, tenantID, DemoSMEEmail, "Sara Ek", rbac.RoleSME, rbac.RoleTester)
	if err != nil {
		return err
	}
	reqs, err := st.ListRequirements(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	tests, err := st.ListTests(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	byTitle := map[string]*model.Requirement{}
	for i := range reqs {
		byTitle[reqs[i].Title] = &reqs[i]
	}
	lockout, login, limits := byTitle["Account lockout"], byTitle["Password login"], byTitle["Transfer limits"]
	if lockout == nil || login == nil || limits == nil || len(tests) == 0 {
		return nil
	}
	cc := &model.ChangeControlSettings{Mode: "warn", LockSignedBaselines: true, UpdatedBy: admin.Email,
		Board:            model.ChangeBoard{Users: []string{admin.Email}, Roles: []string{rbac.RoleManager}, Quorum: "any"},
		SignatureMeaning: "I approve this change and its impact on behalf of the change control board."}
	if err := st.SetChangeControl(ctx, tenantID, p.ID, cc); err != nil {
		return err
	}
	j := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	impactOf := func(cr *model.ChangeRequest) (*model.ChangeImpact, error) {
		x, err := st.LoadImpactIndex(ctx, tenantID, p.ID)
		if err != nil {
			return nil, err
		}
		var roots []store.ImpactRoot
		for _, it := range cr.Items {
			fields := make([]string, 0, len(it.Proposed))
			for k := range it.Proposed {
				fields = append(fields, k)
			}
			sort.Strings(fields)
			content := store.ContentChanged(it.EntityType, fields)
			roots = append(roots, store.ImpactRoot{Kind: it.EntityType, ID: it.EntityID, ReviewOnly: len(content) == 0, Fields: fields})
		}
		imp, _ := x.Analyze(roots, 0)
		return imp, nil
	}
	step := func(cr *model.ChangeRequest, from, to, by, comment string, imp *model.ChangeImpact) error {
		t := store.CRTransition{From: []string{from}, To: to, By: by, Comment: comment, Impact: imp}
		if to == "assessed" {
			notes, effort := "The linked tests and the security guideline document need an update.", "2 days"
			t.ImpactNotes, t.Effort = &notes, &effort
		}
		return st.SetChangeRequestStatus(ctx, tenantID, cr.ID, t)
	}
	submitAssess := func(cr *model.ChangeRequest, assessor string) error {
		imp, err := impactOf(cr)
		if err != nil {
			return err
		}
		if err := step(cr, "draft", "submitted", cr.CreatedBy, "", imp); err != nil {
			return err
		}
		if err := step(cr, "submitted", "assessed", assessor, "effort 2 days", imp); err != nil {
			return err
		}
		return step(cr, "assessed", "in_review", assessor, "", nil)
	}
	sign := func(cr *model.ChangeRequest, decision, comment string) error {
		got, err := st.GetChangeRequest(ctx, tenantID, cr.ID)
		if err != nil {
			return err
		}
		return st.SignChangeRequest(ctx, tenantID, got, &model.ChangeRequestSignoff{UserID: admin.ID, UserEmail: admin.Email, UserName: admin.DisplayName,
			Decision: decision, Meaning: cc.SignatureMeaning, Comment: comment})
	}

	// 1. waiting for the board (the administrator) in My work
	cr1 := &model.ChangeRequest{ProjectID: p.ID, Title: "Lock accounts for 30 minutes instead of 15", Category: "regulatory", Priority: "high", CreatedBy: sara.Email,
		Reason:       "The updated security policy (SEC-POL 4.2) requires a lockout of at least 30 minutes after five failed attempts.",
		ImpactAction: model.ImpactApplySuspect, BaselineOnClose: true, Links: model.ChangeRequestLinks{Issues: []string{"SEC-412"}},
		Items: []model.ChangeRequestItem{{EntityType: model.KindRequirement, EntityID: lockout.ID,
			Proposed: map[string]json.RawMessage{"description": j("After five failed login attempts the account MUST be locked for 30 minutes."), "tags": j(append(append([]string{}, lockout.Tags...), "policy-4.2"))}}}}
	if err := st.CreateChangeRequest(ctx, tenantID, cr1); err != nil {
		return err
	}
	if err := submitAssess(cr1, sara.Email); err != nil {
		return err
	}

	// 2. applied, verified and closed
	cr2 := &model.ChangeRequest{ProjectID: p.ID, Title: "Correct the password rule wording", Category: "correction", Priority: "medium", CreatedBy: admin.Email,
		Reason:       "Review finding: the requirement mixed SHOULD and MUST for the same rule.",
		ImpactAction: model.ImpactApplyNotify,
		Items:        []model.ChangeRequestItem{{EntityType: model.KindRequirement, EntityID: login.ID, Proposed: map[string]json.RawMessage{"rationale": j("Clarified in the March review: the rule is mandatory.")}}}}
	if err := st.CreateChangeRequest(ctx, tenantID, cr2); err != nil {
		return err
	}
	if err := submitAssess(cr2, admin.Email); err != nil {
		return err
	}
	// a manager's approval: the board member is not the requester
	got, err := st.GetChangeRequest(ctx, tenantID, cr2.ID)
	if err != nil {
		return err
	}
	if err := st.SignChangeRequest(ctx, tenantID, got, &model.ChangeRequestSignoff{UserID: sara.ID, UserEmail: "cab@example.com", UserName: "Change board (demo)",
		Decision: "approve", Meaning: cc.SignatureMeaning, Comment: "Wording only; no test impact."}); err != nil {
		return err
	}
	if err := step(cr2, "in_review", "approved", "cab@example.com", "Wording only; no test impact.", nil); err != nil {
		return err
	}
	rq, err := st.GetRequirement(ctx, tenantID, login.ID)
	if err != nil {
		return err
	}
	rq.Rationale = "Clarified in the March review: the rule is mandatory."
	if err := st.UpdateRequirement(ctx, tenantID, rq, false); err != nil {
		return err
	}
	if got, err = st.GetChangeRequest(ctx, tenantID, cr2.ID); err != nil {
		return err
	}
	if err := st.SetChangeRequestItemApplied(ctx, got.Items[0].ID, rq.Version); err != nil {
		return err
	}
	for _, s := range [][3]string{{"approved", "implemented", "impact notify"}, {"implemented", "verified", "Checked in the requirement view."}, {"verified", "closed", ""}} {
		if err := step(cr2, s[0], s[1], admin.Email, s[2], nil); err != nil {
			return err
		}
	}

	// 3. deferred by the board
	cr3 := &model.ChangeRequest{ProjectID: p.ID, Title: "Raise the daily transfer limit", Category: "enhancement", Priority: "low", CreatedBy: sara.Email,
		Reason:       "Business customers asked for a higher limit.",
		ImpactAction: model.ImpactApplySuspect,
		Items:        []model.ChangeRequestItem{{EntityType: model.KindRequirement, EntityID: limits.ID, Proposed: map[string]json.RawMessage{"priority": j("high"), "status": j("review")}}}}
	if err := st.CreateChangeRequest(ctx, tenantID, cr3); err != nil {
		return err
	}
	if err := submitAssess(cr3, sara.Email); err != nil {
		return err
	}
	if err := sign(cr3, "defer", "Decide together with the fraud rules in release 1.6."); err != nil {
		return err
	}
	if err := step(cr3, "in_review", "deferred", admin.Email, "Decide together with the fraud rules in release 1.6.", nil); err != nil {
		return err
	}

	// 4. a draft that also changes a test
	t := tests[0]
	cr4 := &model.ChangeRequest{ProjectID: p.ID, Title: "Test the new lockout duration", Category: "defect_driven", Priority: "medium", CreatedBy: sara.Email,
		Reason: "Follows the policy change of the lockout.", ImpactAction: model.ImpactApplySuspect,
		Items: []model.ChangeRequestItem{{EntityType: model.KindTest, EntityID: t.ID, Proposed: map[string]json.RawMessage{"expected_result": j("The account stays locked for 30 minutes.")}}}}
	return st.CreateChangeRequest(ctx, tenantID, cr4)
}
