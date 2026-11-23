package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Test environments and their bookings (migration 0057). An environment
// belongs to a project or, with project_id NULL, is shared by the
// organisation; every project of the organisation can select and book a
// shared environment.

// Limits of environments and bookings.
const (
	MaxEnvironmentComponents = 100
	MaxEnvURLs               = 20
	MaxEnvironmentCapacity   = 1000
	// MaxBookingDays bounds the length of one booking.
	MaxBookingDays = 366
)

func adEnvSecret(id string) string { return "environments.secret|" + id }

const envCols = `e.id, COALESCE(e.project_id, ''), e.name, e.type, e.urls, e.description, e.components, e.owner_id, e.status, e.status_note, e.status_by_health,
	e.tags, e.configuration_ids, e.variant_ids, e.capacity, e.requires_approval, e.credentials_ref, e.secret <> '', e.health_url, e.health_interval_minutes,
	e.health_status, e.health_checked_at, e.health_error, e.version, e.created_by, e.created_at, e.updated_at,
	COALESCE((SELECT p.key FROM projects p WHERE p.id = e.project_id), ''),
	(SELECT COUNT(*) FROM test_runs r WHERE r.environment_id = e.id)`

func scanEnvironment(r interface{ Scan(...any) error }) (*model.Environment, error) {
	var e model.Environment
	var urls, comps, tags, cfgs, variants string
	var checked sql.NullTime
	if err := r.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Type, &urls, &e.Description, &comps, &e.OwnerID, &e.Status, &e.StatusNote, &e.StatusByHealth,
		&tags, &cfgs, &variants, &e.Capacity, &e.RequiresApproval, &e.CredentialsRef, &e.HasSecret, &e.HealthURL, &e.HealthIntervalMinutes,
		&e.HealthStatus, &checked, &e.HealthError, &e.Version, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.ProjectKey, &e.RunCount); err != nil {
		return nil, err
	}
	e.Shared = e.ProjectID == ""
	e.URLs, e.Tags, e.ConfigurationIDs, e.VariantIDs = decodeTags(urls), decodeTags(tags), decodeTags(cfgs), decodeTags(variants)
	e.Components = decodeComponents(comps)
	e.HealthCheckedAt = timePtr(checked)
	e.CreatedAt, e.UpdatedAt = e.CreatedAt.UTC(), e.UpdatedAt.UTC()
	return &e, nil
}

func decodeComponents(s string) []model.Component {
	out := []model.Component{}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []model.Component{}
	}
	return out
}

