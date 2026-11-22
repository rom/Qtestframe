package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- revisions

// derivedFields are computed or bookkeeping keys ignored when diffing
// snapshots.
var derivedFields = map[string]bool{
	"updated_at": true, "version": true, "folder_path": true, "linked_tests": true,
	"coverage": true, "comment_count": true, "relation_count": true,
	"linked_requirements": true, "last_result": true, "called_by": true,
	"created_at": true, "created_by": true, "position": true,
	// evidence and quality state, not content
	"suspect_links": true, "risk_level": true, "open_defects": true,
}

// SnapshotRequirement returns the JSON snapshot stored for a requirement.
func SnapshotRequirement(r *model.Requirement) json.RawMessage {
	c := *r
	c.FolderPath, c.LinkedTests, c.Coverage, c.CommentCount, c.Relations = "", 0, "", 0, 0
	c.RelationRefs, c.CriteriaCount = nil, 0
	c.SuspectLinks, c.RiskLevel, c.OpenDefects = 0, "", 0
	if c.Criteria == nil {
		c.Criteria = []model.EvaluationCriterion{}
	}
	b, _ := json.Marshal(c)
	return b
}

// SnapshotTest returns the JSON snapshot stored for a test case.
func SnapshotTest(t *model.TestCase) json.RawMessage {
	c := *t
	c.FolderPath, c.LinkedReqs, c.LastResult, c.CommentCount, c.CalledBy = "", 0, "", 0, nil
	c.RelationRefs, c.StepCount, c.Defects = nil, 0, nil
	c.SuspectLinks = 0
	steps := make([]model.TestStep, len(c.Steps))
	for i, st := range c.Steps {
		st.CallTestKey, st.CallTestTitle = "", ""
		steps[i] = st
	}
	c.Steps = steps
	b, _ := json.Marshal(c)
	return b
}

