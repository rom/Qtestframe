package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
)

// Variants of a project and the applicability of requirements and tests
// (see docs/api.md "Variants and applicability").

// Limits of variants.
const (
	MaxVariants          = 500
	MaxVariantName       = 200
	MaxVariantAttributes = 50
	MaxApplicabilityExpr = 1000
	MaxApplicabilityIDs  = 200
)

// CoverageNotApplicable is the coverage of a requirement in a variant it
// does not apply to.
const CoverageNotApplicable = "not_applicable"

const variantCols = `id, project_id, parent_id, name, description, kind, attributes, configuration_ids, position, created_by, created_at, updated_at`

func scanVariant(r interface{ Scan(...any) error }) (*model.Variant, error) {
	var v model.Variant
	var parent sql.NullString
	var attrs, cfgs string
	if err := r.Scan(&v.ID, &v.ProjectID, &parent, &v.Name, &v.Description, &v.Kind, &attrs, &cfgs, &v.Position, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.ParentID = strPtr(parent)
	v.Attributes = map[string]string{}
	_ = json.Unmarshal([]byte(attrs), &v.Attributes)
	if v.Attributes == nil {
		v.Attributes = map[string]string{}
	}
	v.ConfigurationIDs = decodeTags(cfgs)
	return &v, nil
}

// ValidateVariant normalises and checks a variant's own fields.
func ValidateVariant(v *model.Variant) error {
	v.Name = strings.Join(strings.Fields(v.Name), " ")
	v.Description = strings.TrimSpace(v.Description)
	v.Kind = strings.TrimSpace(strings.ToLower(v.Kind))
	if v.Name == "" {
		return Invalidf("variant name is required")
	}
	if utf8.RuneCountInString(v.Name) > MaxVariantName {
		return Invalidf("variant name is longer than %d characters", MaxVariantName)
	}
	if len(v.Description) > 4000 {
		return Invalidf("description is too long")
	}
	if v.Kind != "" && !slices.Contains(model.VariantKinds, v.Kind) {
		return Invalidf("kind must be %s or empty", strings.Join(model.VariantKinds, ", "))
	}
	if len(v.Attributes) > MaxVariantAttributes {
		return Invalidf("a variant can have at most %d attributes", MaxVariantAttributes)
	}
	attrs := map[string]string{}
	for k, val := range v.Attributes {
		k = strings.Join(strings.Fields(k), "_")
		val = strings.TrimSpace(val)
		if k == "" {
			return Invalidf("attribute names are required")
		}
		if !attrNameOK(k) {
			return Invalidf("attribute name %q may only contain letters, digits, _ and -", k)
		}
		switch strings.ToLower(k) {
		case "name", "variant", "kind", "id":
			return Invalidf("attribute name %q is reserved", k)
		}
		if utf8.RuneCountInString(k) > MaxDimensionChars || utf8.RuneCountInString(val) > MaxDimensionChars {
			return Invalidf("attribute names and values have at most %d characters", MaxDimensionChars)
		}
		for other := range attrs {
			if strings.EqualFold(other, k) {
				return Invalidf("attribute %q is given twice", k)
			}
		}
		attrs[k] = val
	}
	v.Attributes = attrs
	v.ConfigurationIDs = uniq(cleanTags(v.ConfigurationIDs))
	if v.ConfigurationIDs == nil {
		v.ConfigurationIDs = []string{}
	}
	return nil
}

func attrNameOK(k string) bool {
	for _, r := range k {
		if !(r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127) {
			return false
		}
	}
	return true
}

// ListVariants returns the variants of a project in hierarchy order
// (parents before their children), each with its path.
func (s *Store) ListVariants(ctx context.Context, tenantID, projectID string) ([]model.Variant, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+variantCols+` FROM variants WHERE tenant_id = ? AND project_id = ? ORDER BY position, name`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var flat []model.Variant
	for rows.Next() {
		v, err := scanVariant(rows)
		if err != nil {
			return nil, err
		}
		flat = append(flat, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewVariantSet(flat).List(), nil
}

// GetVariant returns one variant.
func (s *Store) GetVariant(ctx context.Context, tenantID, id string) (*model.Variant, error) {
	v, err := scanVariant(s.c(ctx).row(ctx, `SELECT `+variantCols+` FROM variants WHERE id = ? AND tenant_id = ?`, id, tenantID))
	return v, mapErr(err, "variant")
}

// FindVariant finds a variant of a project by id or name (ignoring case);
// an unknown reference is invalid.
func (s *Store) FindVariant(ctx context.Context, tenantID, projectID, ref string) (*model.Variant, error) {
	list, err := s.ListVariants(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	if v := NewVariantSet(list).Find(ref); v != nil {
		return v, nil
	}
	return nil, Invalidf("variant %q not found in project", strings.TrimSpace(ref))
}

// checkVariantRefs checks the parent (same project, no cycle) and the
// configurations of a variant.
func checkVariantRefs(ctx context.Context, c conn, tenantID string, v *model.Variant) error {
	if v.ParentID != nil && *v.ParentID == "" {
		v.ParentID = nil
	}
	if v.ParentID != nil {
		seen := map[string]bool{v.ID: true}
		id := *v.ParentID
		for id != "" {
			if seen[id] {
				return Invalidf("a variant cannot be below itself")
			}
			seen[id] = true
			var pid string
			var parent sql.NullString
			if err := c.row(ctx, `SELECT project_id, parent_id FROM variants WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid, &parent); err != nil {
				if db.IsNoRows(err) {
					return Invalidf("parent variant %q not found", id)
				}
				return err
			}
			if pid != v.ProjectID {
				return Invalidf("the parent variant belongs to another project")
			}
			id = parent.String
		}
	}
	for _, cid := range v.ConfigurationIDs {
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM configurations WHERE id = ? AND tenant_id = ?`, cid, tenantID).Scan(&pid); err != nil || pid != v.ProjectID {
			return Invalidf("configuration %q not found in project", cid)
		}
	}
	return nil
}

func checkVariantName(ctx context.Context, c conn, projectID, id, name string) error {
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM variants WHERE project_id = ? AND id <> ? AND LOWER(name) = LOWER(?)`, projectID, id, name).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return Conflictf("a variant named %q already exists", name)
	}
	return nil
}

// CreateVariant adds a variant at the end of the project's list.
func (s *Store) CreateVariant(ctx context.Context, tenantID string, v *model.Variant) error {
	if err := ValidateVariant(v); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, v.ProjectID); err != nil {
		return err
	}
	v.ID = newID()
	v.CreatedAt = s.now()
	v.UpdatedAt = v.CreatedAt
	return s.tx(ctx, func(c conn) error {
		var n int
		var pos sql.NullInt64
		if err := c.row(ctx, `SELECT COUNT(*), MAX(position) FROM variants WHERE project_id = ?`, v.ProjectID).Scan(&n, &pos); err != nil {
			return err
		}
		if n >= MaxVariants {
			return Invalidf("a project can have at most %d variants", MaxVariants)
		}
		v.Position = 0
		if pos.Valid {
			v.Position = int(pos.Int64) + 1
		}
		if err := checkVariantRefs(ctx, c, tenantID, v); err != nil {
			return err
		}
		if err := checkVariantName(ctx, c, v.ProjectID, v.ID, v.Name); err != nil {
			return err
		}
		attrs, _ := json.Marshal(v.Attributes)
		_, err := c.exec(ctx, `INSERT INTO variants (id, tenant_id, project_id, parent_id, name, description, kind, attributes, configuration_ids, position, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			v.ID, tenantID, v.ProjectID, nullStr(v.ParentID), v.Name, v.Description, v.Kind, string(attrs), encodeTags(v.ConfigurationIDs), v.Position, v.CreatedBy, v.CreatedAt, v.UpdatedAt)
		return err
	})
}

// UpdateVariant saves a variant's fields (including its parent and position).
func (s *Store) UpdateVariant(ctx context.Context, tenantID string, v *model.Variant) error {
	if err := ValidateVariant(v); err != nil {
		return err
	}
	v.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		if err := checkVariantRefs(ctx, c, tenantID, v); err != nil {
			return err
		}
		if err := checkVariantName(ctx, c, v.ProjectID, v.ID, v.Name); err != nil {
			return err
		}
		attrs, _ := json.Marshal(v.Attributes)
		res, err := c.exec(ctx, `UPDATE variants SET parent_id = ?, name = ?, description = ?, kind = ?, attributes = ?, configuration_ids = ?, position = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			nullStr(v.ParentID), v.Name, v.Description, v.Kind, string(attrs), encodeTags(v.ConfigurationIDs), v.Position, v.UpdatedAt, v.ID, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "variant")
		}
		return nil
	})
}

// DeleteVariant removes a variant: its children move to its parent, runs
// for it become variant-neutral and it is removed from the applicability
// lists (an item whose list becomes empty and has no expression then
// applies to all variants).
func (s *Store) DeleteVariant(ctx context.Context, tenantID, id string) error {
	v, err := s.GetVariant(ctx, tenantID, id)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `UPDATE variants SET parent_id = ? WHERE parent_id = ? AND tenant_id = ?`, nullStr(v.ParentID), id, tenantID); err != nil {
			return err
		}
		rows, err := c.query(ctx, `SELECT item_type, item_id, variant_ids, expression FROM item_applicability WHERE project_id = ? AND variant_ids LIKE ?`, v.ProjectID, "%"+id+"%")
		if err != nil {
			return err
		}
		type row struct{ typ, item, ids, expr string }
		var found []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.typ, &r.item, &r.ids, &r.expr); err != nil {
				rows.Close()
				return err
			}
			found = append(found, r)
		}
		rows.Close()
		for _, r := range found {
			ids := slices.DeleteFunc(decodeTags(r.ids), func(x string) bool { return x == id })
			if len(ids) == 0 && r.expr == "" {
				_, err = c.exec(ctx, `DELETE FROM item_applicability WHERE item_type = ? AND item_id = ?`, r.typ, r.item)
			} else {
				_, err = c.exec(ctx, `UPDATE item_applicability SET variant_ids = ? WHERE item_type = ? AND item_id = ?`, encodeTags(ids), r.typ, r.item)
			}
			if err != nil {
				return err
			}
		}
		_, err = c.exec(ctx, `DELETE FROM variants WHERE id = ? AND tenant_id = ?`, id, tenantID)
		return err
	})
}

// checkRunVariantBuild checks that a run's variant and build belong to its project.
func checkRunVariantBuild(ctx context.Context, c conn, tenantID, projectID string, variantID, buildID *string) error {
	for _, x := range []struct {
		id         *string
		table, msg string
	}{{variantID, "variants", "variant"}, {buildID, "builds", "build"}} {
		if x.id == nil || *x.id == "" {
			continue
		}
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM `+x.table+` WHERE id = ? AND tenant_id = ?`, *x.id, tenantID).Scan(&pid); err != nil || pid != projectID {
			return Invalidf("%s %q not found in project", x.msg, *x.id)
		}
	}
	return nil
}

// ---------------------------------------------------------------- variant set

// VariantSet answers questions about the variants of a project: the
// hierarchy, inherited attributes and which variants an applicability
// covers.
type VariantSet struct {
	list []model.Variant
	byID map[string]*model.Variant
	// compiled applicability expressions
	exprs map[string]*query.Query
}

// NewVariantSet indexes variants (in any order); List returns them in
// hierarchy order with their paths.
func NewVariantSet(variants []model.Variant) *VariantSet {
	vs := &VariantSet{byID: map[string]*model.Variant{}, exprs: map[string]*query.Query{}}
	children := map[string][]model.Variant{}
	ids := map[string]bool{}
	for _, v := range variants {
		ids[v.ID] = true
	}
	for _, v := range variants {
		parent := ""
		if v.ParentID != nil && ids[*v.ParentID] {
			parent = *v.ParentID
		}
		children[parent] = append(children[parent], v)
	}
	seen := map[string]bool{}
	var walk func(parent, path string)
	walk = func(parent, path string) {
		kids := children[parent]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].Position != kids[j].Position {
				return kids[i].Position < kids[j].Position
			}
			return strings.ToLower(kids[i].Name) < strings.ToLower(kids[j].Name)
		})
		for _, v := range kids {
			if seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			v.Path = v.Name
			if path != "" {
				v.Path = path + " / " + v.Name
			}
			vs.list = append(vs.list, v)
			walk(v.ID, v.Path)
		}
	}
	walk("", "")
	// variants caught in a cycle (not reachable from a root)
	for _, v := range variants {
		if !seen[v.ID] {
			v.Path = v.Name
			vs.list = append(vs.list, v)
		}
	}
	for i := range vs.list {
		vs.byID[vs.list[i].ID] = &vs.list[i]
	}
	return vs
}

