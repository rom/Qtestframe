package app

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// seedCorrelations shows the change and defect correlations in the demo:
// the failed execution of the regression run is a regression of a resolved
// defect (which is reopened and, being critical, blocks the requirements
// its test verifies), and a requirement whose substance changed makes the
// requirement that refines it suspect, transitively (the one refining that
// one too), with the links of their tests, and its own results stale.
func seedCorrelations(ctx context.Context, st *store.Store, tenantID string, run *model.TestRun, changed *model.Requirement) error {
	es, err := st.ListExecutions(ctx, tenantID, run.ID)
	if err != nil {
		return err
	}
	for i := range es {
		if _, err := st.DetectRegressions(ctx, tenantID, &es[i], "seed"); err != nil {
			return err
		}
	}
	if changed == nil {
		return nil
	}
	reason := "requirement " + changed.Key + " changed (description) by seed"
	if err := st.MarkContentChanged(ctx, tenantID, model.KindRequirement, changed.ID); err != nil {
		return err
	}
	// the transitive impact (store/impact.go): the requirement that refines
	// it, the one refining that (two steps), their tests
	x, err := st.LoadImpactIndex(ctx, tenantID, changed.ProjectID)
	if err != nil {
		return err
	}
	_, marks := x.Analyze([]store.ImpactRoot{{Kind: model.KindRequirement, ID: changed.ID}}, 0)
	for _, m := range marks {
		why := reason
		if m.Depth > 1 && m.From != m.Root {
			why = store.PathText(m.Path) + ": " + reason
		}
		switch m.Edge {
		case model.EdgeIssue:
			_, err = st.MarkIssueOutdated(ctx, tenantID, m.ID, why)
		case model.EdgeVerifies:
			_, err = st.MarkLinkSuspect(ctx, tenantID, m.ID, why)
		default:
			_, err = st.MarkRelationSuspect(ctx, tenantID, m.ID, m.End, why)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
