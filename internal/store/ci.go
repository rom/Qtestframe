package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// CI/CD integration: the quality gate (exit criteria of a release, or
// ad-hoc conditions, on the results of a run or build) and the outbox of
// the commit statuses the server posts to the code host (see docs/api.md
// "CI/CD").

// ---------------------------------------------------------------- gate

// DefaultGateConditions are the conditions of a gate without a release
// and without conditions of its own.
var DefaultGateConditions = []string{"mandatory_passed", "no_failed", "no_stale", "no_blocking_defects"}

// gateAliases are shorter or more familiar names of exit criterion types.
var gateAliases = map[string]string{
	"must":           "mandatory_passed",
	"must_verified":  "mandatory_passed",
	"mandatory":      "mandatory_passed",
	"blocking":       "no_blocking_defects",
	"no_blocking":    "no_blocking_defects",
	"failed":         "no_failed",
	"stale":          "no_stale",
	"defects":        "no_open_defects",
	"suspect":        "no_suspect",
	"cycle":          "cycle_executed",
	"risks":          "risks_mitigated",
	"rules":          "trace_rules",
	"residual":       "residual_risk",
	"passed_percent": "passed",
}

// ParseGateCondition parses one gate condition: TYPE[:ARG][>=N], where
// TYPE is an exit criterion type (or an alias such as must, blocking,
// stale), ARG the level of no_open_defects, risks_mitigated, trace_rules
// and residual_risk or the cycle (id or key) of cycle_executed, and N the
// threshold in percent of the percentage criteria. Examples: coverage>=80,
// no_open_defects:major, cycle_executed:CYC-3>=90, must.
func ParseGateCondition(s string) (model.ExitCriterion, error) {
	var c model.ExitCriterion
	in := strings.TrimSpace(s)
	rest := in
	if i := strings.Index(rest, ">="); i >= 0 {
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(rest[i+2:], "%")), 64)
		if err != nil || v < 0 || v > 100 {
			return c, Invalidf("condition %q: the threshold after >= must be a percentage from 0 to 100", in)
		}
		c.Threshold, rest = v, rest[:i]
	}
	arg := ""
	if i := strings.Index(rest, ":"); i >= 0 {
		rest, arg = rest[:i], strings.TrimSpace(rest[i+1:])
	}
	t := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(rest), "-", "_"))
	if a, ok := gateAliases[t]; ok {
		t = a
	}
	if !slices.Contains(model.ExitCriterionTypes, t) {
		return c, Invalidf("condition %q: unknown type; use one of %s", in, strings.Join(model.ExitCriterionTypes, ", "))
	}
	c.Type = t
	if c.Threshold > 0 && !percentCriterion(t) {
		return c, Invalidf("condition %q: %s takes no threshold", in, t)
	}
	if c.Threshold == 0 && percentCriterion(t) && !strings.Contains(in, ">=") {
		c.Threshold = 100
	}
	switch t {
	case "cycle_executed":
		if arg == "" {
			return c, Invalidf("condition %q: cycle_executed needs a cycle, e.g. cycle_executed:CYC-1", in)
		}
		c.CycleID = arg
	case "no_open_defects":
		if arg != "" && !slices.Contains(model.DefectSeverities, arg) {
			return c, Invalidf("condition %q: the severity must be one of %s", in, strings.Join(model.DefectSeverities, ", "))
		}
		c.Level = arg
	case "risks_mitigated", "residual_risk":
		if arg != "" && !slices.Contains(model.RiskLevels, arg) {
			return c, Invalidf("condition %q: the level must be one of %s", in, strings.Join(model.RiskLevels, ", "))
		}
		c.Level = arg
	case "trace_rules":
		if arg == "" {
			arg = "error"
		}
		if arg != "error" && arg != "warning" {
			return c, Invalidf("condition %q: the level must be error or warning", in)
		}
		c.Level = arg
	default:
		if arg != "" {
			return c, Invalidf("condition %q: %s takes no argument", in, t)
		}
	}
	return c, nil
}