// List returns the variants in hierarchy order.
func (vs *VariantSet) List() []model.Variant {
	if vs == nil || vs.list == nil {
		return []model.Variant{}
	}
	return vs.list
}

// Len is the number of variants.
func (vs *VariantSet) Len() int {
	if vs == nil {
		return 0
	}
	return len(vs.list)
}

// Get returns a variant by id (nil when unknown).
func (vs *VariantSet) Get(id string) *model.Variant {
	if vs == nil {
		return nil
	}
	return vs.byID[id]
}

// Find returns a variant by id or name (ignoring case), nil when unknown.
func (vs *VariantSet) Find(ref string) *model.Variant {
	ref = strings.TrimSpace(ref)
	if vs == nil || ref == "" {
		return nil
	}
	if v := vs.byID[ref]; v != nil {
		return v
	}
	for i := range vs.list {
		if strings.EqualFold(vs.list[i].Name, ref) {
			return &vs.list[i]
		}
	}
	for i := range vs.list {
		if strings.EqualFold(vs.list[i].Path, ref) {
			return &vs.list[i]
		}
	}
	return nil
}

// Lineage returns the variant and its ancestors (the variant first).
func (vs *VariantSet) Lineage(id string) []string {
	var out []string
	seen := map[string]bool{}
	for v := vs.Get(id); v != nil && !seen[v.ID]; {
		seen[v.ID] = true
		out = append(out, v.ID)
		if v.ParentID == nil {
			break
		}
		v = vs.Get(*v.ParentID)
	}
	return out
}

