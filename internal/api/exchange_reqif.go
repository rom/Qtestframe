package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/interop"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/reqimport"
	"github.com/rom/qtestframe/internal/store"
)

// ReqIF import with re-import (see internal/interop/reqif.go):
// POST /projects/{pid}/exchange/reqif/preview and …/apply, multipart with
//
//	file            the .reqif or .reqifz
//	mapping         JSON: requirement attribute long name → field key,
//	                "heading", "external_id", "version" or "" to ignore
//	test_mapping    the same for the attributes of test objects ("steps"
//	                for the test steps)
//	object_types    JSON: SPEC-OBJECT-TYPE long name → requirement, test,
//	                heading or ignore
//	relation_types  JSON: SPEC-RELATION-TYPE long name → relation type,
//	                verifies (a trace link) or ignore
//	folder_id       the requirement folder the hierarchy goes below
//	test_folder_id  the test folder the tests' hierarchy goes below
//	hierarchy       headings become folders (default true)
//	refines         a requirement nested below another refines it
//	                (default false)
//
// apply also takes overwrite_conflicts, create, exclude (requirement rows)
// and exclude_tests (test rows). The IDENTIFIERs of the imported objects,
// relations and links are kept (external_refs), so the next import of the
// document updates them. Files embedded in the XHTML of a .reqifz
// (<object data="files/…">, images) become attachments of the item and
// the Markdown points to them.

const refSystemReqIF = "reqif"

// External reference entity types besides the item kinds.
const (
	refRelation = "relation"
	refLink     = "link"
)

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
	// Kind: relation (between requirements) or link (test → requirement)
	Kind string `json:"kind"`
	// ids, when known
	sourceID, targetID, relationID, refID string
}

type relSummary struct {
	Add     int         `json:"add"`
	Remove  int         `json:"remove"`
	Keep    int         `json:"keep"`
	Links   int         `json:"links"`
	Changes []relChange `json:"changes"`
}

// reqifFile is a file embedded in an object of a .reqifz.
type reqifFile struct {
	Object string `json:"object"` // IDENTIFIER
	Ref    string `json:"ref"`    // as referenced in the XHTML
	Name   string `json:"name"`
	Size   int    `json:"size"`
	// Existing: the item already has this file (same name and content).
	Existing    bool `json:"existing"`
	data        []byte
	contentType string
	fileID      string
}

type reqifPreview struct {
	*interop.Diff
	// Tests is the diff of the test objects (nil without any).
	Tests         *interop.Diff          `json:"tests"`
	File          string                 `json:"file"`
	Title         string                 `json:"title"`
	Tool          string                 `json:"tool"`
	Qtestframe    bool                   `json:"qtestframe"`
	Attributes    []interop.ReqIFAttr    `json:"attributes"`
	TestAttribs   []interop.ReqIFAttr    `json:"test_attributes"`
	Targets       []reqifTarget          `json:"targets"`
	TestTargets   []reqifTarget          `json:"test_targets"`
	ObjectTypes   []interop.ReqIFType    `json:"object_types"`
	RelationTypes map[string]string      `json:"relation_types"`
	RelTypeMap    []interop.ReqIFRelType `json:"relation_type_mapping"`
	// RelationTargets are the choices of the relation type mapping.
	RelationTargets []string    `json:"relation_targets"`
	Relations       relSummary  `json:"relations"`
	Folders         int         `json:"folders"`
	TestFolders     int         `json:"test_folders"`
	Files           []reqifFile `json:"files"`
	Skipped         []string    `json:"skipped"`
	Errors          []string    `json:"errors"`
	// Previous counts the objects of this document imported before.
	Previous         int    `json:"previous"`
	TargetFolder     string `json:"target_folder"`
	TestTargetFolder string `json:"test_target_folder"`
	// CanImportTests: the caller may write tests (else test objects are
	// left out).
	CanImportTests bool `json:"can_import_tests"`
}

