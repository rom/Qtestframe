package store

import (
	"context"
	"fmt"

	"github.com/rom/qtestframe/internal/model"
)

// Support for the similarity checks (internal/similarity): the cache of
// the optional embeddings service, and merging a duplicate into the item
// that survives.

// ItemsStamp changes whenever a requirement (kind requirement) or test of
// the project is added, changed or deleted (caches of derived data).
func (s *Store) ItemsStamp(ctx context.Context, tenantID, projectID, kind string) (string, error) {
	table := "requirements"
	if kind == model.KindTest {
		table = "test_cases"
	}
	var n int
	var last any
	if err := s.c(ctx).row(ctx, `SELECT COUNT(*), MAX(updated_at) FROM `+table+` WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID).Scan(&n, &last); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d|%v", n, last), nil
}

// CachedEmbedding is a stored vector of an item.
type CachedEmbedding struct {
	ItemID   string
	Version  int
	TextHash string
	Vector   string // similarity.EncodeVector
}

// CachedEmbeddings returns the stored vectors of a project's items of a
// type for a model, by item id.
func (s *Store) CachedEmbeddings(ctx context.Context, tenantID, projectID, itemType, model string) (map[string]CachedEmbedding, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT item_id, version, text_hash, vector FROM similarity_embeddings WHERE tenant_id = ? AND project_id = ? AND item_type = ? AND model = ?`,
		tenantID, projectID, itemType, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]CachedEmbedding{}
	for rows.Next() {
		var e CachedEmbedding
		if err := rows.Scan(&e.ItemID, &e.Version, &e.TextHash, &e.Vector); err != nil {
			return nil, err
		}
		out[e.ItemID] = e
	}
	return out, rows.Err()
}

