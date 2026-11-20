package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Limits of configurations.
const (
	MaxConfigurationName   = 200
	MaxConfigurationValues = 20
	MaxDimensionChars      = 100
	// MaxRunConfigurations bounds the configurations one run targets.
	MaxRunConfigurations = 50
)

const configCols = `c.id, c.project_id, c.name, c.description, c.dimension_values, c.position, c.created_by, c.created_at, c.updated_at,
	(SELECT COUNT(*) FROM run_configurations rc WHERE rc.configuration_id = c.id)`

func scanConfiguration(r interface{ Scan(...any) error }) (*model.Configuration, error) {
	var c model.Configuration
	var vals string
	if err := r.Scan(&c.ID, &c.ProjectID, &c.Name, &c.Description, &vals, &c.Position, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.RunCount); err != nil {
		return nil, err
	}
	c.Values = []model.DimensionValue{}
	_ = json.Unmarshal([]byte(vals), &c.Values)
	if c.Values == nil {
		c.Values = []model.DimensionValue{}
	}
	return &c, nil
}

// ValidateConfiguration normalises and checks a configuration. Without a
// name, the name is made of its values ("Chrome / Windows").
func ValidateConfiguration(c *model.Configuration) error {
	c.Name = strings.Join(strings.Fields(c.Name), " ")
	c.Description = strings.TrimSpace(c.Description)
	if c.Values == nil {
		c.Values = []model.DimensionValue{}
	}
	if len(c.Values) > MaxConfigurationValues {
		return Invalidf("a configuration can have at most %d dimension values", MaxConfigurationValues)
	}
	seen := map[string]bool{}
	var names []string
	for i := range c.Values {
		v := &c.Values[i]
		v.Dimension = strings.Join(strings.Fields(v.Dimension), " ")
		v.Value = strings.Join(strings.Fields(v.Value), " ")
		if v.Dimension == "" || v.Value == "" {
			return Invalidf("dimension and value are required")
		}
		if utf8.RuneCountInString(v.Dimension) > MaxDimensionChars || utf8.RuneCountInString(v.Value) > MaxDimensionChars {
			return Invalidf("dimensions and values have at most %d characters", MaxDimensionChars)
		}
		k := strings.ToLower(v.Dimension)
		if seen[k] {
			return Invalidf("dimension %q is given twice", v.Dimension)
		}
		seen[k] = true
		names = append(names, v.Value)
	}
	if c.Name == "" {
		c.Name = strings.Join(names, " / ")
	}
	if c.Name == "" {
		return Invalidf("configuration name is required")
	}
	if utf8.RuneCountInString(c.Name) > MaxConfigurationName {
		return Invalidf("configuration name is longer than %d characters", MaxConfigurationName)
	}
	if len(c.Description) > 2000 {
		return Invalidf("description is too long")
	}
	return nil
}

