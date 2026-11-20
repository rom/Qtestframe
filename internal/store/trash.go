package store

// Trash: deleting requirements or tests keeps a snapshot of the deleted rows
// (the items and everything that hangs off them: criteria, steps, categories,
// trace links, relations, dependencies, execution results, comments,
// history and attachments) so the deletion can be undone. Restoring puts the
// rows back with their original IDs and keys; rows whose counterpart has
// been deleted in the meantime (a linked test, a run, a category) are left
// out, and a folder that no longer exists puts the item at the folder root.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// TrashRetention is how long deleted items can be restored.
const TrashRetention = 30 * 24 * time.Hour

// TrashEntry describes one deletion that can be undone.
type TrashEntry struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Kind      string    `json:"kind"`
	ItemIDs   []string  `json:"item_ids"`
	Keys      []string  `json:"keys"`
	Label     string    `json:"label"`
	DeletedBy string    `json:"deleted_by"`
	CreatedAt time.Time `json:"created_at"`
}

// trashRows is a snapshot of rows of one table.
type trashRows struct {
	Table string           `json:"table"`
	Rows  []map[string]any `json:"rows"`
}

// trashRef is a reference from a surviving row to a deleted item that the
// deletion cleared (ON DELETE SET NULL) and a restore sets again.
type trashRef struct {
	Table  string `json:"table"`
	ID     string `json:"id"`
	Column string `json:"column"`
	Value  string `json:"value"`
}

type trashSnapshot struct {
	Sets []trashRows `json:"sets"`
	Refs []trashRef  `json:"refs,omitempty"`
}

// TrashRequirements deletes requirements like DeleteRequirements and keeps
// them in the trash. It returns the number deleted and the trash entry ID
// ("" when nothing was deleted).
func (s *Store) TrashRequirements(ctx context.Context, tenantID, projectID string, ids []string, deletedBy string) (int, string, error) {
	return s.trashItems(ctx, model.KindRequirement, tenantID, projectID, ids, deletedBy)
}

// TrashTests deletes tests like DeleteTests and keeps them in the trash.
func (s *Store) TrashTests(ctx context.Context, tenantID, projectID string, ids []string, deletedBy string) (int, string, error) {
	return s.trashItems(ctx, model.KindTest, tenantID, projectID, ids, deletedBy)
}

func itemTable(kind string) string {
	if kind == model.KindTest {
		return "test_cases"
	}
	return "requirements"
}

