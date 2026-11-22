package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Releases: a version of the product with the requirements it contains,
// the runs that test it and exit criteria that decide go / no-go.

const releaseCols = `id, project_id, name, version, description, status, target_date, exit_criteria, released_at, released_by, release_note, variant_ids, created_by, created_at, updated_at`

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func scanRelease(r interface{ Scan(...any) error }) (*model.Release, error) {
	var rel model.Release
	var crit, variants string
	var at sql.NullTime
	if err := r.Scan(&rel.ID, &rel.ProjectID, &rel.Name, &rel.Version, &rel.Description, &rel.Status, &rel.TargetDate, &crit, &at, &rel.ReleasedBy, &rel.ReleaseNote, &variants, &rel.CreatedBy, &rel.CreatedAt, &rel.UpdatedAt); err != nil {
		return nil, err
	}
	rel.VariantIDs = decodeTags(variants)
	_ = json.Unmarshal([]byte(crit), &rel.ExitCriteria)
	if rel.ExitCriteria == nil {
		rel.ExitCriteria = []model.ExitCriterion{}
	}
	rel.ReleasedAt = timePtr(at)
	return &rel, nil
}

// validateRelease checks and normalises the editable fields.
func validateRelease(r *model.Release) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len(r.Name) > 200 {
		return Invalidf("release name is required (at most 200 characters)")
	}
	r.Version = strings.TrimSpace(r.Version)
	if r.Status == "" {
		r.Status = "planned"
	}
	if !slices.Contains(model.ReleaseStatuses, r.Status) {
		return Invalidf("status must be one of %s", strings.Join(model.ReleaseStatuses, ", "))
	}
	r.TargetDate = strings.TrimSpace(r.TargetDate)
	if r.TargetDate != "" && !dateRe.MatchString(r.TargetDate) {
		return Invalidf("target_date must be YYYY-MM-DD")
	}
	if r.ExitCriteria == nil {
		r.ExitCriteria = []model.ExitCriterion{}
	}
	if len(r.ExitCriteria) > 30 {
		return Invalidf("at most 30 exit criteria")
	}
	for i := range r.ExitCriteria {
		c := &r.ExitCriteria[i]
		if !slices.Contains(model.ExitCriterionTypes, c.Type) {
			return Invalidf("exit criterion type must be one of %s", strings.Join(model.ExitCriterionTypes, ", "))
		}
		if c.Threshold < 0 || c.Threshold > 100 {
			return Invalidf("threshold must be between 0 and 100")
		}
		if c.Threshold == 0 && percentCriterion(c.Type) {
			c.Threshold = 100
		}
		if c.Type == "residual_risk" && !ValidRiskThreshold(c.Level) {
			return Invalidf("the level of residual_risk must be one of %s (or empty for the project's threshold)", strings.Join(model.RiskLevels, ", "))
		}
		if c.Type == "cycle_executed" && strings.TrimSpace(c.CycleID) == "" {
			return Invalidf("cycle_executed needs a cycle_id")
		}
		if c.Type != "cycle_executed" {
			c.CycleID = ""
		}
		if c.Type == "trace_rules" {
			if c.Level == "" {
				c.Level = "error"
			}
			if c.Level != "error" && c.Level != "warning" {
				return Invalidf("the level of trace_rules must be error or warning")
			}
		}
	}
	return nil
}

func percentCriterion(t string) bool {
	return t == "coverage" || t == "passed" || t == "executed" || t == "approved" || t == "cycle_executed"
}

