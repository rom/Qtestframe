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
// requirement that refines it suspect, with the links of its tests, and its
// own results stale.
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
	if _, err := st.MarkLinksSuspect(ctx, tenantID, model.KindRequirement, changed.ID, reason); err != nil {
		return err
	}
	irs, err := st.MarkRelationsSuspect(ctx, tenantID, changed.ID, reason)
	if err != nil {
		return err
	}
	for _, ir := range irs {
		if _, err := st.MarkLinksSuspect(ctx, tenantID, model.KindRequirement, ir.OtherID, "requirement "+changed.Key+" ("+ir.Relation.Type+") changed: "+reason); err != nil {
			return err
		}
	}
	return nil
}
