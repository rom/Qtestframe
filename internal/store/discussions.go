package store

// Discussions: threaded comments on requirements, tests, defects, risks,
// test runs and review requests. A thread is a first comment (parent_id
// '') and its replies (parent_id = the first comment; one level). A thread
// is resolved on its first comment. Deleting a comment keeps it in the
// thread as removed (body and mentions cleared). Mentions are stored as the
// user ids found in the body (the API resolves the @handles).

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// MaxCommentLength bounds comment bodies (in characters).
const MaxCommentLength = 10000

func validComment(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > MaxCommentLength {
		return "", Invalidf("comment must be 1-%d characters", MaxCommentLength)
	}
	return body, nil
}

const commentSelect = `SELECT c.id, COALESCE(c.project_id, ''), c.entity_type, c.entity_id, c.parent_id, c.user_id,
	COALESCE(u.email, ''), COALESCE(u.display_name, ''), COALESCE(u.avatar, ''), c.body, c.created_at, c.updated_at,
	c.resolved_at, c.resolved_by, COALESCE(NULLIF(ru.display_name, ''), ru.email, ''), c.deleted_at, c.deleted_by, c.mentions, c.reactions
	FROM comments c LEFT JOIN users u ON u.id = c.user_id LEFT JOIN users ru ON ru.id = c.resolved_by`

func scanComment(r interface{ Scan(...any) error }) (*model.Comment, error) {
	var c model.Comment
	var resolved, deleted sql.NullTime
	var mentions, reactions string
	if err := r.Scan(&c.ID, &c.ProjectID, &c.EntityType, &c.EntityID, &c.ParentID, &c.UserID, &c.UserEmail, &c.UserName, &c.UserAvatar,
		&c.Body, &c.CreatedAt, &c.UpdatedAt, &resolved, &c.ResolvedBy, &c.ResolvedByName, &deleted, &c.DeletedBy, &mentions, &reactions); err != nil {
		return nil, err
	}
	c.ResolvedAt, c.DeletedAt = timePtr(resolved), timePtr(deleted)
	c.Deleted = c.DeletedAt != nil
	_ = json.Unmarshal([]byte(mentions), &c.Mentions)
	if c.Mentions == nil {
		c.Mentions = []string{}
	}
	_ = json.Unmarshal([]byte(reactions), &c.Reactions)
	if c.Reactions == nil {
		c.Reactions = map[string][]string{}
	}
	return &c, nil
}

