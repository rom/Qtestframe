package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// SeedDemoWorkList gives the demo project owners and assignments so that
// "My work" has something to show for the administrator (adminEmail) and
// for Dana (DemoMemberEmail): owned requirements, tests and risks, an
// assigned defect and test executions, an overdue review request, a waiver
// waiting for the administrator's approval and a risk review that is due.
// It runs after the users exist (seed-demo creates the administrator only
// after the demo project). Nothing happens when the administrator is
// unknown.
func SeedDemoWorkList(ctx context.Context, st *store.Store, tenantID string, p *model.Project, adminEmail string) error {
	admin, err := st.GetUserByEmail(ctx, adminEmail)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	dana, err := demoUser(ctx, st, tenantID, DemoMemberEmail, "Dana Lind", rbac.RoleTester)
	if err != nil {
		return err
	}
	owners := []string{admin.Email, dana.Email}

	// owners of requirements and tests, alternating
	reqs, err := st.ListRequirements(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	for i := range reqs {
		rq := reqs[i]
		owner := owners[i%2]
		if i%3 == 2 && rq.Owner != "" {
			continue // keep some of the named owners (people without an account)
		}
		if _, err := st.BulkUpdateItems(ctx, tenantID, model.KindRequirement, p.ID, []string{rq.ID}, store.BulkUpdate{Owner: &owner}); err != nil {
			return err
		}
	}
	tests, err := st.ListTests(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	for i := range tests {
		owner := owners[(i+1)%2]
		if _, err := st.BulkUpdateItems(ctx, tenantID, model.KindTest, p.ID, []string{tests[i].ID}, store.BulkUpdate{Owner: &owner}); err != nil {
			return err
		}
	}
	// a draft test of the administrator's that is waiting for review
	if len(tests) > 0 {
		review := "review"
		if _, err := st.BulkUpdateItems(ctx, tenantID, model.KindTest, p.ID, []string{tests[len(tests)-1].ID}, store.BulkUpdate{Status: &review, Owner: &admin.Email}); err != nil {
			return err
		}
	}

	// risks: the administrator owns the open ones and reviews one that is due
	risks, err := st.ListRisks(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	reviewed := false
	for i := range risks {
		k := &risks[i]
		if slices.Contains(model.RiskOpenStatuses, k.Status) && i%2 == 0 {
			k.Owner = admin.Email
		}
		if !reviewed && k.Status != "closed" {
			k.ReviewOwner, k.ReviewDate = admin.Email, time.Now().UTC().AddDate(0, 0, -3).Format(time.DateOnly)
			reviewed = true
		}
		if err := st.UpdateRisk(ctx, tenantID, k); err != nil {
			return err
		}
	}

	// an open defect assigned to the administrator
	defects, err := st.ListDefects(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	for i := range defects {
		d := &defects[i]
		if slices.Contains(model.DefectOpenStatuses, d.Status) {
			d.Assignee = admin.Email
			if err := st.UpdateDefect(ctx, tenantID, d); err != nil {
				return err
			}
			break
		}
	}

	// test executions not run yet in an open run: the administrator and Dana
	runs, err := st.ListRuns(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	assigned := 0
	for _, run := range runs {
		if run.Status != "planned" && run.Status != "in_progress" || assigned >= 4 {
			continue
		}
		es, err := st.ListExecutions(ctx, tenantID, run.ID)
		if err != nil {
			return err
		}
		for _, e := range es {
			if e.Status != model.ResultNotRun || assigned >= 4 {
				continue
			}
			who := owners[assigned%2]
			if _, err := st.UpdateExecution(ctx, tenantID, e.ID, "seed", store.ExecutionUpdate{Assignee: &who}); err != nil {
				return err
			}
			assigned++
		}
	}

	// a review request from Dana to the administrator, overdue
	if len(reqs) > 1 {
		if _, err := st.CreateReviewRequests(ctx, tenantID, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: reqs[1].ID,
			RequestedBy: dana.Email, Message: "Please check the wording before the release.",
			DueDate: time.Now().UTC().AddDate(0, 0, -2).Format(time.DateOnly)}, []string{admin.ID}); err != nil && !errors.Is(err, store.ErrConflict) {
			return err
		}
	}

	// reviews the administrator asked Dana for: one still pending (awaiting
	// review), one that came back with changes requested
	if len(reqs) > 3 {
		if _, err := st.CreateReviewRequests(ctx, tenantID, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: reqs[2].ID,
			RequestedBy: admin.Email, Message: "Does this match what the testers need?"}, []string{dana.ID}); err != nil && !errors.Is(err, store.ErrConflict) {
			return err
		}
		back, err := st.CreateReviewRequests(ctx, tenantID, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: reqs[3].ID,
			RequestedBy: admin.Email, Message: "Please review before we baseline."}, []string{dana.ID})
		if err != nil && !errors.Is(err, store.ErrConflict) {
			return err
		}
		for i := range back {
			v := &back[i]
			v.Status, v.Comment = "changes_requested", "The acceptance criteria do not say what happens on a timeout."
			if err := st.UpdateReviewRequest(ctx, tenantID, v); err != nil {
				return err
			}
		}
	}

	// a waiver waiting for the administrator's approval
	if len(reqs) > 6 {
		w := &model.Waiver{ProjectID: p.ID, Type: "waiver", Title: "Accept the slower statement export for 1.4", RequirementIDs: []string{reqs[6].ID},
			Justification: "The export is correct but takes up to 8 seconds for large accounts.", Compensating: "Customers get an e-mail when the export is ready.",
			RequestedBy: dana.Email, Approvers: []string{admin.Email}, Status: "requested", CreatedBy: dana.Email}
		if err := st.CreateWaiver(ctx, tenantID, w); err != nil {
			return err
		}
	}
	return nil
}
