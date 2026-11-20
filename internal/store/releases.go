package store

import (
	"context"
	"database/sql"
	"encoding/json"
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

const releaseCols = `id, project_id, name, version, description, status, target_date, exit_criteria, released_at, released_by, release_note, created_by, created_at, updated_at`

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func scanRelease(r interface{ Scan(...any) error }) (*model.Release, error) {
	var rel model.Release
	var crit string
	var at sql.NullTime
	if err := r.Scan(&rel.ID, &rel.ProjectID, &rel.Name, &rel.Version, &rel.Description, &rel.Status, &rel.TargetDate, &crit, &at, &rel.ReleasedBy, &rel.ReleaseNote, &rel.CreatedBy, &rel.CreatedAt, &rel.UpdatedAt); err != nil {
		return nil, err
	}
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
	}
	return nil
}

func percentCriterion(t string) bool {
	return t == "coverage" || t == "passed" || t == "executed" || t == "approved"
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
	r.ID = newID()
	r.CreatedAt = s.now()
	r.UpdatedAt = r.CreatedAt
	crit, _ := json.Marshal(r.ExitCriteria)
	_, err := s.c(ctx).exec(ctx, `INSERT INTO releases (id, tenant_id, project_id, name, version, description, status, target_date, exit_criteria, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, tenantID, r.ProjectID, r.Name, r.Version, r.Description, r.Status, r.TargetDate, string(crit), r.CreatedBy, r.CreatedAt, r.UpdatedAt)
	return mapErr(err, "release name")
}

// UpdateRelease saves the editable fields; becoming "released" records
// when and by whom.
func (s *Store) UpdateRelease(ctx context.Context, tenantID string, r *model.Release, user string) error {
	if err := validateRelease(r); err != nil {
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
	res, err := s.c(ctx).exec(ctx, `UPDATE releases SET name = ?, version = ?, description = ?, status = ?, target_date = ?, exit_criteria = ?, released_at = ?, released_by = ?, release_note = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		r.Name, r.Version, r.Description, r.Status, r.TargetDate, string(crit), nullTime(r.ReleasedAt), r.ReleasedBy, r.ReleaseNote, r.UpdatedAt, r.ID, tenantID)
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
	for i := range out {
		out[i].Requirements, out[i].Runs = reqs[out[i].ID], runs[out[i].ID]
	}
	return out, nil
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
}

// ReleaseData is everything the exit criteria are evaluated on.
type ReleaseData struct {
	Requirements []ReleaseRequirement
	// Results: test id → latest result in the release's runs.
	Results map[string]string
	// Executions in the release's runs and how many have a result.
	Executions, Executed int
	// FailedTests: keys of tests whose latest result in the release failed or is blocked.
	FailedTests []ReleaseItem
	// OpenDefects returns the open defects at or above a severity (nil when unknown).
	OpenDefects func(level string) []ReleaseItem
	// OpenRisks returns the unmitigated risks at or above a level (nil when unknown).
	OpenRisks func(level string) []ReleaseItem
}

// ReleaseItem is something a criterion refers to.
type ReleaseItem struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	Note  string `json:"note,omitempty"`
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
}