func encodeComponents(c []model.Component) string {
	if c == nil {
		c = []model.Component{}
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// ValidHealthURL reports whether u can be checked: an absolute http or
// https URL.
func ValidHealthURL(u string) bool {
	x, err := url.Parse(u)
	return err == nil && (x.Scheme == "http" || x.Scheme == "https") && x.Host != ""
}

// validateEnvironment checks and normalises the editable fields.
func validateEnvironment(e *model.Environment) error {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || utf8.RuneCountInString(e.Name) > 200 {
		return Invalidf("environment name is required (at most 200 characters)")
	}
	if e.Type == "" {
		e.Type = "test"
	}
	if !slices.Contains(model.EnvironmentTypes, e.Type) {
		return Invalidf("type must be one of %s", strings.Join(model.EnvironmentTypes, ", "))
	}
	if e.Status == "" {
		e.Status = "available"
	}
	if !slices.Contains(model.EnvironmentStatuses, e.Status) {
		return Invalidf("status must be one of %s", strings.Join(model.EnvironmentStatuses, ", "))
	}
	if len(e.Description) > 20000 || len(e.StatusNote) > 2000 || len(e.CredentialsRef) > 2000 || len(e.Secret) > 10000 {
		return Invalidf("description, status note, credentials reference or secret is too long")
	}
	e.URLs = cleanTags(e.URLs)
	if len(e.URLs) > MaxEnvURLs {
		return Invalidf("an environment can have at most %d URLs", MaxEnvURLs)
	}
	for _, u := range e.URLs {
		if len(u) > 2000 {
			return Invalidf("URL too long")
		}
	}
	e.Tags = cleanTags(e.Tags)
	if e.ConfigurationIDs == nil {
		e.ConfigurationIDs = []string{}
	}
	if e.VariantIDs == nil {
		e.VariantIDs = []string{}
	}
	e.ConfigurationIDs, e.VariantIDs = uniq(e.ConfigurationIDs), uniq(e.VariantIDs)
	comps := make([]model.Component, 0, len(e.Components))
	seen := map[string]bool{}
	for _, c := range e.Components {
		c.Name, c.Version = strings.TrimSpace(c.Name), strings.TrimSpace(c.Version)
		if c.Name == "" && c.Version == "" {
			continue
		}
		if c.Name == "" || utf8.RuneCountInString(c.Name) > 100 || utf8.RuneCountInString(c.Version) > 200 {
			return Invalidf("a component needs a name (at most 100 characters) and a version of at most 200 characters")
		}
		if seen[strings.ToLower(c.Name)] {
			return Invalidf("component %q is listed twice", c.Name)
		}
		seen[strings.ToLower(c.Name)] = true
		comps = append(comps, c)
	}
	if len(comps) > MaxEnvironmentComponents {
		return Invalidf("an environment can have at most %d components", MaxEnvironmentComponents)
	}
	e.Components = comps
	if e.Capacity == 0 {
		e.Capacity = 1
	}
	if e.Capacity < 1 || e.Capacity > MaxEnvironmentCapacity {
		return Invalidf("capacity must be 1 to %d", MaxEnvironmentCapacity)
	}
	e.HealthURL = strings.TrimSpace(e.HealthURL)
	if e.HealthURL != "" && !ValidHealthURL(e.HealthURL) {
		return Invalidf("the health check URL must be an http or https URL")
	}
	if e.HealthIntervalMinutes == 0 {
		e.HealthIntervalMinutes = 5
	}
	if e.HealthIntervalMinutes < 1 || e.HealthIntervalMinutes > 1440 {
		return Invalidf("the health check interval must be 1 to 1440 minutes")
	}
	if e.HealthURL == "" {
		e.HealthStatus, e.HealthError, e.HealthCheckedAt = "", "", nil
	}
	return nil
}

func (s *Store) checkEnvironmentRefs(ctx context.Context, c conn, tenantID string, e *model.Environment) error {
	if e.ProjectID != "" {
		if err := checkRunConfigurations(ctx, c, tenantID, e.ProjectID, e.ConfigurationIDs); err != nil {
			return err
		}
		for _, v := range e.VariantIDs {
			id := v
			if err := checkRunVariantBuild(ctx, c, tenantID, e.ProjectID, &id, nil); err != nil {
				return err
			}
		}
	} else if len(e.ConfigurationIDs) > 0 || len(e.VariantIDs) > 0 {
		return Invalidf("a shared environment cannot be linked to the configurations or variants of a project")
	}
	var n int
	q := `SELECT COUNT(*) FROM environments WHERE tenant_id = ? AND project_id = ? AND LOWER(name) = LOWER(?) AND id <> ?`
	args := []any{tenantID, e.ProjectID, e.Name, e.ID}
	if e.ProjectID == "" {
		q = `SELECT COUNT(*) FROM environments WHERE tenant_id = ? AND project_id IS NULL AND LOWER(name) = LOWER(?) AND id <> ?`
		args = []any{tenantID, e.Name, e.ID}
	}
	if err := c.row(ctx, q, args...).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return Conflictf("an environment named %q already exists", e.Name)
	}
	return checkMember(ctx, c, tenantID, e.OwnerID, "owner")
}

func projectArg(projectID string) any {
	if projectID == "" {
		return nil
	}
	return projectID
}

// ListEnvironments returns the environments of a project and, with
// shared, those shared by the organisation (projectID "": only the shared
// ones), by name.
func (s *Store) ListEnvironments(ctx context.Context, tenantID, projectID string, shared bool) ([]model.Environment, error) {
	where := `e.tenant_id = ? AND e.project_id = ?`
	args := []any{tenantID, projectID}
	switch {
	case projectID == "":
		where, args = `e.tenant_id = ? AND e.project_id IS NULL`, []any{tenantID}
	case shared:
		where = `e.tenant_id = ? AND (e.project_id = ? OR e.project_id IS NULL)`
	}
	rows, err := s.c(ctx).query(ctx, `SELECT `+envCols+` FROM environments e WHERE `+where+` ORDER BY LOWER(e.name), e.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Environment{}
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, s.fillBookingState(ctx, out)
}

// fillBookingState sets BookedNow and NextBooking.
func (s *Store) fillBookingState(ctx context.Context, list []model.Environment) error {
	if len(list) == 0 {
		return nil
	}
	now := s.now()
	ids := make([]string, len(list))
	idx := map[string]int{}
	for i, e := range list {
		ids[i] = e.ID
		idx[e.ID] = i
	}
	args := append(anySlice(ids), now)
	rows, err := s.c(ctx).query(ctx, `SELECT environment_id, starts_at, ends_at FROM environment_bookings
		WHERE environment_id IN (`+placeholders(len(ids))+`) AND status IN ('pending', 'approved') AND ends_at > ? ORDER BY starts_at`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var from, to time.Time
		if err := rows.Scan(&id, &from, &to); err != nil {
			return err
		}
		e := &list[idx[id]]
		if !from.After(now) {
			e.BookedNow++
		} else if e.NextBooking == nil {
			t := from.UTC()
			e.NextBooking = &t
		}
	}
	return rows.Err()
}

// GetEnvironment returns an environment (without its secret).
func (s *Store) GetEnvironment(ctx context.Context, tenantID, id string) (*model.Environment, error) {
	e, err := scanEnvironment(s.c(ctx).row(ctx, `SELECT `+envCols+` FROM environments e WHERE e.id = ? AND e.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "environment")
	}
	list := []model.Environment{*e}
	if err := s.fillBookingState(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// FindEnvironment finds an environment a project can use (its own or a
// shared one) by id or name (ignoring case; the project's own first).
func (s *Store) FindEnvironment(ctx context.Context, tenantID, projectID, ref string) (*model.Environment, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, Invalidf("environment is required")
	}
	if e, err := s.GetEnvironment(ctx, tenantID, ref); err == nil && (e.ProjectID == projectID || e.Shared) {
		return e, nil
	}
	var id string
	err := s.c(ctx).row(ctx, `SELECT id FROM environments WHERE tenant_id = ? AND (project_id = ? OR project_id IS NULL) AND LOWER(name) = LOWER(?)
		ORDER BY CASE WHEN project_id IS NULL THEN 1 ELSE 0 END LIMIT 1`, tenantID, projectID, ref).Scan(&id)
	if err != nil {
		if me := mapErr(err, "environment"); errors.Is(me, ErrNotFound) {
			return nil, Invalidf("environment %q not found", ref)
		}
		return nil, err
	}
	return s.GetEnvironment(ctx, tenantID, id)
}

// CreateEnvironment adds an environment (ProjectID "": shared).
func (s *Store) CreateEnvironment(ctx context.Context, tenantID string, e *model.Environment) error {
	if err := validateEnvironment(e); err != nil {
		return err
	}
	if e.ProjectID != "" {
		if _, err := s.GetProject(ctx, tenantID, e.ProjectID); err != nil {
			return err
		}
	}
	e.ID, e.Version = newID(), 1
	e.CreatedAt = s.now()
	e.UpdatedAt = e.CreatedAt
	e.Shared = e.ProjectID == ""
	secret, err := s.seal(e.Secret, adEnvSecret(e.ID))
	if err != nil {
		return err
	}
	e.HasSecret, e.Secret = e.Secret != "", ""
	return s.tx(ctx, func(c conn) error {
		if err := s.checkEnvironmentRefs(ctx, c, tenantID, e); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO environments (id, tenant_id, project_id, name, type, urls, description, components, owner_id, status, status_note, status_by_health,
			tags, configuration_ids, variant_ids, capacity, requires_approval, credentials_ref, secret, health_url, health_interval_minutes, version, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, tenantID, projectArg(e.ProjectID), e.Name, e.Type, encodeList(e.URLs), e.Description, encodeComponents(e.Components), e.OwnerID, e.Status, e.StatusNote, false,
			encodeList(e.Tags), encodeList(e.ConfigurationIDs), encodeList(e.VariantIDs), e.Capacity, e.RequiresApproval, e.CredentialsRef, secret, e.HealthURL, e.HealthIntervalMinutes,
			e.Version, e.CreatedBy, e.CreatedAt, e.UpdatedAt)
		return mapErr(err, "environment name")
	})
}

// UpdateEnvironment saves the editable fields (a new version). The secret
// changes only when Secret is given (or ClearSecret is set). A manual
// status change ends a status set by the health check.
func (s *Store) UpdateEnvironment(ctx context.Context, tenantID string, e *model.Environment) error {
	if err := validateEnvironment(e); err != nil {
		return err
	}
	old, err := s.GetEnvironment(ctx, tenantID, e.ID)
	if err != nil {
		return err
	}
	if old.Status != e.Status {
		e.StatusByHealth = false
	}
	e.UpdatedAt = s.now()
	setSecret := ""
	args := []any{}
	if e.Secret != "" || e.ClearSecret {
		v, err := s.seal(e.Secret, adEnvSecret(e.ID))
		if err != nil {
			return err
		}
		setSecret = `secret = ?, `
		args = append(args, v)
		e.HasSecret = e.Secret != ""
	}
	e.Secret, e.ClearSecret = "", false
	return s.tx(ctx, func(c conn) error {
		if err := s.checkEnvironmentRefs(ctx, c, tenantID, e); err != nil {
			return err
		}
		args = append(args, e.Name, e.Type, encodeList(e.URLs), e.Description, encodeComponents(e.Components), e.OwnerID, e.Status, e.StatusNote, e.StatusByHealth,
			encodeList(e.Tags), encodeList(e.ConfigurationIDs), encodeList(e.VariantIDs), e.Capacity, e.RequiresApproval, e.CredentialsRef, e.HealthURL, e.HealthIntervalMinutes,
			e.HealthStatus, nullTime(e.HealthCheckedAt), e.HealthError, e.UpdatedAt, e.ID, tenantID)
		res, err := c.exec(ctx, `UPDATE environments SET `+setSecret+`name = ?, type = ?, urls = ?, description = ?, components = ?, owner_id = ?, status = ?, status_note = ?, status_by_health = ?,
			tags = ?, configuration_ids = ?, variant_ids = ?, capacity = ?, requires_approval = ?, credentials_ref = ?, health_url = ?, health_interval_minutes = ?,
			health_status = ?, health_checked_at = ?, health_error = ?, version = version + 1, updated_at = ? WHERE id = ? AND tenant_id = ?`, args...)
		if err != nil {
			return mapErr(err, "environment name")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "environment")
		}
		// the name is kept in the free-text field of runs and cycles
		if old.Name != e.Name {
			if _, err := c.exec(ctx, `UPDATE test_runs SET environment = ? WHERE environment_id = ?`, e.Name, e.ID); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `UPDATE test_cycles SET environment = ? WHERE environment_id = ?`, e.Name, e.ID); err != nil {
				return err
			}
		}
		e.Version = old.Version + 1
		return nil
	})
}

