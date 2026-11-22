package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/interop"
	"github.com/rom/qtestframe/internal/model"
)

// Exchanging data with other tools (see internal/interop and docs/api.md
// "Exchange"):
//
//   - the Excel round-trip: GET …/exchange/xlsx exports requirements or
//     tests as a workbook with a stable ID and a Version column; the edited
//     workbook is previewed (…/xlsx/preview: new, changed, unchanged,
//     conflicting and erroneous rows, per cell) and applied (…/xlsx/apply);
//   - the ReqIF import (exchange_reqif.go), with the same preview.
//
// Applying runs in one transaction (atomic) and every item goes through the
// regular create and PATCH handlers, so validation, permissions, history,
// audit, suspect links and notifications are those of a normal save.

const maxExchangeBytes = 20 << 20

func (s *Server) routesExchange(h routeFunc) {
	h("GET /api/v1/projects/{pid}/exchange/xlsx", s.handleExchangeExport, "export")
	h("POST /api/v1/projects/{pid}/exchange/xlsx/preview", s.handleExchangeXLSXPreview, "project.read")       // write of the kind: see exchange.go
	h("POST /api/v1/projects/{pid}/exchange/xlsx/apply", s.atomic(s.handleExchangeXLSXApply), "project.read") // write of the kind
	h("POST /api/v1/projects/{pid}/exchange/reqif/preview", s.handleReqIFPreview, "requirement.write")
	h("POST /api/v1/projects/{pid}/exchange/reqif/apply", s.atomic(s.handleReqIFApply), "requirement.write")
}

func exchangeKind(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "requirement", "requirements":
		return model.KindRequirement, nil
	case "test", "tests":
		return model.KindTest, nil
	}
	return "", badRequest("kind must be requirement or test")
}

// exchangeData is what the exchange knows about the items of a project.
type exchangeData struct {
	kind    string
	fields  []interop.Field
	items   []*interop.Item // in display order
	byID    map[string]*interop.Item
	reqKeys map[string]string // lower key → id
	tstKeys map[string]string
	folders []model.Folder
	paths   map[string]string // folder id → "A/B"
}

func (d *exchangeData) lookup(kind, key string) (string, bool) {
	m := d.reqKeys
	if kind == model.KindTest {
		m = d.tstKeys
	}
	id, ok := m[strings.ToLower(strings.TrimSpace(key))]
	return id, ok
}

// folderPaths returns folder id → "A/B" (names joined by "/").
func folderPaths(folders []model.Folder) map[string]string {
	byID := map[string]model.Folder{}
	for _, f := range folders {
		byID[f.ID] = f
	}
	out := map[string]string{}
	var path func(id string, depth int) string
	path = func(id string, depth int) string {
		if p, ok := out[id]; ok {
			return p
		}
		f, ok := byID[id]
		if !ok || depth > 64 {
			return ""
		}
		p := f.Name
		if f.ParentID != nil {
			if pp := path(*f.ParentID, depth+1); pp != "" {
				p = pp + "/" + f.Name
			}
		}
		out[id] = p
		return p
	}
	for _, f := range folders {
		path(f.ID, 0)
	}
	return out
}