func (s *Store) trashItems(ctx context.Context, kind, tenantID, projectID string, ids []string, deletedBy string) (int, string, error) {
	if len(ids) == 0 {
		return 0, "", nil
	}
	table := itemTable(kind)
	where := ` WHERE tenant_id = ? AND id IN (` + placeholders(len(ids)) + `)`
	args := append([]any{tenantID}, anySlice(ids)...)
	if projectID != "" {
		where += ` AND project_id = ?`
		args = append(args, projectID)
	}
	n := 0
	trashID := ""
	err := s.tx(ctx, func(c conn) error {
		rows, err := c.query(ctx, `SELECT id, project_id, key, title FROM `+table+where+` ORDER BY seq`, args...)
		if err != nil {
			return err
		}
		var found, keys, titles []string
		project := ""
		for rows.Next() {
			var id, pid, key, title string
			if err := rows.Scan(&id, &pid, &key, &title); err != nil {
				rows.Close()
				return err
			}
			if project == "" {
				project = pid
			}
			if pid != project { // one entry per project (never happens through the API)
				continue
			}
			found = append(found, id)
			keys = append(keys, key)
			titles = append(titles, title)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(found) == 0 {
			return nil
		}
		snap, err := snapshotItems(ctx, c, kind, found)
		if err != nil {
			return err
		}
		data, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		label := keys[0] + " " + titles[0]
		if len(keys) > 1 {
			label = strings.Join(keys, ", ")
		}
		if len(label) > 500 {
			label = label[:500]
		}
		now := s.now()
		if _, err := c.exec(ctx, `DELETE FROM trash WHERE tenant_id = ? AND created_at < ?`, tenantID, now.Add(-TrashRetention)); err != nil {
			return err
		}
		trashID = newID()
		if _, err := c.exec(ctx, `INSERT INTO trash (id, tenant_id, project_id, kind, item_ids, keys, label, snapshot, deleted_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			trashID, tenantID, project, kind, encodeTags(found), encodeTags(keys), label, string(data), deletedBy, now); err != nil {
			return err
		}
		if err := deleteEntityData(ctx, c, kind, found); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM `+table+` WHERE tenant_id = ? AND id IN (`+placeholders(len(found))+`)`, append([]any{tenantID}, anySlice(found)...)...)
		if err != nil {
			return err
		}
		m, _ := res.RowsAffected()
		n = int(m)
		return nil
	})
	if n == 0 {
		trashID = ""
	}
	return n, trashID, err
}

// snapshotItems reads every row the deletion of the items removes, parents
// before children (the order a restore inserts them in).
func snapshotItems(ctx context.Context, c conn, kind string, ids []string) (*trashSnapshot, error) {
	snap := &trashSnapshot{}
	add := func(table, where string, args ...any) ([]map[string]any, error) {
		rows, err := selectRows(ctx, c, `SELECT * FROM `+table+` WHERE `+where, args...)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			snap.Sets = append(snap.Sets, trashRows{Table: table, Rows: rows})
		}
		return rows, nil
	}
	in := placeholders(len(ids))
	idArgs := anySlice(ids)
	var attachEntities = map[string][]string{kind: ids}
	if kind == model.KindRequirement {
		if _, err := add("requirements", `id IN (`+in+`)`, idArgs...); err != nil {
			return nil, err
		}
		for _, q := range []struct{ table, where string }{
			{"requirement_categories", `requirement_id IN (` + in + `)`},
			{"evaluation_criteria", `requirement_id IN (` + in + `)`},
			{"trace_links", `requirement_id IN (` + in + `)`},
		} {
			if _, err := add(q.table, q.where, idArgs...); err != nil {
				return nil, err
			}
		}
		if _, err := add("requirement_relations", `source_id IN (`+in+`) OR target_id IN (`+in+`)`, append(idArgs, idArgs...)...); err != nil {
			return nil, err
		}
	} else {
		if _, err := add("test_cases", `id IN (`+in+`)`, idArgs...); err != nil {
			return nil, err
		}
		for _, q := range []struct{ table, where string }{
			{"test_steps", `test_case_id IN (` + in + `)`},
			{"test_case_categories", `test_case_id IN (` + in + `)`},
			{"trace_links", `test_case_id IN (` + in + `)`},
		} {
			if _, err := add(q.table, q.where, idArgs...); err != nil {
				return nil, err
			}
		}
		if _, err := add("test_dependencies", `test_id IN (`+in+`) OR depends_on_id IN (`+in+`)`, append(idArgs, idArgs...)...); err != nil {
			return nil, err
		}
		execs, err := add("test_executions", `test_case_id IN (`+in+`)`, idArgs...)
		if err != nil {
			return nil, err
		}
		if len(execs) > 0 {
			execIDs := rowIDs(execs)
			attachEntities["execution"] = execIDs
			if _, err := add("execution_iterations", `execution_id IN (`+placeholders(len(execIDs))+`)`, anySlice(execIDs)...); err != nil {
				return nil, err
			}
			steps, err := add("step_results", `execution_id IN (`+placeholders(len(execIDs))+`)`, anySlice(execIDs)...)
			if err != nil {
				return nil, err
			}
			if len(steps) > 0 {
				attachEntities["step"] = rowIDs(steps)
			}
		}
		// steps of other tests that call a deleted test lose the call
		rows, err := c.query(ctx, `SELECT id, call_test_id FROM test_steps WHERE call_test_id IN (`+in+`) AND test_case_id NOT IN (`+in+`)`, append(idArgs, idArgs...)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, target string
			if err := rows.Scan(&id, &target); err != nil {
				rows.Close()
				return nil, err
			}
			snap.Refs = append(snap.Refs, trashRef{Table: "test_steps", ID: id, Column: "call_test_id", Value: target})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	for _, tbl := range []string{"comments", "revisions"} {
		if _, err := add(tbl, `entity_type = ? AND entity_id IN (`+in+`)`, append([]any{kind}, idArgs...)...); err != nil {
			return nil, err
		}
	}
	if _, err := add("defect_links", `entity_type = ? AND entity_id IN (`+in+`)`, append([]any{kind}, idArgs...)...); err != nil {
		return nil, err
	}
	if ex := attachEntities["execution"]; len(ex) > 0 {
		if _, err := add("defect_links", `entity_type = 'execution' AND entity_id IN (`+placeholders(len(ex))+`)`, anySlice(ex)...); err != nil {
			return nil, err
		}
	}
	if kind == model.KindRequirement {
		for _, tbl := range []string{"release_requirements", "risk_requirements"} {
			if _, err := add(tbl, `requirement_id IN (`+in+`)`, idArgs...); err != nil {
				return nil, err
			}
		}
	}
	if _, err := add("item_links", `(source_type = ? AND source_id IN (`+in+`)) OR (target_type = ? AND target_id IN (`+in+`))`,
		append(append(append([]any{kind}, idArgs...), kind), idArgs...)...); err != nil {
		return nil, err
	}
	// attachments, with the files (and their contents) the deletion removes
	entityTypes := make([]string, 0, len(attachEntities))
	for et := range attachEntities {
		entityTypes = append(entityTypes, et)
	}
	sort.Strings(entityTypes)
	var links []map[string]any
	for _, et := range entityTypes {
		ents := attachEntities[et]
		rows, err := selectRows(ctx, c, `SELECT * FROM attachments WHERE entity_type = ? AND entity_id IN (`+placeholders(len(ents))+`)`, append([]any{et}, anySlice(ents)...)...)
		if err != nil {
			return nil, err
		}
		links = append(links, rows...)
	}
	if len(links) > 0 {
		seen := map[string]bool{}
		var fileIDs []string
		for _, l := range links {
			if id, _ := l["file_id"].(string); id != "" && !seen[id] {
				seen[id] = true
				fileIDs = append(fileIDs, id)
			}
		}
		if len(fileIDs) > 0 {
			fin := placeholders(len(fileIDs))
			if _, err := add("files", `id IN (`+fin+`)`, anySlice(fileIDs)...); err != nil {
				return nil, err
			}
			if _, err := add("file_blobs", `file_id IN (`+fin+`)`, anySlice(fileIDs)...); err != nil {
				return nil, err
			}
		}
		snap.Sets = append(snap.Sets, trashRows{Table: "attachments", Rows: links})
	}
	return snap, nil
}

func rowIDs(rows []map[string]any) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if id, ok := r["id"].(string); ok {
			out = append(out, id)
		}
	}
	return out
}

