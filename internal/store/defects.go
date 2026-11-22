package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Defects: problems found while testing, linked to requirements, tests,
// executions and runs, optionally mirrored in Jira or GitHub Issues.

const defectCols = `id, project_id, key, title, description, status, severity, priority, assignee, external_system, external_key, external_url, external_status, synced_at, created_by, created_at, updated_at, resolved_at, found_in`

func scanDefect(r interface{ Scan(...any) error }) (*model.Defect, error) {
	var d model.Defect
	var synced, resolved sql.NullTime
	if err := r.Scan(&d.ID, &d.ProjectID, &d.Key, &d.Title, &d.Description, &d.Status, &d.Severity, &d.Priority, &d.Assignee,
		&d.ExternalSystem, &d.ExternalKey, &d.ExternalURL, &d.ExternalStatus, &synced, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &resolved, &d.FoundIn); err != nil {
		return nil, err
	}
	d.SyncedAt, d.ResolvedAt = timePtr(synced), timePtr(resolved)
	return &d, nil
}

// DefectOpen reports whether a status counts as open.
func DefectOpen(status string) bool { return slices.Contains(model.DefectOpenStatuses, status) }

// SeverityAtLeast reports whether severity is at least level ("" = any).
func SeverityAtLeast(severity, level string) bool {
	if level == "" {
		return true
	}
	return slices.Index(model.DefectSeverities, severity) <= slices.Index(model.DefectSeverities, level) && slices.Contains(model.DefectSeverities, severity)
}

func validateDefect(d *model.Defect) error {
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" || len([]rune(d.Title)) > 300 {
		return Invalidf("defect title is required (at most 300 characters)")
	}
	if d.Status == "" {
		d.Status = "open"
	}
	if d.Severity == "" {
		d.Severity = "major"
	}
	if d.Priority == "" {
		d.Priority = "medium"
	}
	switch {
	case !slices.Contains(model.DefectStatuses, d.Status):
		return Invalidf("status must be one of %s", strings.Join(model.DefectStatuses, ", "))
	case !slices.Contains(model.DefectSeverities, d.Severity):
		return Invalidf("severity must be one of %s", strings.Join(model.DefectSeverities, ", "))
	case !slices.Contains(model.Priorities, d.Priority):
		return Invalidf("priority must be one of %s", strings.Join(model.Priorities, ", "))
	case d.FoundIn != "" && !slices.Contains(model.DefectPhases, d.FoundIn):
		return Invalidf("found_in must be one of %s (or empty)", strings.Join(model.DefectPhases, ", "))
	}
	if d.ExternalURL != "" {
		if u, err := url.Parse(d.ExternalURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return Invalidf("external_url must be an http(s) URL")
		}
	}
	return nil
}