// loadExchange loads the items of kind (filtered by q and folder_id of
// the request when filter is set) with their relations and links.
func (s *Server) loadExchange(r *http.Request, p *Principal, pr *model.Project, kind string, filter bool) (*exchangeData, error) {
	ctx := withExchangeStore(r.Context(), s.Store)
	fields, err := interop.FieldsFor(ctx, p.TenantID(), pr.ID, kind)
	if err != nil {
		return nil, err
	}
	d := &exchangeData{kind: kind, fields: fields, byID: map[string]*interop.Item{}, reqKeys: map[string]string{}, tstKeys: map[string]string{}}
	if d.folders, err = s.Store.ListFolders(ctx, p.TenantID(), pr.ID, kind); err != nil {
		return nil, err
	}
	d.paths = folderPaths(d.folders)
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	reqKey, tstKey := map[string]string{}, map[string]string{}
	for _, rq := range reqs {
		d.reqKeys[strings.ToLower(rq.Key)] = rq.ID
		reqKey[rq.ID] = rq.Key
	}
	for _, t := range tests {
		d.tstKeys[strings.ToLower(t.Key)] = t.ID
		tstKey[t.ID] = t.Key
	}
	folderOf := func(id *string) string {
		if id == nil {
			return ""
		}
		return d.paths[*id]
	}
	var only map[string]bool
	if filter && (strings.TrimSpace(r.URL.Query().Get("q")) != "" || r.URL.Query().Get("folder_id") != "") {
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			return nil, err
		}
		only = map[string]bool{}
		q, folder, rec := r.URL.Query().Get("q"), r.URL.Query().Get("folder_id"), boolParam(r, "recursive", true)
		if kind == model.KindTest {
			ts, err := s.filterTests(r, p, pr, pc, q, folder, rec)
			if err != nil {
				return nil, err
			}
			for _, t := range ts {
				only[t.ID] = true
			}
		} else {
			rs, err := s.filterRequirements(r, p, pr, pc, q, folder, rec)
			if err != nil {
				return nil, err
			}
			for _, rq := range rs {
				only[rq.ID] = true
			}
		}
	}
	links, err := s.Store.ListLinks(ctx, p.TenantID(), pr.ID, "", "")
	if err != nil {
		return nil, err
	}
	if kind == model.KindTest {
		for i := range tests {
			t := &tests[i]
			if only != nil && !only[t.ID] {
				continue
			}
			for j := range t.Steps {
				if c := t.Steps[j].CallTestID; c != nil {
					t.Steps[j].CallTestKey = tstKey[*c]
				}
			}
			it := interop.TestItem(t, folderOf(t.FolderID))
			d.items = append(d.items, it)
			d.byID[t.ID] = it
		}
		for _, l := range links {
			if it := d.byID[l.TestCaseID]; it != nil && reqKey[l.RequirementID] != "" {
				it.Links = appendUniq(it.Links, reqKey[l.RequirementID])
			}
		}
	} else {
		for i := range reqs {
			rq := &reqs[i]
			if only != nil && !only[rq.ID] {
				continue
			}
			it := interop.RequirementItem(rq, folderOf(rq.FolderID))
			d.items = append(d.items, it)
			d.byID[rq.ID] = it
		}
		rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
		if err != nil {
			return nil, err
		}
		for _, rel := range rels {
			if it := d.byID[rel.SourceID]; it != nil && reqKey[rel.TargetID] != "" {
				it.Relations[rel.Type] = appendUniq(it.Relations[rel.Type], reqKey[rel.TargetID])
			}
		}
		for _, l := range links {
			if it := d.byID[l.RequirementID]; it != nil && tstKey[l.TestCaseID] != "" {
				it.Links = appendUniq(it.Links, tstKey[l.TestCaseID])
			}
		}
	}
	if err := interop.LoadExtras(ctx, p.TenantID(), pr.ID, kind, d.items); err != nil {
		return nil, err
	}
	return d, nil
}