type reqifUpload struct {
	name    string
	model   *reqimport.ReqIFModel
	plan    *interop.ReqIFPlan
	data    *exchangeData
	tdata   *exchangeData
	preview *reqifPreview
	refs    []store.ExternalRef
	refines bool
	// files by object IDENTIFIER
	files map[string][]*reqifFile
	opts  reqifPlanOpts
}

// reqifPlanOpts control planReqIF: the ReqIF import, or an import from
// another tool whose items are put into a ReqIF model (reqsources.go).
type reqifPlanOpts struct {
	// system of the external references (refSystemReqIF, "jira" …),
	// format (for the audit log) and origin ("the ReqIF document", for
	// the audit of removed relations and links).
	system, format, origin string
	// mappings (see the multipart fields above)
	mapping, testMapping, objectTypes, relationTypes map[string]string
	folderID, testFolderID                           string
	hierarchy, refines                               bool
	// noTests imports requirements only; noRelations leaves the relations
	// alone (neither added nor removed).
	noTests, noRelations bool
	// moreFields adds requirement fields the mapping may target (custom
	// fields).
	moreFields []interop.Field
	// adjust changes the planned rows before they are compared.
	adjust func(d *exchangeData, plan *interop.ReqIFPlan)
}

func reqifTargets(fields []interop.Field, test bool) []reqifTarget {
	out := []reqifTarget{
		{Key: interop.RoleHeading, Label: "Heading (folder)", Type: "role"},
		{Key: interop.RoleExternalID, Label: "External ID", Type: "role"},
	}
	if test {
		out = append(out, reqifTarget{Key: interop.RoleSteps, Label: "Test steps", Type: "role"})
	}
	for _, f := range fields {
		if f.ReadOnly || f.Type == interop.Keys || f.Key == "folder" {
			continue
		}
		out = append(out, reqifTarget{Key: f.Key, Label: f.Label, Type: string(f.Type)})
	}
	return out
}

func jsonMap(r *http.Request, name, what string) (map[string]string, error) {
	var out map[string]string
	if v := strings.TrimSpace(r.FormValue(name)); v != "" && v != "null" {
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, badRequest("%s must be a JSON object of %s: %v", name, what, err)
		}
	}
	return out, nil
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
	mapping, err := jsonMap(r, "mapping", "attribute name → field")
	if err != nil {
		return nil, err
	}
	testMapping, err := jsonMap(r, "test_mapping", "attribute name → field")
	if err != nil {
		return nil, err
	}
	objectTypes, err := jsonMap(r, "object_types", "type name → requirement, test, heading or ignore")
	if err != nil {
		return nil, err
	}
	relationTypes, err := jsonMap(r, "relation_types", "relation type name → relation type, verifies or ignore")
	if err != nil {
		return nil, err
	}
	if err := interop.CheckTypeMappings(objectTypes, relationTypes); err != nil {
		return nil, badRequest("%v", err)
	}
	return s.planReqIF(r, p, pr, up.name, m, reqifPlanOpts{system: refSystemReqIF, format: "reqif", origin: "the ReqIF document",
		mapping: mapping, testMapping: testMapping, objectTypes: objectTypes, relationTypes: relationTypes,
		folderID: strings.TrimSpace(r.FormValue("folder_id")), testFolderID: strings.TrimSpace(r.FormValue("test_folder_id")),
		hierarchy: formBool(r, "hierarchy", true), refines: formBool(r, "refines", false)})
}