// checkCriteriaCycles verifies that the cycles exit criteria refer to
// belong to the project.
func checkCriteriaCycles(ctx context.Context, c conn, tenantID, projectID string, criteria []model.ExitCriterion) error {
	for _, cr := range criteria {
		if cr.CycleID == "" {
			continue
		}
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM test_cycles WHERE id = ? AND tenant_id = ?`, cr.CycleID, tenantID).Scan(&pid); err != nil || pid != projectID {
			return Invalidf("cycle %q of an exit criterion not found in the project", cr.CycleID)
		}
	}
	return nil
}

// checkRelease verifies that a release belongs to the project.
func checkRelease(ctx context.Context, c conn, tenantID, projectID string, id *string) error {
	if id == nil || *id == "" {
		return nil
	}
	var pid string
	if err := c.row(ctx, `SELECT project_id FROM releases WHERE id = ? AND tenant_id = ?`, *id, tenantID).Scan(&pid); err != nil {
		return mapErr(err, "release")
	}
	if pid != projectID {
		return Invalidf("the release belongs to another project")
	}
	return nil
}

// CreateRelease adds a release (with the default exit criteria when none are given).
func (s *Store) CreateRelease(ctx context.Context, tenantID string, r *model.Release) error {
	if r.ExitCriteria == nil {
		r.ExitCriteria = append([]model.ExitCriterion{}, model.DefaultExitCriteria...)
	}
	if err := validateRelease(r); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, r.ProjectID); err != nil {
		return err
	}
	var err error
	if r.VariantIDs, err = s.ResolveVariants(ctx, tenantID, r.ProjectID, r.VariantIDs); err != nil {
		return err
	}
	if err = checkCriteriaCycles(ctx, s.c(ctx), tenantID, r.ProjectID, r.ExitCriteria); err != nil {
		return err
	}
	r.ID = newID()
	r.CreatedAt = s.now()
	r.UpdatedAt = r.CreatedAt
	crit, _ := json.Marshal(r.ExitCriteria)
	_, err = s.c(ctx).exec(ctx, `INSERT INTO releases (id, tenant_id, project_id, name, version, description, status, target_date, exit_criteria, variant_ids, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, tenantID, r.ProjectID, r.Name, r.Version, r.Description, r.Status, r.TargetDate, string(crit), encodeTags(r.VariantIDs), r.CreatedBy, r.CreatedAt, r.UpdatedAt)
	return mapErr(err, "release name")
}

// UpdateRelease saves the editable fields; becoming "released" records
// when and by whom.
func (s *Store) UpdateRelease(ctx context.Context, tenantID string, r *model.Release, user string) error {
	if err := validateRelease(r); err != nil {
		return err
	}
	var err error
	if r.VariantIDs, err = s.ResolveVariants(ctx, tenantID, r.ProjectID, r.VariantIDs); err != nil {
		return err
	}
	if err = checkCriteriaCycles(ctx, s.c(ctx), tenantID, r.ProjectID, r.ExitCriteria); err != nil {
		return err
	}
	r.UpdatedAt = s.now()
	if r.Status == "released" && r.ReleasedAt == nil {
		at := r.UpdatedAt
		r.ReleasedAt, r.ReleasedBy = &at, user
	}
	if r.Status != "released" {
		r.ReleasedAt, r.ReleasedBy = nil, ""
	}
	crit, _ := json.Marshal(r.ExitCriteria)
	res, err := s.c(ctx).exec(ctx, `UPDATE releases SET name = ?, version = ?, description = ?, status = ?, target_date = ?, exit_criteria = ?, released_at = ?, released_by = ?, release_note = ?, variant_ids = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		r.Name, r.Version, r.Description, r.Status, r.TargetDate, string(crit), nullTime(r.ReleasedAt), r.ReleasedBy, r.ReleaseNote, encodeTags(r.VariantIDs), r.UpdatedAt, r.ID, tenantID)
	if err != nil {
		return mapErr(err, "release name")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "release")
	}
	return nil
}

// GetRelease returns a release with its requirement IDs and counts.
func (s *Store) GetRelease(ctx context.Context, tenantID, id string) (*model.Release, error) {
	r, err := scanRelease(s.c(ctx).row(ctx, `SELECT `+releaseCols+` FROM releases WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "release")
	}
	rows, err := s.c(ctx).query(ctx, `SELECT rr.requirement_id FROM release_requirements rr JOIN requirements q ON q.id = rr.requirement_id WHERE rr.release_id = ? ORDER BY q.seq`, id)
	if err != nil {
		return nil, err
	}
	if r.RequirementIDs, err = scanStrings(rows); err != nil {
		return nil, err
	}
	if r.RequirementIDs == nil {
		r.RequirementIDs = []string{}
	}
	r.Requirements = len(r.RequirementIDs)
	if err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM test_runs WHERE release_id = ?`, id).Scan(&r.Runs); err != nil {
		return nil, err
	}
	if err := s.releaseVariantRefs(ctx, tenantID, r.ProjectID, []*model.Release{r}); err != nil {
		return nil, err
	}
	return r, nil
}

// ListReleases lists the releases of a project, newest first.
func (s *Store) ListReleases(ctx context.Context, tenantID, projectID string) ([]model.Release, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+releaseCols+` FROM releases WHERE tenant_id = ? AND project_id = ? ORDER BY created_at DESC`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Release{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	counts := func(q string) (map[string]int, error) {
		rows, err := s.c(ctx).query(ctx, q, projectID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		m := map[string]int{}
		for rows.Next() {
			var id string
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return nil, err
			}
			m[id] = n
		}
		return m, rows.Err()
	}
	reqs, err := counts(`SELECT rr.release_id, COUNT(*) FROM release_requirements rr JOIN releases r ON r.id = rr.release_id WHERE r.project_id = ? GROUP BY rr.release_id`)
	if err != nil {
		return nil, err
	}
	runs, err := counts(`SELECT release_id, COUNT(*) FROM test_runs WHERE project_id = ? AND release_id IS NOT NULL GROUP BY release_id`)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*model.Release, len(out))
	for i := range out {
		out[i].Requirements, out[i].Runs = reqs[out[i].ID], runs[out[i].ID]
		ptrs[i] = &out[i]
	}
	return out, s.releaseVariantRefs(ctx, tenantID, projectID, ptrs)
}

// releaseVariantRefs fills the names of the variants releases target.
func (s *Store) releaseVariantRefs(ctx context.Context, tenantID, projectID string, rels []*model.Release) error {
	if !slices.ContainsFunc(rels, func(r *model.Release) bool { return len(r.VariantIDs) > 0 }) {
		return nil
	}
	list, err := s.ListVariants(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	vs := NewVariantSet(list)
	for _, r := range rels {
		if len(r.VariantIDs) > 0 {
			r.Variants = vs.RefsOf(r.VariantIDs)
		}
	}
	return nil
}

// DeleteRelease removes a release (its runs stay, without a release).
func (s *Store) DeleteRelease(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `UPDATE test_runs SET release_id = NULL WHERE release_id = ?`, id); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `DELETE FROM release_requirements WHERE release_id = ?`, id); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM releases WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "release")
		}
		return nil
	})
}

// SetReleaseRequirements adds and removes requirements of the release's
// project. Returns the number added and removed.
func (s *Store) SetReleaseRequirements(ctx context.Context, tenantID, releaseID string, add, remove []string) (int, int, error) {
	r, err := s.GetRelease(ctx, tenantID, releaseID)
	if err != nil {
		return 0, 0, err
	}
	have := map[string]bool{}
	for _, id := range r.RequirementIDs {
		have[id] = true
	}
	added, removed := 0, 0
	err = s.tx(ctx, func(c conn) error {
		for _, id := range add {
			if have[id] {
				continue
			}
			var pid string
			if err := c.row(ctx, `SELECT project_id FROM requirements WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
				return mapErr(err, "requirement")
			}
			if pid != r.ProjectID {
				return Invalidf("requirement %s belongs to another project", id)
			}
			if _, err := c.exec(ctx, `INSERT INTO release_requirements (release_id, requirement_id) VALUES (?, ?)`, releaseID, id); err != nil {
				return err
			}
			have[id] = true
			added++
		}
		for _, id := range remove {
			res, err := c.exec(ctx, `DELETE FROM release_requirements WHERE release_id = ? AND requirement_id = ?`, releaseID, id)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			removed += int(n)
		}
		return nil
	})
	return added, removed, err
}