// selectRows reads rows as column → value maps.
func selectRows(ctx context.Context, c conn, q string, args ...any) ([]map[string]any, error) {
	rows, err := c.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	table := ""
	if f := strings.Fields(q); len(f) > 3 {
		table = f[3] // SELECT * FROM <table>
	}
	kinds := make([]colKind, len(cols))
	for i, col := range cols {
		kinds[i] = columnKind(table, col, types[i].DatabaseTypeName())
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			m[col] = exportValue(kinds[i], vals[i])
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PurgeTrash deletes trash entries older than TrashRetention (all tenants)
// and returns the count.
func (s *Store) PurgeTrash(ctx context.Context) (int64, error) {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM trash WHERE created_at < ?`, s.now().Add(-TrashRetention))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListTrash lists the restorable deletions of a project, newest first.
func (s *Store) ListTrash(ctx context.Context, tenantID, projectID string) ([]TrashEntry, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, project_id, kind, item_ids, keys, label, deleted_by, created_at FROM trash
		WHERE tenant_id = ? AND project_id = ? AND created_at >= ? ORDER BY created_at DESC LIMIT 200`, tenantID, projectID, s.now().Add(-TrashRetention))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashEntry{}
	for rows.Next() {
		e, err := scanTrash(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanTrash(r scanner) (*TrashEntry, error) {
	var e TrashEntry
	var ids, keys string
	if err := r.Scan(&e.ID, &e.ProjectID, &e.Kind, &ids, &keys, &e.Label, &e.DeletedBy, &e.CreatedAt); err != nil {
		return nil, err
	}
	e.ItemIDs = decodeTags(ids)
	e.Keys = decodeTags(keys)
	return &e, nil
}

// GetTrash returns a trash entry.
func (s *Store) GetTrash(ctx context.Context, tenantID, id string) (*TrashEntry, error) {
	e, err := scanTrash(s.c(ctx).row(ctx, `SELECT id, project_id, kind, item_ids, keys, label, deleted_by, created_at FROM trash WHERE tenant_id = ? AND id = ?`, tenantID, id))
	if err != nil {
		return nil, mapErr(err, "trash entry")
	}
	return e, nil
}

// TrashStatuses returns the status of each item in a trash entry, so the
// caller can check who may restore it.
func (s *Store) TrashStatuses(ctx context.Context, tenantID, id string) ([]string, error) {
	snap, _, err := s.loadTrash(ctx, s.c(ctx), tenantID, id)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, set := range snap.Sets {
		if set.Table == "requirements" || set.Table == "test_cases" {
			for _, r := range set.Rows {
				if st, ok := r["status"].(string); ok {
					out = append(out, st)
				}
			}
		}
	}
	return out, nil
}

func (s *Store) loadTrash(ctx context.Context, c conn, tenantID, id string) (*trashSnapshot, *TrashEntry, error) {
	var data string
	e, err := scanTrashWith(c.row(ctx, `SELECT id, project_id, kind, item_ids, keys, label, deleted_by, created_at, snapshot FROM trash WHERE tenant_id = ? AND id = ?`, tenantID, id), &data)
	if err != nil {
		return nil, nil, mapErr(err, "trash entry")
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(data)))
	dec.UseNumber()
	var snap trashSnapshot
	if err := dec.Decode(&snap); err != nil {
		return nil, nil, err
	}
	return &snap, e, nil
}

func scanTrashWith(r scanner, data *string) (*TrashEntry, error) {
	var e TrashEntry
	var ids, keys string
	if err := r.Scan(&e.ID, &e.ProjectID, &e.Kind, &ids, &keys, &e.Label, &e.DeletedBy, &e.CreatedAt, data); err != nil {
		return nil, err
	}
	e.ItemIDs = decodeTags(ids)
	e.Keys = decodeTags(keys)
	return &e, nil
}

// RestoreTrash puts the items of a trash entry back and removes the entry.
// It returns the entry (kind, item IDs).
func (s *Store) RestoreTrash(ctx context.Context, tenantID, id string) (*TrashEntry, error) {
	var entry *TrashEntry
	err := s.tx(ctx, func(c conn) error {
		snap, e, err := s.loadTrash(ctx, c, tenantID, id)
		if err != nil {
			return err
		}
		if e.CreatedAt.Before(s.now().Add(-TrashRetention)) {
			return Invalidf("the deletion is older than %d days and can no longer be undone", int(TrashRetention.Hours()/24))
		}
		entry = e
		// items that are back (or were never gone): the deleted ones plus
		// existing rows referenced from the snapshot
		have := map[string]map[string]bool{}
		mark := func(table, id string) {
			if have[table] == nil {
				have[table] = map[string]bool{}
			}
			have[table][id] = true
		}
		exists := func(table, id string) (bool, error) {
			if id == "" {
				return false, nil
			}
			if have[table][id] {
				return true, nil
			}
			var one int
			err := c.row(ctx, `SELECT 1 FROM `+table+` WHERE id = ?`, id).Scan(&one)
			if err != nil {
				if db.IsNoRows(err) {
					return false, nil
				}
				return false, err
			}
			mark(table, id)
			return true, nil
		}
		need := func(row map[string]any, pairs ...string) (bool, error) {
			for i := 0; i+1 < len(pairs); i += 2 {
				v, _ := row[pairs[i+1]].(string)
				ok, err := exists(pairs[i], v)
				if err != nil || !ok {
					return false, err
				}
			}
			return true, nil
		}
		for _, set := range snap.Sets {
			for _, row := range set.Rows {
				ok := true
				ignoreConflict := false
				switch set.Table {
				case "requirements", "test_cases":
					if ok, err = need(row, "projects", "project_id"); err != nil {
						return err
					}
					if !ok {
						return Invalidf("the project no longer exists")
					}
					if f, _ := row["folder_id"].(string); f != "" {
						if in, err := exists("folders", f); err != nil {
							return err
						} else if !in {
							row["folder_id"] = nil
						}
					}
				case "requirement_categories", "test_case_categories":
					ok, err = need(row, "categories", "category_id")
				case "evaluation_criteria":
					ok, err = need(row, "requirements", "requirement_id")
				case "test_steps":
					if ok, err = need(row, "test_cases", "test_case_id"); ok {
						if t, _ := row["call_test_id"].(string); t != "" {
							if in, err2 := exists("test_cases", t); err2 != nil {
								return err2
							} else if !in {
								row["call_test_id"] = nil
							}
						}
					}
				case "trace_links":
					if ok, err = need(row, "requirements", "requirement_id", "test_cases", "test_case_id"); ok {
						if cr, _ := row["criterion_id"].(string); cr != "" {
							if in, err2 := exists("evaluation_criteria", cr); err2 != nil {
								return err2
							} else if !in {
								row["criterion_id"] = nil
							}
						}
					}
				case "requirement_relations":
					ok, err = need(row, "requirements", "source_id", "requirements", "target_id")
				case "test_dependencies":
					ok, err = need(row, "test_cases", "test_id", "test_cases", "depends_on_id")
				case "defect_links":
					ignoreConflict = true
					ok, err = need(row, "defects", "defect_id")
				case "release_requirements":
					ignoreConflict = true
					ok, err = need(row, "releases", "release_id", "requirements", "requirement_id")
				case "risk_requirements":
					ignoreConflict = true
					ok, err = need(row, "risks", "risk_id", "requirements", "requirement_id")
				case "item_links":
					ok, err = linkEndsExist(row, exists)
				case "test_executions":
					if ok, err = need(row, "test_runs", "run_id", "test_cases", "test_case_id"); ok {
						if cfg, _ := row["configuration_id"].(string); cfg != "" {
							if in, err2 := exists("configurations", cfg); err2 != nil {
								return err2
							} else if !in {
								row["configuration_id"] = nil
							}
						}
					}
				case "execution_iterations":
					ok, err = need(row, "test_executions", "execution_id")
				case "step_results":
					if ok, err = need(row, "test_executions", "execution_id"); ok {
						if it, _ := row["iteration_id"].(string); it != "" {
							ok, err = need(row, "execution_iterations", "iteration_id")
						}
					}
				case "files":
					ignoreConflict = true
					if p, _ := row["project_id"].(string); p != "" {
						ok, err = need(row, "projects", "project_id")
					}
				case "file_blobs":
					ignoreConflict = true
					ok, err = need(row, "files", "file_id")
				case "attachments":
					ignoreConflict = true
					ok, err = need(row, "files", "file_id")
				}
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				inserted, err := insertRow(ctx, c, set.Table, row, ignoreConflict)
				if err != nil {
					if set.Table == "requirements" || set.Table == "test_cases" {
						return Conflictf("%s cannot be restored: %v", row["key"], err)
					}
					return err
				}
				if id, _ := row["id"].(string); id != "" && inserted {
					mark(set.Table, id)
				} else if id != "" && set.Table == "files" {
					mark(set.Table, id)
				}
			}
		}
		for _, ref := range snap.Refs {
			if ref.Table != "test_steps" || ref.Column != "call_test_id" {
				continue
			}
			if _, err := c.exec(ctx, `UPDATE test_steps SET call_test_id = ? WHERE id = ? AND call_test_id IS NULL`, ref.Value, ref.ID); err != nil {
				return err
			}
		}
		_, err = c.exec(ctx, `DELETE FROM trash WHERE id = ?`, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// linkEndsExist reports whether both ends of an item link snapshot row exist.
func linkEndsExist(row map[string]any, exists func(table, id string) (bool, error)) (bool, error) {
	for _, end := range []struct{ typ, id string }{{"source_type", "source_id"}, {"target_type", "target_id"}} {
		typ, _ := row[end.typ].(string)
		if typ == "url" {
			continue
		}
		t, ok := linkTables[typ]
		if !ok {
			return false, nil
		}
		id, _ := row[end.id].(string)
		if in, err := exists(t.table, id); err != nil || !in {
			return false, err
		}
	}
	return true, nil
}

// insertRow inserts a snapshot row, converting JSON values back to what
// the column holds. With ignoreConflict an existing row is kept.
func insertRow(ctx context.Context, c conn, table string, row map[string]any, ignoreConflict bool) (bool, error) {
	target, _, err := targetColumns(ctx, c, table)
	if err != nil {
		return false, err
	}
	cols := make([]string, 0, len(row))
	for k := range row {
		if _, ok := target[k]; ok { // columns dropped since the deletion are ignored
			cols = append(cols, k)
		}
	}
	sort.Strings(cols)
	args := make([]any, len(cols))
	for i, col := range cols {
		v, err := importValue(target[col].kind, row[col])
		if err != nil {
			return false, fmt.Errorf("%s.%s: %w", table, col, err)
		}
		args[i] = v
	}
	q := `INSERT INTO ` + table + ` (` + strings.Join(cols, ", ") + `) VALUES (` + placeholders(len(cols)) + `)`
	if ignoreConflict {
		q += ` ON CONFLICT DO NOTHING`
	}
	res, err := c.exec(ctx, q, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