// CreateDefect adds a defect with a new key (PROJ-BUG-n) and its links.
func (s *Store) CreateDefect(ctx context.Context, tenantID string, d *model.Defect) error {
	if err := validateDefect(d); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, d.ProjectID); err != nil {
		return err
	}
	d.ID = newID()
	d.CreatedAt = s.now()
	d.UpdatedAt = d.CreatedAt
	if !DefectOpen(d.Status) {
		t := d.CreatedAt
		d.ResolvedAt = &t
	}
	return s.tx(ctx, func(c conn) error {
		seq, key, err := nextKey(ctx, c, d.ProjectID, model.KindDefect)
		if err != nil {
			return err
		}
		d.Key = key
		if _, err := c.exec(ctx, `INSERT INTO defects (id, tenant_id, project_id, seq, key, title, description, status, severity, priority, assignee, external_system, external_key, external_url, external_status, synced_at, created_by, created_at, updated_at, resolved_at, found_in)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, tenantID, d.ProjectID, seq, d.Key, d.Title, d.Description, d.Status, d.Severity, d.Priority, d.Assignee,
			d.ExternalSystem, d.ExternalKey, d.ExternalURL, d.ExternalStatus, nullTime(d.SyncedAt), d.CreatedBy, d.CreatedAt, d.UpdatedAt, nullTime(d.ResolvedAt), d.FoundIn); err != nil {
			return mapErr(err, "defect")
		}
		for _, l := range d.Links {
			if err := addDefectLink(ctx, c, tenantID, d.ProjectID, d.ID, l); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateDefect saves the editable fields (resolved_at follows the status).
func (s *Store) UpdateDefect(ctx context.Context, tenantID string, d *model.Defect) error {
	if err := validateDefect(d); err != nil {
		return err
	}
	d.UpdatedAt = s.now()
	if DefectOpen(d.Status) {
		d.ResolvedAt = nil
	} else if d.ResolvedAt == nil {
		t := d.UpdatedAt
		d.ResolvedAt = &t
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE defects SET title = ?, description = ?, status = ?, severity = ?, priority = ?, assignee = ?, external_system = ?, external_key = ?, external_url = ?, external_status = ?, synced_at = ?, updated_at = ?, resolved_at = ?, found_in = ? WHERE id = ? AND tenant_id = ?`,
		d.Title, d.Description, d.Status, d.Severity, d.Priority, d.Assignee, d.ExternalSystem, d.ExternalKey, d.ExternalURL, d.ExternalStatus, nullTime(d.SyncedAt), d.UpdatedAt, nullTime(d.ResolvedAt), d.FoundIn, d.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "defect")
	}
	return nil
}

// GetDefect returns a defect with its links.
func (s *Store) GetDefect(ctx context.Context, tenantID, id string) (*model.Defect, error) {
	d, err := scanDefect(s.c(ctx).row(ctx, `SELECT `+defectCols+` FROM defects WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "defect")
	}
	links, err := s.defectLinks(ctx, tenantID, []string{d.ID})
	if err != nil {
		return nil, err
	}
	d.Links = links[d.ID]
	if d.Links == nil {
		d.Links = []model.DefectLink{}
	}
	return d, nil
}

// ListDefects lists the defects of a project (newest first) with links.
func (s *Store) ListDefects(ctx context.Context, tenantID, projectID string) ([]model.Defect, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+defectCols+` FROM defects WHERE tenant_id = ? AND project_id = ? ORDER BY seq DESC`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.Defect{}
	var ids []string
	for rows.Next() {
		d, err := scanDefect(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *d)
		ids = append(ids, d.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	links, err := s.defectLinks(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Links = links[out[i].ID]
		if out[i].Links == nil {
			out[i].Links = []model.DefectLink{}
		}
	}
	return out, nil
}

// defectLinks returns the links of defects with keys and titles of the items.
func (s *Store) defectLinks(ctx context.Context, tenantID string, ids []string) (map[string][]model.DefectLink, error) {
	out := map[string][]model.DefectLink{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.c(ctx).query(ctx, `SELECT l.defect_id, l.entity_type, l.entity_id,
		COALESCE(r.key, t.key, et.key, ru.name, ''), COALESCE(r.title, t.title, et.title, '')
		FROM defect_links l
		LEFT JOIN requirements r ON l.entity_type = 'requirement' AND r.id = l.entity_id
		LEFT JOIN test_cases t ON l.entity_type = 'test' AND t.id = l.entity_id
		LEFT JOIN test_executions e ON l.entity_type = 'execution' AND e.id = l.entity_id
		LEFT JOIN test_cases et ON et.id = e.test_case_id
		LEFT JOIN test_runs ru ON l.entity_type = 'run' AND ru.id = l.entity_id
		WHERE l.defect_id IN (`+placeholders(len(ids))+`) ORDER BY l.entity_type`, anySlice(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var l model.DefectLink
		if err := rows.Scan(&id, &l.EntityType, &l.EntityID, &l.Key, &l.Title); err != nil {
			return nil, err
		}
		out[id] = append(out[id], l)
	}
	return out, rows.Err()
}

// addDefectLink links a defect to an item of the same project.
func addDefectLink(ctx context.Context, c conn, tenantID, projectID, defectID string, l model.DefectLink) error {
	if !slices.Contains(model.DefectLinkTypes, l.EntityType) {
		return Invalidf("entity_type must be one of %s", strings.Join(model.DefectLinkTypes, ", "))
	}
	q := map[string]string{
		model.KindRequirement: `SELECT project_id FROM requirements WHERE id = ? AND tenant_id = ?`,
		model.KindTest:        `SELECT project_id FROM test_cases WHERE id = ? AND tenant_id = ?`,
		"execution":           `SELECT r.project_id FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE e.id = ? AND e.tenant_id = ?`,
		"run":                 `SELECT project_id FROM test_runs WHERE id = ? AND tenant_id = ?`,
	}[l.EntityType]
	var pid string
	if err := c.row(ctx, q, l.EntityID, tenantID).Scan(&pid); err != nil {
		return mapErr(err, l.EntityType)
	}
	if pid != projectID {
		return Invalidf("the %s belongs to another project", l.EntityType)
	}
	_, err := c.exec(ctx, `INSERT INTO defect_links (defect_id, entity_type, entity_id) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, defectID, l.EntityType, l.EntityID)
	return err
}

// LinkDefect links a defect to an item.
func (s *Store) LinkDefect(ctx context.Context, tenantID, defectID string, l model.DefectLink) error {
	d, err := s.GetDefect(ctx, tenantID, defectID)
	if err != nil {
		return err
	}
	return addDefectLink(ctx, s.c(ctx), tenantID, d.ProjectID, d.ID, l)
}

// UnlinkDefect removes a link.
func (s *Store) UnlinkDefect(ctx context.Context, tenantID, defectID string, l model.DefectLink) error {
	if _, err := s.GetDefect(ctx, tenantID, defectID); err != nil {
		return err
	}
	res, err := s.c(ctx).exec(ctx, `DELETE FROM defect_links WHERE defect_id = ? AND entity_type = ? AND entity_id = ?`, defectID, l.EntityType, l.EntityID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "defect link")
	}
	return nil
}