func scanComments(rows *sql.Rows) ([]model.Comment, error) {
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

func jsonList(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

// AddComment stores a comment. The caller has checked the entity and sets
// ProjectID; a ParentID must be a comment of the same entity (a reply to a
// reply joins the thread of its parent).
func (s *Store) AddComment(ctx context.Context, tenantID string, c *model.Comment) error {
	body, err := validComment(c.Body)
	if err != nil {
		return err
	}
	if !slices.Contains(model.DiscussionKinds, c.EntityType) {
		return Invalidf("comments cannot be added to %q", c.EntityType)
	}
	return s.tx(ctx, func(cn conn) error {
		if c.ParentID != "" {
			var parent, et, eid string
			var deleted sql.NullTime
			err := cn.row(ctx, `SELECT parent_id, entity_type, entity_id, deleted_at FROM comments WHERE id = ? AND tenant_id = ?`, c.ParentID, tenantID).
				Scan(&parent, &et, &eid, &deleted)
			if err != nil {
				return mapErr(err, "comment")
			}
			if et != c.EntityType || eid != c.EntityID {
				return Invalidf("the comment replied to belongs to another item")
			}
			if parent != "" {
				c.ParentID = parent
			}
		}
		c.ID, c.Body, c.CreatedAt = newID(), body, s.now()
		c.UpdatedAt = c.CreatedAt
		if c.Mentions == nil {
			c.Mentions = []string{}
		}
		c.Reactions = map[string][]string{}
		var pid any
		if c.ProjectID != "" {
			pid = c.ProjectID
		}
		_, err := cn.exec(ctx, `INSERT INTO comments (id, tenant_id, project_id, entity_type, entity_id, parent_id, user_id, body, created_at, updated_at, mentions)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, tenantID, pid, c.EntityType, c.EntityID, c.ParentID, c.UserID, c.Body, c.CreatedAt, c.UpdatedAt, jsonList(c.Mentions))
		return err
	})
}

// ListComments returns the comments of an entity, oldest first (removed
// ones included, with an empty body).
func (s *Store) ListComments(ctx context.Context, tenantID, entityType, entityID string) ([]model.Comment, error) {
	rows, err := s.c(ctx).query(ctx, commentSelect+` WHERE c.tenant_id = ? AND c.entity_type = ? AND c.entity_id = ? ORDER BY c.created_at, c.id`, tenantID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	return scanComments(rows)
}

// GetComment returns one comment.
func (s *Store) GetComment(ctx context.Context, tenantID, id string) (*model.Comment, error) {
	c, err := scanComment(s.c(ctx).row(ctx, commentSelect+` WHERE c.id = ? AND c.tenant_id = ?`, id, tenantID))
	return c, mapErr(err, "comment")
}

// UpdateComment changes the body (and mentions) of a comment that is not
// removed.
func (s *Store) UpdateComment(ctx context.Context, tenantID string, c *model.Comment) error {
	body, err := validComment(c.Body)
	if err != nil {
		return err
	}
	if c.Deleted {
		return Conflictf("a removed comment cannot be edited")
	}
	if c.Mentions == nil {
		c.Mentions = []string{}
	}
	c.Body, c.UpdatedAt = body, s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE comments SET body = ?, mentions = ?, updated_at = ? WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`,
		c.Body, jsonList(c.Mentions), c.UpdatedAt, c.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "comment")
	}
	return nil
}

// RemoveComment marks a comment removed (soft delete): it stays in its
// thread, its body, mentions and reactions are cleared.
func (s *Store) RemoveComment(ctx context.Context, tenantID, id, by string) error {
	now := s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE comments SET body = '', mentions = '[]', reactions = '{}', deleted_at = ?, deleted_by = ? WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`,
		now, by, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "comment")
	}
	return nil
}

// DeleteComment removes a comment row for good (with its replies when it
// starts a thread).
func (s *Store) DeleteComment(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM comments WHERE (id = ? OR parent_id = ?) AND tenant_id = ?`, id, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "comment")
	}
	return nil
}

// ResolveThread resolves (resolved true) or reopens the thread that a
// comment starts; userID is who resolves it.
func (s *Store) ResolveThread(ctx context.Context, tenantID, id, userID string, resolved bool) error {
	var res sql.Result
	var err error
	if resolved {
		res, err = s.c(ctx).exec(ctx, `UPDATE comments SET resolved_at = ?, resolved_by = ? WHERE id = ? AND tenant_id = ? AND parent_id = ''`, s.now(), userID, id, tenantID)
	} else {
		res, err = s.c(ctx).exec(ctx, `UPDATE comments SET resolved_at = NULL, resolved_by = '' WHERE id = ? AND tenant_id = ? AND parent_id = ''`, id, tenantID)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "thread")
	}
	return nil
}

