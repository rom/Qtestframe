package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- settings

// Limits of the web test settings.
const (
	MaxTestVariables    = 100
	MaxTestVariableSize = 4096
	MaxEnvironmentURLs  = 50
)

var variableNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// ValidBaseURL reports whether u is an absolute http(s) URL with a host.
func ValidBaseURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "http" || p.Scheme == "https") && p.Host != "" && !strings.ContainsAny(u, " \t\r\n")
}

// WebTestSettings returns a project's web test settings, secret values
// included (callers must not return them; see RedactWebTestSettings).
func (s *Store) WebTestSettings(ctx context.Context, tenantID, projectID string) (*model.WebTestSettings, error) {
	var raw string
	if err := s.c(ctx).row(ctx, `SELECT web_tests FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&raw); err != nil {
		return nil, mapErr(err, "project")
	}
	var w model.WebTestSettings
	_ = json.Unmarshal([]byte(raw), &w)
	if w.EnvironmentURLs == nil {
		w.EnvironmentURLs = []model.EnvironmentURL{}
	}
	if w.Variables == nil {
		w.Variables = []model.TestVariable{}
	}
	for i := range w.Variables {
		v := &w.Variables[i]
		if v.Secret {
			var err error
			if v.Value, err = s.unseal(ctx, v.Value, adWebVar(projectID, v.Name)); err != nil {
				return nil, fmt.Errorf("web test variable %s: %w", v.Name, err)
			}
		}
		v.HasValue = v.Value != ""
	}
	return &w, nil
}

// RedactWebTestSettings returns a copy without the values of secret
// variables.
func RedactWebTestSettings(w *model.WebTestSettings) *model.WebTestSettings {
	out := *w
	out.EnvironmentURLs = append([]model.EnvironmentURL{}, w.EnvironmentURLs...)
	out.Variables = make([]model.TestVariable, len(w.Variables))
	for i, v := range w.Variables {
		v.HasValue = v.Value != ""
		if v.Secret {
			v.Value = ""
		}
		out.Variables[i] = v
	}
	return &out
}

// SaveWebTestSettings validates and stores web test settings. A secret
// variable sent without a value keeps the stored value of the variable
// of the same name (secrets are write-only).
func (s *Store) SaveWebTestSettings(ctx context.Context, tenantID, projectID string, w *model.WebTestSettings) error {
	cur, err := s.WebTestSettings(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	w.BaseURL = strings.TrimSpace(w.BaseURL)
	if w.BaseURL != "" && !ValidBaseURL(w.BaseURL) {
		return Invalidf("base_url must be an http(s) URL, e.g. https://staging.example.com")
	}
	if len(w.EnvironmentURLs) > MaxEnvironmentURLs {
		return Invalidf("at most %d environment URLs", MaxEnvironmentURLs)
	}
	envs := map[string]bool{}
	clean := []model.EnvironmentURL{}
	for _, e := range w.EnvironmentURLs {
		e.Environment, e.BaseURL = strings.TrimSpace(e.Environment), strings.TrimSpace(e.BaseURL)
		if e.Environment == "" && e.BaseURL == "" {
			continue
		}
		if e.Environment == "" || len(e.Environment) > 200 {
			return Invalidf("every environment URL needs an environment name (at most 200 characters)")
		}
		if !ValidBaseURL(e.BaseURL) {
			return Invalidf("the URL of environment %q must be an http(s) URL", e.Environment)
		}
		k := strings.ToLower(e.Environment)
		if envs[k] {
			return Invalidf("environment %q is listed twice", e.Environment)
		}
		envs[k] = true
		clean = append(clean, e)
	}
	w.EnvironmentURLs = clean
	if len(w.Variables) > MaxTestVariables {
		return Invalidf("at most %d variables", MaxTestVariables)
	}
	old := map[string]model.TestVariable{}
	for _, v := range cur.Variables {
		old[strings.ToUpper(v.Name)] = v
	}
	names := map[string]bool{}
	vars := []model.TestVariable{}
	for _, v := range w.Variables {
		v.Name = strings.TrimSpace(v.Name)
		if !variableNameRe.MatchString(v.Name) {
			return Invalidf("variable name %q must start with a letter or _ and contain only letters, digits and _ (at most 64)", v.Name)
		}
		k := strings.ToUpper(v.Name)
		if names[k] {
			return Invalidf("variable %s is defined twice (names are case-insensitive)", v.Name)
		}
		names[k] = true
		if len(v.Value) > MaxTestVariableSize {
			return Invalidf("the value of %s is longer than %d bytes", v.Name, MaxTestVariableSize)
		}
		if strings.ContainsRune(v.Value, 0) {
			return Invalidf("the value of %s contains a NUL character", v.Name)
		}
		if v.Secret && v.Value == "" {
			if o, ok := old[k]; ok && o.Secret {
				v.Value = o.Value
			}
		}
		v.HasValue = false
		vars = append(vars, v)
	}
	w.Variables = vars
	stored := *w
	stored.Variables = make([]model.TestVariable, len(vars))
	for i, v := range vars {
		if v.Secret {
			if v.Value, err = s.seal(v.Value, adWebVar(projectID, v.Name)); err != nil {
				return err
			}
		}
		stored.Variables[i] = v
	}
	raw, _ := json.Marshal(stored)
	_, err = s.c(ctx).exec(ctx, `UPDATE projects SET web_tests = ? WHERE id = ? AND tenant_id = ?`, string(raw), projectID, tenantID)
	for i := range w.Variables {
		w.Variables[i].HasValue = w.Variables[i].Value != ""
	}
	return err
}

// ---------------------------------------------------------------- jobs

const jobCols = `j.id, j.tenant_id, j.project_id, j.run_id, j.execution_id, j.batch_id, j.seq, j.state, j.after_jobs, j.previous, j.requested_by, j.user_id, j.instance, j.message,
	j.created_at, j.started_at, j.finished_at, t.key, t.title, COALESCE(cf.name, '')`

const jobFrom = ` FROM script_jobs j JOIN test_executions e ON e.id = j.execution_id JOIN test_cases t ON t.id = e.test_case_id LEFT JOIN configurations cf ON cf.id = e.configuration_id`

func scanJob(r interface{ Scan(...any) error }) (*model.ScriptJob, error) {
	var j model.ScriptJob
	var after, prev string
	var st, ft sql.NullTime
	if err := r.Scan(&j.ID, &j.TenantID, &j.ProjectID, &j.RunID, &j.ExecutionID, &j.BatchID, &j.Seq, &j.State, &after, &prev, &j.RequestedBy, &j.UserID, &j.Instance, &j.Message,
		&j.CreatedAt, &st, &ft, &j.TestKey, &j.TestTitle, &j.ConfigurationName); err != nil {
		return nil, err
	}
	j.StartedAt, j.FinishedAt = timePtr(st), timePtr(ft)
	j.After = []string{}
	_ = json.Unmarshal([]byte(after), &j.After)
	_ = json.Unmarshal([]byte(prev), &j.Previous)
	return &j, nil
}

func (s *Store) listJobs(ctx context.Context, q string, args ...any) ([]model.ScriptJob, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+jobCols+jobFrom+` `+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ScriptJob{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// StateOf returns the result part of an execution.
func StateOf(e *model.Execution) model.ExecutionState {
	return model.ExecutionState{Status: e.Status, ExecutedBy: e.ExecutedBy, StartedAt: e.StartedAt, FinishedAt: e.FinishedAt, DurationMS: e.DurationMS}
}

// CreateScriptJobs stores queued jobs (ID, BatchID, Seq and CreatedAt are
// set here unless given). An execution with a job that is still queued or
// running cannot get another one.
func (s *Store) CreateScriptJobs(ctx context.Context, tenantID string, jobs []*model.ScriptJob) error {
	now := s.now()
	return s.tx(ctx, func(c conn) error {
		for _, j := range jobs {
			var n int
			if err := c.row(ctx, `SELECT COUNT(*) FROM script_jobs WHERE execution_id = ? AND state IN ('queued', 'running')`, j.ExecutionID).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return Conflictf("the execution already has a queued or running job")
			}
			if j.ID == "" {
				j.ID = newID()
			}
			if j.BatchID == "" {
				j.BatchID = j.ID
			}
			if j.After == nil {
				j.After = []string{}
			}
			j.State, j.CreatedAt = model.JobQueued, now
			after, _ := json.Marshal(j.After)
			prev, _ := json.Marshal(j.Previous)
			if _, err := c.exec(ctx, `INSERT INTO script_jobs (id, tenant_id, project_id, run_id, execution_id, batch_id, seq, state, after_jobs, previous, requested_by, user_id, instance, message, created_at, heartbeat_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
				j.ID, tenantID, j.ProjectID, j.RunID, j.ExecutionID, j.BatchID, j.Seq, j.State, string(after), string(prev), j.RequestedBy, j.UserID, j.Instance, now, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// LatestExecutionJob returns the newest job of an execution (nil if none).
func (s *Store) LatestExecutionJob(ctx context.Context, tenantID, executionID string) (*model.ScriptJob, error) {
	list, err := s.listJobs(ctx, `WHERE j.execution_id = ? AND j.tenant_id = ? ORDER BY j.created_at DESC, j.seq DESC LIMIT 1`, executionID, tenantID)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// ListRunJobs returns the jobs of a run, newest batch first (at most limit).
func (s *Store) ListRunJobs(ctx context.Context, tenantID, runID string, limit int) ([]model.ScriptJob, error) {
	return s.listJobs(ctx, `WHERE j.run_id = ? AND j.tenant_id = ? ORDER BY j.created_at DESC, j.batch_id, j.seq LIMIT ?`, runID, tenantID, limit)
}

// InstanceJobs returns the queued and running jobs of a server instance
// (all tenants), in queue order.
func (s *Store) InstanceJobs(ctx context.Context, instance string) ([]model.ScriptJob, error) {
	return s.listJobs(ctx, `WHERE j.instance = ? AND j.state IN ('queued', 'running') ORDER BY j.created_at, j.seq`, instance)
}

// UnfinishedJobs returns queued and running jobs of other instances whose
// heartbeat is older than before (all of them when before is zero).
func (s *Store) UnfinishedJobs(ctx context.Context, instance string, before time.Time) ([]model.ScriptJob, error) {
	if before.IsZero() {
		return s.listJobs(ctx, `WHERE j.instance <> ? AND j.state IN ('queued', 'running') ORDER BY j.created_at`, instance)
	}
	return s.listJobs(ctx, `WHERE j.instance <> ? AND j.state IN ('queued', 'running') AND (j.heartbeat_at IS NULL OR j.heartbeat_at < ?) ORDER BY j.created_at`, instance, before.UTC())
}

// JobStates returns the states of jobs by id (unknown ids are left out).
func (s *Store) JobStates(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id, state FROM script_jobs WHERE id IN (`+placeholders(len(ids))+`)`, anySlice(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

// ClaimJob marks a queued job running; false if it is no longer queued.
func (s *Store) ClaimJob(ctx context.Context, id string) (bool, error) {
	now := s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE script_jobs SET state = 'running', started_at = ?, heartbeat_at = ? WHERE id = ? AND state = 'queued'`, now, now, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// FinishJob ends a queued or running job; false if it had already ended.
func (s *Store) FinishJob(ctx context.Context, id, state, message string) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE script_jobs SET state = ?, message = ?, finished_at = ? WHERE id = ? AND state IN ('queued', 'running')`,
		state, truncate(message, 4000), s.now(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// HeartbeatJobs records that an instance is still working on its jobs.
func (s *Store) HeartbeatJobs(ctx context.Context, instance string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE script_jobs SET heartbeat_at = ? WHERE instance = ? AND state IN ('queued', 'running')`, s.now(), instance)
	return err
}

// RestoreExecution gives an execution that is still in progress its
// earlier result back (a job was cancelled, skipped or interrupted) and
// appends note to its output. It reports whether the execution changed.
func (s *Store) RestoreExecution(ctx context.Context, tenantID, executionID string, prev model.ExecutionState, note string) (bool, error) {
	if prev.Status == "" || prev.Status == model.ResultInProgress {
		prev = model.ExecutionState{Status: model.ResultNotRun}
	}
	changed := false
	err := s.tx(ctx, func(c conn) error {
		var runID, output string
		if err := c.row(ctx, `SELECT run_id, output FROM test_executions WHERE id = ? AND tenant_id = ? AND status = 'in_progress'`, executionID, tenantID).Scan(&runID, &output); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		if note != "" {
			if output != "" && !strings.HasSuffix(output, "\n") {
				output += "\n"
			}
			output += "[qtestframe] " + note + "\n"
		}
		if _, err := c.exec(ctx, `UPDATE test_executions SET status = ?, executed_by = ?, started_at = ?, finished_at = ?, duration_ms = ?, output = ? WHERE id = ?`,
			prev.Status, prev.ExecutedBy, nullTime(prev.StartedAt), nullTime(prev.FinishedAt), prev.DurationMS, output, executionID); err != nil {
			return err
		}
		changed = true
		return syncRunStatus(ctx, c, runID, s.now(), false)
	})
	return changed, err
}