// DeleteDefect removes a defect and its links.
func (s *Store) DeleteDefect(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM defect_links WHERE defect_id = ?`, id); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM defects WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "defect")
		}
		return nil
	})
}

// DefectsOf returns the defects linked to an item. For a requirement it
// also includes defects linked to its tests and their executions (Via
// tells how).
type LinkedDefect struct {
	model.Defect
	Via string `json:"via"`
}

func (s *Store) DefectsOf(ctx context.Context, tenantID, entityType, entityID string) ([]LinkedDefect, error) {
	type ref struct{ typ, id, via string }
	refs := []ref{{entityType, entityID, entityType}}
	switch entityType {
	case model.KindRequirement:
		rows, err := s.c(ctx).query(ctx, `SELECT test_case_id FROM trace_links WHERE requirement_id = ? AND tenant_id = ?`, entityID, tenantID)
		if err != nil {
			return nil, err
		}
		tests, err := scanStrings(rows)
		if err != nil {
			return nil, err
		}
		for _, t := range tests {
			refs = append(refs, ref{model.KindTest, t, model.KindTest})
			execs, err := s.executionIDs(ctx, t)
			if err != nil {
				return nil, err
			}
			for _, e := range execs {
				refs = append(refs, ref{"execution", e, model.KindTest})
			}
		}
	case model.KindTest:
		execs, err := s.executionIDs(ctx, entityID)
		if err != nil {
			return nil, err
		}
		for _, e := range execs {
			refs = append(refs, ref{"execution", e, "execution"})
		}
	}
	seen := map[string]bool{}
	out := []LinkedDefect{}
	for _, r := range refs {
		rows, err := s.c(ctx).query(ctx, `SELECT defect_id FROM defect_links WHERE entity_type = ? AND entity_id = ?`, r.typ, r.id)
		if err != nil {
			return nil, err
		}
		ids, err := scanStrings(rows)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			d, err := s.GetDefect(ctx, tenantID, id)
			if err != nil {
				continue
			}
			out = append(out, LinkedDefect{Defect: *d, Via: r.via})
		}
	}
	return out, nil
}

func (s *Store) executionIDs(ctx context.Context, testID string) ([]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM test_executions WHERE test_case_id = ?`, testID)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// OpenDefectsOfRequirements returns the open defects at or above a
// severity linked to the requirements (directly, via their tests or the
// tests' executions).
func (s *Store) OpenDefectsOfRequirements(ctx context.Context, tenantID string, reqIDs []string, level string) ([]model.Defect, error) {
	seen := map[string]bool{}
	var out []model.Defect
	for _, rid := range reqIDs {
		list, err := s.DefectsOf(ctx, tenantID, model.KindRequirement, rid)
		if err != nil {
			return nil, err
		}
		for _, d := range list {
			if !seen[d.ID] && DefectOpen(d.Status) && SeverityAtLeast(d.Severity, level) {
				seen[d.ID] = true
				out = append(out, d.Defect)
			}
		}
	}
	return out, nil
}