// ReleaseStatusReport is the go / no-go of a release.
type ReleaseStatusReport struct {
	Release  *model.Release    `json:"release"`
	Go       bool              `json:"go"`
	Criteria []CriterionResult `json:"criteria"`
	Summary  map[string]int    `json:"summary"`
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
				state = model.ResultNotRun
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
		return CriterionResult{ExitCriterion: c, Met: actual >= c.Threshold, Actual: actual, Total: total, Items: limitItems(items), Available: true}
	}
	var out []CriterionResult
	for _, c := range criteria {
		switch c.Type {
		case "coverage":
			ok := 0
			var miss []ReleaseItem
			for _, r := range d.Requirements {
				if len(r.Tests) > 0 {
					ok++
				} else {
					miss = append(miss, reqItem(r, ""))
				}
			}
			out = append(out, pctResult(c, ok, len(d.Requirements), miss))
		case "passed":
			ok := 0
			var miss []ReleaseItem
			for _, r := range d.Requirements {
				if p, state := allPassed(r); p {
					ok++
				} else {
					miss = append(miss, reqItem(r, state))
				}
			}
			out = append(out, pctResult(c, ok, len(d.Requirements), miss))
		case "mandatory_passed":
			total := 0
			var miss []ReleaseItem
			for _, r := range d.Requirements {
				if !mandatory(r.Obligation) {
					continue
				}
				total++
				if p, state := allPassed(r); !p {
					miss = append(miss, reqItem(r, state))
				}
			}
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(miss) == 0, Actual: float64(len(miss)), Total: total, Items: limitItems(miss), Available: true})
		case "executed":
			out = append(out, pctResult(c, d.Executed, d.Executions, nil))
		case "no_failed":
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(d.FailedTests) == 0, Actual: float64(len(d.FailedTests)), Items: limitItems(d.FailedTests), Available: true})
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
		case "no_open_defects":
			if d.OpenDefects == nil {
				out = append(out, CriterionResult{ExitCriterion: c, Met: true})
				continue
			}
			open := d.OpenDefects(c.Level)
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(open) == 0, Actual: float64(len(open)), Items: limitItems(open), Available: true})
		case "risks_mitigated":
			if d.OpenRisks == nil {
				out = append(out, CriterionResult{ExitCriterion: c, Met: true})
				continue
			}
			open := d.OpenRisks(c.Level)
			out = append(out, CriterionResult{ExitCriterion: c, Met: len(open) == 0, Actual: float64(len(open)), Items: limitItems(open), Available: true})
		}
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
// defects and risks of the scope (nil when not available).
func (s *Store) ReleaseStatus(ctx context.Context, tenantID, id string, defects, risks func(scope []string, level string) []ReleaseItem) (*ReleaseStatusReport, error) {
	rel, err := s.GetRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	links, err := s.ListLinks(ctx, tenantID, rel.ProjectID, "", "")
	if err != nil {
		return nil, err
	}
	scope := map[string]bool{}
	for _, rid := range rel.RequirementIDs {
		scope[rid] = true
	}
	tests := map[string][]string{}
	suspect := map[string]bool{}
	for _, l := range links {
		if scope[l.RequirementID] {
			tests[l.RequirementID] = append(tests[l.RequirementID], l.TestCaseID)
			if l.SuspectSince != nil {
				suspect[l.RequirementID] = true
			}
		}
	}
	d := ReleaseData{Results: map[string]string{}}
	// latest result per test in the release's runs
	rows, err := s.c(ctx).query(ctx, `SELECT e.test_case_id, t.key, t.title, e.status, e.finished_at, e.started_at, r.created_at FROM test_executions e
		JOIN test_runs r ON r.id = e.run_id JOIN test_cases t ON t.id = e.test_case_id WHERE r.release_id = ? AND r.tenant_id = ?`, id, tenantID)
	if err != nil {
		return nil, err
	}
	type res struct {
		key, title, status string
		at                 time.Time
	}
	latest := map[string]res{}
	for rows.Next() {
		var tid, key, title, st string
		var fin, sta sql.NullTime
		var at time.Time
		if err := rows.Scan(&tid, &key, &title, &st, &fin, &sta, &at); err != nil {
			rows.Close()
			return nil, err
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
		if cur, ok := latest[tid]; !ok || (st != model.ResultNotRun && (cur.status == model.ResultNotRun || at.After(cur.at))) {
			latest[tid] = res{key, title, st, at}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for tid, r := range latest {
		d.Results[tid] = r.status
		if r.status == model.ResultFailed || r.status == model.ResultBlocked {
			d.FailedTests = append(d.FailedTests, ReleaseItem{Kind: model.KindTest, ID: tid, Key: r.key, Title: r.title, Note: r.status})
		}
	}
	sort.Slice(d.FailedTests, func(i, j int) bool { return naturalLess(d.FailedTests[i].Key, d.FailedTests[j].Key) })
	sum := map[string]int{"requirements": len(rel.RequirementIDs), "runs": rel.Runs, "executions": 0, "executed": 0, "covered": 0, "passed": 0, "failed": 0, "not_run": 0}
	for _, rid := range rel.RequirementIDs {
		r, err := s.GetRequirement(ctx, tenantID, rid)
		if err != nil {
			continue
		}
		rr := ReleaseRequirement{ID: r.ID, Key: r.Key, Title: r.Title, Obligation: r.Obligation, Status: r.Status, Tests: tests[r.ID], Suspect: suspect[r.ID]}
		d.Requirements = append(d.Requirements, rr)
		if len(rr.Tests) > 0 {
			sum["covered"]++
		}
		switch Coverage(rr.Tests, d.Results) {
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
	rep := &ReleaseStatusReport{Release: rel, Criteria: EvaluateExitCriteria(rel.ExitCriteria, d), Summary: sum}
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
