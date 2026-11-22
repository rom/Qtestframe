package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/model"
)

// Custom fields (see internal/customfield): definitions per project and
// item kind (custom_fields), the organisation's field sets per project type
// (field_set_fields), the archive of deleted fields' values, and the values
// themselves in the custom_fields JSON column of each item table.

// customFieldTables maps an item kind to the table holding its values.
var customFieldTables = map[string]string{
	model.KindRequirement: "requirements", model.KindTest: "test_cases",
	model.KindDefect: "defects", model.KindRisk: "risks", "run": "test_runs",
}

const fieldCols = `id, project_id, entity_type, key, label, type, unit, options, required, default_value, help, position, show_in_list, created_at, updated_at`
const fieldSetCols = `id, project_type, entity_type, key, label, type, unit, options, required, default_value, help, position, show_in_list, created_at, updated_at`

func scanField(r interface{ Scan(...any) error }, set bool) (*model.CustomField, error) {
	var f model.CustomField
	var opts, def string
	scope := &f.ProjectID
	if set {
		scope = &f.ProjectType
	}
	if err := r.Scan(&f.ID, scope, &f.EntityType, &f.Key, &f.Label, &f.Type, &f.Unit, &opts, &f.Required, &def, &f.Help, &f.Position, &f.ShowInList, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(opts), &f.Options)
	if f.Options == nil {
		f.Options = []string{}
	}
	if def != "" {
		_ = json.Unmarshal([]byte(def), &f.Default)
		f.Default = normalizeDecoded(f.Default)
	}
	return &f, nil
}

func scanFields(rows *sql.Rows, set bool) ([]model.CustomField, error) {
	defer rows.Close()
	out := []model.CustomField{}
	for rows.Next() {
		f, err := scanField(rows, set)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// cfErr turns a customfield validation error into ErrInvalid.
func cfErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, customfield.ErrInvalid) {
		return Invalidf("%s", strings.TrimPrefix(err.Error(), customfield.ErrInvalid.Error()+": "))
	}
	return err
}