// ChangedFields lists top-level keys whose values differ between two JSON
// object snapshots, ignoring derived fields. A nil "from" means creation.
func ChangedFields(from, to json.RawMessage) []string {
	var a, b map[string]any
	_ = json.Unmarshal(from, &a)
	_ = json.Unmarshal(to, &b)
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	out := []string{}
	for k := range keys {
		if derivedFields[k] {
			continue
		}
		if k == "custom_fields" {
			// one entry per custom field: custom_fields.<key>
			out = append(out, changedCustom(a[k], b[k])...)
			continue
		}
		if !reflect.DeepEqual(normalize(k, a[k]), normalize(k, b[k])) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// changedCustom lists the custom fields whose values differ between two
// snapshots' custom_fields objects, as custom_fields.<key>.
func changedCustom(from, to any) []string {
	a, _ := from.(map[string]any)
	b, _ := to.(map[string]any)
	var out []string
	for k, v := range a {
		if !reflect.DeepEqual(v, b[k]) {
			out = append(out, "custom_fields."+k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out = append(out, "custom_fields."+k)
		}
	}
	return out
}

// normalize removes volatile IDs inside steps so that re-saving identical
// steps (which get new IDs) is not reported as a change.
func normalize(key string, v any) any {
	if key != "steps" {
		return v
	}
	list, ok := v.([]any)
	if !ok {
		return v
	}
	out := make([]any, len(list))
	for i, x := range list {
		if m, ok := x.(map[string]any); ok {
			c := map[string]any{}
			for k, val := range m {
				if k != "id" && k != "position" && k != "call_test_key" && k != "call_test_title" {
					c[k] = val
				}
			}
			out[i] = c
		} else {
			out[i] = x
		}
	}
	return out
}

// AddRevision stores a snapshot of an entity. changed_fields is computed
// against the previous revision.
func (s *Store) AddRevision(ctx context.Context, tenantID, entityType, entityID string, version int, action, user string, snapshot json.RawMessage) error {
	var prev string
	err := s.c(ctx).row(ctx, `SELECT snapshot FROM revisions WHERE entity_type = ? AND entity_id = ? ORDER BY created_at DESC, version DESC LIMIT 1`, entityType, entityID).Scan(&prev)
	var changed []string
	switch {
	case err == nil:
		changed = ChangedFields(json.RawMessage(prev), snapshot)
	case err == sql.ErrNoRows:
		changed = []string{}
	default:
		return err
	}
	cf, _ := json.Marshal(changed)
	_, err = s.c(ctx).exec(ctx, `INSERT INTO revisions (id, tenant_id, entity_type, entity_id, version, action, changed_fields, snapshot, user_email, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), tenantID, entityType, entityID, version, action, string(cf), string(snapshot), user, s.now())
	return err
}

// ListRevisions returns the revisions of an entity, newest first.
func (s *Store) ListRevisions(ctx context.Context, tenantID, entityType, entityID string) ([]model.Revision, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT r.id, r.entity_type, r.entity_id, r.version, r.action, r.changed_fields, r.snapshot, r.user_email, COALESCE(u.display_name, ''), COALESCE(u.avatar, ''), r.created_at
		FROM revisions r LEFT JOIN users u ON u.email = r.user_email
		WHERE r.tenant_id = ? AND r.entity_type = ? AND r.entity_id = ? ORDER BY r.created_at DESC, r.version DESC`, tenantID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Revision{}
	for rows.Next() {
		r, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// GetRevision returns one revision of an entity.
func (s *Store) GetRevision(ctx context.Context, tenantID, entityType, entityID, id string) (*model.Revision, error) {
	r, err := scanRevision(s.c(ctx).row(ctx, `SELECT r.id, r.entity_type, r.entity_id, r.version, r.action, r.changed_fields, r.snapshot, r.user_email, COALESCE(u.display_name, ''), COALESCE(u.avatar, ''), r.created_at
		FROM revisions r LEFT JOIN users u ON u.email = r.user_email
		WHERE r.id = ? AND r.tenant_id = ? AND r.entity_type = ? AND r.entity_id = ?`, id, tenantID, entityType, entityID))
	return r, mapErr(err, "revision")
}

func scanRevision(r interface{ Scan(...any) error }) (*model.Revision, error) {
	var rev model.Revision
	var changed, snap string
	if err := r.Scan(&rev.ID, &rev.EntityType, &rev.EntityID, &rev.Version, &rev.Action, &changed, &snap, &rev.UserEmail, &rev.UserName, &rev.UserAvatar, &rev.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(changed), &rev.ChangedFields)
	if rev.ChangedFields == nil {
		rev.ChangedFields = []string{}
	}
	rev.Snapshot = json.RawMessage(snap)
	return &rev, nil
}

// ---------------------------------------------------------------- relations

const relationSelect = `SELECT r.id, r.project_id, r.source_id, r.target_id, r.type, r.created_by, r.created_at, s.key, s.title, t.key, t.title
	FROM requirement_relations r JOIN requirements s ON s.id = r.source_id JOIN requirements t ON t.id = r.target_id`

func scanRelation(r interface{ Scan(...any) error }) (*model.Relation, error) {
	var rel model.Relation
	if err := r.Scan(&rel.ID, &rel.ProjectID, &rel.SourceID, &rel.TargetID, &rel.Type, &rel.CreatedBy, &rel.CreatedAt, &rel.SourceKey, &rel.SourceTitle, &rel.TargetKey, &rel.TargetTitle); err != nil {
		return nil, err
	}
	return &rel, nil
}

// CreateRelation links two requirements of the same project.
func (s *Store) CreateRelation(ctx context.Context, tenantID string, rel *model.Relation) error {
	if !model.Valid(model.RelationTypes, rel.Type) {
		return Invalidf("relation type must be one of %s", strings.Join(model.RelationTypes, ", "))
	}
	if rel.SourceID == rel.TargetID {
		return Invalidf("a requirement cannot be related to itself")
	}
	src, err := s.GetRequirement(ctx, tenantID, rel.SourceID)
	if err != nil {
		return err
	}
	dst, err := s.GetRequirement(ctx, tenantID, rel.TargetID)
	if err != nil {
		return Invalidf("target requirement not found")
	}
	if src.ProjectID != dst.ProjectID {
		return Invalidf("related requirements must belong to the same project")
	}
	if rel.Type == "refines" {
		// the target (parent) must not already refine the source, directly or indirectly
		parents, err := s.refinesEdges(ctx, src.ProjectID)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		stack := []string{rel.TargetID}
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if id == rel.SourceID {
				return Invalidf("refines relations would form a cycle")
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			stack = append(stack, parents[id]...)
		}
	}
	rel.ID, rel.ProjectID, rel.CreatedAt = newID(), src.ProjectID, s.now()
	rel.SourceKey, rel.SourceTitle, rel.TargetKey, rel.TargetTitle = src.Key, src.Title, dst.Key, dst.Title
	_, err = s.c(ctx).exec(ctx, `INSERT INTO requirement_relations (id, tenant_id, project_id, source_id, target_id, type, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rel.ID, tenantID, rel.ProjectID, rel.SourceID, rel.TargetID, rel.Type, rel.CreatedBy, rel.CreatedAt)
	return mapErr(err, "relation")
}

// refinesEdges returns child → parents for all refines relations.
func (s *Store) refinesEdges(ctx context.Context, projectID string) (map[string][]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT source_id, target_id FROM requirement_relations WHERE project_id = ? AND type = 'refines'`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[a] = append(out[a], b)
	}
	return out, rows.Err()
}

// ListRelations returns relations of a project, or of one requirement
// (both directions, with Direction set) when requirementID is given.
func (s *Store) ListRelations(ctx context.Context, tenantID, projectID, requirementID string) ([]model.Relation, error) {
	q := relationSelect + ` WHERE r.tenant_id = ?`
	args := []any{tenantID}
	if projectID != "" {
		q += ` AND r.project_id = ?`
		args = append(args, projectID)
	}
	if requirementID != "" {
		q += ` AND (r.source_id = ? OR r.target_id = ?)`
		args = append(args, requirementID, requirementID)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY r.type, s.seq, t.seq`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Relation{}
	for rows.Next() {
		rel, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}
		if requirementID != "" {
			rel.Direction = "outgoing"
			if rel.TargetID == requirementID {
				rel.Direction = "incoming"
			}
		}
		out = append(out, *rel)
	}
	return out, rows.Err()
}

// GetRelation returns one relation.
func (s *Store) GetRelation(ctx context.Context, tenantID, id string) (*model.Relation, error) {
	rel, err := scanRelation(s.c(ctx).row(ctx, relationSelect+` WHERE r.id = ? AND r.tenant_id = ?`, id, tenantID))
	return rel, mapErr(err, "relation")
}

// DeleteRelation removes a relation.
func (s *Store) DeleteRelation(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM requirement_relations WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "relation")
	}
	return nil
}

// ---------------------------------------------------------------- baselines

// CreateBaseline freezes the given requirements and tests.
func (s *Store) CreateBaseline(ctx context.Context, tenantID string, b *model.Baseline, items []model.BaselineItem) error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len(b.Name) > 200 {
		return Invalidf("baseline name is required (max 200 characters)")
	}
	if _, err := s.GetProject(ctx, tenantID, b.ProjectID); err != nil {
		return err
	}
	b.ID, b.CreatedAt = newID(), s.now()
	b.Requirements, b.Tests = 0, 0
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `INSERT INTO baselines (id, tenant_id, project_id, name, description, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			b.ID, tenantID, b.ProjectID, b.Name, b.Description, b.CreatedBy, b.CreatedAt); err != nil {
			return mapErr(err, "baseline name")
		}
		for _, it := range items {
			if _, err := c.exec(ctx, `INSERT INTO baseline_items (baseline_id, entity_type, entity_id, key, title, version, snapshot) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				b.ID, it.EntityType, it.EntityID, it.Key, it.Title, it.Version, string(it.Snapshot)); err != nil {
				return err
			}
			if it.EntityType == model.KindRequirement {
				b.Requirements++
			} else {
				b.Tests++
			}
		}
		return nil
	})
}

const baselineSelect = `SELECT b.id, b.project_id, b.name, b.description, b.created_by, b.created_at,
	(SELECT COUNT(*) FROM baseline_items i WHERE i.baseline_id = b.id AND i.entity_type = 'requirement'),
	(SELECT COUNT(*) FROM baseline_items i WHERE i.baseline_id = b.id AND i.entity_type = 'test')
	FROM baselines b`

func scanBaseline(r interface{ Scan(...any) error }) (*model.Baseline, error) {
	var b model.Baseline
	if err := r.Scan(&b.ID, &b.ProjectID, &b.Name, &b.Description, &b.CreatedBy, &b.CreatedAt, &b.Requirements, &b.Tests); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBaselines returns the baselines of a project, newest first.
func (s *Store) ListBaselines(ctx context.Context, tenantID, projectID string) ([]model.Baseline, error) {
	rows, err := s.c(ctx).query(ctx, baselineSelect+` WHERE b.tenant_id = ? AND b.project_id = ? ORDER BY b.created_at DESC`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Baseline{}
	for rows.Next() {
		b, err := scanBaseline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// GetBaseline returns a baseline with its items.
func (s *Store) GetBaseline(ctx context.Context, tenantID, id string) (*model.Baseline, error) {
	b, err := scanBaseline(s.c(ctx).row(ctx, baselineSelect+` WHERE b.id = ? AND b.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "baseline")
	}
	rows, err := s.c(ctx).query(ctx, `SELECT entity_type, entity_id, key, title, version, snapshot FROM baseline_items WHERE baseline_id = ? ORDER BY entity_type, key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	b.Items = []model.BaselineItem{}
	for rows.Next() {
		var it model.BaselineItem
		var snap string
		if err := rows.Scan(&it.EntityType, &it.EntityID, &it.Key, &it.Title, &it.Version, &snap); err != nil {
			return nil, err
		}
		it.Snapshot = json.RawMessage(snap)
		b.Items = append(b.Items, it)
	}
	sortItems(b.Items)
	return b, rows.Err()
}

// sortItems orders requirements before tests and keys naturally
// (REQ-2 before REQ-10).
func sortItems(items []model.BaselineItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].EntityType != items[j].EntityType {
			return items[i].EntityType == model.KindRequirement
		}
		return naturalLess(items[i].Key, items[j].Key)
	})
}

func naturalLess(a, b string) bool {
	pa, na := splitKey(a)
	pb, nb := splitKey(b)
	if pa != pb {
		return pa < pb
	}
	return na < nb
}

func splitKey(k string) (string, int) {
	i := len(k)
	for i > 0 && k[i-1] >= '0' && k[i-1] <= '9' {
		i--
	}
	n := 0
	for _, c := range k[i:] {
		n = n*10 + int(c-'0')
		if n > 1<<30 {
			break
		}
	}
	return k[:i], n
}

// DeleteBaseline removes a baseline.
func (s *Store) DeleteBaseline(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM baselines WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "baseline")
	}
	return nil
}

// CompareItems diffs two item sets (from → to).
func CompareItems(fromLabel, toLabel string, from, to []model.BaselineItem) model.BaselineDiff {
	d := model.BaselineDiff{From: fromLabel, To: toLabel, Added: []model.BaselineChange{}, Removed: []model.BaselineChange{}, Changed: []model.BaselineChange{}}
	key := func(it model.BaselineItem) string { return it.EntityType + "/" + it.EntityID }
	old := map[string]model.BaselineItem{}
	for _, it := range from {
		old[key(it)] = it
	}
	seen := map[string]bool{}
	sortItems(to)
	for _, it := range to {
		k := key(it)
		seen[k] = true
		o, ok := old[k]
		if !ok {
			d.Added = append(d.Added, model.BaselineChange{EntityType: it.EntityType, EntityID: it.EntityID, Key: it.Key, Title: it.Title, ToVersion: it.Version})
			continue
		}
		fields := ChangedFields(o.Snapshot, it.Snapshot)
		if len(fields) == 0 {
			d.Unchanged++
			continue
		}
		d.Changed = append(d.Changed, model.BaselineChange{EntityType: it.EntityType, EntityID: it.EntityID, Key: it.Key, Title: it.Title,
			FromVersion: o.Version, ToVersion: it.Version, ChangedFields: fields})
	}
	sortItems(from)
	for _, it := range from {
		if !seen[key(it)] {
			d.Removed = append(d.Removed, model.BaselineChange{EntityType: it.EntityType, EntityID: it.EntityID, Key: it.Key, Title: it.Title, FromVersion: it.Version})
		}
	}
	return d
}

// ---------------------------------------------------------------- comments

// MaxCommentLength bounds comment bodies (in characters).
const MaxCommentLength = 10000

func validComment(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > MaxCommentLength {
		return "", Invalidf("comment must be 1-%d characters", MaxCommentLength)
	}
	return body, nil
}

const commentSelect = `SELECT c.id, c.entity_type, c.entity_id, c.user_id, COALESCE(u.email, ''), COALESCE(u.display_name, ''), COALESCE(u.avatar, ''), c.body, c.created_at, c.updated_at
	FROM comments c LEFT JOIN users u ON u.id = c.user_id`

func scanComment(r interface{ Scan(...any) error }) (*model.Comment, error) {
	var c model.Comment
	if err := r.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.UserID, &c.UserEmail, &c.UserName, &c.UserAvatar, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// AddComment stores a comment (the entity must have been checked by the caller).
func (s *Store) AddComment(ctx context.Context, tenantID string, c *model.Comment) error {
	body, err := validComment(c.Body)
	if err != nil {
		return err
	}
	c.ID, c.Body, c.CreatedAt = newID(), body, s.now()
	c.UpdatedAt = c.CreatedAt
	_, err = s.c(ctx).exec(ctx, `INSERT INTO comments (id, tenant_id, entity_type, entity_id, user_id, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, tenantID, c.EntityType, c.EntityID, c.UserID, c.Body, c.CreatedAt, c.UpdatedAt)
	return err
}

// ListComments returns the comments of an entity, oldest first.
func (s *Store) ListComments(ctx context.Context, tenantID, entityType, entityID string) ([]model.Comment, error) {
	rows, err := s.c(ctx).query(ctx, commentSelect+` WHERE c.tenant_id = ? AND c.entity_type = ? AND c.entity_id = ? ORDER BY c.created_at`, tenantID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetComment returns one comment.
func (s *Store) GetComment(ctx context.Context, tenantID, id string) (*model.Comment, error) {
	c, err := scanComment(s.c(ctx).row(ctx, commentSelect+` WHERE c.id = ? AND c.tenant_id = ?`, id, tenantID))
	return c, mapErr(err, "comment")
}

// UpdateComment changes the body of a comment.
func (s *Store) UpdateComment(ctx context.Context, tenantID string, c *model.Comment) error {
	body, err := validComment(c.Body)
	if err != nil {
		return err
	}
	c.Body, c.UpdatedAt = body, s.now()
	_, err = s.c(ctx).exec(ctx, `UPDATE comments SET body = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, c.Body, c.UpdatedAt, c.ID, tenantID)
	return err
}

// DeleteComment removes a comment.
func (s *Store) DeleteComment(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM comments WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "comment")
	}
	return nil
}

// ProjectCounts returns per-item comment counts and relation counts for a
// project (entity id → count).
func (s *Store) ProjectCounts(ctx context.Context, tenantID, projectID string) (comments, relations map[string]int, err error) {
	comments, relations = map[string]int{}, map[string]int{}
	rows, err := s.c(ctx).query(ctx, `SELECT c.entity_id, COUNT(*) FROM comments c WHERE c.tenant_id = ? AND (
		c.entity_id IN (SELECT id FROM requirements WHERE project_id = ?) OR c.entity_id IN (SELECT id FROM test_cases WHERE project_id = ?))
		GROUP BY c.entity_id`, tenantID, projectID, projectID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return nil, nil, err
		}
		comments[id] = n
	}
	rows.Close()
	rows, err = s.c(ctx).query(ctx, `SELECT source_id, target_id FROM requirement_relations WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, nil, err
		}
		relations[a]++
		relations[b]++
	}
	return comments, relations, rows.Err()
}

// TestDefects returns test id → defects recorded on any of its executions.
func (s *Store) TestDefects(ctx context.Context, tenantID, projectID string) (map[string][]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT e.test_case_id, e.defects FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE r.project_id = ? AND e.tenant_id = ? AND e.defects <> '[]'`, projectID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var id, d string
		if err := rows.Scan(&id, &d); err != nil {
			return nil, err
		}
		out[id] = cleanTags(append(out[id], decodeTags(d)...))
	}
	return out, rows.Err()
}

// deleteEntityData removes comments and revisions of deleted items (these
// tables reference entities polymorphically, without foreign keys).
func deleteEntityData(ctx context.Context, c conn, entityType string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := removeAttachments(ctx, c, entityType, ids); err != nil {
		return err
	}
	if entityType == model.KindTest {
		// executions of deleted tests are removed by the FK cascade
		rows, err := c.query(ctx, `SELECT id FROM test_executions WHERE test_case_id IN (`+placeholders(len(ids))+`)`, anySlice(ids)...)
		if err != nil {
			return err
		}
		execs, err := scanStrings(rows)
		if err != nil {
			return err
		}
		if err := removeExecutionAttachments(ctx, c, execs); err != nil {
			return err
		}
		if err := deleteDefectLinks(ctx, c, "execution", execs); err != nil {
			return err
		}
	}
	if _, err := c.exec(ctx, `DELETE FROM review_requests WHERE entity_type = ? AND entity_id IN (`+placeholders(len(ids))+`)`, append([]any{entityType}, anySlice(ids)...)...); err != nil {
		return err
	}
	if err := deleteItemLinks(ctx, c, entityType, ids); err != nil {
		return err
	}
	if err := deleteDefectLinks(ctx, c, entityType, ids); err != nil {
		return err
	}
	for _, tbl := range []string{"comments", "revisions"} {
		args := append([]any{entityType}, anySlice(ids)...)
		if _, err := c.exec(ctx, `DELETE FROM `+tbl+` WHERE entity_type = ? AND entity_id IN (`+placeholders(len(ids))+`)`, args...); err != nil {
			return err
		}
	}
	return nil
}