func appendUniq(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// handleExchangeExport: GET /projects/{pid}/exchange/xlsx?kind=&q=&folder_id=
func (s *Server) handleExchangeExport(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	kind, err := exchangeKind(r.URL.Query().Get("kind"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if perm := readPerm(kind); !p.Can(perm) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(perm)+" required")
		return
	}
	d, err := s.loadExchange(r, p, pr, kind, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var buf bytes.Buffer
	now := time.Now().UTC()
	if err := interop.WriteWorkbook(&buf, interop.Workbook{Kind: kind, ProjectID: pr.ID, ProjectKey: pr.Key, ProjectName: pr.Name,
		Fields: d.fields, Items: d.items, GeneratedAt: now}); err != nil {
		s.fail(w, r, err)
		return
	}
	name := fmt.Sprintf("%s-%s-roundtrip-%s.xlsx", pr.Key, strings.ToLower(interop.SheetName(kind)), now.Format("20060102-150405"))
	s.audit(r.Context(), p, "project.export", "project", pr.ID, fmt.Sprintf("xlsx round-trip %ss (%d)", kind, len(d.items)))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(buf.Bytes())
}

// exchangeUpload is a parsed workbook with its diff.
type exchangeUpload struct {
	name string
	data *exchangeData
	diff *interop.Diff
}

func (s *Server) readExchangeXLSX(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project) (*exchangeUpload, error) {
	up, err := readUpload(w, r, "file", maxExchangeBytes)
	if err != nil {
		return nil, err
	}
	kind := ""
	if v := r.FormValue("kind"); v != "" {
		if kind, err = exchangeKind(v); err != nil {
			return nil, err
		}
	}
	sh, err := interop.ReadWorkbook(up.data, kind, func(k string) []interop.Field {
		fs, ferr := interop.FieldsFor(r.Context(), p.TenantID(), pr.ID, k)
		if ferr != nil {
			fs = interop.BuiltinFields(k)
		}
		return fs
	})
	var nw *interop.ErrNotWorkbook
	if errors.As(err, &nw) {
		return nil, badRequest("%s", nw.Msg)
	}
	if err != nil {
		return nil, err
	}
	if !p.Can(writePerm(sh.Kind)) {
		return nil, forbidden("permission " + string(writePerm(sh.Kind)) + " required to import " + sh.Kind + "s")
	}
	d, err := s.loadExchange(r, p, pr, sh.Kind, false)
	if err != nil {
		return nil, err
	}
	rows := sh.Incoming(func(key string) string {
		id, _ := d.lookup(sh.Kind, key)
		return id
	}, func(id string) bool { return d.byID[id] != nil })
	diff := interop.Compare(sh.Kind, d.fields, rows, d.byID, d.lookup)
	diff.Ignored = sh.Ignored
	diff.Warnings = append(diff.Warnings, sh.Warnings...)
	if sh.ProjectID != "" && sh.ProjectID != pr.ID {
		diff.Warnings = append(diff.Warnings, "the workbook was exported from another project: rows with an ID of that project are errors; clear the IDs to create the items here")
	}
	for _, k := range sh.Columns {
		if f := interop.FieldByKey(d.fields, k); f != nil {
			diff.Columns = append(diff.Columns, *f)
		}
	}
	if diff.Columns == nil {
		diff.Columns = []interop.Field{}
	}
	s.checkReviewPerms(p, d, diff)
	return &exchangeUpload{name: up.name, data: d, diff: diff}, nil
}

func forbidden(msg string) error { return fmt.Errorf("%w: %s", errForbidden, msg) }

// apiErr is an error with its own status and code (see fail).
type apiErr struct {
	status    int
	code, msg string
}

func (e *apiErr) Error() string { return e.msg }

// checkReviewPerms marks rows that set a review status (approved, ready …)
// without the review permission.
func (s *Server) checkReviewPerms(p *Principal, d *exchangeData, diff *interop.Diff) {
	if p.Can(reviewPerm(d.kind)) {
		return
	}
	for i := range diff.Rows {
		rd := &diff.Rows[i]
		v, ok := rd.Values["status"]
		if !ok || rd.Status == interop.StatusError {
			continue
		}
		old := ""
		if it := d.byID[rd.ID]; it != nil {
			if it.Req != nil {
				old = it.Req.Status
			} else {
				old = it.Test.Status
			}
		}
		if isReviewDecision(d.kind, old, v) {
			rd.Errors = append(rd.Errors, interop.CellError{Field: "status", Label: "Status", Message: "permission " + string(reviewPerm(d.kind)) + " required to set the status to " + v})
			recount(diff, rd, interop.StatusError)
		}
	}
}

func recount(diff *interop.Diff, rd *interop.RowDiff, status string) {
	dec := func(st string, n int) {
		switch st {
		case interop.StatusNew:
			diff.Summary.New += n
		case interop.StatusChanged:
			diff.Summary.Changed += n
		case interop.StatusUnchanged:
			diff.Summary.Unchanged += n
		case interop.StatusConflict:
			diff.Summary.Conflicts += n
		default:
			diff.Summary.Errors += n
		}
	}
	dec(rd.Status, -1)
	rd.Status = status
	dec(status, 1)
}

type exchangePreview struct {
	*interop.Diff
	File string `json:"file"`
}

func (s *Server) handleExchangeXLSXPreview(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ex, err := s.readExchangeXLSX(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, exchangePreview{Diff: ex.diff, File: ex.name})
}

func (s *Server) handleExchangeXLSXApply(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ex, err := s.readExchangeXLSX(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	opt, err := applyOptionsOf(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.applyDiff(r, p, pr, ex.data, ex.diff, opt)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, ex.data.kind+".import", "project", pr.ID, fmt.Sprintf("%s (xlsx round-trip): %s", ex.name, res.summary()))
	writeJSON(w, http.StatusOK, res)
}

// applyOptions are the user's choices of an apply.
type applyOptions struct {
	overwrite bool
	create    bool
	exclude   map[int]bool
}

func applyOptionsOf(r *http.Request) (applyOptions, error) {
	o := applyOptions{overwrite: formBool(r, "overwrite_conflicts", false), create: formBool(r, "create", true), exclude: map[int]bool{}}
	if v := strings.TrimSpace(r.FormValue("exclude")); v != "" {
		var idx []int
		if err := json.Unmarshal([]byte(v), &idx); err != nil {
			return o, badRequest("exclude must be a JSON array of row indexes: %v", err)
		}
		for _, i := range idx {
			o.exclude[i] = true
		}
	}
	return o, nil
}

type appliedItem struct {
	Index    int    `json:"index"`
	Location string `json:"location"`
	ID       string `json:"id"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	Action   string `json:"action"` // created | updated
	Version  int    `json:"version"`
}

type skippedRow struct {
	Index    int    `json:"index"`
	Location string `json:"location"`
	Title    string `json:"title"`
	Reason   string `json:"reason"` // excluded | conflict | error | new
}

type applyResult struct {
	Created          int           `json:"created"`
	Updated          int           `json:"updated"`
	Unchanged        int           `json:"unchanged"`
	Skipped          int           `json:"skipped"`
	FoldersCreated   int           `json:"folders_created"`
	RelationsAdded   int           `json:"relations_added"`
	RelationsRemoved int           `json:"relations_removed"`
	LinksAdded       int           `json:"links_added"`
	LinksRemoved     int           `json:"links_removed"`
	Items            []appliedItem `json:"items"`
	SkippedRows      []skippedRow  `json:"skipped_rows"`
	// ids by diff row index (rows applied or unchanged)
	ids      map[int]string
	versions map[int]int
}

func (a *applyResult) summary() string {
	return fmt.Sprintf("%d created, %d updated, %d unchanged, %d skipped, %d folders, relations +%d −%d, links +%d −%d",
		a.Created, a.Updated, a.Unchanged, a.Skipped, a.FoldersCreated, a.RelationsAdded, a.RelationsRemoved, a.LinksAdded, a.LinksRemoved)
}

// subCall runs a handler for one item inside the current request (and
// transaction), as batch updates do.
func (s *Server) subCall(r *http.Request, p *Principal, fn handlerFunc, method string, values map[string]string, body any, ifMatch int) *heldResponse {
	b, _ := json.Marshal(body)
	sub := r.Clone(context.WithValue(r.Context(), batchKey{}, true))
	sub.Method = method
	sub.Body = io.NopCloser(bytes.NewReader(b))
	sub.ContentLength = int64(len(b))
	sub.Header = r.Header.Clone()
	sub.Header.Set("Content-Type", "application/json")
	sub.Header.Del("If-Match")
	if ifMatch > 0 {
		sub.Header.Set("If-Match", `"`+strconv.Itoa(ifMatch)+`"`)
	}
	for k, v := range values {
		sub.SetPathValue(k, v)
	}
	rec := &heldResponse{header: http.Header{}}
	fn(rec, sub, p)
	return rec
}

// rowFailure turns a failed sub call into an error naming the row.
func rowFailure(rec *heldResponse, rd *interop.RowDiff) error {
	var e apiError
	_ = json.Unmarshal(rec.body.Bytes(), &e)
	if e.Error == "" {
		e.Error = http.StatusText(rec.status())
	}
	status := rec.status()
	if status == http.StatusPreconditionFailed {
		status = http.StatusConflict
		e.Code = "stale"
		e.Error = "the item was changed by someone else meanwhile; preview the file again"
	}
	if e.Code == "" {
		e.Code = "error"
	}
	return &apiErr{status: status, code: e.Code, msg: fmt.Sprintf("%s (%s): %s", rd.Location, firstNonBlank(rd.Key, rd.Title), e.Error)}
}

func firstNonBlank(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// applyDiff saves the rows of a diff that the user confirmed.
func (s *Server) applyDiff(r *http.Request, p *Principal, pr *model.Project, d *exchangeData, diff *interop.Diff, opt applyOptions) (*applyResult, error) {
	ctx := r.Context()
	res := &applyResult{Items: []appliedItem{}, SkippedRows: []skippedRow{}, ids: map[int]string{}, versions: map[int]int{}}
	fr, err := s.newFolderResolver(ctx, p, pr.ID, d.kind)
	if err != nil {
		return nil, err
	}
	create, update := s.handleCreateRequirement, s.handleUpdateRequirement
	if d.kind == model.KindTest {
		create, update = s.handleCreateTest, s.handleUpdateTest
	}
	type pending struct {
		rd *interop.RowDiff
		id string
	}
	var keyed []pending
	for i := range diff.Rows {
		rd := &diff.Rows[i]
		skip := ""
		switch {
		case opt.exclude[rd.Index]:
			skip = "excluded"
		case rd.Status == interop.StatusError:
			skip = "error"
		case rd.Status == interop.StatusConflict && !opt.overwrite:
			skip = "conflict"
		case rd.Status == interop.StatusNew && !opt.create:
			skip = "new"
		}
		if skip != "" {
			res.SkippedRows = append(res.SkippedRows, skippedRow{Index: rd.Index, Location: rd.Location, Title: rd.Title, Reason: skip})
			continue
		}
		if rd.Status == interop.StatusUnchanged {
			res.Unchanged++
			res.ids[rd.Index], res.versions[rd.Index] = rd.ID, rd.Version
			continue
		}
		cur := d.byID[rd.ID]
		body, err := interop.Body(d.fields, rd, cur)
		if err != nil {
			return nil, badRequest("%s: %v", rd.Location, err)
		}
		if v, ok := rd.Values["folder"]; ok || (rd.Status == interop.StatusNew && v != "") {
			fid, err := fr.resolve(nil, strings.Split(v, "/"))
			if err != nil {
				return nil, err
			}
			body["folder_id"] = fid
		}
		var rec *heldResponse
		action := "updated"
		if rd.Status == interop.StatusNew {
			action = "created"
			if d.kind == model.KindRequirement {
				body["create_test"] = false
			}
			rec = s.subCall(r, p, create, http.MethodPost, map[string]string{"pid": pr.ID}, body, 0)
		} else {
			base := rd.Version
			if rd.Status == interop.StatusChanged && rd.BaseVersion > 0 {
				base = rd.BaseVersion
			}
			if len(body) == 0 {
				// only relations or links changed
				res.Updated++
				res.ids[rd.Index], res.versions[rd.Index] = rd.ID, rd.Version
				res.Items = append(res.Items, appliedItem{Index: rd.Index, Location: rd.Location, ID: rd.ID, Key: rd.Key, Title: rd.Title, Action: action, Version: rd.Version})
				keyed = append(keyed, pending{rd, rd.ID})
				continue
			}
			rec = s.subCall(r, p, update, http.MethodPatch, map[string]string{"id": rd.ID}, body, base)
		}
		if rec.status() >= 400 {
			return nil, rowFailure(rec, rd)
		}
		var out struct {
			ID      string `json:"id"`
			Key     string `json:"key"`
			Title   string `json:"title"`
			Version int    `json:"version"`
		}
		_ = json.Unmarshal(rec.body.Bytes(), &out)
		if action == "created" {
			res.Created++
		} else {
			res.Updated++
		}
		res.ids[rd.Index], res.versions[rd.Index] = out.ID, out.Version
		res.Items = append(res.Items, appliedItem{Index: rd.Index, Location: rd.Location, ID: out.ID, Key: out.Key, Title: out.Title, Action: action, Version: out.Version})
		keyed = append(keyed, pending{rd, out.ID})
	}
	// relations and links, once every item exists
	for _, pd := range keyed {
		for _, f := range d.fields {
			v, ok := pd.rd.Values[f.Key]
			if !ok || f.Type != interop.Keys || f.ReadOnly {
				continue
			}
			var targets []string
			for _, k := range interop.SplitList(v) {
				if id, ok := d.lookup(model.KindRequirement, k); ok {
					targets = append(targets, id)
				}
			}
			if f.Relation != "" {
				if err := s.syncRelations(ctx, p, pr, pd.id, f.Relation, targets, res); err != nil {
					return nil, fmt.Errorf("%s: %w", pd.rd.Location, err)
				}
			} else if f.Key == "verifies" && d.kind == model.KindTest {
				if err := s.syncVerifies(ctx, p, pr, pd.id, targets, res); err != nil {
					return nil, fmt.Errorf("%s: %w", pd.rd.Location, err)
				}
			}
		}
	}
	res.Skipped = len(res.SkippedRows)
	res.FoldersCreated = fr.created
	return res, nil
}

// syncRelations makes the outgoing relations of type typ of source the
// targets.
func (s *Server) syncRelations(ctx context.Context, p *Principal, pr *model.Project, source, typ string, targets []string, res *applyResult) error {
	rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, source)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, t := range targets {
		want[t] = true
	}
	have := map[string]bool{}
	for _, rel := range rels {
		if rel.SourceID != source || rel.Type != typ {
			continue
		}
		have[rel.TargetID] = true
		if !want[rel.TargetID] {
			if err := s.Store.DeleteRelation(ctx, p.TenantID(), rel.ID); err != nil {
				return err
			}
			s.audit(ctx, p, "relation.delete", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
			res.RelationsRemoved++
		}
	}
	sort.Strings(targets)
	for _, t := range targets {
		if have[t] {
			continue
		}
		have[t] = true
		rel := &model.Relation{SourceID: source, TargetID: t, Type: typ, CreatedBy: p.User.Email}
		if err := s.Store.CreateRelation(ctx, p.TenantID(), rel); err != nil {
			return err
		}
		s.audit(ctx, p, "relation.create", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
		res.RelationsAdded++
	}
	return nil
}

// syncVerifies makes the requirements a test verifies the targets.
func (s *Server) syncVerifies(ctx context.Context, p *Principal, pr *model.Project, test string, targets []string, res *applyResult) error {
	links, err := s.Store.ListLinks(ctx, p.TenantID(), pr.ID, "", test)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, t := range targets {
		want[t] = true
	}
	have := map[string]bool{}
	var touched []string
	for _, l := range links {
		have[l.RequirementID] = true
		if !want[l.RequirementID] {
			if err := s.Store.DeleteLink(ctx, p.TenantID(), l.ID); err != nil {
				return err
			}
			s.audit(ctx, p, "link.delete", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
			res.LinksRemoved++
			touched = append(touched, l.RequirementID)
		}
	}
	for _, t := range targets {
		if have[t] {
			continue
		}
		have[t] = true
		l := &model.TraceLink{RequirementID: t, TestCaseID: test}
		if err := s.Store.CreateLink(ctx, p.TenantID(), l); err != nil {
			return err
		}
		s.audit(ctx, p, "link.create", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
		res.LinksAdded++
		touched = append(touched, t)
	}
	if len(touched) > 0 {
		s.syncEvidence(ctx, p, pr.ID, touched)
	}
	return nil
}