func encodeDefault(v any) string {
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// normalizeDecoded turns JSON lists ([]any of strings) into []string.
func normalizeDecoded(v any) any {
	if l, ok := v.([]any); ok {
		out := make([]string, 0, len(l))
		for _, e := range l {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return v
}

// decodeCustom reads a custom_fields column (nil when empty).
func decodeCustom(s string) model.CustomValues {
	if s == "" || s == "{}" {
		return nil
	}
	var m model.CustomValues
	if json.Unmarshal([]byte(s), &m) != nil || len(m) == 0 {
		return nil
	}
	for k, v := range m {
		m[k] = normalizeDecoded(v)
	}
	return m
}

// encodeCustom writes a custom_fields column.
func encodeCustom(m model.CustomValues) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// ---------------------------------------------------------------- project fields

// ListCustomFields returns a project's field definitions of one item kind
// ("" for all) in order.
func (s *Store) ListCustomFields(ctx context.Context, tenantID, projectID, entity string) ([]model.CustomField, error) {
	return listFields(ctx, s.c(ctx), tenantID, projectID, entity)
}

func listFields(ctx context.Context, c conn, tenantID, projectID, entity string) ([]model.CustomField, error) {
	q := `SELECT ` + fieldCols + ` FROM custom_fields WHERE tenant_id = ? AND project_id = ?`
	args := []any{tenantID, projectID}
	if entity != "" {
		q += ` AND entity_type = ?`
		args = append(args, entity)
	}
	rows, err := c.query(ctx, q+` ORDER BY entity_type, position, key`, args...)
	if err != nil {
		return nil, err
	}
	return scanFields(rows, false)
}

// GetCustomField returns a project field definition.
func (s *Store) GetCustomField(ctx context.Context, tenantID, id string) (*model.CustomField, error) {
	f, err := scanField(s.c(ctx).row(ctx, `SELECT `+fieldCols+` FROM custom_fields WHERE id = ? AND tenant_id = ?`, id, tenantID), false)
	if err != nil {
		return nil, mapErr(err, "custom field")
	}
	return f, nil
}

func nextFieldPosition(ctx context.Context, c conn, table, scopeCol string, scope any, entity string) (int, error) {
	var n sql.NullInt64
	err := c.row(ctx, `SELECT MAX(position) FROM `+table+` WHERE `+scopeCol+` = ? AND entity_type = ?`, scope, entity).Scan(&n)
	if !n.Valid {
		return 0, err
	}
	return int(n.Int64) + 1, err
}

// CreateCustomField adds a field definition to a project.
func (s *Store) CreateCustomField(ctx context.Context, tenantID string, f *model.CustomField) error {
	if err := cfErr(customfield.ValidateDefinition(f)); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, f.ProjectID); err != nil {
		return err
	}
	return s.tx(ctx, func(c conn) error { return insertField(ctx, c, s.now(), tenantID, f) })
}

func insertField(ctx context.Context, c conn, now time.Time, tenantID string, f *model.CustomField) error {
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM custom_fields WHERE project_id = ? AND entity_type = ?`, f.ProjectID, f.EntityType).Scan(&n); err != nil {
		return err
	}
	if n >= customfield.MaxFields {
		return Invalidf("at most %d custom fields per item kind", customfield.MaxFields)
	}
	if f.Position <= 0 {
		p, err := nextFieldPosition(ctx, c, "custom_fields", "project_id", f.ProjectID, f.EntityType)
		if err != nil {
			return err
		}
		f.Position = p
	}
	f.ID = newID()
	f.CreatedAt = now
	f.UpdatedAt = f.CreatedAt
	opts, _ := json.Marshal(f.Options)
	_, err := c.exec(ctx, `INSERT INTO custom_fields (`+fieldCols+`, tenant_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.ProjectID, f.EntityType, f.Key, f.Label, f.Type, f.Unit, string(opts), f.Required, encodeDefault(f.Default), f.Help, f.Position, f.ShowInList, f.CreatedAt, f.UpdatedAt, tenantID)
	return mapErr(err, "custom field key")
}

// UpdateCustomField stores the editable parts of a definition: label,
// unit, options, required, default, help, position and show_in_list. The
// key and item kind never change; the type only while no item has a value.
func (s *Store) UpdateCustomField(ctx context.Context, tenantID string, f *model.CustomField) error {
	old, err := s.GetCustomField(ctx, tenantID, f.ID)
	if err != nil {
		return err
	}
	f.ProjectID, f.Key, f.EntityType, f.CreatedAt = old.ProjectID, old.Key, old.EntityType, old.CreatedAt
	if err := cfErr(customfield.ValidateDefinition(f)); err != nil {
		return err
	}
	if f.Type != old.Type {
		n, err := s.CustomFieldUsage(ctx, tenantID, old)
		if err != nil {
			return err
		}
		if n > 0 {
			return Conflictf("the type of %s cannot change while %d items have a value", old.Key, n)
		}
	}
	f.UpdatedAt = s.now()
	opts, _ := json.Marshal(f.Options)
	_, err = s.c(ctx).exec(ctx, `UPDATE custom_fields SET label = ?, type = ?, unit = ?, options = ?, required = ?, default_value = ?, help = ?, position = ?, show_in_list = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		f.Label, f.Type, f.Unit, string(opts), f.Required, encodeDefault(f.Default), f.Help, f.Position, f.ShowInList, f.UpdatedAt, f.ID, tenantID)
	return err
}

// itemValues returns id, key and the custom values of every item of a
// project's kind that has a value for key ("" for any).
func itemValues(ctx context.Context, c conn, entity, projectID, key string) ([]model.ArchivedCFValue, map[string]model.CustomValues, error) {
	table := customFieldTables[entity]
	keyCol := "key"
	if entity == "run" {
		keyCol = "name"
	}
	rows, err := c.query(ctx, `SELECT id, `+keyCol+`, custom_fields FROM `+table+` WHERE project_id = ? AND custom_fields <> '{}'`, projectID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []model.ArchivedCFValue
	all := map[string]model.CustomValues{}
	for rows.Next() {
		var id, k, js string
		if err := rows.Scan(&id, &k, &js); err != nil {
			return nil, nil, err
		}
		m := decodeCustom(js)
		if v, ok := m[key]; ok {
			out = append(out, model.ArchivedCFValue{ID: id, Key: k, Value: v})
			all[id] = m
		}
	}
	return out, all, rows.Err()
}

// CustomFieldUsage returns the number of items with a value for a field.
func (s *Store) CustomFieldUsage(ctx context.Context, tenantID string, f *model.CustomField) (int, error) {
	vals, _, err := itemValues(ctx, s.c(ctx), f.EntityType, f.ProjectID, f.Key)
	return len(vals), err
}

// DeleteCustomField deletes a definition. The values of the items are
// removed from the items and kept in the field archive, which is returned.
func (s *Store) DeleteCustomField(ctx context.Context, tenantID, id, user string) (*model.CustomFieldArchive, error) {
	f, err := s.GetCustomField(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	a := &model.CustomFieldArchive{ID: newID(), ProjectID: f.ProjectID, EntityType: f.EntityType, FieldKey: f.Key, Label: f.Label,
		Definition: *f, DeletedBy: user, DeletedAt: s.now()}
	err = s.tx(ctx, func(c conn) error {
		vals, all, err := itemValues(ctx, c, f.EntityType, f.ProjectID, f.Key)
		if err != nil {
			return err
		}
		table := customFieldTables[f.EntityType]
		for _, v := range vals {
			m := all[v.ID]
			delete(m, f.Key)
			if _, err := c.exec(ctx, `UPDATE `+table+` SET custom_fields = ? WHERE id = ?`, encodeCustom(m), v.ID); err != nil {
				return err
			}
		}
		a.Values, a.ItemCount = vals, len(vals)
		if a.Values == nil {
			a.Values = []model.ArchivedCFValue{}
		}
		def, _ := json.Marshal(a.Definition)
		vj, _ := json.Marshal(a.Values)
		if _, err := c.exec(ctx, `INSERT INTO custom_field_archive (id, tenant_id, project_id, entity_type, field_key, label, definition, item_values, item_count, deleted_by, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, tenantID, a.ProjectID, a.EntityType, a.FieldKey, a.Label, string(def), string(vj), a.ItemCount, a.DeletedBy, a.DeletedAt); err != nil {
			return err
		}
		_, err = c.exec(ctx, `DELETE FROM custom_fields WHERE id = ? AND tenant_id = ?`, f.ID, tenantID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ListCustomFieldArchive returns the archived values of a project's
// deleted fields, newest first (withValues: include the values).
func (s *Store) ListCustomFieldArchive(ctx context.Context, tenantID, projectID string, withValues bool) ([]model.CustomFieldArchive, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, project_id, entity_type, field_key, label, definition, item_values, item_count, deleted_by, deleted_at FROM custom_field_archive WHERE tenant_id = ? AND project_id = ? ORDER BY deleted_at DESC, id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CustomFieldArchive{}
	for rows.Next() {
		var a model.CustomFieldArchive
		var def, vals string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.EntityType, &a.FieldKey, &a.Label, &def, &vals, &a.ItemCount, &a.DeletedBy, &a.DeletedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(def), &a.Definition)
		if withValues {
			_ = json.Unmarshal([]byte(vals), &a.Values)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- field sets

// ListFieldSet returns the organisation's field set of a project type (""
// for every type).
func (s *Store) ListFieldSet(ctx context.Context, tenantID, projectType string) ([]model.CustomField, error) {
	q := `SELECT ` + fieldSetCols + ` FROM field_set_fields WHERE tenant_id = ?`
	args := []any{tenantID}
	if projectType != "" {
		q += ` AND project_type = ?`
		args = append(args, projectType)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY project_type, entity_type, position, key`, args...)
	if err != nil {
		return nil, err
	}
	return scanFields(rows, true)
}

// GetFieldSetField returns a field of an organisation field set.
func (s *Store) GetFieldSetField(ctx context.Context, tenantID, id string) (*model.CustomField, error) {
	f, err := scanField(s.c(ctx).row(ctx, `SELECT `+fieldSetCols+` FROM field_set_fields WHERE id = ? AND tenant_id = ?`, id, tenantID), true)
	if err != nil {
		return nil, mapErr(err, "field set field")
	}
	return f, nil
}

// CreateFieldSetField adds a field to the field set of a project type.
func (s *Store) CreateFieldSetField(ctx context.Context, tenantID string, f *model.CustomField) error {
	if !slices.Contains(model.ProjectTypes, f.ProjectType) {
		return Invalidf("project_type must be one of %s", strings.Join(model.ProjectTypes, ", "))
	}
	if err := cfErr(customfield.ValidateDefinition(f)); err != nil {
		return err
	}
	return s.tx(ctx, func(c conn) error {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM field_set_fields WHERE tenant_id = ? AND project_type = ? AND entity_type = ?`, tenantID, f.ProjectType, f.EntityType).Scan(&n); err != nil {
			return err
		}
		if n >= customfield.MaxFields {
			return Invalidf("at most %d fields per item kind", customfield.MaxFields)
		}
		if f.Position <= 0 {
			var m sql.NullInt64
			if err := c.row(ctx, `SELECT MAX(position) FROM field_set_fields WHERE tenant_id = ? AND project_type = ? AND entity_type = ?`, tenantID, f.ProjectType, f.EntityType).Scan(&m); err != nil {
				return err
			}
			if m.Valid {
				f.Position = int(m.Int64) + 1
			}
		}
		f.ID, f.CreatedAt = newID(), s.now()
		f.UpdatedAt = f.CreatedAt
		opts, _ := json.Marshal(f.Options)
		_, err := c.exec(ctx, `INSERT INTO field_set_fields (`+fieldSetCols+`, tenant_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, f.ProjectType, f.EntityType, f.Key, f.Label, f.Type, f.Unit, string(opts), f.Required, encodeDefault(f.Default), f.Help, f.Position, f.ShowInList, f.CreatedAt, f.UpdatedAt, tenantID)
		return mapErr(err, "field key")
	})
}

// UpdateFieldSetField stores the editable parts of a field set field
// (everything but the project type, item kind and key).
func (s *Store) UpdateFieldSetField(ctx context.Context, tenantID string, f *model.CustomField) error {
	old, err := s.GetFieldSetField(ctx, tenantID, f.ID)
	if err != nil {
		return err
	}
	f.ProjectType, f.Key, f.EntityType, f.CreatedAt = old.ProjectType, old.Key, old.EntityType, old.CreatedAt
	if err := cfErr(customfield.ValidateDefinition(f)); err != nil {
		return err
	}
	f.UpdatedAt = s.now()
	opts, _ := json.Marshal(f.Options)
	_, err = s.c(ctx).exec(ctx, `UPDATE field_set_fields SET label = ?, type = ?, unit = ?, options = ?, required = ?, default_value = ?, help = ?, position = ?, show_in_list = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		f.Label, f.Type, f.Unit, string(opts), f.Required, encodeDefault(f.Default), f.Help, f.Position, f.ShowInList, f.UpdatedAt, f.ID, tenantID)
	return err
}

// DeleteFieldSetField removes a field from a field set (projects keep the
// copies they already have).
func (s *Store) DeleteFieldSetField(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM field_set_fields WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ApplyFieldSet copies the organisation's field set of a project type into
// a project: fields whose key the project already has for the item kind are
// skipped. It returns the keys added and skipped ("<kind>.<key>").
func (s *Store) ApplyFieldSet(ctx context.Context, tenantID, projectID, projectType string) (added, skipped []string, err error) {
	set, err := s.ListFieldSet(ctx, tenantID, projectType)
	if err != nil {
		return nil, nil, err
	}
	added, skipped = []string{}, []string{}
	err = s.tx(ctx, func(c conn) error {
		have, err := listFields(ctx, c, tenantID, projectID, "")
		if err != nil {
			return err
		}
		exists := map[string]bool{}
		for _, f := range have {
			exists[f.EntityType+"."+f.Key] = true
		}
		for _, f := range set {
			name := f.EntityType + "." + f.Key
			if exists[name] {
				skipped = append(skipped, name)
				continue
			}
			nf := f
			nf.ProjectType, nf.ProjectID, nf.Position = "", projectID, 0
			if err := insertField(ctx, c, s.now(), tenantID, &nf); err != nil {
				return err
			}
			exists[name] = true
			added = append(added, name)
		}
		return nil
	})
	return added, skipped, err
}

// ---------------------------------------------------------------- values

// isMember reports whether an e-mail address is a member of the tenant.
func isMember(ctx context.Context, c conn, tenantID, email string) bool {
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM memberships m JOIN users u ON u.id = m.user_id WHERE m.tenant_id = ? AND LOWER(u.email) = ?`, tenantID, strings.ToLower(email)).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// IsMemberEmail reports whether an e-mail address belongs to a member of
// the organisation (the values of user fields).
func (s *Store) IsMemberEmail(ctx context.Context, tenantID, email string) bool {
	return isMember(ctx, s.c(ctx), tenantID, email)
}

// checkCustom validates an item's custom values against the definitions
// of its project and kind (old: the stored values on update, nil on create).
func checkCustom(ctx context.Context, c conn, tenantID, projectID, entity string, values, old model.CustomValues, create bool) (model.CustomValues, error) {
	return checkCustomOpt(ctx, c, tenantID, projectID, entity, values, old, create, false)
}

func checkCustomOpt(ctx context.Context, c conn, tenantID, projectID, entity string, values, old model.CustomValues, create, lenient bool) (model.CustomValues, error) {
	defs, err := listFields(ctx, c, tenantID, projectID, entity)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 && len(values) == 0 {
		return nil, nil
	}
	out, err := customfield.Validate(defs, values, customfield.Options{Create: create, Lenient: lenient, Old: old,
		IsUser: func(email string) bool { return isMember(ctx, c, tenantID, email) }})
	return out, cfErr(err)
}

// CheckCustomValues validates custom values for a new item of a project
// (used by imports to report errors per row before saving).
func (s *Store) CheckCustomValues(ctx context.Context, tenantID, projectID, entity string, values model.CustomValues) (model.CustomValues, error) {
	return checkCustom(ctx, s.c(ctx), tenantID, projectID, entity, values, nil, true)
}

// CustomFieldsByKind returns a project's definitions grouped by item kind.
func (s *Store) CustomFieldsByKind(ctx context.Context, tenantID, projectID string) (map[string][]model.CustomField, error) {
	defs, err := s.ListCustomFields(ctx, tenantID, projectID, "")
	if err != nil {
		return nil, err
	}
	out := map[string][]model.CustomField{}
	for _, d := range defs {
		out[d.EntityType] = append(out[d.EntityType], d)
	}
	for k := range out {
		sort.SliceStable(out[k], func(i, j int) bool { return out[k][i].Position < out[k][j].Position })
	}
	return out, nil
}

// checkItemCustom validates the custom values of a stored item being
// updated against its project's definitions (the stored values are the
// old ones). A missing item is ErrNotFound.
func (s *Store) checkItemCustom(ctx context.Context, tenantID, entity, id string, values model.CustomValues) (model.CustomValues, error) {
	c := s.c(ctx)
	var projectID, old string
	if err := c.row(ctx, `SELECT project_id, custom_fields FROM `+customFieldTables[entity]+` WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&projectID, &old); err != nil {
		return nil, mapErr(err, entity)
	}
	return checkCustom(ctx, c, tenantID, projectID, entity, values, decodeCustom(old), false)
}

type lenientKey struct{}

// lenientCustomFields marks ctx so that new items need not fill required
// custom fields (imports; defaults still apply).
func lenientCustomFields(ctx context.Context) context.Context {
	return context.WithValue(ctx, lenientKey{}, true)
}

func isLenientCustom(ctx context.Context) bool {
	v, _ := ctx.Value(lenientKey{}).(bool)
	return v
}