// planReqIF maps a ReqIF model to requirements and tests and compares
// them with the stored ones (the items, relations and links imported from
// it before are found by their external references in o.system).
func (s *Server) planReqIF(r *http.Request, p *Principal, pr *model.Project, name string, m *reqimport.ReqIFModel, o reqifPlanOpts) (*reqifUpload, error) {
	mapping, testMapping := o.mapping, o.testMapping
	d, err := s.loadExchange(r, p, pr, model.KindRequirement, false)
	if err != nil {
		return nil, err
	}
	d.fields = append(d.fields, o.moreFields...)
	if err := interop.CheckMapping(d.fields, mapping); err != nil {
		return nil, badRequest("%v", err)
	}
	canTests := p.Can(rbac.TestWrite) && !o.noTests
	var td *exchangeData
	if canTests {
		if td, err = s.loadExchange(r, p, pr, model.KindTest, false); err != nil {
			return nil, err
		}
		if err := interop.CheckMapping(td.fields, testMapping); err != nil {
			return nil, badRequest("test_mapping: %v", err)
		}
	}
	root, testRoot := "", ""
	if id := o.folderID; id != "" {
		var ok bool
		if root, ok = d.paths[id]; !ok {
			return nil, badRequest("folder_id: requirement folder not found in project")
		}
	}
	if id := o.testFolderID; id != "" && td != nil {
		var ok bool
		if testRoot, ok = td.paths[id]; !ok {
			return nil, badRequest("test_folder_id: test folder not found in project")
		}
	}
	refs, err := s.Store.ListExternalRefs(r.Context(), p.TenantID(), pr.ID, o.system)
	if err != nil {
		return nil, err
	}
	reqRef, testRef := map[string]store.ExternalRef{}, map[string]store.ExternalRef{}
	for _, ref := range refs {
		switch {
		case ref.EntityType == model.KindRequirement && d.byID[ref.EntityID] != nil:
			reqRef[ref.ExternalID] = ref
		case ref.EntityType == model.KindTest && td != nil && td.byID[ref.EntityID] != nil:
			testRef[ref.ExternalID] = ref
		}
	}
	previous := 0
	matcher := func(refs map[string]store.ExternalRef, prefix string, byID map[string]*interop.Item) interop.Match {
		return func(ident string) (string, int) {
			if ref, ok := refs[ident]; ok {
				previous++
				return ref.EntityID, ref.Version
			}
			// an export of this project (see export/reqif.go)
			if id, ok := strings.CutPrefix(ident, prefix); ok && byID[id] != nil {
				return id, 0
			}
			return "", 0
		}
	}
	opt := interop.ReqIFOptions{Mapping: mapping, TestMapping: testMapping, ObjectTypes: o.objectTypes, RelationTypes: o.relationTypes,
		Hierarchy: o.hierarchy, Refines: o.refines, Root: root, TestRoot: testRoot}
	if td != nil {
		opt.TestFields = td.fields
		opt.MatchTest = matcher(testRef, "qtf-test-", td.byID)
	}
	plan := interop.PlanReqIF(m, d.fields, opt, matcher(reqRef, "qtf-req-", d.byID))
	if o.noRelations {
		plan.Relations = nil
	}
	if o.adjust != nil {
		o.adjust(d, plan)
	}
	u := &reqifUpload{name: name, model: m, plan: plan, data: d, tdata: td, refs: refs, refines: opt.Refines, files: map[string][]*reqifFile{}, opts: o}

	// embedded files: those the item has already stand for their path
	if err := s.planReqIFFiles(r, p, u); err != nil {
		return nil, err
	}
	if td != nil {
		s.resolveCalls(u)
	}

	diff := interop.Compare(model.KindRequirement, d.fields, plan.Rows, d.byID, d.lookup)
	s.checkReviewPerms(p, d, diff)
	diff.Columns = usedColumns(d.fields, plan.Rows)
	diff.Warnings = append(diff.Warnings, m.Warnings...)
	if !canTests && !o.noTests {
		for _, t := range plan.Types {
			if interop.GuessObjectKind(t.Name) == interop.ObjTest {
				diff.Warnings = append(diff.Warnings, fmt.Sprintf("the %d objects of the type %q are test cases: importing them needs the permission test.write", t.Objects, t.Name))
			}
		}
	}
	targets := []string{interop.RelVerifies}
	targets = append(targets, model.RelationTypes...)
	pv := &reqifPreview{Diff: diff, File: name, Title: m.Title, Tool: firstNonBlank(m.SourceTool, m.ToolID), Qtestframe: m.Qtestframe,
		Attributes: plan.Attributes, TestAttribs: plan.TestAttributes, Targets: reqifTargets(d.fields, false), ObjectTypes: plan.Types,
		RelationTypes: plan.RelTypes, RelTypeMap: plan.RelationTypes, RelationTargets: append(targets, interop.RelIgnore),
		Folders: plan.Folders, TestFolders: plan.TestFolders, Skipped: plan.Skipped, Errors: m.Errors, TargetFolder: root,
		TestTargetFolder: testRoot, CanImportTests: canTests, Files: []reqifFile{}}
	if td != nil {
		pv.TestTargets = reqifTargets(td.fields, true)
		if len(plan.TestRows) > 0 {
			tdiff := interop.Compare(model.KindTest, td.fields, plan.TestRows, td.byID, td.lookup)
			s.checkReviewPerms(p, td, tdiff)
			tdiff.Columns = usedColumns(td.fields, plan.TestRows)
			pv.Tests = tdiff
		}
	}
	pv.Previous = previous
	for _, row := range append(append([]interop.Incoming{}, plan.Rows...), plan.TestRows...) {
		for _, f := range u.files[row.Ref] {
			pv.Files = append(pv.Files, *f)
		}
	}
	u.preview = pv
	if o.noRelations {
		pv.Relations = relSummary{Changes: []relChange{}}
		return u, nil
	}
	rels, err := s.planRelations(r, p, pr, u, u.knownIDs(nil, nil))
	if err != nil {
		return nil, err
	}
	pv.Relations = rels
	return u, nil
}