// Attributes returns the attributes of a variant including those it
// inherits (its own values win).
func (vs *VariantSet) Attributes(id string) map[string]string {
	out := map[string]string{}
	line := vs.Lineage(id)
	for i := len(line) - 1; i >= 0; i-- {
		for k, v := range vs.Get(line[i]).Attributes {
			for old := range out {
				if strings.EqualFold(old, k) {
					delete(out, old)
				}
			}
			out[k] = v
		}
	}
	return out
}

// variantRecord is what an applicability expression is evaluated on: the
// variant (and ancestor) names as name / variant, kind and attributes. An
// unqualified term matches the names only.
type variantRecord struct {
	names []string
	m     query.MapRecord
}

func (r variantRecord) Values(field string) ([]string, bool) { return r.m.Values(field) }
func (r variantRecord) AllValues() []string                  { return r.names }

func (vs *VariantSet) record(id string) variantRecord {
	var names, kinds []string
	for _, a := range vs.Lineage(id) {
		v := vs.Get(a)
		names = append(names, v.Name)
		if v.Kind != "" {
			kinds = append(kinds, v.Kind)
		}
	}
	m := query.MapRecord{"name": names, "variant": names, "kind": kinds, "id": {id}}
	for k, v := range vs.Attributes(id) {
		m[strings.ToLower(k)] = []string{v}
	}
	return variantRecord{names: names, m: m}
}

