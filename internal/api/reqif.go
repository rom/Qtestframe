package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// ReqIF export of a project: GET /projects/{pid}/export with format=reqif
// or reqifz (see handleExport and docs/api.md "Export"). Besides the
// requirements (with their custom fields, criteria, relations and
// folders) the export can hold, by include=tests,records, the test cases
// with their folders, steps, custom fields and "Verifies" links, and the
// verification records; baseline=<id> exports the frozen versions of a
// baseline's items instead of the current ones.

// reqifFormat returns "reqif" or "reqifz" when the format parameter names
// a ReqIF export, else "".
func reqifFormat(f string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(f)), ".") {
	case "reqif":
		return "reqif"
	case "reqifz":
		return "reqifz"
	}
	return ""
}

// reqifIncludes parses include= (tests, records).
func reqifIncludes(v string) (tests, records bool, err error) {
	for _, x := range strings.Split(v, ",") {
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "":
		case "tests", "test":
			tests = true
		case "records", "verification_records", "verification":
			records = true
		default:
			return false, false, badRequest("include: unknown value %q (use tests, records)", strings.TrimSpace(x))
		}
	}
	return
}

func customStrings(cv model.CustomValues) map[string][]string {
	if len(cv) == 0 {
		return nil
	}
	out := map[string][]string{}
	for k, v := range cv {
		out[k] = customfield.Strings(v)
	}
	return out
}

func reqifCustomDefs(defs []model.CustomField) []export.ReqIFCustomField {
	var out []export.ReqIFCustomField
	for _, f := range defs {
		out = append(out, export.ReqIFCustomField{Key: f.Key, Name: customfield.Column(&f), Type: f.Type, Options: f.Options})
	}
	return out
}

// reqifFolders returns the folders of kind, with keep set only those that
// hold one of the items (directly or below).
func reqifFolders(folders []model.Folder, kind string, itemFolders []*string, filtered bool) []export.ReqIFFolder {
	parent := map[string]string{}
	for _, f := range folders {
		if f.Kind == kind && f.ParentID != nil {
			parent[f.ID] = *f.ParentID
		}
	}
	keep := map[string]bool{}
	for _, fid := range itemFolders {
		for id := fid; id != nil && !keep[*id]; {
			keep[*id] = true
			if pid, ok := parent[*id]; ok {
				id = &pid
			} else {
				id = nil
			}
		}
	}
	var out []export.ReqIFFolder
	for _, f := range folders {
		if f.Kind != kind || (filtered && !keep[f.ID]) {
			continue
		}
		rf := export.ReqIFFolder{ID: f.ID, Name: f.Name, CreatedAt: f.CreatedAt}
		if f.ParentID != nil {
			rf.ParentID = *f.ParentID
		}
		out = append(out, rf)
	}
	return out
}

