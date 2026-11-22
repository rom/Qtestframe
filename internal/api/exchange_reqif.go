package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/interop"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
	"github.com/rom/qtestframe/internal/store"
)

// ReqIF import with re-import (see internal/interop/reqif.go):
// POST /projects/{pid}/exchange/reqif/preview and …/apply, multipart with
// file, mapping (JSON: attribute long name → field key, "heading",
// "external_id", "version" or "" to ignore), folder_id (the folder the
// hierarchy goes below), hierarchy (headings become folders; default
// true); apply also takes overwrite_conflicts and exclude. The IDENTIFIERs
// of the imported objects and relations are kept (external_refs), so the
// next import of the document updates them.

const refSystemReqIF = "reqif"

type reqifTarget struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type relChange struct {
	Ref      string `json:"ref"`
	Type     string `json:"type"`
	FileType string `json:"file_type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Action   string `json:"action"` // add | remove | keep
	// ids, when known
	sourceID, targetID, relationID, refID string
}

type relSummary struct {
	Add     int         `json:"add"`
	Remove  int         `json:"remove"`
	Keep    int         `json:"keep"`
	Changes []relChange `json:"changes"`
}

type reqifPreview struct {
	*interop.Diff
	File          string              `json:"file"`
	Title         string              `json:"title"`
	Tool          string              `json:"tool"`
	Qtestframe    bool                `json:"qtestframe"`
	Attributes    []interop.ReqIFAttr `json:"attributes"`
	Targets       []reqifTarget       `json:"targets"`
	RelationTypes map[string]string   `json:"relation_types"`
	Relations     relSummary          `json:"relations"`
	Folders       int                 `json:"folders"`
	Skipped       []string            `json:"skipped"`
	Errors        []string            `json:"errors"`
	// Previous counts the objects of this document imported before.
	Previous     int    `json:"previous"`
	TargetFolder string `json:"target_folder"`
}

type reqifUpload struct {
	name    string
	model   *reqimport.ReqIFModel
	plan    *interop.ReqIFPlan
	data    *exchangeData
	preview *reqifPreview
	refs    []store.ExternalRef
}

func reqifTargets(fields []interop.Field) []reqifTarget {
	out := []reqifTarget{
		{Key: interop.RoleHeading, Label: "Heading (folder)", Type: "role"},
		{Key: interop.RoleExternalID, Label: "External ID", Type: "role"},
	}
	for _, f := range fields {
		if f.ReadOnly || f.Type == interop.Keys || f.Key == "folder" {
			continue
		}
		out = append(out, reqifTarget{Key: f.Key, Label: f.Label, Type: string(f.Type)})
	}
	return out
}

func (s *Server) readReqIF(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project) (*reqifUpload, error) {
	up, err := readUpload(w, r, "file", maxExchangeBytes)
	if err != nil {
		return nil, err
	}
	m, err := reqimport.ParseReqIFModel(up.data)
	if errors.Is(err, reqimport.ErrInvalid) {
		return nil, badRequest("%s", strings.TrimPrefix(err.Error(), reqimport.ErrInvalid.Error()+": "))
	}
	if err != nil {
		return nil, err
	}
	var mapping map[string]string
	if v := strings.TrimSpace(r.FormValue("mapping")); v != "" && v != "null" {
		if err := json.Unmarshal([]byte(v), &mapping); err != nil {
			return nil, badRequest("mapping must be a JSON object of attribute name → field: %v", err)
		}
	}
	d, err := s.loadExchange(r, p, pr, model.KindRequirement, false)
	if err != nil {
		return nil, err
	}
	if err := interop.CheckMapping(d.fields, mapping); err != nil {
		return nil, badRequest("%v", err)
	}
	root := ""
	if id := strings.TrimSpace(r.FormValue("folder_id")); id != "" {
		var ok bool
		if root, ok = d.paths[id]; !ok {
			return nil, badRequest("folder_id: requirement folder not found in project")
		}
	}
	refs, err := s.Store.ListExternalRefs(r.Context(), p.TenantID(), pr.ID, refSystemReqIF)
	if err != nil {
		return nil, err
	}
	reqRef := map[string]store.ExternalRef{}
	for _, ref := range refs {
		if ref.EntityType == model.KindRequirement && d.byID[ref.EntityID] != nil {
			reqRef[ref.ExternalID] = ref
		}
	}
	previous := 0
	match := func(ident string) (string, int) {
		if ref, ok := reqRef[ident]; ok {
			previous++
			return ref.EntityID, ref.Version
		}
		// an export of this project (see export/reqif.go)
		if id, ok := strings.CutPrefix(ident, "qtf-req-"); ok && d.byID[id] != nil {
			return id, 0
		}
		return "", 0
	}
	plan := interop.PlanReqIF(m, d.fields, interop.ReqIFOptions{Mapping: mapping, Hierarchy: formBool(r, "hierarchy", true), Root: root}, match)
	diff := interop.Compare(model.KindRequirement, d.fields, plan.Rows, d.byID, d.lookup)
	s.checkReviewPerms(p, d, diff)
	cols := map[string]bool{}
	for _, row := range plan.Rows {
		for k := range row.Cells {
			cols[k] = true
		}
	}
	diff.Columns = []interop.Field{}
	for _, f := range d.fields {
		if cols[f.Key] {
			diff.Columns = append(diff.Columns, f)
		}
	}
	diff.Warnings = append(diff.Warnings, m.Warnings...)
	pv := &reqifPreview{Diff: diff, File: up.name, Title: m.Title, Tool: firstNonBlank(m.SourceTool, m.ToolID), Qtestframe: m.Qtestframe,
		Attributes: plan.Attributes, Targets: reqifTargets(d.fields), RelationTypes: plan.RelTypes, Folders: plan.Folders,
		Skipped: plan.Skipped, Errors: m.Errors, Previous: previous, TargetFolder: root}
	u := &reqifUpload{name: up.name, model: m, plan: plan, data: d, preview: pv, refs: refs}
	ids := map[string]string{}
	for i, row := range diff.Rows {
		if row.ID != "" && row.Status != interop.StatusError {
			ids[plan.Rows[i].Ref] = row.ID
		}
	}
	rels, err := s.planRelations(r, p, pr, u, ids)
	if err != nil {
		return nil, err
	}
	pv.Relations = rels
	return u, nil
}

// planRelations compares the relations of the document with the stored
// ones; ids maps object IDENTIFIERs to requirement ids (as far as known).
func (s *Server) planRelations(r *http.Request, p *Principal, pr *model.Project, u *reqifUpload, ids map[string]string) (relSummary, error) {
	out := relSummary{Changes: []relChange{}}
	stored, err := s.Store.ListRelations(r.Context(), p.TenantID(), pr.ID, "")
	if err != nil {
		return out, err
	}
	byID := map[string]model.Relation{}
	byEnds := map[string]model.Relation{}
	for _, rel := range stored {
		byID[rel.ID] = rel
		byEnds[rel.SourceID+"\x00"+rel.TargetID+"\x00"+rel.Type] = rel
	}
	relRef := map[string]store.ExternalRef{}
	for _, ref := range u.refs {
		if ref.EntityType == "relation" {
			relRef[ref.ExternalID] = ref
		}
	}
	label := map[string]string{} // identifier → key or location
	for i, row := range u.plan.Rows {
		label[row.Ref] = firstNonBlank(u.preview.Rows[i].Key, row.Location)
	}
	keyOf := func(id string) string {
		if it := u.data.byID[id]; it != nil {
			return it.Key()
		}
		return id
	}
	inFile := map[string]bool{}
	for _, pl := range u.plan.Relations {
		inFile[pl.Ref] = true
		c := relChange{Ref: pl.Ref, Type: pl.Type, FileType: pl.FileType, Source: label[pl.Source], Target: label[pl.Target],
			sourceID: ids[pl.Source], targetID: ids[pl.Target], Action: "add"}
		if ref, ok := relRef[pl.Ref]; ok {
			c.refID = ref.ID
			if rel, ok := byID[ref.EntityID]; ok && rel.SourceID == c.sourceID && rel.TargetID == c.targetID && rel.Type == c.Type {
				c.Action, c.relationID = "keep", rel.ID
			}
		}
		if c.Action == "add" && c.sourceID != "" && c.targetID != "" {
			if rel, ok := byEnds[c.sourceID+"\x00"+c.targetID+"\x00"+c.Type]; ok {
				c.Action, c.relationID = "keep", rel.ID
			}
		}
		out.Changes = append(out.Changes, c)
	}
	// relations imported before that the document no longer has
	inDoc := map[string]bool{}
	for _, id := range ids {
		inDoc[id] = true
	}
	for _, ref := range u.refs {
		if ref.EntityType != "relation" || inFile[ref.ExternalID] {
			continue
		}
		rel, ok := byID[ref.EntityID]
		if !ok || !inDoc[rel.SourceID] || !inDoc[rel.TargetID] {
			continue
		}
		out.Changes = append(out.Changes, relChange{Ref: ref.ExternalID, Type: rel.Type, Source: keyOf(rel.SourceID), Target: keyOf(rel.TargetID),
			Action: "remove", sourceID: rel.SourceID, targetID: rel.TargetID, relationID: rel.ID, refID: ref.ID})
	}
	for _, c := range out.Changes {
		switch c.Action {
		case "add":
			out.Add++
		case "remove":
			out.Remove++
		default:
			out.Keep++
		}
	}
	sort.SliceStable(out.Changes, func(i, j int) bool { return out.Changes[i].Action < out.Changes[j].Action })
	return out, nil
}

func (s *Server) handleReqIFPreview(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	u, err := s.readReqIF(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pv := *u.preview
	if len(pv.Relations.Changes) > 500 {
		pv.Relations.Changes = pv.Relations.Changes[:500]
	}
	writeJSON(w, http.StatusOK, pv)
}

type reqifApplyResult struct {
	*applyResult
	ReferencesSaved int `json:"references_saved"`
}

func (s *Server) handleReqIFApply(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	u, err := s.readReqIF(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	opt, err := applyOptionsOf(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	res, err := s.applyDiff(r, p, pr, u.data, u.preview.Diff, opt)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := reqifApplyResult{applyResult: res}
	source := firstNonBlank(u.model.ToolID, u.model.SourceTool)
	ids := map[string]string{}
	for i, row := range u.plan.Rows {
		id, ok := res.ids[i]
		if !ok || id == "" {
			continue
		}
		ids[row.Ref] = id
		ref := &store.ExternalRef{ProjectID: pr.ID, System: refSystemReqIF, EntityType: model.KindRequirement, EntityID: id,
			ExternalID: row.Ref, Source: source, Version: res.versions[i]}
		if err := s.Store.SaveExternalRef(ctx, p.TenantID(), ref); err != nil {
			s.fail(w, r, err)
			return
		}
		out.ReferencesSaved++
	}
	// relations between objects of the document, imported now or before
	for i, row := range u.preview.Rows {
		if _, ok := ids[u.plan.Rows[i].Ref]; !ok && row.ID != "" && row.Status != interop.StatusError {
			ids[u.plan.Rows[i].Ref] = row.ID
		}
	}
	rels, err := s.planRelations(r, p, pr, u, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, c := range rels.Changes {
		if c.sourceID == "" || c.targetID == "" {
			continue // an end was not imported
		}
		switch c.Action {
		case "remove":
			if err := s.Store.DeleteRelation(ctx, p.TenantID(), c.relationID); err != nil && !errors.Is(err, store.ErrNotFound) {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "relation.delete", "relation", c.relationID, c.Source+" "+c.Type+" "+c.Target+" (removed in the ReqIF document)")
			_ = s.Store.DeleteExternalRef(ctx, p.TenantID(), c.refID)
			res.RelationsRemoved++
			continue
		case "add":
			rel := &model.Relation{SourceID: c.sourceID, TargetID: c.targetID, Type: c.Type, CreatedBy: p.User.Email}
			if err := s.Store.CreateRelation(ctx, p.TenantID(), rel); err != nil {
				s.fail(w, r, badRequest("SPEC-RELATION %s: %v", c.Ref, err))
				return
			}
			s.audit(ctx, p, "relation.create", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
			c.relationID = rel.ID
			res.RelationsAdded++
		}
		if err := s.Store.SaveExternalRef(ctx, p.TenantID(), &store.ExternalRef{ProjectID: pr.ID, System: refSystemReqIF, EntityType: "relation",
			EntityID: c.relationID, ExternalID: c.Ref, Source: source}); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.audit(ctx, p, "requirement.import", "project", pr.ID, fmt.Sprintf("%s (reqif%s): %s", u.name, toolNote(source), res.summary()))
	writeJSON(w, http.StatusOK, out)
}

func toolNote(tool string) string {
	if tool == "" {
		return ""
	}
	return ", " + tool
}