// ParseApplicability checks an applicability expression.
func ParseApplicability(expr string) (*query.Query, error) {
	if utf8.RuneCountInString(expr) > MaxApplicabilityExpr {
		return nil, Invalidf("the applicability expression is longer than %d characters", MaxApplicabilityExpr)
	}
	return query.Parse(expr, nil)
}

// Applies reports whether an applicability covers a variant: always for
// "all variants"; else when a listed variant is the variant or one of its
// ancestors, or the expression matches the variant.
func (vs *VariantSet) Applies(a *model.Applicability, variantID string) bool {
	if a.All() {
		return true
	}
	line := vs.Lineage(variantID)
	for _, id := range a.VariantIDs {
		if slices.Contains(line, id) {
			return true
		}
	}
	if a.Expression == "" || len(line) == 0 {
		return false
	}
	q, ok := vs.exprs[a.Expression]
	if !ok {
		q, _ = ParseApplicability(a.Expression)
		vs.exprs[a.Expression] = q
	}
	return q != nil && !q.IsEmpty() && q.Match(vs.record(variantID))
}

// AppliesTo returns the variants an applicability covers, in order.
func (vs *VariantSet) AppliesTo(a *model.Applicability) []model.Variant {
	out := []model.Variant{}
	for _, v := range vs.List() {
		if vs.Applies(a, v.ID) {
			out = append(out, v)
		}
	}
	return out
}

// Refs fills the variant names of an applicability's list.
func (vs *VariantSet) Refs(a *model.Applicability) {
	if a == nil {
		return
	}
	a.Variants = []model.VariantRef{}
	for _, id := range a.VariantIDs {
		if v := vs.Get(id); v != nil {
			a.Variants = append(a.Variants, model.VariantRef{ID: v.ID, Name: v.Name})
		}
	}
}