// ParseGateConditions parses a list of conditions (see ParseGateCondition);
// a condition may also be a comma-separated list.
func ParseGateConditions(list []string) ([]model.ExitCriterion, error) {
	out := []model.ExitCriterion{}
	for _, l := range list {
		for _, part := range strings.Split(l, ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			c, err := ParseGateCondition(part)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	if len(out) > 30 {
		return nil, Invalidf("at most 30 gate conditions")
	}
	return out, nil
}

// GateQuery says what a quality gate evaluates.
type GateQuery struct {
	ProjectID string
	// ReleaseID: the release whose scope (and, without Criteria, exit
	// criteria) is checked; "" an ad-hoc gate on RequirementIDs.
	ReleaseID string
	// Variant (id or name): evaluate for this variant only.
	Variant string
	// RunID and BuildID restrict the results to a run or to the runs of a
	// build (with a release: its runs in the build).
	RunID, BuildID string
	// Criteria replace the release's exit criteria (nil: the release's,
	// or DefaultGateConditions without a release).
	Criteria []model.ExitCriterion
	// RequirementIDs: the scope of an ad-hoc gate (nil: every requirement
	// of the project that is not rejected or obsolete).
	RequirementIDs []string
}

// runScope selects the runs whose results a gate evaluates.
type runScope struct {
	projectID, releaseID, runID, buildID string
}

func (rs runScope) where() (string, []any) {
	var conds []string
	var args []any
	add := func(cond, v string) {
		if v != "" {
			conds = append(conds, cond)
			args = append(args, v)
		}
	}
	add("r.project_id = ?", rs.projectID)
	add("r.release_id = ?", rs.releaseID)
	add("r.id = ?", rs.runID)
	add("r.build_id = ?", rs.buildID)
	if len(conds) == 0 {
		return "1 = 0", nil
	}
	return strings.Join(conds, " AND "), args
}

// GateStatus evaluates a quality gate: the exit criteria of a release (or
// the given criteria on its scope), or ad-hoc criteria on the project's
// requirements, on the results of the release's runs, a run or a build.
// Waivers are respected as in the release status (without a release only
// project-wide waivers count). defects, risks and rules are as for
// ReleaseStatus.
func (s *Store) GateStatus(ctx context.Context, tenantID string, q GateQuery, defects, risks func(scope []string, level string) []ReleaseItem, rules ...func(projectID string, reqs, tests []string, level string) []ReleaseItem) (*ReleaseStatusReport, error) {
	var rel *model.Release
	scope := runScope{runID: q.RunID, buildID: q.BuildID}
	if q.ReleaseID != "" {
		r, err := s.GetRelease(ctx, tenantID, q.ReleaseID)
		if err != nil {
			return nil, err
		}
		if r.ProjectID != q.ProjectID {
			return nil, ErrNotFound
		}
		rel = r
		scope.releaseID = r.ID
	} else {
		rel = &model.Release{ProjectID: q.ProjectID, RequirementIDs: q.RequirementIDs, VariantIDs: []string{}}
		if rel.RequirementIDs == nil {
			reqs, err := s.ListRequirements(ctx, tenantID, q.ProjectID)
			if err != nil {
				return nil, err
			}
			rel.RequirementIDs = []string{}
			for _, r := range reqs {
				if r.Status != "rejected" && r.Status != "obsolete" {
					rel.RequirementIDs = append(rel.RequirementIDs, r.ID)
				}
			}
		}
		rel.Requirements = len(rel.RequirementIDs)
		scope.projectID = q.ProjectID
		if q.Criteria == nil {
			q.Criteria, _ = ParseGateConditions(DefaultGateConditions)
		}
	}
	if q.Criteria != nil {
		rel.ExitCriteria = q.Criteria
	}
	if q.RunID != "" || q.BuildID != "" || q.ReleaseID == "" {
		w, args := scope.where()
		if err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM test_runs r WHERE r.tenant_id = ? AND `+w, append([]any{tenantID}, args...)...).Scan(&rel.Runs); err != nil {
			return nil, err
		}
	}
	switch {
	case strings.TrimSpace(q.Variant) != "":
		return s.evaluateRelease(ctx, tenantID, rel, scope, q.Variant, defects, risks, rules...)
	case len(rel.VariantIDs) > 0:
		return s.releaseGates(ctx, tenantID, rel, scope, defects, risks, rules...)
	}
	return s.evaluateRelease(ctx, tenantID, rel, scope, "", defects, risks, rules...)
}

// ---------------------------------------------------------------- deliveries

// CILogSize: finished CI deliveries kept per project.
const CILogSize = 50

const ciDeliveryColumns = `id, project_id, run_id, build_id, commit_sha, merge_request, status, attempts, status_code, error, outcome, created_by, created_at, next_attempt_at, sent_at`

func scanCIDelivery(r interface{ Scan(...any) error }) (*model.CIDelivery, error) {
	var d model.CIDelivery
	var next, sent sql.NullTime
	if err := r.Scan(&d.ID, &d.ProjectID, &d.RunID, &d.BuildID, &d.CommitSHA, &d.MergeRequest, &d.Status, &d.Attempts, &d.StatusCode, &d.Error, &d.Outcome, &d.CreatedBy, &d.CreatedAt, &next, &sent); err != nil {
		return nil, err
	}
	if next.Valid {
		d.NextAttempt = &next.Time
	}
	if sent.Valid {
		d.SentAt = &sent.Time
	}
	return &d, nil
}

// AddCIDelivery queues a post of a run's results at a commit. When a post
// for the same run, commit and merge request is still waiting, none is
// added (it includes the new results when it is sent): the answer is its
// id and false.
func (s *Store) AddCIDelivery(ctx context.Context, tenantID string, d *model.CIDelivery) (string, bool, error) {
	d.CommitSHA = strings.ToLower(strings.TrimSpace(d.CommitSHA))
	if !shaRe.MatchString(d.CommitSHA) {
		return "", false, Invalidf("the commit must be a hexadecimal SHA")
	}
	var id string
	created := false
	err := s.tx(ctx, func(c conn) error {
		err := c.row(ctx, `SELECT id FROM ci_deliveries WHERE project_id = ? AND run_id = ? AND commit_sha = ? AND merge_request = ? AND status IN (?, ?)`,
			d.ProjectID, d.RunID, d.CommitSHA, d.MergeRequest, model.CIQueued, model.CIRetry).Scan(&id)
		if err == nil {
			_, err = c.exec(ctx, `UPDATE ci_deliveries SET build_id = ?, status = ?, next_attempt_at = ? WHERE id = ?`, d.BuildID, model.CIQueued, s.now(), id)
			return err
		}
		if !db.IsNoRows(err) {
			return err
		}
		id, created = newID(), true
		now := s.now()
		_, err = c.exec(ctx, `INSERT INTO ci_deliveries (id, tenant_id, project_id, run_id, build_id, commit_sha, merge_request, status, attempts, status_code, error, outcome, created_by, created_at, next_attempt_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, '', '', ?, ?, ?)`, id, tenantID, d.ProjectID, d.RunID, d.BuildID, d.CommitSHA, d.MergeRequest, model.CIQueued, d.CreatedBy, now, now)
		return err
	})
	return id, created, err
}

// CIJob is a claimed CI delivery.
type CIJob struct {
	TenantID string
	Delivery model.CIDelivery
}

// ClaimCIDelivery takes a due delivery for lease; nil when it is not due
// or another server or goroutine has it.
func (s *Store) ClaimCIDelivery(ctx context.Context, id string, lease time.Duration) (*CIJob, error) {
	now := s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE ci_deliveries SET status = ?, next_attempt_at = ? WHERE id = ? AND status IN (?, ?, ?) AND next_attempt_at <= ?`,
		model.CISending, now.Add(lease), id, model.CIQueued, model.CIRetry, model.CISending, now)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	var j CIJob
	if err := s.c(ctx).row(ctx, `SELECT tenant_id FROM ci_deliveries WHERE id = ?`, id).Scan(&j.TenantID); err != nil {
		return nil, mapErr(err, "CI delivery")
	}
	d, err := scanCIDelivery(s.c(ctx).row(ctx, `SELECT `+ciDeliveryColumns+` FROM ci_deliveries WHERE id = ?`, id))
	if err != nil {
		return nil, mapErr(err, "CI delivery")
	}
	j.Delivery = *d
	return &j, nil
}

// CIOutcome is the result of one attempt.
type CIOutcome struct {
	OK         bool
	StatusCode int
	Error      string
	// Outcome: what was posted, e.g. "qtestframe/tests failed; note updated".
	Outcome string
	// RetryAt: when to try again (nil: the delivery is finished).
	RetryAt *time.Time
}

// FinishCIDelivery records an attempt.
func (s *Store) FinishCIDelivery(ctx context.Context, id string, o CIOutcome) error {
	now := s.now()
	status := model.CISent
	var sent, next any = now, nil
	switch {
	case o.OK:
	case o.RetryAt != nil:
		status, sent, next = model.CIRetry, nil, o.RetryAt.UTC()
	default:
		status, sent = model.CIFailed, nil
	}
	return s.tx(ctx, func(c conn) error {
		var projectID string
		if err := c.row(ctx, `SELECT project_id FROM ci_deliveries WHERE id = ?`, id).Scan(&projectID); err != nil {
			return mapErr(err, "CI delivery")
		}
		if _, err := c.exec(ctx, `UPDATE ci_deliveries SET status = ?, attempts = attempts + 1, status_code = ?, error = ?, outcome = ?, sent_at = ?, next_attempt_at = ? WHERE id = ?`,
			status, o.StatusCode, shortText(o.Error, 500), shortText(o.Outcome, 500), sent, next, id); err != nil {
			return err
		}
		if status == model.CIRetry {
			return nil
		}
		_, err := c.exec(ctx, `DELETE FROM ci_deliveries WHERE project_id = ? AND status IN (?, ?) AND id NOT IN (
			SELECT id FROM ci_deliveries WHERE project_id = ? AND status IN (?, ?) ORDER BY created_at DESC, id DESC LIMIT ?)`,
			projectID, model.CISent, model.CIFailed, projectID, model.CISent, model.CIFailed, CILogSize)
		return err
	})
}

// DueCIDeliveries returns the IDs of the deliveries due now, oldest first.
func (s *Store) DueCIDeliveries(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM ci_deliveries WHERE status IN (?, ?, ?) AND next_attempt_at <= ? ORDER BY next_attempt_at, id LIMIT ?`,
		model.CIQueued, model.CIRetry, model.CISending, s.now(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListCIDeliveries returns a project's deliveries, newest first.
func (s *Store) ListCIDeliveries(ctx context.Context, tenantID, projectID string, limit int) ([]model.CIDelivery, error) {
	if limit <= 0 || limit > CILogSize*2 {
		limit = CILogSize
	}
	rows, err := s.c(ctx).query(ctx, `SELECT `+ciDeliveryColumns+` FROM ci_deliveries WHERE tenant_id = ? AND project_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, tenantID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CIDelivery{}
	for rows.Next() {
		d, err := scanCIDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// FindCommitMergeRequest returns the open merge request (IID) whose source
// branch is branch, from the code changes the webhooks recorded ("" none).
func (s *Store) FindCommitMergeRequest(ctx context.Context, tenantID, projectID, branch string) (string, error) {
	if strings.TrimSpace(branch) == "" {
		return "", nil
	}
	var ref string
	err := s.c(ctx).row(ctx, `SELECT ref FROM code_changes WHERE tenant_id = ? AND project_id = ? AND kind = ? AND provider = ? AND branch = ? AND status = ? ORDER BY updated_at DESC LIMIT 1`,
		tenantID, projectID, model.CodeMergeRequest, model.ProviderGitLab, branch, "open").Scan(&ref)
	if db.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("merge request of branch: %w", err)
	}
	return ref, nil
}

// CodeChangesByRef returns the code changes of a project of a kind whose
// ref is one of refs: merge / pull request numbers exactly, commits by
// their SHA or a prefix of at least 7 characters. Unknown refs are left
// out.
func (s *Store) CodeChangesByRef(ctx context.Context, tenantID, projectID, kind string, refs []string) ([]model.CodeChange, error) {
	out := []model.CodeChange{}
	seen := map[string]bool{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || len(ref) > 64 {
			continue
		}
		q := `SELECT ` + codeChangeColumns + ` FROM code_changes WHERE tenant_id = ? AND project_id = ? AND kind = ? AND `
		args := []any{tenantID, projectID, kind}
		if kind == model.CodeCommit {
			if len(ref) < 7 || !shaRe.MatchString(ref) {
				continue
			}
			q += `LOWER(ref) LIKE ?`
			args = append(args, strings.ToLower(ref)+"%")
		} else {
			q += `ref = ?`
			args = append(args, strings.TrimLeft(ref, "!#"))
		}
		rows, err := s.c(ctx).query(ctx, q+` ORDER BY occurred_at DESC LIMIT 20`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			c, err := scanCodeChange(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, *c)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if err := s.codeChangeItems(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}