func usedColumns(fields []interop.Field, rows []interop.Incoming) []interop.Field {
	cols := map[string]bool{}
	for _, row := range rows {
		for k := range row.Cells {
			cols[k] = true
		}
	}
	out := []interop.Field{}
	for _, f := range fields {
		if cols[f.Key] {
			out = append(out, f)
		}
	}
	return out
}

// knownIDs maps object IDENTIFIERs to item ids: the rows matched to items
// and, after an apply, the created ones (res, tres).
func (u *reqifUpload) knownIDs(res, tres *applyResult) map[string]string {
	ids := map[string]string{}
	add := func(rows []interop.Incoming, diff *interop.Diff, res *applyResult) {
		if diff == nil {
			return
		}
		for i, row := range diff.Rows {
			if res != nil {
				if id, ok := res.ids[i]; ok && id != "" {
					ids[rows[i].Ref] = id
					continue
				}
			}
			if row.ID != "" && row.Status != interop.StatusError {
				ids[rows[i].Ref] = row.ID
			}
		}
	}
	add(u.plan.Rows, u.preview.Diff, res)
	add(u.plan.TestRows, u.preview.Tests, tres)
	return ids
}

// planReqIFFiles finds the files of the archive embedded in the objects'
// XHTML. For an item that has the same file already (same name and
// content, e.g. on a re-import), the Markdown is pointed to that file so
// that it does not show as a change.
func (s *Server) planReqIFFiles(r *http.Request, p *Principal, u *reqifUpload) error {
	objects := map[string]*reqimport.ReqIFObject{}
	for i := range u.model.Objects {
		objects[u.model.Objects[i].Identifier] = &u.model.Objects[i]
	}
	do := func(rows []interop.Incoming, kind string) error {
		for i := range rows {
			row := &rows[i]
			o := objects[row.Ref]
			if o == nil {
				continue
			}
			names := make([]string, 0, len(o.XHTML))
			for n := range o.XHTML {
				names = append(names, n)
			}
			sort.Strings(names)
			seen := map[string]bool{}
			var files []*reqifFile
			for _, n := range names {
				for _, ref := range reqimport.EmbeddedRefs(o.XHTML[n]) {
					data, zipPath, ok := u.model.File(ref)
					if !ok || seen[zipPath] {
						continue
					}
					seen[zipPath] = true
					ct := mime.TypeByExtension(strings.ToLower(path.Ext(zipPath)))
					if ct == "" {
						ct = http.DetectContentType(data)
					}
					files = append(files, &reqifFile{Object: row.Ref, Ref: ref, Name: path.Base(zipPath), Size: len(data), data: data, contentType: ct})
				}
			}
			if len(files) == 0 {
				continue
			}
			u.files[row.Ref] = files
			if row.ID == "" {
				continue
			}
			have, err := s.Store.ListAttachments(r.Context(), p.TenantID(), kind, row.ID)
			if err != nil {
				return err
			}
			for _, f := range files {
				sum := sha256.Sum256(f.data)
				for _, h := range have {
					if h.SHA256 == hex.EncodeToString(sum[:]) && strings.EqualFold(h.Name, store.CleanFileName(f.Name)) {
						f.Existing, f.fileID = true, h.ID
					}
				}
				if f.Existing {
					for k, c := range row.Cells {
						c.Raw = pointFile(c.Raw, f.Ref, f.fileID)
						row.Cells[k] = c
					}
				}
			}
		}
		return nil
	}
	if err := do(u.plan.Rows, model.KindRequirement); err != nil {
		return err
	}
	return do(u.plan.TestRows, model.KindTest)
}