func (s *Server) exportReqIF(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project, zipped bool) {
	ctx := r.Context()
	q := r.URL.Query()
	withTests, withRecords, err := reqifIncludes(q.Get("include"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if withTests && !p.Can(rbac.TestRead) {
		s.fail(w, r, forbidden("permission test.read required to export tests"))
		return
	}
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	expr := q.Get("q")
	var (
		reqs     []model.Requirement
		tests    []model.TestCase
		baseline *model.Baseline
	)
	if bid := strings.TrimSpace(q.Get("baseline")); bid != "" {
		if strings.TrimSpace(expr) != "" {
			s.fail(w, r, badRequest("q cannot be combined with baseline"))
			return
		}
		if baseline, err = s.Store.GetBaseline(ctx, p.TenantID(), bid); err != nil || baseline.ProjectID != pr.ID {
			if err == nil || errors.Is(err, store.ErrNotFound) {
				err = badRequest("baseline: not found in project")
			}
			s.fail(w, r, err)
			return
		}
		for _, it := range baseline.Items {
			switch it.EntityType {
			case model.KindRequirement:
				var rq model.Requirement
				if json.Unmarshal(it.Snapshot, &rq) == nil && rq.ID != "" {
					reqs = append(reqs, rq)
				}
			case model.KindTest:
				var t model.TestCase
				if withTests && json.Unmarshal(it.Snapshot, &t) == nil && t.ID != "" {
					tests = append(tests, t)
				}
			}
		}
	} else {
		if reqs, err = s.filterRequirements(r, p, pr, pc, reqExpr(expr), "", true); err != nil {
			s.fail(w, r, err)
			return
		}
		if withTests {
			all, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			// with a filter: the tests that verify an exported requirement
			in := map[string]bool{}
			for _, rq := range reqs {
				in[rq.ID] = true
			}
			linked := map[string]bool{}
			for _, l := range pc.links {
				if in[l.RequirementID] {
					linked[l.TestCaseID] = true
				}
			}
			for _, t := range all {
				if strings.TrimSpace(reqExpr(expr)) == "" || linked[t.ID] {
					tests = append(tests, t)
				}
			}
		}
	}
	rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	doc := export.ReqIFDocument{ProjectID: pr.ID, ProjectKey: pr.Key, Title: pr.Key + " – " + pr.Name, ToolVersion: Version, GeneratedAt: now,
		Comment: "Requirements of project " + pr.Key + " exported by Qtestframe"}
	if withTests {
		doc.Comment = "Requirements and tests of project " + pr.Key + " exported by Qtestframe"
	}
	if baseline != nil {
		doc.Comment += "; baseline: " + baseline.Name
		doc.Title += " – " + baseline.Name
	} else if strings.TrimSpace(expr) != "" {
		doc.Comment += "; filter: " + strings.TrimSpace(expr)
	}
	cfDefs, err := s.Store.ListCustomFields(ctx, p.TenantID(), pr.ID, model.KindRequirement)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	doc.CustomFields = reqifCustomDefs(cfDefs)

	// folders: all of the kind, or with a filter or baseline only those
	// that contain an exported item (directly or below)
	limited := baseline != nil || strings.TrimSpace(reqExpr(expr)) != ""
	var fids []*string
	for _, rq := range reqs {
		fids = append(fids, rq.FolderID)
	}
	doc.Folders = reqifFolders(pc.folders, model.KindRequirement, fids, limited)

	attachments := func(kind, id string) ([]export.ReqIFFile, error) {
		if !zipped {
			return nil, nil
		}
		files, err := s.Store.ListAttachments(ctx, p.TenantID(), kind, id)
		if err != nil {
			return nil, err
		}
		var out []export.ReqIFFile
		for _, f := range files {
			id := f.ID
			out = append(out, export.ReqIFFile{ID: f.ID, Name: f.Name, ContentType: f.ContentType,
				Load: func() ([]byte, error) { return s.Store.FileData(ctx, id) }})
		}
		return out, nil
	}
	for i := range reqs {
		rq := &reqs[i]
		_, cats := pc.catValues(rq.CategoryIDs)
		x := export.ReqIFRequirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, Description: rq.Description, Obligation: rq.Obligation,
			Type: rq.Type, Priority: rq.Priority, Status: rq.Status, Source: rq.Source, Rationale: rq.Rationale, Owner: rq.Owner,
			CreatedBy: rq.CreatedBy, Tags: rq.Tags, Categories: cats, Version: rq.Version, CreatedAt: rq.CreatedAt, UpdatedAt: rq.UpdatedAt,
			Custom: customStrings(rq.CustomFields), D3fend: rq.D3fend, Emb3d: rq.Emb3d}
		if rq.FolderID != nil {
			x.FolderID = *rq.FolderID
		}
		for _, c := range rq.Criteria {
			x.Criteria = append(x.Criteria, export.ReqIFCriterion{Description: c.Description, Method: c.Method, Threshold: c.Threshold})
		}
		if x.Attachments, err = attachments(model.KindRequirement, rq.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		doc.Requirements = append(doc.Requirements, x)
	}
	for _, rel := range rels {
		doc.Relations = append(doc.Relations, export.ReqIFRelation{ID: rel.ID, Type: rel.Type, SourceID: rel.SourceID, TargetID: rel.TargetID, CreatedAt: rel.CreatedAt})
	}

	if withTests {
		tcDefs, err := s.Store.ListCustomFields(ctx, p.TenantID(), pr.ID, model.KindTest)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		doc.TestCustomFields = reqifCustomDefs(tcDefs)
		testFolders, err := s.Store.ListFolders(ctx, p.TenantID(), pr.ID, model.KindTest)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		keyOf := map[string]string{}
		if all, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID); err == nil {
			for _, t := range all {
				keyOf[t.ID] = t.Key
			}
		}
		var tf []*string
		for _, t := range tests {
			tf = append(tf, t.FolderID)
			keyOf[t.ID] = t.Key
		}
		doc.TestFolders = reqifFolders(testFolders, model.KindTest, tf, limited)
		for i := range tests {
			t := &tests[i]
			x := export.ReqIFTest{ID: t.ID, Key: t.Key, Title: t.Title, Description: t.Description, Preconditions: t.Preconditions,
				Expected: t.ExpectedResult, ExecutionType: t.ExecutionType, Priority: t.Priority, Status: t.Status, CreatedBy: t.CreatedBy,
				EstimatedMinutes: t.EstimatedMin, Version: t.Version, Tags: t.Tags, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
				Custom: customStrings(t.CustomFields), D3fend: t.D3fend, Emb3d: t.Emb3d}
			if t.FolderID != nil {
				x.FolderID = *t.FolderID
			}
			for _, st := range t.Steps {
				es := export.ReqIFStep{Action: st.Action, Data: st.Data, Expected: st.Expected}
				if st.CallTestID != nil {
					es.Call = firstNonBlank(keyOf[*st.CallTestID], st.CallTestKey)
				}
				x.Steps = append(x.Steps, es)
			}
			if x.Attachments, err = attachments(model.KindTest, t.ID); err != nil {
				s.fail(w, r, err)
				return
			}
			doc.Tests = append(doc.Tests, x)
		}
		for _, l := range pc.links {
			doc.Links = append(doc.Links, export.ReqIFLink{ID: l.ID, RequirementID: l.RequirementID, TestID: l.TestCaseID, CreatedAt: l.CreatedAt})
		}
	}
	if withRecords {
		recs, err := s.Store.ListRecords(ctx, p.TenantID(), pr.ID, "")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, rc := range recs {
			x := export.ReqIFRecord{ID: rc.ID, RequirementID: rc.RequirementID, RequirementKey: rc.RequirementKey, Method: rc.Method,
				Result: rc.Result, Criterion: rc.CriterionText, PerformedBy: rc.PerformedBy, PerformedOn: rc.PerformedOn, Note: rc.Note,
				ValidUntil: rc.ValidUntil, Validity: rc.Validity, CreatedAt: rc.CreatedAt, UpdatedAt: rc.UpdatedAt}
			if rc.Signoff != nil {
				x.SignedOffBy = firstNonBlank(rc.Signoff.Name, rc.Signoff.By)
			}
			doc.Records = append(doc.Records, x)
		}
	}

	name := export.ReqIFFileName(pr.Key, now, zipped)
	format := "reqif"
	if zipped {
		format = "reqifz"
		w.Header().Set("Content-Type", export.ReqIFZContentType)
	} else {
		w.Header().Set("Content-Type", export.ReqIFContentType)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if zipped {
		err = export.WriteReqIFZ(w, doc)
	} else {
		err = export.WriteReqIF(w, doc)
	}
	if err != nil {
		s.Log.Error("reqif export failed", "err", err)
		return
	}
	what := "requirements"
	if withTests {
		what += ", tests"
	}
	if withRecords {
		what += ", verification records"
	}
	if baseline != nil {
		what += " of baseline " + baseline.Name
	}
	s.audit(ctx, p, "project.export", "project", pr.ID, format+" "+what)
}