// DeleteEnvironment removes an environment and its bookings; runs, cycles,
// sessions and results keep its name (and the results their component
// versions).
func (s *Store) DeleteEnvironment(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM environments WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "environment")
		}
		return nil
	})
}

// EnvironmentSecret returns the decrypted secret of an environment ("":
// none).
func (s *Store) EnvironmentSecret(ctx context.Context, tenantID, id string) (string, error) {
	var v string
	if err := s.c(ctx).row(ctx, `SELECT secret FROM environments WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&v); err != nil {
		return "", mapErr(err, "environment")
	}
	return s.unseal(ctx, v, adEnvSecret(id))
}

// SnapshotEnvironment returns the JSON snapshot of an environment stored
// in its history (derived fields and the secret left out).
func SnapshotEnvironment(e *model.Environment) json.RawMessage {
	c := *e
	c.Secret, c.ClearSecret = "", false
	c.BookedNow, c.NextBooking, c.RunCount, c.ProjectKey = 0, nil, 0, ""
	c.HealthCheckedAt, c.HealthStatus, c.HealthError = nil, "", ""
	b, _ := json.Marshal(c)
	return b
}

// ---------------------------------------------------------------- health checks

// HealthCheck is an environment whose health check is due.
type HealthCheck struct {
	TenantID      string
	EnvironmentID string
	ProjectID     string
	Name          string
	URL           string
}

// DueHealthChecks lists the environments with a health check URL whose
// check is due (never checked, or the interval has passed), not in
// maintenance and not retired.
func (s *Store) DueHealthChecks(ctx context.Context, now time.Time, limit int) ([]HealthCheck, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT tenant_id, id, COALESCE(project_id, ''), name, health_url, health_interval_minutes, health_checked_at FROM environments
		WHERE health_url <> '' AND status NOT IN ('maintenance', 'retired') ORDER BY health_checked_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HealthCheck
	for rows.Next() {
		var h HealthCheck
		var every int
		var at sql.NullTime
		if err := rows.Scan(&h.TenantID, &h.EnvironmentID, &h.ProjectID, &h.Name, &h.URL, &every, &at); err != nil {
			return nil, err
		}
		if at.Valid && now.Sub(at.Time) < time.Duration(every)*time.Minute {
			continue
		}
		out = append(out, h)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

// RecordHealth stores the outcome of a health check. A failing check marks
// an available (or in use) environment broken; a passing check makes an
// environment the health check marked broken available again. It returns
// the status before and after.
func (s *Store) RecordHealth(ctx context.Context, tenantID, id string, ok bool, errText string, at time.Time) (before, after string, err error) {
	err = s.tx(ctx, func(c conn) error {
		var byHealth bool
		if err := c.row(ctx, `SELECT status, status_by_health FROM environments WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&before, &byHealth); err != nil {
			return mapErr(err, "environment")
		}
		after = before
		health, note := "up", ""
		if !ok {
			health = "down"
		}
		switch {
		case !ok && (before == "available" || before == "in_use"):
			after, byHealth = "broken", true
			note = truncate("Health check failed: "+errText, 2000)
		case ok && before == "broken" && byHealth:
			after, byHealth = "available", false
		}
		q := `UPDATE environments SET health_status = ?, health_error = ?, health_checked_at = ?`
		args := []any{health, truncate(errText, 2000), at}
		if after != before {
			q += `, status = ?, status_by_health = ?, status_note = ?, updated_at = ?`
			args = append(args, after, byHealth, note, at)
		}
		_, err := c.exec(ctx, q+` WHERE id = ?`, append(args, id)...)
		return err
	})
	return before, after, err
}

