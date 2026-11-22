package store

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/verification"
)

// VariantVerificationInput is VerificationInput for one variant, by the
// rules of VariantCoverage: only the trace links between a requirement
// and a test that both apply to the variant count, and a test's result is
// its combined latest result over the runs and configurations that count
// for the variant (stale passes are no evidence, as without a variant).
// Step results belong to a test's overall latest execution: they are kept
// only where the variant's result is that one. Verification records and
// exploratory sessions are not tied to variants and count as they are.
// Variant "" is VerificationInput.
func (s *Store) VariantVerificationInput(ctx context.Context, tenantID, projectID string, last map[string]string, vc *VariantCoverage, variant string) (*verification.Input, error) {
	in, err := s.VerificationInput(ctx, tenantID, projectID, last)
	if err != nil || variant == "" || vc == nil {
		return in, err
	}
	stale, err := s.LoadStaleIndex(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	links := in.Links[:0:0]
	for _, l := range in.Links {
		if vc.AppliesTo(l.RequirementID, variant) && vc.AppliesTo(l.TestCaseID, variant) {
			links = append(links, l)
		}
	}
	in.Links = links
	vlast := map[string]string{}
	for tid := range in.Tests {
		if st := vc.TestResult(tid, variant); st != "" && st != model.ResultNotRun {
			vlast[tid] = st
		}
	}
	vlast = stale.FilterLast(vlast)
	for tid := range in.Steps {
		if vlast[tid] == "" || vlast[tid] != in.Last[tid] {
			delete(in.Steps, tid)
		}
	}
	in.Last = vlast
	return in, nil
}
