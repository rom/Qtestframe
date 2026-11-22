package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Support for the built-in consistency checks (internal/tracerules):
// dismissed findings ("not a duplicate") and the references to items that
// were deleted into the trash.

// ConsistencyDismissal is a dismissed finding of a built-in check about a
// pair of items (ItemA < ItemB).
type ConsistencyDismissal struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Check     string    `json:"check"`
	ItemA     string    `json:"item_a"`
	ItemB     string    `json:"item_b"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// DismissableChecks are the built-in checks whose findings about a pair
// of items can be dismissed: a suspected duplicate that is none, and a
// dependency or prerequisite on a deleted item that has been dealt with.
var DismissableChecks = []string{model.CheckDuplicateSuspect, model.CheckUnmetDependency, model.CheckBrokenPrerequisite}

func orderPair(a, b string) (string, string) {
	if b < a {
		return b, a
	}
	return a, b
}

// DismissConsistency records that the finding of check about item (an
// item of the project) and other is not a problem. For duplicate_suspect
// both are requirements of the project; for the other checks other may be
// a deleted item. Dismissing twice is not an error.
func (s *Store) DismissConsistency(ctx context.Context, tenantID, projectID, check, item, other, by string) (*ConsistencyDismissal, error) {
	if !slices.Contains(DismissableChecks, check) {
		return nil, Invalidf("findings of %q cannot be dismissed", check)
	}
	if item == "" || other == "" || item == other {
		return nil, Invalidf("two different items are needed")
	}
	table := "requirements"
	if check == model.CheckBrokenPrerequisite {
		table = "test_cases"
	}
	ids := []string{item}
	if check == model.CheckDuplicateSuspect {
		ids = append(ids, other)
	}
	for _, id := range ids {
		var pid string
		if err := s.c(ctx).row(ctx, `SELECT project_id FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
			return nil, mapErr(err, "item")
		}
		if pid != projectID {
			return nil, Invalidf("the item belongs to another project")
		}
	}
	a, b := orderPair(item, other)
	d := &ConsistencyDismissal{ID: newID(), ProjectID: projectID, Check: check, ItemA: a, ItemB: b, CreatedBy: by, CreatedAt: s.now()}
	_, err := s.c(ctx).exec(ctx, `INSERT INTO consistency_dismissals (id, tenant_id, project_id, check_kind, item_a, item_b, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		d.ID, tenantID, projectID, check, a, b, by, d.CreatedAt)
	return d, err
}

// UndismissConsistency removes a dismissal (no error when there is none).
func (s *Store) UndismissConsistency(ctx context.Context, tenantID, projectID, check, a, b string) error {
	a, b = orderPair(a, b)
	_, err := s.c(ctx).exec(ctx, `DELETE FROM consistency_dismissals WHERE tenant_id = ? AND project_id = ? AND check_kind = ? AND item_a = ? AND item_b = ?`, tenantID, projectID, check, a, b)
	return err
}

// ListConsistencyDismissals lists the dismissals of a project.
func (s *Store) ListConsistencyDismissals(ctx context.Context, tenantID, projectID string) ([]ConsistencyDismissal, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, project_id, check_kind, item_a, item_b, created_by, created_at FROM consistency_dismissals WHERE tenant_id = ? AND project_id = ? ORDER BY created_at, id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConsistencyDismissal{}
	for rows.Next() {
		var d ConsistencyDismissal
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Check, &d.ItemA, &d.ItemB, &d.CreatedBy, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DismissalKey is the key of a pair in a set of dismissals.
func DismissalKey(check, a, b string) string {
	a, b = orderPair(a, b)
	return check + "|" + a + "|" + b
}

// TrashedItem is a requirement or test in the trash.
type TrashedItem struct {
	Kind, ID, Key, Title string
}

// TrashedRefs are the references that deleting items into the trash
// removed: relations and test prerequisites whose other end still exists,
// with the deleted items they pointed to.
type TrashedRefs struct {
	Items        map[string]TrashedItem
	Relations    []model.Relation
	Dependencies []model.TestDependency
}

func rowString(row map[string]any, col string) string {
	switch v := row[col].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// TrashedReferences reads the trash of a project (entries within the
// retention period). Only references from items that still exist are
// returned, so the checks can report them on the surviving item.
func (s *Store) TrashedReferences(ctx context.Context, tenantID, projectID string) (*TrashedRefs, error) {
	out := &TrashedRefs{Items: map[string]TrashedItem{}}
	rows, err := s.c(ctx).query(ctx, `SELECT snapshot FROM trash WHERE tenant_id = ? AND project_id = ? AND created_at >= ? ORDER BY created_at`,
		tenantID, projectID, s.now().Add(-TrashRetention))
	if err != nil {
		return nil, err
	}
	var snaps []string
	for rows.Next() {
		var snap string
		if err := rows.Scan(&snap); err != nil {
			rows.Close()
			return nil, err
		}
		snaps = append(snaps, snap)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var rels []model.Relation
	var deps []model.TestDependency
	for _, raw := range snaps {
		var snap trashSnapshot
		if err := json.Unmarshal([]byte(raw), &snap); err != nil {
			continue // an unreadable entry hides nothing else
		}
		for _, set := range snap.Sets {
			for _, row := range set.Rows {
				switch set.Table {
				case "requirements", "test_cases":
					kind := model.KindRequirement
					if set.Table == "test_cases" {
						kind = model.KindTest
					}
					id := rowString(row, "id")
					out.Items[id] = TrashedItem{Kind: kind, ID: id, Key: rowString(row, "key"), Title: rowString(row, "title")}
				case "requirement_relations":
					rels = append(rels, model.Relation{ID: rowString(row, "id"), ProjectID: projectID, SourceID: rowString(row, "source_id"), TargetID: rowString(row, "target_id"), Type: rowString(row, "type")})
				case "test_dependencies":
					deps = append(deps, model.TestDependency{ID: rowString(row, "id"), ProjectID: projectID, TestID: rowString(row, "test_id"), DependsOnID: rowString(row, "depends_on_id")})
				}
			}
		}
	}
	if len(out.Items) == 0 {
		return out, nil
	}
	exists, err := s.existingItems(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for _, r := range rels {
		// one end deleted, the other still there
		if _, gone := out.Items[r.TargetID]; gone && exists[r.SourceID] && !exists[r.TargetID] {
			out.Relations = append(out.Relations, r)
		} else if _, gone := out.Items[r.SourceID]; gone && exists[r.TargetID] && !exists[r.SourceID] {
			out.Relations = append(out.Relations, r)
		}
	}
	for _, d := range deps {
		if _, gone := out.Items[d.DependsOnID]; gone && exists[d.TestID] && !exists[d.DependsOnID] {
			out.Dependencies = append(out.Dependencies, d)
		}
	}
	return out, nil
}

// existingItems returns the ids of the project's requirements and tests.
func (s *Store) existingItems(ctx context.Context, tenantID, projectID string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, table := range []string{"requirements", "test_cases"} {
		rows, err := s.c(ctx).query(ctx, `SELECT id FROM `+table+` WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ReleaseScopes returns the requirement ids of every release of a project.
func (s *Store) ReleaseScopes(ctx context.Context, tenantID, projectID string) (map[string][]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT rr.release_id, rr.requirement_id FROM release_requirements rr JOIN releases r ON r.id = rr.release_id
		WHERE r.tenant_id = ? AND r.project_id = ? ORDER BY rr.release_id, rr.requirement_id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var rel, req string
		if err := rows.Scan(&rel, &req); err != nil {
			return nil, err
		}
		out[rel] = append(out[rel], req)
	}
	return out, rows.Err()
}