// ---------------------------------------------------------------- run, cycle and session environment

// environmentFor checks that an environment can be used in a project (its
// own or shared, not retired) and returns its name.
func environmentFor(ctx context.Context, c conn, tenantID, projectID, id string) (string, error) {
	var name, status string
	var pid sql.NullString
	err := c.row(ctx, `SELECT name, status, project_id FROM environments WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&name, &status, &pid)
	if err != nil || (pid.Valid && pid.String != projectID) {
		return "", Invalidf("environment %q not found in the project", id)
	}
	if status == "retired" {
		return "", Invalidf("environment %s is retired", name)
	}
	return name, nil
}

// resolveRunEnvironment sets the environment of a run: a given id is
// checked (and its name kept in the free-text field); without one, a
// free-text environment naming an environment of the project (or a shared
// one) is linked to it.
func resolveRunEnvironment(ctx context.Context, c conn, tenantID, projectID string, id **string, text *string) error {
	if *id != nil && **id != "" {
		name, err := environmentFor(ctx, c, tenantID, projectID, **id)
		if err != nil {
			return err
		}
		*text = name
		return nil
	}
	*id = nil
	t := strings.TrimSpace(*text)
	if t == "" {
		return nil
	}
	var eid string
	err := c.row(ctx, `SELECT id FROM environments WHERE tenant_id = ? AND (project_id = ? OR project_id IS NULL) AND LOWER(name) = LOWER(?) AND status <> 'retired'
		ORDER BY CASE WHEN project_id IS NULL THEN 1 ELSE 0 END LIMIT 1`, tenantID, projectID, t).Scan(&eid)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	*id = &eid
	return nil
}

// environmentTables are the items that select an environment.
var environmentTables = map[string]string{"run": "test_runs", "cycle": "test_cycles", model.KindExploratory: "exploratory_sessions"}

// SetItemEnvironment sets (or with nil clears) the environment of a run,
// cycle or exploratory session; runs and cycles keep its name in their
// free-text environment.
func (s *Store) SetItemEnvironment(ctx context.Context, tenantID, kind, id string, envID *string) error {
	table, ok := environmentTables[kind]
	if !ok {
		return Invalidf("unknown item type %q", kind)
	}
	return s.tx(ctx, func(c conn) error {
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
			return mapErr(err, kind)
		}
		name := ""
		if envID != nil && *envID != "" {
			n, err := environmentFor(ctx, c, tenantID, pid, *envID)
			if err != nil {
				return err
			}
			name = n
		} else {
			envID = nil
		}
		if table == "exploratory_sessions" {
			_, err := c.exec(ctx, `UPDATE exploratory_sessions SET environment_id = ? WHERE id = ?`, nullStr(envID), id)
			return err
		}
		if envID == nil {
			_, err := c.exec(ctx, `UPDATE `+table+` SET environment_id = NULL WHERE id = ?`, id)
			return err
		}
		_, err := c.exec(ctx, `UPDATE `+table+` SET environment_id = ?, environment = ? WHERE id = ?`, *envID, name, id)
		return err
	})
}

// ItemEnvironment returns the environment id of a cycle or session (nil
// none).
func (s *Store) ItemEnvironment(ctx context.Context, tenantID, kind, id string) (*string, error) {
	table, ok := environmentTables[kind]
	if !ok {
		return nil, Invalidf("unknown item type %q", kind)
	}
	var v sql.NullString
	if err := s.c(ctx).row(ctx, `SELECT environment_id FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&v); err != nil {
		return nil, mapErr(err, kind)
	}
	return strPtr(v), nil
}