// OpenDefectCounts returns, per requirement of a project, the number of
// open defects linked to it directly or through its tests.
func (s *Store) OpenDefectCounts(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT d.id, COALESCE(tl.requirement_id, el.requirement_id, l.entity_id) FROM defects d
		JOIN defect_links l ON l.defect_id = d.id
		LEFT JOIN trace_links tl ON l.entity_type = 'test' AND tl.test_case_id = l.entity_id
		LEFT JOIN test_executions e ON l.entity_type = 'execution' AND e.id = l.entity_id
		LEFT JOIN trace_links el ON el.test_case_id = e.test_case_id
		WHERE d.tenant_id = ? AND d.project_id = ? AND d.status IN ('open', 'in_progress')
		AND (l.entity_type = 'requirement' OR tl.requirement_id IS NOT NULL OR el.requirement_id IS NOT NULL)`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var did, rid string
		if err := rows.Scan(&did, &rid); err != nil {
			return nil, err
		}
		out[rid]++
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- tracker settings

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var jiraKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,19}$`)

// gitlabProjectRe: a numeric project ID or a path group/[subgroup/…]project.
var gitlabProjectRe = regexp.MustCompile(`^(?:[0-9]+|[A-Za-z0-9_.][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.][A-Za-z0-9_.-]*)+)$`)

