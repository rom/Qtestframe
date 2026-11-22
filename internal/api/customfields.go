package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/reqimport"
)

// Custom fields and item templates (customfield, store/customfields.go,
// store/templates.go).
//
// Permissions: reading definitions and templates needs project.read;
// changing a project's fields, templates and default templates
// project.update (the project settings permission); the organisation's
// field sets and templates per project type settings.manage.

func (s *Server) routesCustomFields(h routeFunc) {
	h("GET /api/v1/projects/{pid}/custom-fields", s.handleListCustomFields, "project.read")
	h("POST /api/v1/projects/{pid}/custom-fields", s.handleCreateCustomField, "project.update")
	h("POST /api/v1/projects/{pid}/custom-fields/apply-set", s.handleApplyFieldSet, "project.update")
	h("GET /api/v1/projects/{pid}/custom-fields/archive", s.handleCustomFieldArchive, "project.read")
	h("GET /api/v1/custom-fields/{id}", s.handleGetCustomField, "project.read")
	h("GET /api/v1/custom-fields/{id}/usage", s.handleCustomFieldUsage, "project.read")
	h("PATCH /api/v1/custom-fields/{id}", s.handleUpdateCustomField, "project.update")
	h("DELETE /api/v1/custom-fields/{id}", s.handleDeleteCustomField, "project.update")

	h("GET /api/v1/field-sets", s.handleListFieldSets, "project.read")
	h("POST /api/v1/field-sets/{type}/fields", s.handleCreateFieldSetField, "settings.manage")
	h("PATCH /api/v1/field-sets/fields/{id}", s.handleUpdateFieldSetField, "settings.manage")
	h("DELETE /api/v1/field-sets/fields/{id}", s.handleDeleteFieldSetField, "settings.manage")

	h("GET /api/v1/projects/{pid}/templates", s.handleListProjectTemplates, "project.read")
	h("POST /api/v1/projects/{pid}/templates", s.handleCreateProjectTemplate, "project.update")
	h("PUT /api/v1/projects/{pid}/templates/default", s.handleSetDefaultTemplate, "project.update")
	h("PATCH /api/v1/templates/{id}", s.handleUpdateTemplate(false), "project.update")
	h("DELETE /api/v1/templates/{id}", s.handleDeleteTemplate(false), "project.update")

	h("GET /api/v1/template-sets", s.handleListTemplateSets, "project.read")
	h("POST /api/v1/template-sets/{type}/templates", s.handleCreateOrgTemplate, "settings.manage")
	h("PATCH /api/v1/template-sets/templates/{id}", s.handleUpdateTemplate(true), "settings.manage")
	h("DELETE /api/v1/template-sets/templates/{id}", s.handleDeleteTemplate(true), "settings.manage")
}

// customFieldPrefix makes cf.<key> a field of the query schemas.
var customFieldPrefix = []string{"cf."}

// riskSchema: the query fields of risks.
var riskSchema = &query.Schema{
	Fields:   []string{"id", "key", "title", "description", "category", "likelihood", "impact", "score", "level", "status", "mitigation", "owner", "review_owner", "review_date", "created_by"},
	Prefixes: customFieldPrefix,
}

func riskRecord(k *model.Risk) query.MapRecord {
	return withCustom(k.CustomFields, query.MapRecord{"id": {k.ID}, "key": {k.Key}, "title": {k.Title}, "description": {k.Description},
		"category": {k.Category}, "likelihood": {strconv.Itoa(k.Likelihood)}, "impact": {strconv.Itoa(k.Impact)}, "score": {strconv.Itoa(k.Score)},
		"level": {k.Level}, "status": {k.Status}, "mitigation": {k.Mitigation}, "owner": {k.Owner},
		"review_owner": {k.ReviewOwner}, "review_date": {k.ReviewDate}, "created_by": {k.CreatedBy}})
}

// withCustom adds the custom values of an item to its query record as
// cf.<key>.
func withCustom(cv model.CustomValues, rec query.MapRecord) query.MapRecord {
	for k, v := range cv {
		rec["cf."+k] = customfield.Strings(v)
	}
	return rec
}

// sortByQuery orders items (and their records) by the sort: terms of q.
func sortByQuery[T any](q *query.Query, items []T, recs []query.MapRecord) {
	if len(q.Sort()) == 0 || len(items) != len(recs) {
		return
	}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return q.Compare(recs[idx[a]], recs[idx[b]]) < 0 })
	sorted := make([]T, len(items))
	for i, j := range idx {
		sorted[i] = items[j]
	}
	copy(items, sorted)
}