// snapshotExecutionEnvironment records the environment of the run and the
// versions of its components on an execution that got a final result (and
// clears them when the result is taken back).
func snapshotExecutionEnvironment(ctx context.Context, c conn, execID, runID string, final bool) error {
	if !final {
		_, err := c.exec(ctx, `UPDATE test_executions SET environment_id = NULL, environment_name = '', environment_components = '[]' WHERE id = ?`, execID)
		return err
	}
	var envID sql.NullString
	var text string
	if err := c.row(ctx, `SELECT environment_id, environment FROM test_runs WHERE id = ?`, runID).Scan(&envID, &text); err != nil {
		return err
	}
	name, comps := strings.TrimSpace(text), "[]"
	if envID.Valid {
		if err := c.row(ctx, `SELECT name, components FROM environments WHERE id = ?`, envID.String).Scan(&name, &comps); err != nil && err != sql.ErrNoRows {
			return err
		}
	}
	_, err := c.exec(ctx, `UPDATE test_executions SET environment_id = ?, environment_name = ?, environment_components = ? WHERE id = ?`,
		nullStr(strPtr(envID)), name, comps, execID)
	return err
}

// ReleaseEnvironments lists the environments (with the component versions
// at execution time) the final results of a release's runs were obtained
// in: one row per environment and set of versions, most executions first.
func (s *Store) ReleaseEnvironments(ctx context.Context, tenantID, releaseID string) ([]model.ReleaseEnvironment, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT COALESCE(e.environment_id, ''), e.environment_name, e.environment_components, COALESCE(env.type, ''), e.status, e.finished_at
		FROM test_executions e JOIN test_runs r ON r.id = e.run_id LEFT JOIN environments env ON env.id = e.environment_id
		WHERE r.release_id = ? AND r.tenant_id = ? AND e.status IN ('passed', 'failed', 'blocked', 'skipped') AND e.environment_name <> ''`, releaseID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct{ id, name, comps string }
	idx := map[key]int{}
	out := []model.ReleaseEnvironment{}
	for rows.Next() {
		var k key
		var typ, status string
		var at sql.NullTime
		if err := rows.Scan(&k.id, &k.name, &k.comps, &typ, &status, &at); err != nil {
			return nil, err
		}
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, model.ReleaseEnvironment{EnvironmentID: k.id, Name: k.name, Type: typ, Components: decodeComponents(k.comps)})
		}
		x := &out[i]
		x.Executions++
		switch status {
		case model.ResultPassed:
			x.Passed++
		case model.ResultFailed:
			x.Failed++
		case model.ResultBlocked:
			x.Blocked++
		}
		if t := timePtr(at); t != nil && (x.LastAt == nil || t.After(*x.LastAt)) {
			x.LastAt = t
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(out, func(a, b model.ReleaseEnvironment) int {
		if a.Executions != b.Executions {
			return b.Executions - a.Executions
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

// ---------------------------------------------------------------- bookings

// BookingConflictError: a booking does not fit in the capacity of the
// environment. It is an ErrConflict.
type BookingConflictError struct {
	Conflict model.BookingConflict
}

func (e *BookingConflictError) Error() string {
	return fmt.Sprintf("the environment is fully booked at that time (capacity %d, %d overlapping bookings)", e.Conflict.Capacity, len(e.Conflict.Overlapping))
}

// Is makes it an ErrConflict.
func (e *BookingConflictError) Is(target error) bool { return target == ErrConflict }

const bookingCols = `b.id, b.environment_id, COALESCE(b.project_id, ''), b.user_id, b.booked_by, b.starts_at, b.ends_at, b.purpose, b.run_id, b.cycle_id, b.session_id,
	b.status, b.decided_by, b.decided_at, b.decision_note, b.created_at, b.updated_at, env.name, COALESCE(p.key, ''),
	COALESCE(r.name, ''), COALESCE(cy.name, ''), COALESCE(x.key, '')`

const bookingFrom = ` FROM environment_bookings b JOIN environments env ON env.id = b.environment_id LEFT JOIN projects p ON p.id = b.project_id
	LEFT JOIN test_runs r ON r.id = b.run_id LEFT JOIN test_cycles cy ON cy.id = b.cycle_id LEFT JOIN exploratory_sessions x ON x.id = b.session_id`

func scanBooking(r interface{ Scan(...any) error }) (*model.Booking, error) {
	var b model.Booking
	var run, cycle, session sql.NullString
	var decided sql.NullTime
	if err := r.Scan(&b.ID, &b.EnvironmentID, &b.ProjectID, &b.UserID, &b.BookedBy, &b.StartsAt, &b.EndsAt, &b.Purpose, &run, &cycle, &session,
		&b.Status, &b.DecidedBy, &decided, &b.DecisionNote, &b.CreatedAt, &b.UpdatedAt, &b.EnvironmentName, &b.ProjectKey,
		&b.RunName, &b.CycleName, &b.SessionKey); err != nil {
		return nil, err
	}
	b.RunID, b.CycleID, b.SessionID = strPtr(run), strPtr(cycle), strPtr(session)
	b.DecidedAt = timePtr(decided)
	b.StartsAt, b.EndsAt, b.CreatedAt, b.UpdatedAt = b.StartsAt.UTC(), b.EndsAt.UTC(), b.CreatedAt.UTC(), b.UpdatedAt.UTC()
	return &b, nil
}

func (s *Store) listBookings(ctx context.Context, where string, args ...any) ([]model.Booking, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+bookingCols+bookingFrom+` WHERE `+where+` ORDER BY b.starts_at, b.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// BookingFilter limits a list of bookings: the time window (zero: open),
// the statuses (none: all).
type BookingFilter struct {
	From, To time.Time
	Statuses []string
}

func (f BookingFilter) where(where string, args []any) (string, []any) {
	if !f.From.IsZero() {
		where += ` AND b.ends_at > ?`
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where += ` AND b.starts_at < ?`
		args = append(args, f.To)
	}
	if len(f.Statuses) > 0 {
		where += ` AND b.status IN (` + placeholders(len(f.Statuses)) + `)`
		args = append(args, anySlice(f.Statuses)...)
	}
	return where, args
}

// ListBookings returns the bookings of an environment.
func (s *Store) ListBookings(ctx context.Context, tenantID, envID string, f BookingFilter) ([]model.Booking, error) {
	where, args := f.where(`b.environment_id = ? AND b.tenant_id = ?`, []any{envID, tenantID})
	return s.listBookings(ctx, where, args...)
}

// ListProjectBookings returns the bookings of a project's environments and
// the bookings made for the project (of shared environments).
func (s *Store) ListProjectBookings(ctx context.Context, tenantID, projectID string, f BookingFilter) ([]model.Booking, error) {
	where, args := f.where(`b.tenant_id = ? AND (env.project_id = ? OR b.project_id = ?)`, []any{tenantID, projectID, projectID})
	return s.listBookings(ctx, where, args...)
}

// PendingBookings returns the bookings of a tenant waiting for approval.
func (s *Store) PendingBookings(ctx context.Context, tenantID string) ([]model.Booking, error) {
	return s.listBookings(ctx, `b.tenant_id = ? AND b.status = 'pending'`, tenantID)
}

// ItemBookings returns the bookings linked to a run, cycle or session.
func (s *Store) ItemBookings(ctx context.Context, tenantID, kind, id string) ([]model.Booking, error) {
	col := map[string]string{"run": "b.run_id", "cycle": "b.cycle_id", model.KindExploratory: "b.session_id"}[kind]
	if col == "" {
		return nil, Invalidf("unknown item type %q", kind)
	}
	return s.listBookings(ctx, `b.tenant_id = ? AND `+col+` = ?`, tenantID, id)
}

// GetBooking returns a booking.
func (s *Store) GetBooking(ctx context.Context, tenantID, id string) (*model.Booking, error) {
	b, err := scanBooking(s.c(ctx).row(ctx, `SELECT `+bookingCols+bookingFrom+` WHERE b.id = ? AND b.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "booking")
	}
	return b, nil
}

// checkCapacity fails with a BookingConflictError when the booking would
// exceed the capacity at any moment of its time.
func checkCapacity(ctx context.Context, c conn, b *model.Booking, capacity int) error {
	rows, err := c.query(ctx, `SELECT `+bookingCols+bookingFrom+` WHERE b.environment_id = ? AND b.id <> ? AND b.status IN ('pending', 'approved')
		AND b.starts_at < ? AND b.ends_at > ? ORDER BY b.starts_at, b.id`, b.EnvironmentID, b.ID, b.EndsAt, b.StartsAt)
	if err != nil {
		return err
	}
	var over []model.Booking
	for rows.Next() {
		x, err := scanBooking(rows)
		if err != nil {
			rows.Close()
			return err
		}
		over = append(over, *x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if MaxOverlap(over, b.StartsAt, b.EndsAt)+1 > capacity {
		return &BookingConflictError{Conflict: model.BookingConflict{Capacity: capacity, Overlapping: over}}
	}
	return nil
}

// MaxOverlap returns the largest number of the bookings that overlap one
// another at a moment inside [from, to).
func MaxOverlap(list []model.Booking, from, to time.Time) int {
	type ev struct {
		at time.Time
		d  int
	}
	var evs []ev
	for _, b := range list {
		s, e := b.StartsAt, b.EndsAt
		if s.Before(from) {
			s = from
		}
		if e.After(to) {
			e = to
		}
		if !s.Before(e) {
			continue
		}
		evs = append(evs, ev{s, 1}, ev{e, -1})
	}
	// ends before starts at the same moment (touching bookings do not overlap)
	slices.SortFunc(evs, func(a, b ev) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}
		return a.d - b.d
	})
	n, best := 0, 0
	for _, e := range evs {
		n += e.d
		best = max(best, n)
	}
	return best
}

// validateBooking checks the time and the links of a booking for project
// projectID.
func validateBooking(ctx context.Context, c conn, tenantID string, b *model.Booking) error {
	if b.StartsAt.IsZero() || b.EndsAt.IsZero() || !b.EndsAt.After(b.StartsAt) {
		return Invalidf("a booking needs a start and an end after the start")
	}
	if b.EndsAt.Sub(b.StartsAt) > MaxBookingDays*24*time.Hour {
		return Invalidf("a booking can last at most %d days", MaxBookingDays)
	}
	b.StartsAt, b.EndsAt = b.StartsAt.UTC().Truncate(time.Second), b.EndsAt.UTC().Truncate(time.Second)
	b.Purpose = strings.TrimSpace(b.Purpose)
	if utf8.RuneCountInString(b.Purpose) > 2000 {
		return Invalidf("the purpose is too long")
	}
	for _, x := range []struct {
		id         *string
		table, msg string
	}{{b.RunID, "test_runs", "run"}, {b.CycleID, "test_cycles", "cycle"}, {b.SessionID, "exploratory_sessions", "exploratory session"}} {
		if x.id == nil || *x.id == "" {
			continue
		}
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM `+x.table+` WHERE id = ? AND tenant_id = ?`, *x.id, tenantID).Scan(&pid); err != nil || pid != b.ProjectID {
			return Invalidf("%s %q not found in the project of the booking", x.msg, *x.id)
		}
	}
	return nil
}

// CreateBooking books an environment. The booking is pending when the
// environment requires approval (unless approved is set: the booker may
// approve it), else approved. It fails with a BookingConflictError when the
// environment is fully booked at that time, and for a retired, broken or
// maintenance environment.
func (s *Store) CreateBooking(ctx context.Context, tenantID string, b *model.Booking, approved bool) error {
	b.ID = newID()
	b.CreatedAt = s.now()
	b.UpdatedAt = b.CreatedAt
	return s.tx(ctx, func(c conn) error {
		var pid sql.NullString
		var capacity int
		var status string
		var needs bool
		if err := c.row(ctx, `SELECT project_id, capacity, status, requires_approval FROM environments WHERE id = ? AND tenant_id = ?`, b.EnvironmentID, tenantID).
			Scan(&pid, &capacity, &status, &needs); err != nil {
			return mapErr(err, "environment")
		}
		if pid.Valid {
			if b.ProjectID != "" && b.ProjectID != pid.String {
				return Invalidf("the environment belongs to another project")
			}
			b.ProjectID = pid.String
		}
		if status == "retired" {
			return Invalidf("a retired environment cannot be booked")
		}
		if err := validateBooking(ctx, c, tenantID, b); err != nil {
			return err
		}
		b.Status = "approved"
		if needs && !approved {
			b.Status = "pending"
		} else if needs {
			b.DecidedBy, b.DecidedAt = b.BookedBy, &b.CreatedAt
		}
		if err := checkCapacity(ctx, c, b, capacity); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO environment_bookings (id, tenant_id, environment_id, project_id, user_id, booked_by, starts_at, ends_at, purpose, run_id, cycle_id, session_id,
			status, decided_by, decided_at, decision_note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			b.ID, tenantID, b.EnvironmentID, projectArg(b.ProjectID), b.UserID, b.BookedBy, b.StartsAt, b.EndsAt, b.Purpose, nullStr(b.RunID), nullStr(b.CycleID), nullStr(b.SessionID),
			b.Status, b.DecidedBy, nullTime(b.DecidedAt), b.DecisionNote, b.CreatedAt, b.UpdatedAt)
		return err
	})
}

// UpdateBooking saves the time, purpose and links of a pending or approved
// booking; a changed time of an approved booking of an environment that
// requires approval needs approval again (unless approved is set).
func (s *Store) UpdateBooking(ctx context.Context, tenantID string, b *model.Booking, approved bool) error {
	old, err := s.GetBooking(ctx, tenantID, b.ID)
	if err != nil {
		return err
	}
	if !slices.Contains(model.BookingActiveStatuses, old.Status) {
		return Conflictf("a %s booking cannot be changed", old.Status)
	}
	b.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		var capacity int
		var needs bool
		if err := c.row(ctx, `SELECT capacity, requires_approval FROM environments WHERE id = ?`, b.EnvironmentID).Scan(&capacity, &needs); err != nil {
			return mapErr(err, "environment")
		}
		if err := validateBooking(ctx, c, tenantID, b); err != nil {
			return err
		}
		if needs && !approved && (!b.StartsAt.Equal(old.StartsAt) || !b.EndsAt.Equal(old.EndsAt)) {
			b.Status, b.DecidedBy, b.DecidedAt, b.DecisionNote = "pending", "", nil, ""
		}
		if err := checkCapacity(ctx, c, b, capacity); err != nil {
			return err
		}
		_, err := c.exec(ctx, `UPDATE environment_bookings SET starts_at = ?, ends_at = ?, purpose = ?, run_id = ?, cycle_id = ?, session_id = ?, status = ?,
			decided_by = ?, decided_at = ?, decision_note = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			b.StartsAt, b.EndsAt, b.Purpose, nullStr(b.RunID), nullStr(b.CycleID), nullStr(b.SessionID), b.Status, b.DecidedBy, nullTime(b.DecidedAt), b.DecisionNote,
			b.UpdatedAt, b.ID, tenantID)
		return err
	})
}

// DecideBooking approves or rejects a pending booking (approving checks
// the capacity again), or cancels a pending or approved one (status
// cancelled).
func (s *Store) DecideBooking(ctx context.Context, tenantID, id, status, user, note string) (*model.Booking, error) {
	if !slices.Contains([]string{"approved", "rejected", "cancelled"}, status) {
		return nil, Invalidf("decision must be approved, rejected or cancelled")
	}
	b, err := s.GetBooking(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	switch {
	case status == "cancelled" && !slices.Contains(model.BookingActiveStatuses, b.Status):
		return nil, Conflictf("a %s booking cannot be cancelled", b.Status)
	case status != "cancelled" && b.Status != "pending":
		return nil, Conflictf("the booking is not waiting for approval (it is %s)", b.Status)
	}
	if utf8.RuneCountInString(note) > 2000 {
		return nil, Invalidf("the note is too long")
	}
	now := s.now()
	err = s.tx(ctx, func(c conn) error {
		if status == "approved" {
			var capacity int
			if err := c.row(ctx, `SELECT capacity FROM environments WHERE id = ?`, b.EnvironmentID).Scan(&capacity); err != nil {
				return err
			}
			if err := checkCapacity(ctx, c, b, capacity); err != nil {
				return err
			}
		}
		if status == "cancelled" {
			_, err := c.exec(ctx, `UPDATE environment_bookings SET status = 'cancelled', updated_at = ? WHERE id = ?`, now, id)
			return err
		}
		_, err := c.exec(ctx, `UPDATE environment_bookings SET status = ?, decided_by = ?, decided_at = ?, decision_note = ?, updated_at = ? WHERE id = ?`,
			status, user, now, strings.TrimSpace(note), now, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetBooking(ctx, tenantID, id)
}

// DeleteBooking removes a booking.
func (s *Store) DeleteBooking(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM environment_bookings WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "booking")
	}
	return nil
}

// LinkEnvironmentNames links the runs and cycles of a project whose
// free-text environment names one of the project's environments (or a
// shared one) and that have no environment item yet, and records the
// environment on their final results that have none (as migration 0057
// does for existing data). It returns the number of runs linked.
func (s *Store) LinkEnvironmentNames(ctx context.Context, tenantID, projectID string) (int, error) {
	n := 0
	err := s.tx(ctx, func(c conn) error {
		for _, table := range []string{"test_runs", "test_cycles"} {
			rows, err := c.query(ctx, `SELECT id, environment FROM `+table+` WHERE tenant_id = ? AND project_id = ? AND environment_id IS NULL AND TRIM(environment) <> ''`, tenantID, projectID)
			if err != nil {
				return err
			}
			type item struct{ id, env string }
			var items []item
			for rows.Next() {
				var it item
				if err := rows.Scan(&it.id, &it.env); err != nil {
					rows.Close()
					return err
				}
				items = append(items, it)
			}
			rows.Close()
			for _, it := range items {
				var id *string
				text := it.env
				if err := resolveRunEnvironment(ctx, c, tenantID, projectID, &id, &text); err != nil {
					return err
				}
				if id == nil {
					continue
				}
				if _, err := c.exec(ctx, `UPDATE `+table+` SET environment_id = ? WHERE id = ?`, *id, it.id); err != nil {
					return err
				}
				if table == "test_runs" {
					n++
				}
			}
		}
		rows, err := c.query(ctx, `SELECT e.id, e.run_id FROM test_executions e JOIN test_runs r ON r.id = e.run_id
			WHERE r.tenant_id = ? AND r.project_id = ? AND r.environment_id IS NOT NULL AND e.environment_id IS NULL AND e.status IN ('passed', 'failed', 'blocked', 'skipped')`, tenantID, projectID)
		if err != nil {
			return err
		}
		var pairs [][2]string
		for rows.Next() {
			var p [2]string
			if err := rows.Scan(&p[0], &p[1]); err != nil {
				rows.Close()
				return err
			}
			pairs = append(pairs, p)
		}
		rows.Close()
		for _, p := range pairs {
			if err := snapshotExecutionEnvironment(ctx, c, p[0], p[1], true); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}