// ToggleReaction adds the user's reaction to a comment, or takes it back
// when it is there; it returns the comment's reactions.
func (s *Store) ToggleReaction(ctx context.Context, tenantID, id, userID, reaction string) (map[string][]string, error) {
	if !slices.Contains(model.CommentReactions, reaction) {
		return nil, Invalidf("reaction must be one of %s", strings.Join(model.CommentReactions, ", "))
	}
	var out map[string][]string
	err := s.tx(ctx, func(c conn) error {
		var raw string
		var deleted sql.NullTime
		if err := c.row(ctx, `SELECT reactions, deleted_at FROM comments WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&raw, &deleted); err != nil {
			return mapErr(err, "comment")
		}
		if deleted.Valid {
			return Conflictf("a removed comment cannot get reactions")
		}
		m := map[string][]string{}
		_ = json.Unmarshal([]byte(raw), &m)
		if i := slices.Index(m[reaction], userID); i >= 0 {
			m[reaction] = slices.Delete(m[reaction], i, i+1)
			if len(m[reaction]) == 0 {
				delete(m, reaction)
			}
		} else {
			m[reaction] = append(m[reaction], userID)
		}
		b, _ := json.Marshal(m)
		out = m
		_, err := c.exec(ctx, `UPDATE comments SET reactions = ? WHERE id = ? AND tenant_id = ?`, string(b), id, tenantID)
		return err
	})
	return out, err
}

// deleteComments removes the discussions of deleted entities.
func deleteComments(ctx context.Context, c conn, entityType string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := c.exec(ctx, `DELETE FROM comments WHERE entity_type = ? AND entity_id IN (`+placeholders(len(ids))+`)`, append([]any{entityType}, anySlice(ids)...)...)
	return err
}

// ---------------------------------------------------------------- summaries

// DiscussionStats summarises the discussion of one item.
type DiscussionStats struct {
	// Comments that are not removed.
	Comments int
	// Open threads: not resolved, with at least one comment not removed.
	Open int
	// Mentioned: user ids mentioned in its comments (not removed).
	Mentioned []string
	// OpenMentioned: user ids mentioned in its open threads.
	OpenMentioned []string
}

// threadRow is one comment for DiscussionIndex.
type threadRow struct {
	id, entityID, parentID string
	resolved, deleted      bool
	mentions               []string
}

// DiscussionIndex returns the discussion summary of every item of a
// project that has comments (entity id → stats).
func (s *Store) DiscussionIndex(ctx context.Context, tenantID, projectID string) (map[string]*DiscussionStats, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, entity_id, parent_id, resolved_at IS NOT NULL, deleted_at IS NOT NULL, mentions FROM comments
		WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []threadRow
	for rows.Next() {
		var t threadRow
		var raw string
		if err := rows.Scan(&t.id, &t.entityID, &t.parentID, &t.resolved, &t.deleted, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &t.mentions)
		list = append(list, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return discussionIndex(list), nil
}

func discussionIndex(list []threadRow) map[string]*DiscussionStats {
	type thread struct {
		entity     string
		resolved   bool
		live       bool
		mentioned  []string
		rootLoaded bool
	}
	threads := map[string]*thread{}
	get := func(id string) *thread {
		if threads[id] == nil {
			threads[id] = &thread{}
		}
		return threads[id]
	}
	out := map[string]*DiscussionStats{}
	stats := func(entity string) *DiscussionStats {
		if out[entity] == nil {
			out[entity] = &DiscussionStats{}
		}
		return out[entity]
	}
	for _, r := range list {
		root := r.id
		if r.parentID != "" {
			root = r.parentID
		}
		th := get(root)
		th.entity = r.entityID
		if r.parentID == "" {
			th.resolved, th.rootLoaded = r.resolved, true
		}
		if !r.deleted {
			th.live = true
			st := stats(r.entityID)
			st.Comments++
			st.Mentioned = appendNew(st.Mentioned, r.mentions...)
			th.mentioned = appendNew(th.mentioned, r.mentions...)
		}
	}
	for _, th := range threads {
		if th.rootLoaded && th.live && !th.resolved {
			st := stats(th.entity)
			st.Open++
			st.OpenMentioned = appendNew(st.OpenMentioned, th.mentioned...)
		}
	}
	return out
}

func appendNew(list []string, vals ...string) []string {
	for _, v := range vals {
		if v != "" && !slices.Contains(list, v) {
			list = append(list, v)
		}
	}
	return list
}

// ---------------------------------------------------------------- thread lists

// Thread statuses of DiscussionFilter.
const (
	ThreadsOpen     = "open"
	ThreadsResolved = "resolved"
	ThreadsAll      = "all"
)

// DiscussionFilter selects threads for ListDiscussionThreads.
type DiscussionFilter struct {
	// ProjectIDs limits the threads to these projects (nil: every project
	// of the tenant; an empty non-nil list: none).
	ProjectIDs []string
	// Status: open (default), resolved or all.
	Status string
	// EntityType limits the threads to one kind of item.
	EntityType string
	// Participant: threads the user wrote in.
	Participant string
	// Mentioned: threads that mention the user.
	Mentioned string
	// Text: threads with a comment containing it (case-insensitive).
	Text string
	// Limit bounds the result (default and maximum 500).
	Limit int
}

// ListDiscussionThreads lists threads, the latest activity first.
func (s *Store) ListDiscussionThreads(ctx context.Context, tenantID string, f DiscussionFilter) ([]model.DiscussionThread, error) {
	if f.ProjectIDs != nil && len(f.ProjectIDs) == 0 {
		return []model.DiscussionThread{}, nil
	}
	if f.Status == "" {
		f.Status = ThreadsOpen
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 500
	}
	where := []string{"tenant_id = ?", "project_id IS NOT NULL", "deleted_at IS NULL"}
	args := []any{tenantID}
	if f.ProjectIDs != nil {
		where = append(where, "project_id IN ("+placeholders(len(f.ProjectIDs))+")")
		args = append(args, anySlice(f.ProjectIDs)...)
	}
	if f.EntityType != "" {
		where = append(where, "entity_type = ?")
		args = append(args, f.EntityType)
	}
	if f.Participant != "" {
		where = append(where, "user_id = ?")
		args = append(args, f.Participant)
	}
	if f.Mentioned != "" {
		where = append(where, "mentions LIKE ?")
		args = append(args, `%"`+f.Mentioned+`"%`)
	}
	if f.Text != "" {
		where = append(where, "LOWER(body) LIKE ?")
		args = append(args, "%"+strings.ToLower(f.Text)+"%")
	}
	rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT CASE WHEN parent_id = '' THEN id ELSE parent_id END FROM comments WHERE `+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, err
	}
	roots, err := scanStrings(rows)
	if err != nil {
		return nil, err
	}
	threads, err := s.loadThreads(ctx, tenantID, roots)
	if err != nil {
		return nil, err
	}
	out := threads[:0]
	for _, t := range threads {
		open := t.Root.ResolvedAt == nil
		if (f.Status == ThreadsOpen && !open) || (f.Status == ThreadsResolved && open) {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastAt.Equal(out[j].LastAt) {
			return out[i].LastAt.After(out[j].LastAt)
		}
		return out[i].Root.ID < out[j].Root.ID
	})
	if len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// OpenThreadsMentioning lists the unresolved threads that mention a user,
// in the given projects (nil: all of the tenant), the latest activity
// first. It is the "mentioned in a discussion" source of a personal work
// list.
func (s *Store) OpenThreadsMentioning(ctx context.Context, tenantID, userID string, projectIDs []string) ([]model.DiscussionThread, error) {
	return s.ListDiscussionThreads(ctx, tenantID, DiscussionFilter{ProjectIDs: projectIDs, Status: ThreadsOpen, Mentioned: userID})
}

// loadThreads builds the threads of the given first comments (threads
// without a comment that is not removed are left out).
func (s *Store) loadThreads(ctx context.Context, tenantID string, roots []string) ([]model.DiscussionThread, error) {
	byRoot := map[string][]model.Comment{}
	for start := 0; start < len(roots); start += 400 {
		part := roots[start:min(start+400, len(roots))]
		in := placeholders(len(part))
		rows, err := s.c(ctx).query(ctx, commentSelect+` WHERE c.tenant_id = ? AND (c.id IN (`+in+`) OR c.parent_id IN (`+in+`)) ORDER BY c.created_at, c.id`,
			append(append([]any{tenantID}, anySlice(part)...), anySlice(part)...)...)
		if err != nil {
			return nil, err
		}
		list, err := scanComments(rows)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			root := c.ID
			if c.ParentID != "" {
				root = c.ParentID
			}
			byRoot[root] = append(byRoot[root], c)
		}
	}
	var out []model.DiscussionThread
	for _, id := range roots {
		list := byRoot[id]
		var t model.DiscussionThread
		found, live := false, false
		for _, c := range list {
			if c.ID == id {
				t.Root, found = c, true
			} else {
				t.Replies++
			}
			if c.Deleted {
				continue
			}
			live = true
			if !c.CreatedAt.Before(t.LastAt) {
				t.LastAt = c.CreatedAt
				t.LastBy = c.UserName
				if t.LastBy == "" {
					t.LastBy = c.UserEmail
				}
			}
			t.Participants = appendNew(t.Participants, c.UserID)
			t.Mentioned = appendNew(t.Mentioned, c.Mentions...)
		}
		if !found || !live {
			continue
		}
		if t.Participants == nil {
			t.Participants = []string{}
		}
		if t.Mentioned == nil {
			t.Mentioned = []string{}
		}
		out = append(out, t)
	}
	if err := s.fillThreadEntities(ctx, tenantID, out); err != nil {
		return nil, err
	}
	return out, nil
}

// entityLabelQueries look up the project key, key and title of discussed
// items per entity type (one ? for the ids).
var entityLabelQueries = map[string]string{
	model.KindRequirement: `SELECT x.id, p.key, x.key, x.title FROM requirements x JOIN projects p ON p.id = x.project_id WHERE x.id IN (%s)`,
	model.KindTest:        `SELECT x.id, p.key, x.key, x.title FROM test_cases x JOIN projects p ON p.id = x.project_id WHERE x.id IN (%s)`,
	model.KindDefect:      `SELECT x.id, p.key, x.key, x.title FROM defects x JOIN projects p ON p.id = x.project_id WHERE x.id IN (%s)`,
	model.KindRisk:        `SELECT x.id, p.key, x.key, x.title FROM risks x JOIN projects p ON p.id = x.project_id WHERE x.id IN (%s)`,
	model.DiscussionRun:   `SELECT x.id, p.key, '', x.name FROM test_runs x JOIN projects p ON p.id = x.project_id WHERE x.id IN (%s)`,
	model.DiscussionReview: `SELECT x.id, p.key, COALESCE(r.key, t.key, ''), COALESCE(r.title, t.title, '') FROM review_requests x JOIN projects p ON p.id = x.project_id
		LEFT JOIN requirements r ON x.entity_type = 'requirement' AND r.id = x.entity_id LEFT JOIN test_cases t ON x.entity_type = 'test' AND t.id = x.entity_id WHERE x.id IN (%s)`,
}

// EntityLabel is the project key, key and title of a discussed item.
type EntityLabel struct{ ProjectKey, Key, Title string }

// EntityLabels returns the labels of items of one entity type (id → label).
func (s *Store) EntityLabels(ctx context.Context, entityType string, ids []string) (map[string]EntityLabel, error) {
	out := map[string]EntityLabel{}
	q, ok := entityLabelQueries[entityType]
	if !ok || len(ids) == 0 {
		return out, nil
	}
	for start := 0; start < len(ids); start += 400 {
		part := ids[start:min(start+400, len(ids))]
		rows, err := s.c(ctx).query(ctx, strings.Replace(q, "%s", placeholders(len(part)), 1), anySlice(part)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var l EntityLabel
			if err := rows.Scan(&id, &l.ProjectKey, &l.Key, &l.Title); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = l
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) fillThreadEntities(ctx context.Context, tenantID string, threads []model.DiscussionThread) error {
	ids := map[string][]string{}
	for _, t := range threads {
		ids[t.Root.EntityType] = append(ids[t.Root.EntityType], t.Root.EntityID)
	}
	for et, list := range ids {
		labels, err := s.EntityLabels(ctx, et, list)
		if err != nil {
			return err
		}
		for i := range threads {
			if threads[i].Root.EntityType == et {
				l := labels[threads[i].Root.EntityID]
				threads[i].ProjectKey, threads[i].EntityKey, threads[i].EntityTitle = l.ProjectKey, l.Key, l.Title
			}
		}
	}
	return nil
}

// ThreadComments returns the comments of the thread that a comment belongs
// to (first comment first).
func (s *Store) ThreadComments(ctx context.Context, tenantID, id string) ([]model.Comment, error) {
	c, err := s.GetComment(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	root := c.ID
	if c.ParentID != "" {
		root = c.ParentID
	}
	rows, err := s.c(ctx).query(ctx, commentSelect+` WHERE c.tenant_id = ? AND (c.id = ? OR c.parent_id = ?) ORDER BY c.parent_id <> '', c.created_at, c.id`, tenantID, root, root)
	if err != nil {
		return nil, err
	}
	return scanComments(rows)
}

// ProjectDiscussions returns the comments (not removed) of the given items
// of one entity type, oldest first (entity id → comments); for generated
// documents.
func (s *Store) ProjectDiscussions(ctx context.Context, tenantID, entityType string, ids []string) (map[string][]model.Comment, error) {
	out := map[string][]model.Comment{}
	for start := 0; start < len(ids); start += 400 {
		part := ids[start:min(start+400, len(ids))]
		rows, err := s.c(ctx).query(ctx, commentSelect+` WHERE c.tenant_id = ? AND c.entity_type = ? AND c.deleted_at IS NULL AND c.entity_id IN (`+placeholders(len(part))+`) ORDER BY c.created_at, c.id`,
			append([]any{tenantID, entityType}, anySlice(part)...)...)
		if err != nil {
			return nil, err
		}
		list, err := scanComments(rows)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			out[c.EntityID] = append(out[c.EntityID], c)
		}
	}
	return out, nil
}
