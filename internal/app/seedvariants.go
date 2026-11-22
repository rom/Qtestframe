package app

import (
	"context"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedVariantsAndCode gives the demo project variants (products and
// markets), applicability, a run for a variant, CI builds and code changes
// that refer to requirements and tests.
func seedVariantsAndCode(ctx context.Context, st *store.Store, tenantID string, p *model.Project, reqs []*model.Requirement, testIDs []string, regression *model.TestRun) error {
	variant := func(name, kind string, parent *model.Variant, attrs map[string]string) (*model.Variant, error) {
		v := &model.Variant{ProjectID: p.ID, Name: name, Kind: kind, Attributes: attrs, CreatedBy: "seed"}
		if parent != nil {
			v.ParentID = &parent.ID
		}
		return v, st.CreateVariant(ctx, tenantID, v)
	}
	if _, err := variant("Retail banking", "product", nil, map[string]string{"segment": "retail"}); err != nil {
		return err
	}
	business, err := variant("Business banking", "product", nil, map[string]string{"segment": "business"})
	if err != nil {
		return err
	}
	eu, err := variant("EU", "market", nil, map[string]string{"region": "EU", "currency": "EUR"})
	if err != nil {
		return err
	}
	if _, err := variant("Germany", "market", eu, map[string]string{"lang": "de"}); err != nil {
		return err
	}
	if _, err := variant("US", "market", nil, map[string]string{"region": "US", "currency": "USD"}); err != nil {
		return err
	}
	// single sign-on is for business customers; the statement export for retail
	if len(reqs) > 5 {
		if _, err := st.SetApplicability(ctx, tenantID, p.ID, model.KindRequirement, []string{reqs[2].ID}, &model.Applicability{VariantIDs: []string{business.ID}}, "seed"); err != nil {
			return err
		}
		if _, err := st.SetApplicability(ctx, tenantID, p.ID, model.KindRequirement, []string{reqs[5].ID}, &model.Applicability{Expression: "segment:retail"}, "seed"); err != nil {
			return err
		}
	}

	// builds: the regression run's and an EU acceptance run's
	now := st.Now()
	rc1 := &model.Build{ProjectID: p.ID, ExternalID: "4711", Number: "1.4.0-rc1", CommitSHA: "3f2c9e1a7b4d5c6e8f9012345678abcdef012345", Branch: "main",
		PipelineURL: "https://ci.example.org/banking/pipelines/4711", CreatedBy: "seed"}
	if _, err := st.UpsertBuild(ctx, tenantID, rc1); err != nil {
		return err
	}
	if err := st.SetRunBuild(ctx, tenantID, regression.ID, rc1, nil); err != nil {
		return err
	}
	rc2 := &model.Build{ProjectID: p.ID, ExternalID: "4730", Number: "1.4.0-rc2", CommitSHA: "9a8b7c6d5e4f30211234567890abcdef98765432", Branch: "main",
		PipelineURL: "https://ci.example.org/banking/pipelines/4730", CreatedBy: "seed"}
	if _, err := st.UpsertBuild(ctx, tenantID, rc2); err != nil {
		return err
	}
	n := min(3, len(testIDs))
	run := &model.TestRun{ProjectID: p.ID, Name: "EU acceptance", Environment: "staging-eu", Build: rc2.Number, CreatedBy: "seed", VariantID: &eu.ID, BuildID: &rc2.ID}
	if err := st.CreateRun(ctx, tenantID, run, testIDs[:n]); err != nil {
		return err
	}
	es, err := st.ListExecutions(ctx, tenantID, run.ID)
	if err != nil {
		return err
	}
	passed := model.ResultPassed
	for _, e := range es {
		if _, err := st.UpdateExecution(ctx, tenantID, e.ID, "seed", store.ExecutionUpdate{Status: &passed}); err != nil {
			return err
		}
	}

	// commits and a merge request naming requirements and tests
	type change struct {
		c     model.CodeChange
		items []model.CodeChangeItem
	}
	tests := func(i int) []model.CodeChangeItem {
		if i < len(testIDs) {
			return []model.CodeChangeItem{{ItemType: model.KindTest, ItemID: testIDs[i]}}
		}
		return nil
	}
	changes := []change{
		{model.CodeChange{Kind: model.CodeCommit, Provider: model.ProviderGitLab, Repository: "banking/portal", Ref: rc1.CommitSHA,
			Title: "Generic error message on failed login (" + reqs[0].Key + ")", URL: "https://gitlab.example.org/banking/portal/-/commit/" + rc1.CommitSHA,
			Author: "Dana Developer", Branch: "main", OccurredAt: now.Add(-6 * 24 * time.Hour)},
			append([]model.CodeChangeItem{{ItemType: model.KindRequirement, ItemID: reqs[0].ID}}, tests(0)...)},
		{model.CodeChange{Kind: model.CodeMergeRequest, Provider: model.ProviderGitLab, Repository: "banking/portal", Ref: "42",
			Title: "Lock accounts after 10 failed attempts", URL: "https://gitlab.example.org/banking/portal/-/merge_requests/42",
			Author: "dana", Branch: "feature/" + reqs[1].Key + "-lockout", TargetBranch: "main", Status: "merged", OccurredAt: now.Add(-5 * 24 * time.Hour)},
			[]model.CodeChangeItem{{ItemType: model.KindRequirement, ItemID: reqs[1].ID}}},
		{model.CodeChange{Kind: model.CodeCommit, Provider: model.ProviderGitLab, Repository: "banking/portal", Ref: rc2.CommitSHA,
			Title: "Reject transfers above the daily limit (" + reqs[3].Key + ")", URL: "https://gitlab.example.org/banking/portal/-/commit/" + rc2.CommitSHA,
			Author: "Erik Engineer", Branch: "main", OccurredAt: now.Add(-2 * 24 * time.Hour)},
			[]model.CodeChangeItem{{ItemType: model.KindRequirement, ItemID: reqs[3].ID}}},
	}
	for _, ch := range changes {
		c := ch.c
		c.ProjectID = p.ID
		if _, err := st.UpsertCodeChange(ctx, tenantID, &c); err != nil {
			return err
		}
		if _, err := st.LinkCodeChange(ctx, tenantID, c.ID, ch.items, model.CodeLinkAuto, ""); err != nil {
			return err
		}
	}
	return nil
}