// ListConfigurations returns the configurations of a project in order.
func (s *Store) ListConfigurations(ctx context.Context, tenantID, projectID string) ([]model.Configuration, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+configCols+` FROM configurations c WHERE c.tenant_id = ? AND c.project_id = ? ORDER BY c.position, c.name`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Configuration{}
	for rows.Next() {
		c, err := scanConfiguration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetConfiguration returns one configuration.
func (s *Store) GetConfiguration(ctx context.Context, tenantID, id string) (*model.Configuration, error) {
	c, err := scanConfiguration(s.c(ctx).row(ctx, `SELECT `+configCols+` FROM configurations c WHERE c.id = ? AND c.tenant_id = ?`, id, tenantID))
	return c, mapErr(err, "configuration")
}

// FindConfiguration finds a configuration of a project by id or by name
// (ignoring case).
func (s *Store) FindConfiguration(ctx context.Context, tenantID, projectID, ref string) (*model.Configuration, error) {
	ref = strings.TrimSpace(ref)
	list, err := s.ListConfigurations(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == ref {
			return &list[i], nil
		}
	}
	for i := range list {
		if strings.EqualFold(list[i].Name, ref) {
			return &list[i], nil
		}
	}
	return nil, Invalidf("configuration %q not found in project", ref)
}

// CreateConfiguration adds a configuration at the end of the project's list.
func (s *Store) CreateConfiguration(ctx context.Context, tenantID string, c *model.Configuration) error {
	if err := ValidateConfiguration(c); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, c.ProjectID); err != nil {
		return err
	}
	c.ID = newID()
	c.CreatedAt = s.now()
	c.UpdatedAt = c.CreatedAt
	return s.tx(ctx, func(cn conn) error {
		var pos sql.NullInt64
		if err := cn.row(ctx, `SELECT MAX(position) FROM configurations WHERE project_id = ?`, c.ProjectID).Scan(&pos); err != nil {
			return err
		}
		c.Position = 0
		if pos.Valid {
			c.Position = int(pos.Int64) + 1
		}
		if err := checkConfigName(ctx, cn, c.ProjectID, c.ID, c.Name); err != nil {
			return err
		}
		_, err := cn.exec(ctx, `INSERT INTO configurations (id, tenant_id, project_id, name, description, dimension_values, position, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, tenantID, c.ProjectID, c.Name, c.Description, encodeJSON(c.Values), c.Position, c.CreatedBy, c.CreatedAt, c.UpdatedAt)
		return mapErr(err, "configuration")
	})
}

// checkConfigName rejects a name another configuration of the project
// already has (ignoring case).
func checkConfigName(ctx context.Context, c conn, projectID, id, name string) error {
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM configurations WHERE project_id = ? AND id <> ? AND LOWER(name) = LOWER(?)`, projectID, id, name).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return Conflictf("a configuration named %q already exists", name)
	}
	return nil
}

// UpdateConfiguration stores name, description, values and position.
func (s *Store) UpdateConfiguration(ctx context.Context, tenantID string, c *model.Configuration) error {
	if err := ValidateConfiguration(c); err != nil {
		return err
	}
	c.UpdatedAt = s.now()
	return s.tx(ctx, func(cn conn) error {
		if err := checkConfigName(ctx, cn, c.ProjectID, c.ID, c.Name); err != nil {
			return err
		}
		res, err := cn.exec(ctx, `UPDATE configurations SET name = ?, description = ?, dimension_values = ?, position = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			c.Name, c.Description, encodeJSON(c.Values), c.Position, c.UpdatedAt, c.ID, tenantID)
		if err != nil {
			return mapErr(err, "configuration")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "configuration")
		}
		return nil
	})
}

// DeleteConfiguration removes a configuration that no run targets.
func (s *Store) DeleteConfiguration(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		var runs, execs int
		if err := c.row(ctx, `SELECT COUNT(*) FROM run_configurations WHERE configuration_id = ?`, id).Scan(&runs); err != nil {
			return err
		}
		if err := c.row(ctx, `SELECT COUNT(*) FROM test_executions WHERE configuration_id = ?`, id).Scan(&execs); err != nil {
			return err
		}
		if runs > 0 || execs > 0 {
			return Conflictf("the configuration is used by %d run(s); remove it from those runs (or delete them) first", max(runs, 1))
		}
		res, err := c.exec(ctx, `DELETE FROM configurations WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "configuration")
		}
		return nil
	})
}

// runConfigurations returns the configurations a run targets, in order.
func runConfigurations(ctx context.Context, c conn, runID string) ([]model.ConfigurationRef, error) {
	rows, err := c.query(ctx, `SELECT c.id, c.name FROM run_configurations rc JOIN configurations c ON c.id = rc.configuration_id WHERE rc.run_id = ? ORDER BY rc.position, c.position`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ConfigurationRef{}
	for rows.Next() {
		var r model.ConfigurationRef
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// runConfigurationsOf returns run id → configurations for the runs of a project.
func (s *Store) runConfigurationsOf(ctx context.Context, projectID string) (map[string][]model.ConfigurationRef, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT rc.run_id, c.id, c.name FROM run_configurations rc JOIN configurations c ON c.id = rc.configuration_id
		WHERE c.project_id = ? ORDER BY rc.run_id, rc.position, c.position`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]model.ConfigurationRef{}
	for rows.Next() {
		var run string
		var r model.ConfigurationRef
		if err := rows.Scan(&run, &r.ID, &r.Name); err != nil {
			return nil, err
		}
		out[run] = append(out[run], r)
	}
	return out, rows.Err()
}

// checkRunConfigurations verifies that ids are configurations of the project.
func checkRunConfigurations(ctx context.Context, c conn, tenantID, projectID string, ids []string) error {
	if len(ids) > MaxRunConfigurations {
		return Invalidf("a run can target at most %d configurations", MaxRunConfigurations)
	}
	for _, id := range ids {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM configurations WHERE id = ? AND tenant_id = ? AND project_id = ?`, id, tenantID, projectID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return Invalidf("configuration %s not found in project", id)
		}
	}
	return nil
}

// RunConfigChange is the outcome of SetRunConfigurations.
type RunConfigChange struct {
	// Added / Removed executions.
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// SetRunConfigurations makes a run target exactly the configurations ids
// (in this order):
//   - for an added configuration every test of the run gets an execution;
//     when the run had no configuration, its executions are assigned to
//     the first added configuration;
//   - the executions of a removed configuration are deleted, which is
//     refused while one of them has a result; removing the last
//     configuration keeps its executions as executions without
//     configuration.
func (s *Store) SetRunConfigurations(ctx context.Context, tenantID, runID string, ids []string) (*RunConfigChange, error) {
	run, err := s.GetRun(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	ids = uniq(ids)
	out := &RunConfigChange{}
	err = s.tx(ctx, func(c conn) error {
		if err := checkRunConfigurations(ctx, c, tenantID, run.ProjectID, ids); err != nil {
			return err
		}
		cur, err := runConfigurations(ctx, c, runID)
		if err != nil {
			return err
		}
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		had := map[string]bool{}
		var removed []string
		for _, r := range cur {
			had[r.ID] = true
			if !want[r.ID] {
				removed = append(removed, r.ID)
			}
		}
		var added []string
		for _, id := range ids {
			if !had[id] {
				added = append(added, id)
			}
		}
		// executions of removed configurations
		for i, cfg := range removed {
			if len(ids) == 0 && i == 0 {
				// the run keeps one set of executions, now without configuration
				if _, err := c.exec(ctx, `UPDATE test_executions SET configuration_id = NULL WHERE run_id = ? AND configuration_id = ?`, runID, cfg); err != nil {
					return err
				}
				continue
			}
			var withResult int
			if err := c.row(ctx, `SELECT COUNT(*) FROM test_executions WHERE run_id = ? AND configuration_id = ? AND status <> 'not_run'`, runID, cfg).Scan(&withResult); err != nil {
				return err
			}
			if withResult > 0 {
				return Conflictf("%d execution(s) of the configuration already have a result; set them back to not run or delete them first", withResult)
			}
			rows, err := c.query(ctx, `SELECT id FROM test_executions WHERE run_id = ? AND configuration_id = ?`, runID, cfg)
			if err != nil {
				return err
			}
			execs, err := scanStrings(rows)
			if err != nil {
				return err
			}
			if len(execs) > 0 {
				if err := removeExecutionAttachments(ctx, c, execs); err != nil {
					return err
				}
				if err := deleteDefectLinks(ctx, c, "execution", execs); err != nil {
					return err
				}
				res, err := c.exec(ctx, `DELETE FROM test_executions WHERE id IN (`+placeholders(len(execs))+`)`, anySlice(execs)...)
				if err != nil {
					return err
				}
				n, _ := res.RowsAffected()
				out.Removed += int(n)
			}
		}
		if _, err := c.exec(ctx, `DELETE FROM run_configurations WHERE run_id = ?`, runID); err != nil {
			return err
		}
		for i, id := range ids {
			if _, err := c.exec(ctx, `INSERT INTO run_configurations (run_id, configuration_id, position) VALUES (?, ?, ?)`, runID, id, i); err != nil {
				return err
			}
		}
		if len(cur) == 0 && len(added) > 0 {
			// executions without configuration become those of the first one
			if _, err := c.exec(ctx, `UPDATE test_executions SET configuration_id = ? WHERE run_id = ? AND configuration_id IS NULL`, added[0], runID); err != nil {
				return err
			}
		}
		if len(added) > 0 {
			tests, err := runTests(ctx, c, runID)
			if err != nil {
				return err
			}
			n, err := s.addExecutions(ctx, c, tenantID, run.ProjectID, runID, tests)
			if err != nil {
				return err
			}
			out.Added = n
		}
		return syncRunStatus(ctx, c, runID, s.now(), out.Added > 0)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// runTests lists the distinct tests of a run in execution order.
func runTests(ctx context.Context, c conn, runID string) ([]string, error) {
	rows, err := c.query(ctx, `SELECT test_case_id, MIN(position) FROM test_executions WHERE run_id = ? GROUP BY test_case_id ORDER BY MIN(position)`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		var pos int
		if err := rows.Scan(&id, &pos); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- coverage per configuration

// NoConfiguration is the key of results recorded without a configuration.
const NoConfiguration = ""

// ConfigResults holds, per test, the latest result in every configuration
// the test was executed (or planned) in: test id → configuration id
// (NoConfiguration for executions without one) → latest result other than
// not_run, or "" when the test is only planned in that configuration.
type ConfigResults map[string]map[string]string

type unitResult struct {
	status string
	at     time.Time
}

// configResults collects ConfigResults from rows of (test id,
// configuration id, status, finished_at, started_at, run created_at).
func configResults(rows *sql.Rows) (ConfigResults, error) {
	defer rows.Close()
	best := map[string]map[string]unitResult{}
	for rows.Next() {
		var tid, st string
		var cfg sql.NullString
		var fin, start sql.NullTime
		var created time.Time
		if err := rows.Scan(&tid, &cfg, &st, &fin, &start, &created); err != nil {
			return nil, err
		}
		at := created
		if start.Valid {
			at = start.Time
		}
		if fin.Valid {
			at = fin.Time
		}
		if best[tid] == nil {
			best[tid] = map[string]unitResult{}
		}
		cur, ok := best[tid][cfg.String]
		switch {
		case st == model.ResultNotRun:
			if !ok {
				best[tid][cfg.String] = unitResult{}
			}
		case cur.status == "" || at.After(cur.at):
			best[tid][cfg.String] = unitResult{st, at}
		}
	}
	out := ConfigResults{}
	for tid, m := range best {
		out[tid] = map[string]string{}
		for cfg, r := range m {
			out[tid][cfg] = r.status
		}
	}
	return out, rows.Err()
}

// LastConfigResults returns the latest result of every test of a project
// per configuration (see ConfigResults).
func (s *Store) LastConfigResults(ctx context.Context, tenantID, projectID string) (ConfigResults, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT e.test_case_id, e.configuration_id, e.status, e.finished_at, e.started_at, r.created_at
		FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.project_id = ? AND e.tenant_id = ?`, projectID, tenantID)
	if err != nil {
		return nil, err
	}
	return configResults(rows)
}

// Units returns the configurations of a test with their latest result; a
// test that was never planned anywhere has one unit without configuration
// and without result.
func (r ConfigResults) Units(testID string) map[string]string {
	if m := r[testID]; len(m) > 0 {
		return m
	}
	return map[string]string{NoConfiguration: ""}
}

// Combined is the result of a test over all its configurations: failed or
// blocked when it failed or is blocked in one, passed when it passed in
// every configuration it is planned in, else the latest state without
// final verdict (in_progress, skipped or not_run).
func (r ConfigResults) Combined(testID string) string {
	units := r.Units(testID)
	passed, failed, blocked := 0, 0, 0
	other := model.ResultNotRun
	for _, st := range units {
		switch st {
		case model.ResultPassed:
			passed++
		case model.ResultFailed:
			failed++
		case model.ResultBlocked:
			blocked++
		case model.ResultInProgress:
			other = st
		case model.ResultSkipped:
			if other != model.ResultInProgress {
				other = st
			}
		}
	}
	switch {
	case failed > 0:
		return model.ResultFailed
	case blocked > 0:
		return model.ResultBlocked
	case passed == len(units):
		return model.ResultPassed
	case passed > 0:
		return model.ResultInProgress
	}
	return other
}

// CoverageOf computes the coverage of a requirement verified by the linked
// tests, counting every configuration a test is planned in separately:
// failed when a result in any configuration failed or is blocked, passed
// when every test passed in every one of its configurations, partial when
// some passed, otherwise not_run (none without linked tests). Without
// configurations this is exactly Coverage with the latest results.
func CoverageOf(linked []string, res ConfigResults) string {
	if len(linked) == 0 {
		return "none"
	}
	passed, failed, total := 0, 0, 0
	for _, t := range linked {
		for _, st := range res.Units(t) {
			total++
			switch st {
			case model.ResultPassed:
				passed++
			case model.ResultFailed, model.ResultBlocked:
				failed++
			}
		}
	}
	switch {
	case failed > 0:
		return "failed"
	case passed == total:
		return "passed"
	case passed > 0:
		return "partial"
	}
	return "not_run"
}

// CoverageInConfiguration is the coverage of a requirement in one
// configuration (NoConfiguration: results without configuration): like
// Coverage over the linked tests planned in that configuration; tests not
// planned in it do not count. none without linked tests, not_run when no
// linked test is planned in the configuration.
func CoverageInConfiguration(linked []string, res ConfigResults, cfg string) string {
	if len(linked) == 0 {
		return "none"
	}
	var planned []string
	last := map[string]string{}
	for _, t := range linked {
		if st, ok := res[t][cfg]; ok {
			planned = append(planned, t)
			last[t] = st
		}
	}
	if len(planned) == 0 {
		return "not_run"
	}
	return Coverage(planned, last)
}

// InConfiguration returns test id → latest result in one configuration
// (NoConfiguration for results without one); tests never planned in it
// are missing.
func (r ConfigResults) InConfiguration(cfg string) map[string]string {
	out := map[string]string{}
	for tid, m := range r {
		if st, ok := m[cfg]; ok && st != "" {
			out[tid] = st
		}
	}
	return out
}