// ---------------------------------------------------------------- applicability

// ListApplicability returns item id → applicability of the project's
// requirements and tests that do not apply to all variants.
func (s *Store) ListApplicability(ctx context.Context, tenantID, projectID string) (map[string]*model.Applicability, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT item_id, variant_ids, expression, updated_by, updated_at FROM item_applicability WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*model.Applicability{}
	for rows.Next() {
		var id, ids string
		var a model.Applicability
		var at time.Time
		if err := rows.Scan(&id, &ids, &a.Expression, &a.UpdatedBy, &at); err != nil {
			return nil, err
		}
		a.VariantIDs = decodeTags(ids)
		a.UpdatedAt = &at
		if !a.All() {
			out[id] = &a
		}
	}
	return out, rows.Err()
}

// GetApplicability returns the applicability of one item (empty: all variants).
func (s *Store) GetApplicability(ctx context.Context, tenantID, itemType, itemID string) (*model.Applicability, error) {
	var ids string
	var at time.Time
	a := &model.Applicability{VariantIDs: []string{}}
	err := s.c(ctx).row(ctx, `SELECT variant_ids, expression, updated_by, updated_at FROM item_applicability WHERE item_type = ? AND item_id = ? AND tenant_id = ?`, itemType, itemID, tenantID).
		Scan(&ids, &a.Expression, &a.UpdatedBy, &at)
	if db.IsNoRows(err) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	a.VariantIDs, a.UpdatedAt = decodeTags(ids), &at
	return a, nil
}