// DefectTracker returns a project's tracker settings including the token
// (for the server's own use; the API masks it).
func (s *Store) DefectTracker(ctx context.Context, tenantID, projectID string) (*model.DefectTracker, error) {
	var raw string
	if err := s.c(ctx).row(ctx, `SELECT defect_tracker FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&raw); err != nil {
		return nil, mapErr(err, "project")
	}
	var t model.DefectTracker
	_ = json.Unmarshal([]byte(raw), &t)
	var err error
	if t.Token, err = s.unseal(ctx, t.Token, adTracker(projectID)); err != nil {
		return nil, fmt.Errorf("tracker token: %w", err)
	}
	t.HasToken = t.Token != ""
	return &t, nil
}

// SaveDefectTracker validates and stores tracker settings; an empty token
// keeps the stored one unless clearToken.
func (s *Store) SaveDefectTracker(ctx context.Context, tenantID, projectID string, t *model.DefectTracker, clearToken bool) error {
	cur, err := s.DefectTracker(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	t.Type = strings.TrimSpace(t.Type)
	t.URL = strings.TrimRight(strings.TrimSpace(t.URL), "/")
	switch t.Type {
	case "":
		*t = model.DefectTracker{}
	case "jira":
		if t.URL == "" || !strings.HasPrefix(t.URL, "https://") && !strings.HasPrefix(t.URL, "http://") {
			return Invalidf("the Jira URL must be an http(s) URL, e.g. https://example.atlassian.net")
		}
		t.ProjectKey = strings.ToUpper(strings.TrimSpace(t.ProjectKey))
		if !jiraKeyRe.MatchString(t.ProjectKey) {
			return Invalidf("project_key must be a Jira project key, e.g. SHOP")
		}
		if t.IssueType == "" {
			t.IssueType = "Bug"
		}
	case "github":
		if t.URL == "" {
			t.URL = "https://api.github.com"
		}
		if !strings.HasPrefix(t.URL, "https://") && !strings.HasPrefix(t.URL, "http://") {
			return Invalidf("the GitHub API URL must be an http(s) URL")
		}
		t.Repo = strings.TrimSpace(t.Repo)
		if !repoRe.MatchString(t.Repo) {
			return Invalidf("repo must be owner/repository")
		}
	case "gitlab":
		if t.URL == "" {
			t.URL = "https://gitlab.com"
		}
		t.URL = strings.TrimRight(strings.TrimSuffix(t.URL, "/api/v4"), "/")
		if u, err := url.Parse(t.URL); err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.User != nil {
			return Invalidf("the GitLab URL must be an https URL, e.g. https://gitlab.com or https://gitlab.example.com")
		}
		p := strings.TrimSpace(t.Project)
		p = strings.TrimPrefix(p, t.URL+"/") // a pasted project URL
		t.Project = strings.Trim(p, "/")
		if !gitlabProjectRe.MatchString(t.Project) || slices.ContainsFunc(strings.Split(t.Project, "/"), func(s string) bool { return strings.Trim(s, ".") == "" }) {
			return Invalidf("project must be a GitLab project ID or path, e.g. 42 or group/subgroup/project")
		}
	default:
		return Invalidf("type must be jira, github, gitlab or empty")
	}
	if t.Type != "gitlab" {
		t.Project = ""
	}
	if t.Type == "jira" {
		// Jira issues get no milestone or assignee from here
		t.Milestone, t.Assignee, t.Labels = "", "", ""
		if t.RequirementIssueType = strings.TrimSpace(t.RequirementIssueType); t.RequirementIssueType == "" {
			t.RequirementIssueType = "Task"
		}
	} else {
		t.IssueType, t.RequirementIssueType = "", ""
	}
	t.Labels = strings.TrimSpace(t.Labels)
	t.Milestone = strings.TrimSpace(t.Milestone)
	t.Assignee = strings.TrimSpace(t.Assignee)
	t.RequirementLabels = strings.TrimSpace(t.RequirementLabels)
	for name, v := range map[string]string{"labels": t.Labels, "milestone": t.Milestone, "assignee": t.Assignee, "requirement_labels": t.RequirementLabels, "requirement_issue_type": t.RequirementIssueType, "issue_type": t.IssueType} {
		if utf8.RuneCountInString(v) > 500 {
			return Invalidf("%s may have at most 500 characters", name)
		}
	}
	if t.Token == "" && !clearToken && t.Type != "" {
		t.Token = cur.Token
	}
	t.HasToken = false
	stored := *t
	if stored.Token, err = s.seal(t.Token, adTracker(projectID)); err != nil {
		return err
	}
	raw, _ := json.Marshal(stored)
	_, err = s.c(ctx).exec(ctx, `UPDATE projects SET defect_tracker = ? WHERE id = ? AND tenant_id = ?`, string(raw), projectID, tenantID)
	t.HasToken = t.Token != ""
	return err
}

// MarkDefectSynced records an external status read from the tracker.
func (s *Store) MarkDefectSynced(ctx context.Context, tenantID string, d *model.Defect, externalStatus, status string) error {
	now := s.now()
	d.ExternalStatus, d.SyncedAt = externalStatus, &now
	if status != "" {
		d.Status = status
	}
	return s.UpdateDefect(ctx, tenantID, d)
}

// deleteDefectLinks removes the defect links of deleted items.
func deleteDefectLinks(ctx context.Context, c conn, entityType string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := c.exec(ctx, `DELETE FROM defect_links WHERE entity_type = ? AND entity_id IN (`+placeholders(len(ids))+`)`, append([]any{entityType}, anySlice(ids)...)...)
	return err
}

// ValidateDefect checks and normalises a defect's fields without saving it.
func ValidateDefect(d *model.Defect) error { return validateDefect(d) }