// SaveEmbeddings stores vectors of a project's items (replacing the
// previous vector of the item and model).
func (s *Store) SaveEmbeddings(ctx context.Context, tenantID, projectID, itemType, model string, list []CachedEmbedding) error {
	if len(list) == 0 {
		return nil
	}
	now := s.now()
	return s.tx(ctx, func(c conn) error {
		for _, e := range list {
			if _, err := c.exec(ctx, `DELETE FROM similarity_embeddings WHERE item_type = ? AND item_id = ? AND model = ?`, itemType, e.ItemID, model); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `INSERT INTO similarity_embeddings (tenant_id, project_id, item_type, item_id, model, version, text_hash, vector, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				tenantID, projectID, itemType, e.ItemID, model, e.Version, e.TextHash, e.Vector, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// MergeCounts says what merging a duplicate moved to the surviving item.
type MergeCounts struct {
	// Links: trace links (to tests of a requirement, to requirements of a
	// test); Relations: relations of a requirement; Dependencies:
	// prerequisites of a test (both directions); ItemLinks: item links.
	Links        int `json:"links"`
	Relations    int `json:"relations"`
	Dependencies int `json:"dependencies"`
	ItemLinks    int `json:"item_links"`
	// Dropped: references the survivor already had (or that would point
	// to itself); they are deleted with the duplicate's copy.
	Dropped int `json:"dropped"`
}

// moveRefs re-points the rows of table whose column col is from to to;
// rows that would duplicate one of to's (same values in the other
// columns, keys) or point to itself (self) are deleted instead.
func moveRefs(ctx context.Context, c conn, table, col, other, from, to string, counts *int, dropped *int) error {
	rows, err := c.query(ctx, `SELECT id, `+other+` FROM `+table+` WHERE `+col+` = ?`, from)
	if err != nil {
		return err
	}
	type ref struct{ id, other string }
	var refs []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.id, &r.other); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range refs {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ? AND `+other+` = ?`, to, r.other).Scan(&n); err != nil {
			return err
		}
		if n > 0 || r.other == to {
			if _, err := c.exec(ctx, `DELETE FROM `+table+` WHERE id = ?`, r.id); err != nil {
				return err
			}
			*dropped++
			continue
		}
		if _, err := c.exec(ctx, `UPDATE `+table+` SET `+col+` = ? WHERE id = ?`, to, r.id); err != nil {
			return err
		}
		*counts++
	}
	return nil
}

// MergeItems moves the references of item from (a requirement or test of
// the project) to item to: trace links, relations or prerequisites, and
// item links. The items themselves are not changed (the caller obsoletes
// from and records the duplicates relation or link).
func (s *Store) MergeItems(ctx context.Context, tenantID, projectID, kind, from, to string) (*MergeCounts, error) {
	if from == to || from == "" || to == "" {
		return nil, Invalidf("two different items are needed")
	}
	table := "requirements"
	if kind == model.KindTest {
		table = "test_cases"
	} else if kind != model.KindRequirement {
		return nil, Invalidf("kind must be requirement or test")
	}
	out := &MergeCounts{}
	err := s.tx(ctx, func(c conn) error {
		for _, id := range []string{from, to} {
			var pid string
			if err := c.row(ctx, `SELECT project_id FROM `+table+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
				return mapErr(err, kind)
			}
			if pid != projectID {
				return Invalidf("the item belongs to another project")
			}
		}
		if kind == model.KindRequirement {
			// criteria of the duplicate do not exist on the survivor
			if _, err := c.exec(ctx, `DELETE FROM link_criteria WHERE link_id IN (SELECT id FROM trace_links WHERE requirement_id = ?)`, from); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `UPDATE trace_links SET criterion_id = NULL WHERE requirement_id = ?`, from); err != nil {
				return err
			}
			if err := moveRefs(ctx, c, "trace_links", "requirement_id", "test_case_id", from, to, &out.Links, &out.Dropped); err != nil {
				return err
			}
			for _, cols := range [][2]string{{"source_id", "target_id"}, {"target_id", "source_id"}} {
				if err := moveRelations(ctx, c, cols[0], cols[1], from, to, out); err != nil {
					return err
				}
			}
		} else {
			if err := moveRefs(ctx, c, "trace_links", "test_case_id", "requirement_id", from, to, &out.Links, &out.Dropped); err != nil {
				return err
			}
			if err := moveRefs(ctx, c, "test_dependencies", "test_id", "depends_on_id", from, to, &out.Dependencies, &out.Dropped); err != nil {
				return err
			}
			if err := moveRefs(ctx, c, "test_dependencies", "depends_on_id", "test_id", from, to, &out.Dependencies, &out.Dropped); err != nil {
				return err
			}
		}
		// item links of the item in both directions (a link between the
		// two items is dropped)
		if _, err := c.exec(ctx, `DELETE FROM item_links WHERE tenant_id = ? AND ((source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?) OR (source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?))`,
			tenantID, kind, from, kind, to, kind, to, kind, from); err != nil {
			return err
		}
		res, err := c.exec(ctx, `UPDATE item_links SET source_id = ? WHERE tenant_id = ? AND source_type = ? AND source_id = ?`, to, tenantID, kind, from)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		res, err = c.exec(ctx, `UPDATE item_links SET target_id = ? WHERE tenant_id = ? AND target_type = ? AND target_id = ?`, to, tenantID, kind, from)
		if err != nil {
			return err
		}
		m, _ := res.RowsAffected()
		out.ItemLinks = int(n + m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// moveRelations re-points the relations whose column col is from; a
// relation the survivor already has (same other end and type), or one
// between the two items, is dropped.
func moveRelations(ctx context.Context, c conn, col, other, from, to string, out *MergeCounts) error {
	rows, err := c.query(ctx, `SELECT id, `+other+`, type FROM requirement_relations WHERE `+col+` = ?`, from)
	if err != nil {
		return err
	}
	type rel struct{ id, other, typ string }
	var rels []rel
	for rows.Next() {
		var r rel
		if err := rows.Scan(&r.id, &r.other, &r.typ); err != nil {
			rows.Close()
			return err
		}
		rels = append(rels, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range rels {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM requirement_relations WHERE `+col+` = ? AND `+other+` = ? AND type = ?`, to, r.other, r.typ).Scan(&n); err != nil {
			return err
		}
		if n > 0 || r.other == to {
			if _, err := c.exec(ctx, `DELETE FROM requirement_relations WHERE id = ?`, r.id); err != nil {
				return err
			}
			out.Dropped++
			continue
		}
		if _, err := c.exec(ctx, `UPDATE requirement_relations SET `+col+` = ? WHERE id = ?`, to, r.id); err != nil {
			return err
		}
		out.Relations++
	}
	return nil
}