// pointFile replaces Markdown links and images to ref by the file's URL.
func pointFile(text, ref, fileID string) string {
	return strings.ReplaceAll(text, "]("+ref+")", "](/api/v1/files/"+fileID+"/content)")
}

// resolveCalls turns the test keys of call steps (keys in the other tool)
// into keys of this project: the test the called object was imported as,
// else a test with that key; a call that cannot be resolved (a test that
// is new in this import) becomes a plain step.
func (s *Server) resolveCalls(u *reqifUpload) {
	for i := range u.plan.TestRows {
		row := &u.plan.TestRows[i]
		for j := range row.Steps {
			st := &row.Steps[j]
			call := strings.TrimSpace(st.Call)
			if call == "" {
				continue
			}
			if ident, ok := u.plan.TestKeys[strings.ToLower(call)]; ok {
				id := ""
				for _, tr := range u.plan.TestRows {
					if tr.Ref == ident {
						id = tr.ID
					}
				}
				if it := u.tdata.byID[id]; id != "" && it != nil {
					st.Call = it.Key()
					continue
				}
			} else if _, ok := u.tdata.lookup(model.KindTest, call); ok {
				continue
			}
			st.Action = strings.TrimSpace("Call " + call + " " + st.Action)
			st.Call = ""
			row.Warnings = append(row.Warnings, "the called test "+call+" is not in the project yet: imported as a plain step (import again to make it a call)")
		}
	}
}