// ---------------------------------------------------------------- evaluation

// ReleaseRequirement is a scope requirement as the evaluation sees it.
type ReleaseRequirement struct {
	ID, Key, Title, Obligation, Status string
	Tests                              []string
	Suspect                            bool
	// Blocking: open defects that block the requirement (DefectBlocks).
	Blocking []model.DefectBlock
	// Waiver: the keys of the approved, unexpired waivers that accept the
	// requirement for the release ("" none).
	Waiver string
}

// ReleaseData is everything the exit criteria are evaluated on.
type ReleaseData struct {
	Requirements []ReleaseRequirement
	// Results: test id → latest result in the release's runs (a stale
	// pass is no result).
	Results map[string]string
	// Stale: tests whose latest pass in the release's runs is stale.
	Stale map[string]bool
	// Executions in the release's runs and how many have a result.
	Executions, Executed int
	// FailedTests: keys of tests whose latest result in the release failed or is blocked.
	FailedTests []ReleaseItem
	// OpenDefects returns the open defects at or above a severity (nil when unknown).
	OpenDefects func(level string) []ReleaseItem
	// OpenRisks returns the unmitigated risks at or above a level (nil when unknown).
	OpenRisks func(level string) []ReleaseItem
	// RuleViolations returns the items of the scope that break traceability
	// rules of at least a severity (nil when unknown).
	RuleViolations func(level string) []ReleaseItem
	// TestRequirements: test id → the scope requirements it verifies.
	TestRequirements map[string][]string
	// ResidualRisks returns the risks of the scope whose residual level is
	// above a threshold ("" = the project's): those not accepted and those
	// accepted with a valid sign-off (nil when unknown).
	ResidualRisks func(threshold string) (open, accepted []ReleaseItem)
	// Cycles returns the plan of a test cycle (ok false when the cycle
	// does not exist; nil when unknown).
	Cycles func(id string) (plan []model.TestCycleTest, ok bool)
}

// ReleaseItem is something a criterion refers to.
type ReleaseItem struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	Note  string `json:"note,omitempty"`
	// Variants: in a release evaluated per variant, the targeted variants
	// in which the item keeps the criterion from being met (or is waived).
	Variants []string `json:"variants,omitempty"`
}

// CriterionResult is the evaluation of one exit criterion.
type CriterionResult struct {
	model.ExitCriterion
	Met bool `json:"met"`
	// Actual value (percent for percentage criteria, else a count).
	Actual float64 `json:"actual"`
	// Total the percentage refers to.
	Total int `json:"total"`
	// Items that keep the criterion from being met (at most 50).
	Items []ReleaseItem `json:"items"`
	// Available is false when the data for the criterion does not exist.
	Available bool `json:"available"`
	// Waived: requirements (or tests of only such requirements) that would
	// keep the criterion from being met but are accepted by a waiver.
	Waived []ReleaseItem `json:"waived"`
	// Accepted: risks above the threshold with a valid "accept risk"
	// sign-off (residual_risk).
	Accepted []ReleaseItem `json:"accepted,omitempty"`
	// State: pass | waived (met thanks to waivers or accepted risks) | fail.
	State string `json:"state"`
}

// Criterion states.
const (
	CriterionPass   = "pass"
	CriterionWaived = "waived"
	CriterionFail   = "fail"
)

func criterionState(c *CriterionResult) string {
	switch {
	case !c.Met:
		return CriterionFail
	case len(c.Waived) > 0 || len(c.Accepted) > 0:
		return CriterionWaived
	}
	return CriterionPass
}

// VariantGate is the evaluation of a release for one targeted variant.
type VariantGate struct {
	Variant  model.VariantRef  `json:"variant"`
	Go       bool              `json:"go"`
	Criteria []CriterionResult `json:"criteria"`
	Summary  map[string]int    `json:"summary"`
}