// ---------------------------------------------------------------- project fields

func (s *Server) handleListCustomFields(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	entity := r.URL.Query().Get("entity_type")
	if entity != "" && !slices.Contains(model.CustomFieldEntities, entity) {
		s.fail(w, r, badRequest("entity_type must be one of %s", strings.Join(model.CustomFieldEntities, ", ")))
		return
	}
	defs, err := s.Store.ListCustomFields(r.Context(), p.TenantID(), pr.ID, entity)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(defs))
}

func (s *Server) handleCreateCustomField(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var f model.CustomField
	if err := decode(r, &f); err != nil {
		s.fail(w, r, err)
		return
	}
	f.ProjectID, f.ProjectType = pr.ID, ""
	if err := s.Store.CreateCustomField(r.Context(), p.TenantID(), &f); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "custom_field.create", "custom_field", f.ID, f.EntityType+"."+f.Key+" ("+f.Type+")")
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleGetCustomField(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, err := s.Store.GetCustomField(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleCustomFieldUsage(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, err := s.Store.GetCustomField(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.Store.CustomFieldUsage(r.Context(), p.TenantID(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": f.ID, "key": f.Key, "items": n})
}

func (s *Server) handleUpdateCustomField(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, err := s.Store.GetCustomField(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// options are replaced as a whole when given
	old := f.Options
	f.Options = nil
	if err := jsonMerge(body, f); err != nil {
		s.fail(w, r, err)
		return
	}
	if f.Options == nil {
		f.Options = old
	}
	if err := s.Store.UpdateCustomField(r.Context(), p.TenantID(), f); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "custom_field.update", "custom_field", f.ID, f.EntityType+"."+f.Key)
	writeJSON(w, http.StatusOK, f)
}

// handleDeleteCustomField deletes a definition; its values move to the
// field archive. Without ?confirm=true it answers 409
// confirmation_required with the number of items that have a value.
func (s *Server) handleDeleteCustomField(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	f, err := s.Store.GetCustomField(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !boolParam(r, "confirm", false) {
		n, err := s.Store.CustomFieldUsage(ctx, p.TenantID(), f)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{"code": "confirmation_required", "usage": n,
			"error": "deleting " + f.Key + " archives the values of " + strconv.Itoa(n) + " items; repeat with ?confirm=true"})
		return
	}
	a, err := s.Store.DeleteCustomField(ctx, p.TenantID(), f.ID, p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "custom_field.delete", "custom_field", f.ID, f.EntityType+"."+f.Key+" ("+strconv.Itoa(a.ItemCount)+" values archived)")
	a.Values = nil
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleCustomFieldArchive(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListCustomFieldArchive(r.Context(), p.TenantID(), pr.ID, boolParam(r, "values", false))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleApplyFieldSet(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		ProjectType string `json:"project_type"`
	}
	if err := decodeOptional(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.ProjectType == "" {
		req.ProjectType = pr.Type
	}
	if !slices.Contains(model.ProjectTypes, req.ProjectType) {
		s.fail(w, r, badRequest("project_type must be one of %s", strings.Join(model.ProjectTypes, ", ")))
		return
	}
	added, skipped, err := s.Store.ApplyFieldSet(r.Context(), p.TenantID(), pr.ID, req.ProjectType)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "field_set.apply", "project", pr.ID, req.ProjectType+": "+strconv.Itoa(len(added))+" fields added")
	writeJSON(w, http.StatusOK, map[string]any{"project_type": req.ProjectType, "added": added, "skipped": skipped})
}

// ---------------------------------------------------------------- field sets

func (s *Server) handleListFieldSets(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	t := r.URL.Query().Get("project_type")
	if t != "" && !slices.Contains(model.ProjectTypes, t) {
		s.fail(w, r, badRequest("project_type must be one of %s", strings.Join(model.ProjectTypes, ", ")))
		return
	}
	list, err := s.Store.ListFieldSet(r.Context(), p.TenantID(), t)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleCreateFieldSetField(w http.ResponseWriter, r *http.Request, p *Principal) {
	var f model.CustomField
	if err := decode(r, &f); err != nil {
		s.fail(w, r, err)
		return
	}
	f.ProjectType, f.ProjectID = r.PathValue("type"), ""
	if err := s.Store.CreateFieldSetField(r.Context(), p.TenantID(), &f); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "field_set.create", "field_set_field", f.ID, f.ProjectType+": "+f.EntityType+"."+f.Key)
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleUpdateFieldSetField(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, err := s.Store.GetFieldSetField(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	old := f.Options
	f.Options = nil
	if err := jsonMerge(body, f); err != nil {
		s.fail(w, r, err)
		return
	}
	if f.Options == nil {
		f.Options = old
	}
	if err := s.Store.UpdateFieldSetField(r.Context(), p.TenantID(), f); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "field_set.update", "field_set_field", f.ID, f.ProjectType+": "+f.EntityType+"."+f.Key)
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFieldSetField(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, err := s.Store.GetFieldSetField(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteFieldSetField(r.Context(), p.TenantID(), f.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "field_set.delete", "field_set_field", f.ID, f.ProjectType+": "+f.EntityType+"."+f.Key)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- templates

// templateLang is the language of the built-in templates for a request.
func templateLang(r *http.Request, p *Principal) string {
	return reportLang(r, p, "")
}

func templateKind(r *http.Request) (string, error) {
	k := r.URL.Query().Get("kind")
	if k != "" && k != model.KindRequirement && k != model.KindTest {
		return "", badRequest("kind must be requirement or test")
	}
	return k, nil
}

// handleListProjectTemplates lists the templates a project can use: its
// own, the organisation's for its type and the built-in ones for its type,
// with the project default marked.
func (s *Server) handleListProjectTemplates(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	kind, err := templateKind(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	own, err := s.Store.ListTemplates(ctx, p.TenantID(), false, pr.ID, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	org, err := s.Store.ListTemplates(ctx, p.TenantID(), true, pr.Type, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defaults, err := s.Store.TemplateDefaults(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := append(append(own, org...), customfield.Builtins(pr.Type, kind, templateLang(r, p))...)
	for i := range out {
		out[i].Default = defaults[out[i].Kind] == out[i].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "defaults": defaults})
}

func (s *Server) handleCreateProjectTemplate(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var t model.ItemTemplate
	if err := decode(r, &t); err != nil {
		s.fail(w, r, err)
		return
	}
	t.ProjectID, t.ProjectType, t.CreatedBy, t.Position = pr.ID, "", p.User.Email, 0
	if err := s.Store.CreateTemplate(r.Context(), p.TenantID(), &t); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "template.create", "template", t.ID, t.Kind+": "+t.Name)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleSetDefaultTemplate(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Kind       string `json:"kind"`
		TemplateID string `json:"template_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetTemplateDefault(r.Context(), p.TenantID(), pr.ID, req.Kind, req.TemplateID); err != nil {
		s.fail(w, r, err)
		return
	}
	detail := req.Kind + ": " + req.TemplateID
	if req.TemplateID == "" {
		detail = req.Kind + ": none"
	}
	s.audit(r.Context(), p, "template.default", "project", pr.ID, detail)
	defaults, err := s.Store.TemplateDefaults(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"defaults": defaults})
}

func (s *Server) handleUpdateTemplate(org bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		t, err := s.Store.GetTemplate(r.Context(), p.TenantID(), org, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		body, err := readBody(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		keys, err := patchKeys(body)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if keys["content"] {
			t.Content = model.TemplateContent{} // content is replaced as a whole
		}
		if err := jsonMerge(body, t); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.UpdateTemplate(r.Context(), p.TenantID(), org, t); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, "template.update", "template", t.ID, t.Kind+": "+t.Name)
		writeJSON(w, http.StatusOK, t)
	}
}

func (s *Server) handleDeleteTemplate(org bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		t, err := s.Store.GetTemplate(r.Context(), p.TenantID(), org, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.DeleteTemplate(r.Context(), p.TenantID(), org, t.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, "template.delete", "template", t.ID, t.Kind+": "+t.Name)
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleListTemplateSets lists the organisation's templates and the
// built-in ones, per project type (?project_type= for one).
func (s *Server) handleListTemplateSets(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	pt := r.URL.Query().Get("project_type")
	if pt != "" && !slices.Contains(model.ProjectTypes, pt) {
		s.fail(w, r, badRequest("project_type must be one of %s", strings.Join(model.ProjectTypes, ", ")))
		return
	}
	kind, err := templateKind(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	org, err := s.Store.ListTemplates(r.Context(), p.TenantID(), true, pt, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := org
	lang := templateLang(r, p)
	for _, t := range model.ProjectTypes {
		if pt == "" || pt == t {
			out = append(out, customfield.Builtins(t, kind, lang)...)
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateOrgTemplate(w http.ResponseWriter, r *http.Request, p *Principal) {
	var t model.ItemTemplate
	if err := decode(r, &t); err != nil {
		s.fail(w, r, err)
		return
	}
	t.ProjectType, t.ProjectID, t.CreatedBy, t.Position = r.PathValue("type"), "", p.User.Email, 0
	if err := s.Store.CreateTemplate(r.Context(), p.TenantID(), &t); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "template.create", "template", t.ID, t.ProjectType+" "+t.Kind+": "+t.Name)
	writeJSON(w, http.StatusCreated, t)
}

// applyFieldSetOnCreate gives a new project the organisation's field set
// of its type.
func (s *Server) applyFieldSetOnCreate(r *http.Request, p *Principal, pr *model.Project) {
	added, _, err := s.Store.ApplyFieldSet(r.Context(), p.TenantID(), pr.ID, pr.Type)
	if err != nil {
		s.Log.Warn("field set not applied", "project", pr.Key, "err", err)
		return
	}
	if len(added) > 0 {
		s.audit(r.Context(), p, "field_set.apply", "project", pr.ID, pr.Type+": "+strconv.Itoa(len(added))+" fields added")
	}
}

// restorableCustom returns the custom values an item gets when it is
// restored to an older version: the old values of the fields the project
// still has (values that no longer fit a field are left out), and the
// current values of keys the project has no field for.
func (s *Server) restorableCustom(ctx context.Context, p *Principal, projectID, entity string, old, cur model.CustomValues) model.CustomValues {
	defs, err := s.Store.ListCustomFields(ctx, p.TenantID(), projectID, entity)
	if err != nil {
		return cur
	}
	out := model.CustomValues{}
	known := map[string]bool{}
	for i := range defs {
		f := &defs[i]
		known[f.Key] = true
		if v, ok := old[f.Key]; ok {
			if n, err := customfield.Normalize(f, v); err == nil && n != nil {
				out[f.Key] = n
			}
		}
	}
	for k, v := range cur {
		if !known[k] {
			out[k] = v
		}
	}
	return out
}

// customColumns are the export column headers of custom fields: "Label"
// or "Label (unit)".
func customColumns(defs []model.CustomField) []string {
	out := make([]string, len(defs))
	for i := range defs {
		out[i] = customfield.Column(&defs[i])
	}
	return out
}

// customCells are an item's values for customColumns, in the re-importable
// text form (numbers without the unit, true/false, options comma
// separated).
func customCells(defs []model.CustomField, values model.CustomValues) []string {
	out := make([]string, len(defs))
	for i := range defs {
		out[i] = customfield.Plain(values[defs[i].Key])
	}
	return out
}

// importCustom converts the custom field texts of an import candidate;
// values that do not fit their field are left out with a warning.
func (s *Server) importCustom(r *http.Request, p *Principal, pr *model.Project, c reqimport.Candidate, warnings *[]string) model.CustomValues {
	if len(c.CustomFields) == 0 {
		return nil
	}
	defs, err := s.Store.ListCustomFields(r.Context(), p.TenantID(), pr.ID, model.KindRequirement)
	if err != nil {
		return nil
	}
	out := model.CustomValues{}
	for i := range defs {
		f := &defs[i]
		text, ok := c.CustomFields[f.Key]
		if !ok {
			continue
		}
		v, err := customfield.Parse(f, text)
		if err != nil {
			*warnings = append(*warnings, c.Location+": "+strings.TrimPrefix(err.Error(), customfield.ErrInvalid.Error()+": ")+"; value not imported")
			continue
		}
		if e, isUser := v.(string); isUser && f.Type == model.FieldUser && !s.Store.IsMemberEmail(r.Context(), p.TenantID(), e) {
			*warnings = append(*warnings, c.Location+": "+f.Key+": "+e+" is not a member of the organisation; value not imported")
			continue
		}
		if v != nil {
			out[f.Key] = v
		}
	}
	return out
}

// mergeCustom merges a custom_fields JSON object of a PATCH into an item's
// values (null removes a value; validated when the item is saved).
func mergeCustom(raw json.RawMessage, dst *model.CustomValues) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if *dst == nil {
		*dst = model.CustomValues{}
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return badRequest("custom_fields must be an object of field key → value: %v", err)
	}
	return nil
}