// planRelations compares the relations and links of the document with
// the stored ones; ids maps object IDENTIFIERs to item ids (as far as
// known).
func (s *Server) planRelations(r *http.Request, p *Principal, pr *model.Project, u *reqifUpload, ids map[string]string) (relSummary, error) {
	ctx := r.Context()
	out := relSummary{Changes: []relChange{}}
	stored, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
	if err != nil {
		return out, err
	}
	links, err := s.Store.ListLinks(ctx, p.TenantID(), pr.ID, "", "")
	if err != nil {
		return out, err
	}
	byID := map[string]model.Relation{}
	byEnds := map[string]model.Relation{}
	for _, rel := range stored {
		byID[rel.ID] = rel
		byEnds[rel.SourceID+"\x00"+rel.TargetID+"\x00"+rel.Type] = rel
	}
	linkByID := map[string]model.TraceLink{}
	linkByEnds := map[string]model.TraceLink{}
	for _, l := range links {
		linkByID[l.ID] = l
		linkByEnds[l.TestCaseID+"\x00"+l.RequirementID] = l
	}
	relRef, linkRef := map[string]store.ExternalRef{}, map[string]store.ExternalRef{}
	for _, ref := range u.refs {
		switch ref.EntityType {
		case refRelation:
			relRef[ref.ExternalID] = ref
		case refLink:
			linkRef[ref.ExternalID] = ref
		}
	}
	label := map[string]string{} // identifier → key or location
	for i, row := range u.plan.Rows {
		label[row.Ref] = firstNonBlank(u.preview.Rows[i].Key, row.Location)
	}
	if u.preview.Tests != nil {
		for i, row := range u.plan.TestRows {
			label[row.Ref] = firstNonBlank(u.preview.Tests.Rows[i].Key, row.Location)
		}
	}
	keyOf := func(id string) string {
		if it := u.data.byID[id]; it != nil {
			return it.Key()
		}
		if u.tdata != nil {
			if it := u.tdata.byID[id]; it != nil {
				return it.Key()
			}
		}
		return id
	}
	inFile := map[string]bool{}
	for _, pl := range u.plan.Relations {
		if pl.Type == interop.RelVerifies && u.tdata == nil {
			continue
		}
		inFile[pl.Ref] = true
		c := relChange{Ref: pl.Ref, Type: pl.Type, FileType: pl.FileType, Source: label[pl.Source], Target: label[pl.Target],
			sourceID: ids[pl.Source], targetID: ids[pl.Target], Action: "add", Kind: refRelation}
		if pl.Type == interop.RelVerifies {
			c.Kind = refLink
			if ref, ok := linkRef[pl.Ref]; ok {
				c.refID = ref.ID
				if l, ok := linkByID[ref.EntityID]; ok && l.TestCaseID == c.sourceID && l.RequirementID == c.targetID {
					c.Action, c.relationID = "keep", l.ID
				}
			}
			if c.Action == "add" && c.sourceID != "" && c.targetID != "" {
				if l, ok := linkByEnds[c.sourceID+"\x00"+c.targetID]; ok {
					c.Action, c.relationID = "keep", l.ID
				}
			}
		} else {
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
		}
		out.Changes = append(out.Changes, c)
	}
	// relations and links imported before that the document no longer has
	inDoc := map[string]bool{}
	for _, id := range ids {
		inDoc[id] = true
	}
	for _, ref := range u.refs {
		if inFile[ref.ExternalID] || (strings.HasPrefix(ref.ExternalID, "hierarchy:") && !u.refines) {
			continue
		}
		switch ref.EntityType {
		case refRelation:
			rel, ok := byID[ref.EntityID]
			if !ok || !inDoc[rel.SourceID] || !inDoc[rel.TargetID] {
				continue
			}
			out.Changes = append(out.Changes, relChange{Ref: ref.ExternalID, Type: rel.Type, Source: keyOf(rel.SourceID), Target: keyOf(rel.TargetID),
				Action: "remove", Kind: refRelation, sourceID: rel.SourceID, targetID: rel.TargetID, relationID: rel.ID, refID: ref.ID})
		case refLink:
			l, ok := linkByID[ref.EntityID]
			if !ok || u.tdata == nil || !inDoc[l.TestCaseID] || !inDoc[l.RequirementID] {
				continue
			}
			out.Changes = append(out.Changes, relChange{Ref: ref.ExternalID, Type: interop.RelVerifies, Source: keyOf(l.TestCaseID), Target: keyOf(l.RequirementID),
				Action: "remove", Kind: refLink, sourceID: l.TestCaseID, targetID: l.RequirementID, relationID: l.ID, refID: ref.ID})
		}
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
		if c.Kind == refLink {
			out.Links++
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
	// Tests are the test rows' share of the totals (nil without tests).
	Tests           *applyResult `json:"tests"`
	ReferencesSaved int          `json:"references_saved"`
	FilesAttached   int          `json:"files_attached"`
}

// prepareFiles stores the new embedded files of the rows that will be
// saved and points the rows' Markdown to them.
func (s *Server) prepareFiles(r *http.Request, p *Principal, pr *model.Project, u *reqifUpload, rows []interop.Incoming, diff *interop.Diff) error {
	if diff == nil {
		return nil
	}
	for i := range diff.Rows {
		rd := &diff.Rows[i]
		for _, f := range u.files[rows[i].Ref] {
			if f.Existing || rd.Status == interop.StatusError {
				continue
			}
			fi := &model.FileInfo{ProjectID: &pr.ID, Kind: model.FileAttachment, Name: f.Name, ContentType: f.contentType, UploadedBy: p.User.Email}
			if err := s.Store.CreateFile(r.Context(), p.TenantID(), fi, f.data); err != nil {
				return err
			}
			f.fileID = fi.ID
			for k, v := range rd.Values {
				rd.Values[k] = pointFile(v, f.Ref, fi.ID)
			}
			for j := range rd.Steps {
				rd.Steps[j].Action = pointFile(rd.Steps[j].Action, f.Ref, fi.ID)
				rd.Steps[j].Expected = pointFile(rd.Steps[j].Expected, f.Ref, fi.ID)
			}
		}
	}
	return nil
}

// attachFiles attaches the new files to the saved items (and drops those
// of rows that were not saved).
func (s *Server) attachFiles(r *http.Request, p *Principal, u *reqifUpload, rows []interop.Incoming, res *applyResult, kind string) (int, error) {
	ctx := r.Context()
	n := 0
	for i, row := range rows {
		for _, f := range u.files[row.Ref] {
			if f.Existing || f.fileID == "" {
				continue
			}
			id := ""
			if res != nil {
				id = res.ids[i]
			}
			if id == "" {
				_ = s.Store.DeleteFile(ctx, p.TenantID(), f.fileID)
				continue
			}
			if err := s.Store.Attach(ctx, f.fileID, kind, id); err != nil {
				return n, err
			}
			s.audit(ctx, p, "attachment.add", kind, id, f.Name+" (ReqIF import)")
			n++
		}
	}
	return n, nil
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
	topt := opt
	topt.exclude = map[int]bool{}
	if v := strings.TrimSpace(r.FormValue("exclude_tests")); v != "" {
		var idx []int
		if err := json.Unmarshal([]byte(v), &idx); err != nil {
			s.fail(w, r, badRequest("exclude_tests must be a JSON array of row indexes: %v", err))
			return
		}
		for _, i := range idx {
			topt.exclude[i] = true
		}
	}
	s.applyReqIF(w, r, p, pr, u, opt, topt)
}

// applyReqIF saves a planned import: the requirements and tests, their
// external references, embedded files and the relations and links
// between them.
func (s *Server) applyReqIF(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project, u *reqifUpload, opt, topt applyOptions) {
	ctx := r.Context()
	if err := s.prepareFiles(r, p, pr, u, u.plan.Rows, u.preview.Diff); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.prepareFiles(r, p, pr, u, u.plan.TestRows, u.preview.Tests); err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.applyDiff(r, p, pr, u.data, u.preview.Diff, opt)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var tres *applyResult
	if u.preview.Tests != nil {
		if tres, err = s.applyDiff(r, p, pr, u.tdata, u.preview.Tests, topt); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	out := reqifApplyResult{applyResult: res, Tests: tres}
	source := firstNonBlank(u.model.ToolID, u.model.SourceTool)
	saveRefs := func(rows []interop.Incoming, res *applyResult, kind string) error {
		if res == nil {
			return nil
		}
		for i, row := range rows {
			id, ok := res.ids[i]
			if !ok || id == "" {
				continue
			}
			ref := &store.ExternalRef{ProjectID: pr.ID, System: u.opts.system, EntityType: kind, EntityID: id,
				ExternalID: row.Ref, Source: source, Version: res.versions[i]}
			if err := s.Store.SaveExternalRef(ctx, p.TenantID(), ref); err != nil {
				return err
			}
			out.ReferencesSaved++
		}
		return nil
	}
	if err := saveRefs(u.plan.Rows, res, model.KindRequirement); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := saveRefs(u.plan.TestRows, tres, model.KindTest); err != nil {
		s.fail(w, r, err)
		return
	}
	for _, x := range []struct {
		rows []interop.Incoming
		res  *applyResult
		kind string
	}{{u.plan.Rows, res, model.KindRequirement}, {u.plan.TestRows, tres, model.KindTest}} {
		n, err := s.attachFiles(r, p, u, x.rows, x.res, x.kind)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out.FilesAttached += n
	}
	// relations and links between objects of the document, imported now
	// or before
	rels := relSummary{}
	if !u.opts.noRelations {
		if rels, err = s.planRelations(r, p, pr, u, u.knownIDs(res, tres)); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	relLabel := "link"
	if u.opts.format == "reqif" {
		relLabel = "SPEC-RELATION"
	}
	var touched []string
	for _, c := range rels.Changes {
		if c.sourceID == "" || c.targetID == "" {
			continue // an end was not imported
		}
		link := c.Kind == refLink
		switch c.Action {
		case "remove":
			if link {
				if err := s.Store.DeleteLink(ctx, p.TenantID(), c.relationID); err != nil && !errors.Is(err, store.ErrNotFound) {
					s.fail(w, r, err)
					return
				}
				s.audit(ctx, p, "link.delete", "link", c.relationID, c.Target+" -> "+c.Source+" (removed in "+u.opts.origin+")")
				res.LinksRemoved++
				touched = append(touched, c.targetID)
			} else {
				if err := s.Store.DeleteRelation(ctx, p.TenantID(), c.relationID); err != nil && !errors.Is(err, store.ErrNotFound) {
					s.fail(w, r, err)
					return
				}
				s.audit(ctx, p, "relation.delete", "relation", c.relationID, c.Source+" "+c.Type+" "+c.Target+" (removed in "+u.opts.origin+")")
				res.RelationsRemoved++
			}
			_ = s.Store.DeleteExternalRef(ctx, p.TenantID(), c.refID)
			continue
		case "add":
			if link {
				l := &model.TraceLink{RequirementID: c.targetID, TestCaseID: c.sourceID}
				if err := s.Store.CreateLink(ctx, p.TenantID(), l); err != nil {
					s.fail(w, r, badRequest("%s %s: %v", relLabel, c.Ref, err))
					return
				}
				s.audit(ctx, p, "link.create", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
				c.relationID = l.ID
				res.LinksAdded++
				touched = append(touched, c.targetID)
			} else {
				rel := &model.Relation{SourceID: c.sourceID, TargetID: c.targetID, Type: c.Type, CreatedBy: p.User.Email}
				if err := s.Store.CreateRelation(ctx, p.TenantID(), rel); err != nil {
					s.fail(w, r, badRequest("%s %s: %v", relLabel, c.Ref, err))
					return
				}
				s.audit(ctx, p, "relation.create", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
				c.relationID = rel.ID
				res.RelationsAdded++
			}
		}
		if err := s.Store.SaveExternalRef(ctx, p.TenantID(), &store.ExternalRef{ProjectID: pr.ID, System: u.opts.system, EntityType: c.Kind,
			EntityID: c.relationID, ExternalID: c.Ref, Source: source}); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if len(touched) > 0 {
		s.syncEvidence(ctx, p, pr.ID, touched)
	}
	if tres != nil {
		res.merge(tres)
	}
	s.audit(ctx, p, "requirement.import", "project", pr.ID, fmt.Sprintf("%s (%s%s): %s", u.name, u.opts.format, toolNote(source), res.summary()))
	writeJSON(w, http.StatusOK, out)
}

// merge adds the counts and items of o (another kind's apply) to a.
func (a *applyResult) merge(o *applyResult) {
	a.Created += o.Created
	a.Updated += o.Updated
	a.Unchanged += o.Unchanged
	a.Skipped += o.Skipped
	a.FoldersCreated += o.FoldersCreated
	a.RelationsAdded += o.RelationsAdded
	a.RelationsRemoved += o.RelationsRemoved
	a.LinksAdded += o.LinksAdded
	a.LinksRemoved += o.LinksRemoved
	a.Items = append(a.Items, o.Items...)
	a.SkippedRows = append(a.SkippedRows, o.SkippedRows...)
}

func toolNote(tool string) string {
	if tool == "" {
		return ""
	}
	return ", " + tool
}