// ValidateApplicability normalises an applicability and checks its
// variants (of the project) and expression.
func (s *Store) ValidateApplicability(ctx context.Context, tenantID, projectID string, a *model.Applicability) error {
	a.Expression = strings.TrimSpace(a.Expression)
	if strings.EqualFold(a.Expression, "all") || a.Expression == "*" {
		a.Expression = ""
	}
	if _, err := ParseApplicability(a.Expression); err != nil {
		return err
	}
	if len(a.VariantIDs) > MaxApplicabilityIDs {
		return Invalidf("at most %d variants can be listed", MaxApplicabilityIDs)
	}
	if len(a.VariantIDs) == 0 {
		a.VariantIDs = []string{}
		return nil
	}
	list, err := s.ListVariants(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	vs := NewVariantSet(list)
	ids := []string{}
	for _, ref := range a.VariantIDs {
		v := vs.Find(ref)
		if v == nil {
			return Invalidf("variant %q not found in project", ref)
		}
		if !slices.Contains(ids, v.ID) {
			ids = append(ids, v.ID)
		}
	}
	a.VariantIDs = ids
	return nil
}

// SetApplicability stores the applicability of items of a project (all of
// itemType, checked to belong to the project); "all variants" removes the
// rows. It returns the number of items.
func (s *Store) SetApplicability(ctx context.Context, tenantID, projectID, itemType string, itemIDs []string, a *model.Applicability, user string) (int, error) {
	table := "requirements"
	switch itemType {
	case model.KindRequirement:
	case model.KindTest:
		table = "test_cases"
	default:
		return 0, Invalidf("item_type must be requirement or test")
	}
	if err := s.ValidateApplicability(ctx, tenantID, projectID, a); err != nil {
		return 0, err
	}
	itemIDs = uniq(itemIDs)
	now := s.now()
	err := s.tx(ctx, func(c conn) error {
		if len(itemIDs) == 0 {
			return nil
		}
		rows, err := c.query(ctx, `SELECT id FROM `+table+` WHERE tenant_id = ? AND project_id = ? AND id IN (`+placeholders(len(itemIDs))+`)`, append([]any{tenantID, projectID}, anySlice(itemIDs)...)...)
		if err != nil {
			return err
		}
		found, err := scanStrings(rows)
		if err != nil {
			return err
		}
		if len(found) != len(itemIDs) {
			return Invalidf("%d of the items are not %ss of the project", len(itemIDs)-len(found), itemType)
		}
		for _, id := range itemIDs {
			if _, err := c.exec(ctx, `DELETE FROM item_applicability WHERE item_type = ? AND item_id = ?`, itemType, id); err != nil {
				return err
			}
			if a.All() {
				continue
			}
			if _, err := c.exec(ctx, `INSERT INTO item_applicability (item_type, item_id, tenant_id, project_id, variant_ids, expression, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				itemType, id, tenantID, projectID, encodeTags(a.VariantIDs), a.Expression, user, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if !a.All() {
		a.UpdatedBy, a.UpdatedAt = user, &now
	}
	return len(itemIDs), nil
}

// deleteVariantCodeData removes the applicability and code links of
// deleted requirements or tests.
func deleteVariantCodeData(ctx context.Context, c conn, entityType string, ids []string) error {
	args := append([]any{entityType}, anySlice(ids)...)
	for _, tbl := range []string{"item_applicability", "code_change_items"} {
		if _, err := c.exec(ctx, `DELETE FROM `+tbl+` WHERE item_type = ? AND item_id IN (`+placeholders(len(ids))+`)`, args...); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- coverage per variant

// execUnit is one execution as the variant and build coverage sees it.
type execUnit struct {
	test, cfg, status string
	at                time.Time
	variant, build    string
	run               string
}

// projectExecutions returns every execution of a project's runs with its
// run's variant and build.
func (s *Store) projectExecutions(ctx context.Context, tenantID, projectID string) ([]execUnit, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT e.test_case_id, e.configuration_id, e.status, e.finished_at, e.started_at, r.created_at, r.variant_id, r.build_id, r.id
		FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.project_id = ? AND e.tenant_id = ?`, projectID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []execUnit
	for rows.Next() {
		var u execUnit
		var cfg, variant, build sql.NullString
		var fin, start sql.NullTime
		if err := rows.Scan(&u.test, &cfg, &u.status, &fin, &start, &u.at, &variant, &build, &u.run); err != nil {
			return nil, err
		}
		if start.Valid {
			u.at = start.Time
		}
		if fin.Valid {
			u.at = fin.Time
		}
		u.cfg, u.variant, u.build = cfg.String, variant.String, build.String
		out = append(out, u)
	}
	return out, rows.Err()
}

// aggregateUnits builds ConfigResults from the executions keep accepts,
// with the semantics of LastConfigResults (the latest result other than
// not_run per test and configuration; "" when only planned).
func aggregateUnits(units []execUnit, keep func(*execUnit) bool) ConfigResults {
	best := map[string]map[string]unitResult{}
	for i := range units {
		u := &units[i]
		if keep != nil && !keep(u) {
			continue
		}
		if best[u.test] == nil {
			best[u.test] = map[string]unitResult{}
		}
		cur, ok := best[u.test][u.cfg]
		switch {
		case u.status == model.ResultNotRun:
			if !ok {
				best[u.test][u.cfg] = unitResult{}
			}
		case cur.status == "" || u.at.After(cur.at):
			best[u.test][u.cfg] = unitResult{u.status, u.at}
		}
	}
	out := ConfigResults{}
	for tid, m := range best {
		out[tid] = map[string]string{}
		for cfg, r := range m {
			out[tid][cfg] = r.status
		}
	}
	return out
}

// CoverageFunc computes the coverage of a requirement from the tests that
// count for it and their results. It is the hook for other verification
// rules (e.g. criterion-level verification): VariantCoverage passes it
// only the requirement's linked tests that apply to the variant and only
// the results that count for the variant.
type CoverageFunc func(requirementID string, tests []string, res ConfigResults) string

// DefaultCoverage is CoverageOf.
func DefaultCoverage(_ string, tests []string, res ConfigResults) string {
	return CoverageOf(tests, res)
}

// VariantCoverage computes coverage and results per variant (× test
// configuration):
//
//   - a requirement or test counts for a variant when its applicability
//     covers the variant;
//   - the results that count for a variant are those of runs for the
//     variant or one of its ancestors and of variant-neutral runs, in the
//     configurations the variant is tested in (all when it names none;
//     results without configuration always count);
//   - the coverage of a requirement in a variant is Verdict over its linked
//     tests that apply to the variant (not_applicable when the requirement
//     does not apply).
//
// Variant "" means no variant filter: every item and result counts.
type VariantCoverage struct {
	Variants *VariantSet
	Applic   map[string]*model.Applicability
	R2T      map[string][]string
	Verdict  CoverageFunc
	units    []execUnit
	cache    map[string]ConfigResults
}

// LoadVariantCoverage loads what VariantCoverage needs for a project; r2t
// is requirement → linked tests (store.LinkIndex).
func (s *Store) LoadVariantCoverage(ctx context.Context, tenantID, projectID string, r2t map[string][]string) (*VariantCoverage, error) {
	list, err := s.ListVariants(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	applic, err := s.ListApplicability(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	units, err := s.projectExecutions(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &VariantCoverage{Variants: NewVariantSet(list), Applic: applic, R2T: r2t, units: units}, nil
}

// AppliesTo reports whether an item applies to a variant ("" : yes).
func (vc *VariantCoverage) AppliesTo(itemID, variantID string) bool {
	if variantID == "" {
		return true
	}
	return vc.Variants.Applies(vc.Applic[itemID], variantID)
}

// Counts reports whether a result of a run for runVariant ("" neutral) in
// configuration cfg counts for variantID.
func (vc *VariantCoverage) Counts(runVariant, cfg, variantID string) bool {
	if variantID == "" {
		return true
	}
	if runVariant != "" && !slices.Contains(vc.Variants.Lineage(variantID), runVariant) {
		return false
	}
	v := vc.Variants.Get(variantID)
	if cfg != NoConfiguration && v != nil && len(v.ConfigurationIDs) > 0 && !slices.Contains(v.ConfigurationIDs, cfg) {
		return false
	}
	return true
}

// Results returns the latest results per test and configuration that
// count for a variant.
func (vc *VariantCoverage) Results(variantID string) ConfigResults {
	if vc.cache == nil {
		vc.cache = map[string]ConfigResults{}
	}
	if r, ok := vc.cache[variantID]; ok {
		return r
	}
	r := aggregateUnits(vc.units, func(u *execUnit) bool { return vc.Counts(u.variant, u.cfg, variantID) })
	vc.cache[variantID] = r
	return r
}

// Tests returns the linked tests of a requirement that apply to a variant.
func (vc *VariantCoverage) Tests(requirementID, variantID string) []string {
	var out []string
	for _, t := range vc.R2T[requirementID] {
		if vc.AppliesTo(t, variantID) {
			out = append(out, t)
		}
	}
	return out
}

func (vc *VariantCoverage) verdict(requirementID string, tests []string, res ConfigResults) string {
	if vc.Verdict != nil {
		return vc.Verdict(requirementID, tests, res)
	}
	return DefaultCoverage(requirementID, tests, res)
}

// Requirement is the coverage of a requirement in a variant.
func (vc *VariantCoverage) Requirement(requirementID, variantID string) string {
	if !vc.AppliesTo(requirementID, variantID) {
		return CoverageNotApplicable
	}
	return vc.verdict(requirementID, vc.Tests(requirementID, variantID), vc.Results(variantID))
}

// RequirementIn is the coverage of a requirement in a variant and one
// configuration (NoConfiguration: results without configuration); tests
// not planned in the configuration do not count.
func (vc *VariantCoverage) RequirementIn(requirementID, variantID, cfg string) string {
	if !vc.AppliesTo(requirementID, variantID) {
		return CoverageNotApplicable
	}
	res := vc.Results(variantID)
	tests := vc.Tests(requirementID, variantID)
	if len(tests) == 0 {
		return "none"
	}
	sub := ConfigResults{}
	var planned []string
	for _, t := range tests {
		if st, ok := res[t][cfg]; ok {
			planned = append(planned, t)
			sub[t] = map[string]string{cfg: st}
		}
	}
	if len(planned) == 0 {
		return model.ResultNotRun
	}
	return vc.verdict(requirementID, planned, sub)
}

// TestResult is the combined result of a test over the configurations
// that count for a variant ("" when the test does not apply).
func (vc *VariantCoverage) TestResult(testID, variantID string) string {
	if !vc.AppliesTo(testID, variantID) {
		return ""
	}
	return vc.Results(variantID).Combined(testID)
}

// RunCounts reports whether results of a run for runVariant count for variantID.
func (vc *VariantCoverage) RunCounts(runVariant *string, variantID string) bool {
	rv := ""
	if runVariant != nil {
		rv = *runVariant
	}
	return vc.Counts(rv, NoConfiguration, variantID)
}

// NewVariantCoverage builds a VariantCoverage from variants and
// applicability already loaded; it loads the project's executions.
func (s *Store) NewVariantCoverage(ctx context.Context, tenantID, projectID string, vs *VariantSet, applic map[string]*model.Applicability, r2t map[string][]string) (*VariantCoverage, error) {
	units, err := s.projectExecutions(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return &VariantCoverage{Variants: vs, Applic: applic, R2T: r2t, units: units}, nil
}