// ReleaseStatusReport is the go / no-go of a release.
type ReleaseStatusReport struct {
	Release  *model.Release    `json:"release"`
	Go       bool              `json:"go"`
	Criteria []CriterionResult `json:"criteria"`
	Summary  map[string]int    `json:"summary"`
	// Waivers in force for the release (approved, not expired; for the
	// release or the whole project) that cover scope requirements.
	Waivers []model.Waiver `json:"waivers"`
	// RiskThreshold in effect and ResidualRisks: the risks of the scope
	// whose residual level is above it (Note: the residual level, with
	// ", accepted" when signed off).
	RiskThreshold string        `json:"risk_threshold"`
	ResidualRisks []ReleaseItem `json:"residual_risks"`
	// Variant the release was evaluated for (nil: all variants).
	Variant *model.VariantRef `json:"variant,omitempty"`
	// Variants: for a release that targets variants (evaluated without
	// ?variant=), the gate of each targeted variant; Go is then true only
	// when every variant's is and Criteria combine them.
	Variants []VariantGate `json:"variants,omitempty"`
}

func mandatory(ob string) bool {
	return ob == "must" || ob == "shall" || ob == "must_not" || ob == "shall_not"
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// EvaluateExitCriteria evaluates criteria on data.
func EvaluateExitCriteria(criteria []model.ExitCriterion, d ReleaseData) []CriterionResult {
	reqItem := func(r ReleaseRequirement, note string) ReleaseItem {
		return ReleaseItem{Kind: model.KindRequirement, ID: r.ID, Key: r.Key, Title: r.Title, Note: note}
	}
	waivedItem := func(r ReleaseRequirement) ReleaseItem { return reqItem(r, r.Waiver) }
	waivedReq := map[string]string{}
	for _, r := range d.Requirements {
		if r.Waiver != "" {
			waivedReq[r.ID] = r.Waiver
		}
	}
	allPassed := func(r ReleaseRequirement) (bool, string) {
		if len(r.Tests) == 0 {
			return false, "none"
		}
		state := model.ResultPassed
		for _, t := range r.Tests {
			switch d.Results[t] {
			case model.ResultPassed:
			case model.ResultFailed, model.ResultBlocked:
				return false, model.ResultFailed
			default:
				if d.Stale[t] && state == model.ResultPassed {
					state = "stale"
				} else {
					state = model.ResultNotRun
				}
			}
		}
		return state == model.ResultPassed, state
	}
	pctResult := func(c model.ExitCriterion, ok, total int, items []ReleaseItem) CriterionResult {
		// nothing to measure (an empty scope, no executions) is not a pass
		if total == 0 {
			return CriterionResult{ExitCriterion: c, Met: false, Total: 0, Items: []ReleaseItem{}, Available: false}
		}
		actual := round1(float64(ok) * 100 / float64(total))
		return CriterionResult{ExitCriterion: c, Met: actual >= c.Threshold, Actual: actual, Total: total, Items: limitItems(items), Available: true, Waived: []ReleaseItem{}}
	}
	withWaived := func(r CriterionResult, waived []ReleaseItem) CriterionResult {
		r.Waived = limitItems(waived)
		return r
	}
	var out []CriterionResult
	for _, c := range criteria {
		switch c.Type {
		case "coverage":
			ok := 0
			var miss, waived []ReleaseItem
			for _, r := range d.Requirements {
				switch {
				case len(r.Tests) > 0:
					ok++
				case r.Waiver != "":
					ok++
					waived = append(waived, waivedItem(r))
				default:
					miss = append(miss, reqItem(r, ""))
				}
			}
			out = append(out, withWaived(pctResult(c, ok, len(d.Requirements), miss), waived))
		case "passed":
			ok := 0
			var miss, waived []ReleaseItem
			for _, r := range d.Requirements {
				if p, state := allPassed(r); p {
					ok++
				} else if r.Waiver != "" {
					ok++
					waived = append(waived, waivedItem(r))
				} else {
					miss = append(miss, reqItem(r, state))
				}
			}
			out = append(out, withWaived(pctResult(c, ok, len(d.Requirements), miss), waived))
		case "mandatory_passed":
			total := 0
			var miss, waived []ReleaseItem
			for _, r := range d.Requirements {
				if !mandatory(r.Obligation) {
					continue
				}
				total++
				if p, state := allPassed(r); !p {
					if r.Waiver != "" {
						waived = append(waived, waivedItem(r))
					} else {
						miss = append(miss, reqItem(r, state))
					}
				}
			}
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(miss) == 0, Actual: float64(len(miss)), Total: total, Items: limitItems(miss), Available: true, Waived: limitItems(waived)})
		case "executed":
			out = append(out, pctResult(c, d.Executed, d.Executions, nil))
		case "no_failed":
			// a failed test of only waived requirements is accepted with them
			var failed, waived []ReleaseItem
			for _, t := range d.FailedTests {
				reqs := d.TestRequirements[t.ID]
				all := len(reqs) > 0
				var keys []string
				for _, id := range reqs {
					if waivedReq[id] == "" {
						all = false
						break
					}
					if !slices.Contains(keys, waivedReq[id]) {
						keys = append(keys, waivedReq[id])
					}
				}
				if all {
					t.Note = strings.Join(keys, ", ")
					waived = append(waived, t)
				} else {
					failed = append(failed, t)
				}
			}
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(failed) == 0, Actual: float64(len(failed)), Items: limitItems(failed), Available: true, Waived: limitItems(waived)})
		case "residual_risk":
			if d.ResidualRisks == nil {
				out = append(out, CriterionResult{ExitCriterion: c, Met: true})
				continue
			}
			open, accepted := d.ResidualRisks(c.Level)
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(open) == 0, Actual: float64(len(open)), Items: limitItems(open), Available: true, Accepted: accepted})
		case "approved":
			ok := 0
			var miss []ReleaseItem
			for _, r := range d.Requirements {
				if r.Status == "approved" || r.Status == "implemented" || r.Status == "verified" {
					ok++
				} else {
					miss = append(miss, reqItem(r, r.Status))
				}
			}
			out = append(out, pctResult(c, ok, len(d.Requirements), miss))
		case "no_suspect":
			var miss []ReleaseItem
			for _, r := range d.Requirements {
				if r.Suspect {
					miss = append(miss, reqItem(r, ""))
				}
			}
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(miss) == 0, Actual: float64(len(miss)), Items: limitItems(miss), Available: true})
		case "no_blocking_defects":
			// a waiver in force accepts a blocked requirement too
			var miss, waived []ReleaseItem
			for _, r := range d.Requirements {
				for _, b := range r.Blocking {
					if b.Level == model.DefectBlocks {
						if r.Waiver != "" {
							waived = append(waived, reqItem(r, r.Waiver))
						} else {
							miss = append(miss, reqItem(r, b.Key+" "+b.Severity))
						}
						break
					}
				}
			}
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(miss) == 0, Actual: float64(len(miss)), Items: limitItems(miss), Available: true, Waived: limitItems(waived)})
		case "no_open_defects":
			if d.OpenDefects == nil {
				out = append(out, CriterionResult{ExitCriterion: c, Met: true})
				continue
			}
			open := d.OpenDefects(c.Level)
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(open) == 0, Actual: float64(len(open)), Items: limitItems(open), Available: true})
		case "trace_rules":
			if d.RuleViolations == nil {
				out = append(out, CriterionResult{ExitCriterion: c, Met: true})
				continue
			}
			bad := d.RuleViolations(c.Level)
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(bad) == 0, Actual: float64(len(bad)), Items: limitItems(bad), Available: true})
		case "cycle_executed":
			var plan []model.TestCycleTest
			ok := false
			if d.Cycles != nil {
				plan, ok = d.Cycles(c.CycleID)
			}
			if !ok {
				out = append(out, CriterionResult{ExitCriterion: c, Met: false, Items: []ReleaseItem{}, Available: false})
				continue
			}
			done := 0
			var miss []ReleaseItem
			for _, x := range plan {
				if x.Executed() {
					done++
				} else {
					note := x.Status
					if x.ConfigurationName != "" {
						note = x.ConfigurationName + ", " + note
					}
					miss = append(miss, ReleaseItem{Kind: model.KindTest, ID: x.TestCaseID, Key: x.TestKey, Title: x.TestTitle, Note: note})
				}
			}
			out = append(out, pctResult(c, done, len(plan), miss))
		case "risks_mitigated":
			if d.OpenRisks == nil {
				out = append(out, CriterionResult{ExitCriterion: c, Met: true})
				continue
			}
			open := d.OpenRisks(c.Level)
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(open) == 0, Actual: float64(len(open)), Items: limitItems(open), Available: true})
		}
	}
	for i := range out {
		if out[i].Items == nil {
			out[i].Items = []ReleaseItem{}
		}
		if out[i].Waived == nil {
			out[i].Waived = []ReleaseItem{}
		}
		out[i].State = criterionState(&out[i])
	}
	if out == nil {
		out = []CriterionResult{}
	}
	return out
}

