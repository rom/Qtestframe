package store

import (
	"context"
	"sort"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/verification"
)

// Suspect links and "status follows evidence".
//
// When the content of a requirement or test changes, its trace links are
// marked suspect: the test may no longer verify what the requirement says.
// Someone confirms each link after checking it. With the default settings
// an implemented requirement becomes verified when all its linked tests
// passed and no link is suspect, and a verified requirement goes back to
// implemented when a linked test fails or has not been run, or a link is
// suspect. Requirements without tests are never changed automatically.

// RequirementContentFields and TestContentFields are the snapshot fields
// whose change makes the links of an item suspect.
var (
	RequirementContentFields = []string{"title", "description", "obligation", "criteria"}
	TestContentFields        = []string{"title", "description", "preconditions", "steps", "expected_result", "script", "execution_type", "format", "gherkin"}
)

// ContentChanged returns the content fields among changed (for kind).
func ContentChanged(kind string, changed []string) []string {
	fields := RequirementContentFields
	if kind == model.KindTest {
		fields = TestContentFields
	}
	var out []string
	for _, c := range changed {
		for _, f := range fields {
			if c == f {
				out = append(out, c)
			}
		}
	}
	return out
}

// MarkLinksSuspect marks the trace links of a requirement or test suspect
// (links already suspect keep their date and reason). Returns the IDs of the
// requirements whose links changed.
func (s *Store) MarkLinksSuspect(ctx context.Context, tenantID, kind, id, reason string) ([]string, error) {
	col := "requirement_id"
	if kind == model.KindTest {
		col = "test_case_id"
	}
	rows, err := s.c(ctx).query(ctx, `SELECT requirement_id FROM trace_links WHERE tenant_id = ? AND `+col+` = ? AND suspect_since IS NULL`, tenantID, id)
	if err != nil {
		return nil, err
	}
	reqs, err := scanStrings(rows)
	if err != nil || len(reqs) == 0 {
		return nil, err
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	_, err = s.c(ctx).exec(ctx, `UPDATE trace_links SET suspect_since = ?, suspect_reason = ? WHERE tenant_id = ? AND `+col+` = ? AND suspect_since IS NULL`, s.now(), reason, tenantID, id)
	return reqs, err
}

// ConfirmLinks clears the suspect mark of links: by link ID, or all links
// of a requirement or test (kind + itemID). Returns the confirmed links.
func (s *Store) ConfirmLinks(ctx context.Context, tenantID string, linkIDs []string, kind, itemID string) ([]model.TraceLink, error) {
	where := `tenant_id = ? AND suspect_since IS NOT NULL`
	args := []any{tenantID}
	switch {
	case len(linkIDs) > 0:
		where += ` AND id IN (` + placeholders(len(linkIDs)) + `)`
		args = append(args, anySlice(linkIDs)...)
	case kind == model.KindRequirement:
		where += ` AND requirement_id = ?`
		args = append(args, itemID)
	case kind == model.KindTest:
		where += ` AND test_case_id = ?`
		args = append(args, itemID)
	default:
		return nil, Invalidf("nothing to confirm")
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM trace_links WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	ids, err := scanStrings(rows)
	if err != nil || len(ids) == 0 {
		return []model.TraceLink{}, err
	}
	out := []model.TraceLink{}
	for _, id := range ids {
		l, err := s.GetLink(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	_, err = s.c(ctx).exec(ctx, `UPDATE trace_links SET suspect_since = NULL, suspect_reason = '' WHERE `+where, args...)
	return out, err
}

// EvidenceStatus is the status a requirement should have after its
// evidence changed, or "" when it stays as it is.
func EvidenceStatus(status string, tests []string, last map[string]string, suspect bool) string {
	if len(tests) == 0 || (status != "implemented" && status != "verified") {
		return ""
	}
	allPassed := true
	for _, t := range tests {
		if last[t] != model.ResultPassed {
			allPassed = false
			break
		}
	}
	switch {
	case status == "implemented" && allPassed && !suspect:
		return "verified"
	case status == "verified" && (!allPassed || suspect):
		return "implemented"
	}
	return ""
}

// StatusChange is a requirement status changed by its evidence.
type StatusChange struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// SyncEvidenceStatus lets the status of requirements of a project (reqIDs
// and the requirements they refine, or all when nil) follow their
// verification (package verification: criteria, tests, verification
// records and the roll-up of children) unless the tenant turned it off:
// implemented becomes verified when the requirement is verified, verified
// goes back to implemented when it no longer is. Requirements without any
// evidence or children are never changed. Each change bumps the version
// and is recorded as a revision by user.
func (s *Store) SyncEvidenceStatus(ctx context.Context, tenantID, projectID string, reqIDs []string, user string) ([]StatusChange, error) {
	st, err := s.TenantSettings(ctx, tenantID)
	if err != nil || st.ManualVerification {
		return nil, err
	}
	res, err := s.ProjectVerification(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	// stale results are already dropped by VerificationInput (staleness.go)
	if reqIDs == nil {
		for id := range res {
			reqIDs = append(reqIDs, id)
		}
		sort.Strings(reqIDs)
	} else {
		// a parent's verification depends on its children
		reqIDs = append(append([]string{}, reqIDs...), verification.Ancestors(res, reqIDs)...)
	}
	var out []StatusChange
	seen := map[string]bool{}
	for _, id := range reqIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		v := res[id]
		if v == nil || (v.Own == model.VerNone && len(v.Children) == 0) {
			continue
		}
		next := ""
		switch {
		case v.Status == "implemented" && v.Verified:
			next = "verified"
		case v.Status == "verified" && !v.Verified:
			next = "implemented"
		}
		if next == "" {
			continue
		}
		r, err := s.GetRequirement(ctx, tenantID, id)
		if err != nil || r.ProjectID != projectID {
			continue
		}
		reason := "all linked tests passed"
		if v.CriteriaTotal > 0 || len(v.Children) > 0 || hasRecord(v) {
			reason = "verified: every criterion and child has passing evidence"
		}
		if next == "implemented" {
			reason = verification.Why(v)
		}
		r.Version++
		r.UpdatedAt = s.now()
		if _, err := s.c(ctx).exec(ctx, `UPDATE requirements SET status = ?, version = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, next, r.Version, r.UpdatedAt, id, tenantID); err != nil {
			return out, err
		}
		from := r.Status
		r.Status = next
		if err := s.AddRevision(ctx, tenantID, model.KindRequirement, id, r.Version, "evidence", user, SnapshotRequirement(r)); err != nil {
			return out, err
		}
		out = append(out, StatusChange{ID: id, Key: r.Key, From: from, To: next, Reason: reason})
	}
	return out, nil
}

func hasRecord(v *model.RequirementVerification) bool {
	for _, e := range v.Evidence {
		if e.Kind == "record" {
			return true
		}
	}
	return false
}