func limitItems(items []ReleaseItem) []ReleaseItem {
	if items == nil {
		return []ReleaseItem{}
	}
	if len(items) > 50 {
		return items[:50]
	}
	return items
}

// ReleaseStatus evaluates a release. defects and risks provide the open
// defects and risks of the scope, rules the items of the scope (scope
// requirements and the tests linked to them) that break traceability rules
// (each nil when not available).
func (s *Store) ReleaseStatus(ctx context.Context, tenantID, id string, defects, risks func(scope []string, level string) []ReleaseItem, rules ...func(projectID string, reqs, tests []string, level string) []ReleaseItem) (*ReleaseStatusReport, error) {
	return s.ReleaseStatusForVariant(ctx, tenantID, id, "", defects, risks, rules...)
}

// ReleaseStatusForVariant evaluates a release for one variant (id or name):
// the scope is the release's requirements that apply to the variant, each
// verified by its linked tests that apply to it, and only results that
// count for the variant are used (see VariantCoverage): those of runs for
// the variant or one of its ancestors and of variant-neutral runs, in the
// configurations the variant is tested in. Only waivers for every variant
// or limited to the variant (or one of its ancestors) accept requirements.
//
// Variant "": a release that targets variants is evaluated for each of them
// (Variants) and is a go only when every targeted variant is; the criteria
// then combine the variants (met when met in every variant). A release
// that targets none is evaluated without a variant, as without variants
// (only waivers for every variant count).
func (s *Store) ReleaseStatusForVariant(ctx context.Context, tenantID, id, variant string, defects, risks func(scope []string, level string) []ReleaseItem, rules ...func(projectID string, reqs, tests []string, level string) []ReleaseItem) (*ReleaseStatusReport, error) {
	if strings.TrimSpace(variant) != "" {
		return s.releaseStatusOne(ctx, tenantID, id, variant, defects, risks, rules...)
	}
	rel, err := s.GetRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if len(rel.VariantIDs) == 0 {
		return s.releaseStatusOne(ctx, tenantID, id, "", defects, risks, rules...)
	}
	return s.releaseGates(ctx, tenantID, rel, defects, risks, rules...)
}

// releaseStatusOne evaluates a release for one variant ("" none).
func (s *Store) releaseStatusOne(ctx context.Context, tenantID, id, variant string, defects, risks func(scope []string, level string) []ReleaseItem, rules ...func(projectID string, reqs, tests []string, level string) []ReleaseItem) (*ReleaseStatusReport, error) {
	rel, err := s.GetRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	links, err := s.ListLinks(ctx, tenantID, rel.ProjectID, "", "")
	if err != nil {
		return nil, err
	}
	var vc *VariantCoverage
	var variantRef *model.VariantRef
	variantID := ""
	if strings.TrimSpace(variant) != "" {
		v, err := s.FindVariant(ctx, tenantID, rel.ProjectID, variant)
		if err != nil {
			return nil, err
		}
		if vc, err = s.LoadVariantCoverage(ctx, tenantID, rel.ProjectID, nil); err != nil {
			return nil, err
		}
		variantID, variantRef = v.ID, &model.VariantRef{ID: v.ID, Name: v.Name}
		in := []string{}
		for _, rid := range rel.RequirementIDs {
			if vc.AppliesTo(rid, variantID) {
				in = append(in, rid)
			}
		}
		rel.RequirementIDs = in
	}
	scope := map[string]bool{}
	for _, rid := range rel.RequirementIDs {
		scope[rid] = true
	}
	tests := map[string][]string{}
	suspect := map[string]bool{}
	for _, l := range links {
		if scope[l.RequirementID] && (vc == nil || vc.AppliesTo(l.TestCaseID, variantID)) {
			tests[l.RequirementID] = append(tests[l.RequirementID], l.TestCaseID)
			if l.SuspectSince != nil {
				suspect[l.RequirementID] = true
			}
		}
	}
	d := ReleaseData{Results: map[string]string{}, Stale: map[string]bool{}}
	stale, err := s.LoadStaleIndex(ctx, tenantID, rel.ProjectID)
	if err != nil {
		return nil, err
	}
	blocks, err := s.DefectBlocks(ctx, tenantID, rel.ProjectID, stale.Settings())
	if err != nil {
		return nil, err
	}
	// latest result per test and configuration in the release's runs
	rows, err := s.c(ctx).query(ctx, `SELECT e.test_case_id, t.key, t.title, e.configuration_id, e.status, e.finished_at, e.started_at, r.created_at, r.variant_id FROM test_executions e
		JOIN test_runs r ON r.id = e.run_id JOIN test_cases t ON t.id = e.test_case_id WHERE r.release_id = ? AND r.tenant_id = ?`, id, tenantID)
	if err != nil {
		return nil, err
	}
	type res struct {
		status string
		at     time.Time
	}
	latest := map[string]map[string]res{}
	names := map[string][2]string{}
	for rows.Next() {
		var tid, key, title, st string
		var cfg, runVariant sql.NullString
		var fin, sta sql.NullTime
		var at time.Time
		if err := rows.Scan(&tid, &key, &title, &cfg, &st, &fin, &sta, &at, &runVariant); err != nil {
			rows.Close()
			return nil, err
		}
		if vc != nil && (!vc.AppliesTo(tid, variantID) || !vc.Counts(runVariant.String, cfg.String, variantID)) {
			continue
		}
		if fin.Valid {
			at = fin.Time
		} else if sta.Valid {
			at = sta.Time
		}
		d.Executions++
		if st != model.ResultNotRun && st != model.ResultInProgress {
			d.Executed++
		}
		names[tid] = [2]string{key, title}
		if latest[tid] == nil {
			latest[tid] = map[string]res{}
		}
		if cur, ok := latest[tid][cfg.String]; !ok || (st != model.ResultNotRun && (cur.status == model.ResultNotRun || at.After(cur.at))) {
			latest[tid][cfg.String] = res{st, at}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	units := ConfigResults{}
	for tid, m := range latest {
		units[tid] = map[string]string{}
		for cfg, r := range m {
			units[tid][cfg] = r.status
			if r.status == model.ResultNotRun {
				units[tid][cfg] = ""
			}
			// a stale pass is no evidence (staleness.go)
			if r.status == model.ResultPassed && stale.Check(r.at, tid).Stale {
				units[tid][cfg] = ""
				d.Stale[tid] = true
			}
		}
		// a test counts as passed only when it passed in every configuration
		st := units.Combined(tid)
		d.Results[tid] = st
		if st == model.ResultFailed || st == model.ResultBlocked {
			d.FailedTests = append(d.FailedTests, ReleaseItem{Kind: model.KindTest, ID: tid, Key: names[tid][0], Title: names[tid][1], Note: st})
		}
	}
	sort.Slice(d.FailedTests, func(i, j int) bool { return naturalLess(d.FailedTests[i].Key, d.FailedTests[j].Key) })
	sum := map[string]int{"requirements": len(rel.RequirementIDs), "runs": rel.Runs, "executions": 0, "executed": 0, "covered": 0, "passed": 0, "failed": 0, "not_run": 0,
		"waived": 0, "blocked": 0, "stale": len(d.Stale)}
	waivers, err := s.ActiveWaivers(ctx, tenantID, rel.ProjectID, rel.ID)
	if err != nil {
		return nil, err
	}
	inForce := []model.Waiver{}
	seenWaiver := map[string]bool{}
	d.TestRequirements = map[string][]string{}
	for _, rid := range rel.RequirementIDs {
		for _, tid := range tests[rid] {
			d.TestRequirements[tid] = append(d.TestRequirements[tid], rid)
		}
	}
	for _, rid := range rel.RequirementIDs {
		r, err := s.GetRequirement(ctx, tenantID, rid)
		if err != nil {
			continue
		}
		var keys []string
		for _, w := range waivers[r.ID] {
			// a waiver limited to variants accepts only in them
			if len(w.VariantIDs) > 0 && (vc == nil || !vc.Variants.Covers(w.VariantIDs, variantID)) {
				continue
			}
			keys = append(keys, w.Key)
			if !seenWaiver[w.ID] {
				seenWaiver[w.ID] = true
				inForce = append(inForce, w)
			}
		}
		rr := ReleaseRequirement{ID: r.ID, Key: r.Key, Title: r.Title, Obligation: r.Obligation, Status: r.Status, Tests: tests[r.ID], Suspect: suspect[r.ID], Blocking: blocks[r.ID], Waiver: strings.Join(keys, ", ")}
		if rr.Waiver != "" {
			sum["waived"]++
		}
		d.Requirements = append(d.Requirements, rr)
		if len(rr.Tests) > 0 {
			sum["covered"]++
		}
		if n, _ := BlockCounts(rr.Blocking); n > 0 {
			sum["blocked"]++
		}
		switch CoverageOf(rr.Tests, units) {
		case model.ResultPassed:
			sum["passed"]++
		case model.ResultFailed:
			sum["failed"]++
		default:
			sum["not_run"]++
		}
	}
	sum["executions"], sum["executed"] = d.Executions, d.Executed
	if defects != nil {
		d.OpenDefects = func(level string) []ReleaseItem { return defects(rel.RequirementIDs, level) }
	}
	if risks != nil {
		d.OpenRisks = func(level string) []ReleaseItem { return risks(rel.RequirementIDs, level) }
	}
	if len(rules) > 0 && rules[0] != nil {
		var scopeTests []string
		seen := map[string]bool{}
		for _, rid := range rel.RequirementIDs {
			for _, tid := range tests[rid] {
				if !seen[tid] {
					seen[tid] = true
					scopeTests = append(scopeTests, tid)
				}
			}
		}
		d.RuleViolations = func(level string) []ReleaseItem {
			return rules[0](rel.ProjectID, rel.RequirementIDs, scopeTests, level)
		}
	}
	projectThreshold, err := s.RiskThreshold(ctx, tenantID, rel.ProjectID)
	if err != nil {
		return nil, err
	}
	projectRisks, err := s.ListRisks(ctx, tenantID, rel.ProjectID)
	if err != nil {
		return nil, err
	}
	d.ResidualRisks = func(threshold string) (open, accepted []ReleaseItem) {
		return residualAbove(projectRisks, scope, EffectiveThreshold(threshold, projectThreshold))
	}
	d.Cycles = func(cycleID string) ([]model.TestCycleTest, bool) {
		var pid string
		if err := s.c(ctx).row(ctx, `SELECT project_id FROM test_cycles WHERE id = ? AND tenant_id = ?`, cycleID, tenantID).Scan(&pid); err != nil || pid != rel.ProjectID {
			return nil, false
		}
		plan, err := s.CycleTests(ctx, tenantID, cycleID)
		return plan, err == nil
	}
	releaseThreshold := ""
	for _, c := range rel.ExitCriteria {
		if c.Type == "residual_risk" && c.Level != "" {
			releaseThreshold = c.Level
		}
	}
	rep := &ReleaseStatusReport{Release: rel, Criteria: EvaluateExitCriteria(rel.ExitCriteria, d), Summary: sum, Waivers: inForce,
		RiskThreshold: EffectiveThreshold(releaseThreshold, projectThreshold), ResidualRisks: []ReleaseItem{}, Variant: variantRef}
	open, accepted := residualAbove(projectRisks, scope, rep.RiskThreshold)
	rep.ResidualRisks = append(append(rep.ResidualRisks, open...), accepted...)
	sum["residual_above_threshold"] = len(open)
	rep.Go = true
	for _, c := range rep.Criteria {
		if !c.Met {
			rep.Go = false
		}
	}
	return rep, nil
}

// ReleaseNames returns the names of releases by ID (for run lists).
func (s *Store) ReleaseNames(ctx context.Context, tenantID, projectID string) (map[string]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, name FROM releases WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// ValidateRelease checks and normalises a release's fields without saving it.
func ValidateRelease(r *model.Release) error { return validateRelease(r) }

// ValidDate reports whether s is a YYYY-MM-DD date.
func ValidDate(s string) bool { return dateRe.MatchString(s) }

// residualAbove returns the risks (not closed) linked to scope requirements
// whose residual level is above threshold: those without and those with a
// valid "accept risk" sign-off.
func residualAbove(risks []model.Risk, scope map[string]bool, threshold string) (open, accepted []ReleaseItem) {
	open, accepted = []ReleaseItem{}, []ReleaseItem{}
	for _, k := range risks {
		if k.Status == "closed" || !LevelAbove(k.ResidualLevel, threshold) || !slices.ContainsFunc(k.RequirementIDs, func(id string) bool { return scope[id] }) {
			continue
		}
		it := ReleaseItem{Kind: model.KindRisk, ID: k.ID, Key: k.Key, Title: k.Title, Note: k.ResidualLevel}
		if k.AcceptanceValid {
			it.Note += ", accepted"
			accepted = append(accepted, it)
		} else {
			open = append(open, it)
		}
	}
	return open, accepted
}

// releaseGates evaluates a release that targets variants: for each of them
// (a gate) and combined (see ReleaseStatusForVariant).
func (s *Store) releaseGates(ctx context.Context, tenantID string, rel *model.Release, defects, risks func(scope []string, level string) []ReleaseItem, rules ...func(projectID string, reqs, tests []string, level string) []ReleaseItem) (*ReleaseStatusReport, error) {
	rep, err := s.releaseStatusOne(ctx, tenantID, rel.ID, "", defects, risks, rules...)
	if err != nil {
		return nil, err
	}
	rep.Variants = []VariantGate{}
	seenWaiver := map[string]bool{}
	for _, w := range rep.Waivers {
		seenWaiver[w.ID] = true
	}
	for _, vid := range rel.VariantIDs {
		one, err := s.releaseStatusOne(ctx, tenantID, rel.ID, vid, defects, risks, rules...)
		if err != nil {
			if errors.Is(err, ErrInvalid) {
				continue // a variant deleted meanwhile
			}
			return nil, err
		}
		rep.Variants = append(rep.Variants, VariantGate{Variant: *one.Variant, Go: one.Go, Criteria: one.Criteria, Summary: one.Summary})
		for _, w := range one.Waivers {
			if !seenWaiver[w.ID] {
				seenWaiver[w.ID] = true
				rep.Waivers = append(rep.Waivers, w)
			}
		}
	}
	if len(rep.Variants) == 0 {
		return rep, nil
	}
	rep.Criteria = CombineGates(rep.Variants)
	rep.Go = true
	goCount := 0
	for _, g := range rep.Variants {
		if g.Go {
			goCount++
		} else {
			rep.Go = false
		}
	}
	rep.Summary["variants"], rep.Summary["variants_go"] = len(rep.Variants), goCount
	return rep, nil
}

// CombineGates combines the criteria of the gates of the targeted variants
// (all evaluated on the same exit criteria): a criterion is met when it is
// met in every variant; Actual is the worst variant's (the lowest percentage,
// the highest count); Items, Waived and Accepted are those of every variant,
// each with the variants it appears in.
func CombineGates(gates []VariantGate) []CriterionResult {
	out := []CriterionResult{}
	if len(gates) == 0 {
		return out
	}
	merge := func(dst []ReleaseItem, src []ReleaseItem, variant string) []ReleaseItem {
		for _, it := range src {
			i := slices.IndexFunc(dst, func(x ReleaseItem) bool { return x.Kind == it.Kind && x.ID == it.ID })
			if i < 0 {
				it.Variants = []string{variant}
				dst = append(dst, it)
			} else if !slices.Contains(dst[i].Variants, variant) {
				dst[i].Variants = append(dst[i].Variants, variant)
			}
		}
		return dst
	}
	for i := range gates[0].Criteria {
		c := CriterionResult{ExitCriterion: gates[0].Criteria[i].ExitCriterion, Met: true, Items: []ReleaseItem{}, Waived: []ReleaseItem{}}
		first := true
		var accepted []ReleaseItem
		for _, g := range gates {
			if i >= len(g.Criteria) {
				continue
			}
			v := g.Criteria[i]
			c.Met = c.Met && v.Met
			c.Available = c.Available || v.Available
			worse := v.Actual > c.Actual
			if percentCriterion(c.Type) {
				worse = v.Actual < c.Actual
			}
			if v.Available && (first || worse) {
				c.Actual, c.Total, first = v.Actual, v.Total, false
			}
			c.Items = merge(c.Items, v.Items, g.Variant.Name)
			c.Waived = merge(c.Waived, v.Waived, g.Variant.Name)
			accepted = merge(accepted, v.Accepted, g.Variant.Name)
		}
		c.Items, c.Waived = limitItems(c.Items), limitItems(c.Waived)
		if len(accepted) > 0 {
			c.Accepted = accepted
		}
		c.State = criterionState(&c)
		out = append(out, c)
	}
	return out
}
