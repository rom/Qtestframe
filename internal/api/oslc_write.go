package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/oslc"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/reqimport"
)

// OSLC writes: the creation factories (POST to the query base), update
// (PUT with If-Match: the ETag is the item version, so a stale update
// answers 412 like a PATCH of the web client) and delete. Every save goes
// through the regular create, PATCH and DELETE handlers (validation,
// permissions, history, audit, suspect links, notifications).
//
// A PUT changes the properties it carries and leaves the others; with
// oslc.properties (selective update) the listed properties that the body
// does not carry are cleared. Link properties replace the links of their
// kind: oslc_rm:validatedBy and oslc_qm:validatesRequirement the trace
// links, oslc_rm:elaborates / elaboratedBy (refines), constrainedBy /
// constrains (depends on), satisfies / satisfiedBy (derived from),
// dcterms:relation (relates to) and qtf:<type> the requirement relations;
// a value that is not a resource of the project (a work item in another
// tool) is kept as an item link to its URL, labelled with the property.

const oslcMaxBody = 4 << 20

// relGroup is a set of link properties that write the relations of one
// type and direction.
type relGroup struct {
	typ      string
	incoming bool
	preds    []string
}

func oslcRelGroups(q string) []relGroup {
	rm := oslc.NSRM
	gs := []relGroup{
		{"refines", false, []string{q + "refines", rm + "elaborates", rm + "decomposes"}},
		{"refines", true, []string{rm + "elaboratedBy", rm + "decomposedBy"}},
		{"depends_on", false, []string{q + "depends_on", rm + "constrainedBy"}},
		{"depends_on", true, []string{rm + "constrains"}},
		{"derived_from", false, []string{q + "derived_from", rm + "satisfies"}},
		{"derived_from", true, []string{rm + "satisfiedBy"}},
		{"relates_to", false, []string{q + "relates_to", oslc.NSDCTerms + "relation"}},
	}
	for _, rt := range []string{"provides", "conflicts_with", "duplicates"} {
		gs = append(gs, relGroup{rt, false, []string{q + rt}})
	}
	return gs
}

// oslcInput is a request body mapped to a create / PATCH body and the
// links to write.
type oslcInput struct {
	body map[string]any
	// rels: relation group index → target (or source) item ids, for the
	// groups the body carries
	rels map[int][]string
	// links: the item ids at the other end of the trace links (nil when
	// the body does not carry the property)
	links []string
	// urls: link label → URLs, for the properties the body carries
	urls   map[string][]string
	folder *string // folder path
}

type oslcBodyErr struct {
	status int
	msg    string
}

func (e *oslcBodyErr) Error() string { return e.msg }

func oslcBad(status int, msg string) error { return &oslcBodyErr{status, msg} }

// oslcIDs resolves item ids and keys of a project.
type oslcIDs struct{ req, test map[string]string }

func (s *Server) loadOSLCIDs(ctx context.Context, p *Principal, pr *model.Project) (*oslcIDs, error) {
	ids := &oslcIDs{req: map[string]string{}, test: map[string]string{}}
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	for _, rq := range reqs {
		ids.req[rq.ID], ids.req[strings.ToLower(rq.Key)] = rq.ID, rq.ID
	}
	tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range tests {
		ids.test[t.ID], ids.test[strings.ToLower(t.Key)] = t.ID, t.ID
	}
	return ids, nil
}

func (ids *oslcIDs) find(kind, idOrKey string) (string, bool) {
	m := ids.req
	if kind == model.KindTest {
		m = ids.test
	}
	if id, ok := m[idOrKey]; ok {
		return id, true
	}
	id, ok := m[strings.ToLower(idOrKey)]
	return id, ok
}

// readOSLCInput parses the body of a POST or PUT about resource (its IRI;
// the collection for a POST).
func (s *Server) readOSLCInput(r *http.Request, p *Principal, pr *model.Project, kind, resource string, ids *oslcIDs) (*oslcInput, error) {
	mt := oslc.MediaOf(r.Header.Get("Content-Type"))
	if mt == "" {
		return nil, oslcBad(http.StatusUnsupportedMediaType, "send the resource as text/turtle, application/rdf+xml or application/ld+json")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, oslcMaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(data) > oslcMaxBody {
		return nil, oslcBad(http.StatusRequestEntityTooLarge, "the resource is too large")
	}
	g, err := oslc.Parse(data, mt, resource)
	if err != nil {
		return nil, oslcBad(http.StatusBadRequest, err.Error())
	}
	_, u := s.oslcGraph(r)
	q := u.vocab()
	class := oslc.NSRM + "Requirement"
	if kind == model.KindTest {
		class = oslc.NSQM + "TestCase"
	}
	subj, ok := g.SubjectOf(class, resource)
	if !ok {
		return nil, oslcBad(http.StatusBadRequest, "the body describes no resource")
	}
	// oslc.properties: a selective update of the listed properties
	selected := map[string]bool{}
	if v := strings.TrimSpace(r.URL.Query().Get("oslc.properties")); v != "" {
		prefixes, err := oslc.ParsePrefixes(r.URL.Query().Get("oslc.prefix"), g.Prefixes)
		if err != nil {
			return nil, oslcBad(http.StatusBadRequest, err.Error())
		}
		for _, n := range strings.Split(v, ",") {
			pre, local, ok := strings.Cut(strings.TrimSpace(n), ":")
			ns, known := prefixes[pre]
			if !ok || !known {
				return nil, oslcBad(http.StatusBadRequest, "oslc.properties: unknown property "+strings.TrimSpace(n))
			}
			selected[ns+local] = true
		}
	}
	present := func(preds ...string) bool {
		for _, pd := range preds {
			if (len(selected) > 0 && selected[pd]) || (len(selected) == 0 && g.Has(subj, pd)) {
				return true
			}
		}
		return false
	}
	text := func(pred string, markdown bool) (string, bool) {
		if !present(pred) {
			return "", false
		}
		for _, o := range g.Objects(subj, pred) {
			if !o.IsLiteral() {
				continue
			}
			if o.Datatype == oslc.XMLLiteral {
				if markdown {
					return reqimport.XHTMLToMarkdown(o.Literal), true
				}
				return reqimport.XHTMLText(o.Literal), true
			}
			return o.Literal, true
		}
		return "", true
	}
	in := &oslcInput{body: map[string]any{}, rels: map[int][]string{}, urls: map[string][]string{}}
	type field struct {
		pred, key string
		markdown  bool
	}
	fields := []field{{oslc.NSDCTerms + "title", "title", false}, {oslc.NSDCTerms + "description", "description", true}}
	if kind == model.KindRequirement {
		for _, k := range []string{"status", "obligation", "type", "priority", "source", "rationale", "owner"} {
			fields = append(fields, field{q + k, k, k == "rationale"})
		}
	} else {
		fields = append(fields, field{q + "status", "status", false}, field{q + "priority", "priority", false},
			field{q + "executionType", "execution_type", false}, field{q + "preconditions", "preconditions", true},
			field{q + "expectedResult", "expected_result", true})
	}
	for _, f := range fields {
		if v, ok := text(f.pred, f.markdown); ok {
			in.body[f.key] = strings.TrimSpace(v)
		}
	}
	if kind == model.KindTest {
		if v, ok := text(q+"estimatedMinutes", false); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil && strings.TrimSpace(v) != "" {
				return nil, oslcBad(http.StatusBadRequest, "qtf:estimatedMinutes must be a whole number")
			}
			in.body["estimated_minutes"] = n
		}
	}
	if present(oslc.NSDCTerms + "subject") {
		tags := []string{}
		for _, o := range g.Objects(subj, oslc.NSDCTerms+"subject") {
			if o.IsLiteral() && strings.TrimSpace(o.Literal) != "" {
				tags = append(tags, strings.TrimSpace(o.Literal))
			}
		}
		in.body["tags"] = tags
	}
	if v, ok := text(q+"folder", false); ok {
		v = strings.TrimSpace(v)
		in.folder = &v
	}

	// link properties
	urlProps := map[string]bool{}
	for _, pp := range oslcURLLinkProps[kind] {
		urlProps[pp] = true
	}
	urlOf := func(pred string, o oslc.Term) error {
		if o.IRI == "" || !urlProps[pred] {
			return oslcBad(http.StatusBadRequest, linkLabel(g, pred)+": "+termValue(o)+" is not a resource of this project")
		}
		label := linkLabel(g, pred)
		in.urls[label] = append(in.urls[label], o.IRI)
		return nil
	}
	// our resources: by IRI
	ours := func(o oslc.Term, want string) (string, bool) {
		k, id, ok := u.itemOf(pr.ID, o.IRI)
		if !ok || k != want {
			return "", false
		}
		return ids.find(k, id)
	}
	for pred := range urlProps {
		if present(pred) {
			in.urls[linkLabel(g, pred)] = []string{}
		}
	}
	linkPred, other := oslc.NSRM+"validatedBy", model.KindTest
	if kind == model.KindTest {
		linkPred, other = oslc.NSQM+"validatesRequirement", model.KindRequirement
	}
	if present(linkPred) {
		in.links = []string{}
		for _, o := range g.Objects(subj, linkPred) {
			if id, ok := ours(o, other); ok {
				in.links = append(in.links, id)
			} else if err := urlOf(linkPred, o); err != nil {
				return nil, err
			}
		}
	}
	if kind == model.KindRequirement {
		for gi, grp := range oslcRelGroups(q) {
			if !present(grp.preds...) {
				continue
			}
			in.rels[gi] = []string{}
			for _, pred := range grp.preds {
				for _, o := range g.Objects(subj, pred) {
					if id, ok := ours(o, model.KindRequirement); ok {
						in.rels[gi] = appendUniq(in.rels[gi], id)
					} else if err := urlOf(pred, o); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	// the remaining URL-only properties
	for pred := range urlProps {
		if pred == linkPred || !present(pred) {
			continue
		}
		isRel := false
		for _, grp := range oslcRelGroups(q) {
			for _, gp := range grp.preds {
				isRel = isRel || gp == pred
			}
		}
		if isRel && kind == model.KindRequirement {
			continue
		}
		for _, o := range g.Objects(subj, pred) {
			if err := urlOf(pred, o); err != nil {
				return nil, err
			}
		}
	}
	return in, nil
}

// failOSLC answers an error of the OSLC write handlers.
func (s *Server) failOSLC(w http.ResponseWriter, r *http.Request, err error) {
	var be *oslcBodyErr
	if errors.As(err, &be) {
		s.writeOSLCError(w, r, be.status, "bad_request", be.msg)
		return
	}
	var ae *apiErr
	if errors.As(err, &ae) {
		s.writeOSLCError(w, r, ae.status, ae.code, ae.msg)
		return
	}
	s.fail(w, r, err)
}

// subFailure turns a failed sub call into an error.
func subFailure(rec *heldResponse) error {
	var e apiError
	_ = json.Unmarshal(rec.body.Bytes(), &e)
	if e.Error == "" {
		e.Error = http.StatusText(rec.status())
	}
	if e.Code == "" {
		e.Code = "error"
	}
	return &apiErr{status: rec.status(), code: e.Code, msg: e.Error}
}

// applyLinks writes the links and relations of in for the item.
func (s *Server) applyOSLCLinks(ctx context.Context, p *Principal, pr *model.Project, kind, id string, in *oslcInput, q string) error {
	res := &applyResult{}
	if in.links != nil {
		if !p.Can(rbac.TestWrite) {
			return forbidden("permission test.write required to change trace links")
		}
		if kind == model.KindTest {
			if err := s.syncVerifies(ctx, p, pr, id, in.links, res); err != nil {
				return err
			}
		} else if err := s.syncValidatedBy(ctx, p, pr, id, in.links); err != nil {
			return err
		}
	}
	groups := oslcRelGroups(q)
	gis := make([]int, 0, len(in.rels))
	for gi := range in.rels {
		gis = append(gis, gi)
	}
	sort.Ints(gis)
	for _, gi := range gis {
		grp, ids := groups[gi], in.rels[gi]
		if grp.incoming {
			if err := s.syncIncoming(ctx, p, pr, id, grp.typ, ids); err != nil {
				return err
			}
		} else if err := s.syncRelations(ctx, p, pr, id, grp.typ, ids, res); err != nil {
			return err
		}
	}
	labels := make([]string, 0, len(in.urls))
	for l := range in.urls {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	for _, l := range labels {
		if err := s.syncURLLinks(ctx, p, kind, id, l, in.urls[l]); err != nil {
			return err
		}
	}
	return nil
}

// syncValidatedBy makes the tests verifying a requirement the targets.
func (s *Server) syncValidatedBy(ctx context.Context, p *Principal, pr *model.Project, req string, tests []string) error {
	links, err := s.Store.ListLinks(ctx, p.TenantID(), pr.ID, req, "")
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, t := range tests {
		want[t] = true
	}
	have := map[string]bool{}
	changed := false
	for _, l := range links {
		if l.RequirementID != req {
			continue
		}
		have[l.TestCaseID] = true
		if !want[l.TestCaseID] {
			if err := s.Store.DeleteLink(ctx, p.TenantID(), l.ID); err != nil {
				return err
			}
			s.audit(ctx, p, "link.delete", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
			changed = true
		}
	}
	for _, t := range tests {
		if have[t] {
			continue
		}
		have[t] = true
		l := &model.TraceLink{RequirementID: req, TestCaseID: t}
		if err := s.Store.CreateLink(ctx, p.TenantID(), l); err != nil {
			return err
		}
		s.audit(ctx, p, "link.create", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
		changed = true
	}
	if changed {
		s.syncEvidence(ctx, p, pr.ID, []string{req})
	}
	return nil
}

// syncIncoming makes the requirements with a relation of type typ to
// target the sources.
func (s *Server) syncIncoming(ctx context.Context, p *Principal, pr *model.Project, target, typ string, sources []string) error {
	rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, target)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, src := range sources {
		want[src] = true
	}
	have := map[string]bool{}
	for _, rel := range rels {
		if rel.TargetID != target || rel.Type != typ {
			continue
		}
		have[rel.SourceID] = true
		if !want[rel.SourceID] {
			if err := s.Store.DeleteRelation(ctx, p.TenantID(), rel.ID); err != nil {
				return err
			}
			s.audit(ctx, p, "relation.delete", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
		}
	}
	for _, src := range sources {
		if have[src] {
			continue
		}
		have[src] = true
		rel := &model.Relation{SourceID: src, TargetID: target, Type: typ, CreatedBy: p.User.Email}
		if err := s.Store.CreateRelation(ctx, p.TenantID(), rel); err != nil {
			return err
		}
		s.audit(ctx, p, "relation.create", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
	}
	return nil
}

// syncURLLinks makes the item links to URLs labelled label the urls.
func (s *Server) syncURLLinks(ctx context.Context, p *Principal, kind, id, label string, urls []string) error {
	links, err := s.Store.ListItemLinks(ctx, p.TenantID(), kind, id)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, u := range urls {
		want[u] = true
	}
	have := map[string]bool{}
	for _, l := range links {
		if l.TargetType != "url" || l.Label != label || l.Direction == "incoming" || l.SourceID != id {
			continue
		}
		have[l.URL] = true
		if !want[l.URL] {
			if err := s.Store.DeleteItemLink(ctx, p.TenantID(), l.ID); err != nil {
				return err
			}
			s.audit(ctx, p, "item_link.delete", kind, id, l.URL)
		}
	}
	for _, u := range urls {
		if have[u] {
			continue
		}
		have[u] = true
		l := &model.ItemLink{SourceType: kind, SourceID: id, TargetType: "url", URL: u, Label: label, CreatedBy: p.User.Email}
		if err := s.Store.CreateItemLink(ctx, p.TenantID(), l); err != nil {
			return err
		}
		s.audit(ctx, p, "item_link.create", kind, id, label+" "+u)
	}
	return nil
}

func (s *Server) oslcHandlers(kind string) (create, update, del handlerFunc) {
	if kind == model.KindTest {
		return s.handleCreateTest, s.handleUpdateTest, s.handleDeleteTest
	}
	return s.handleCreateRequirement, s.handleUpdateRequirement, s.handleDeleteRequirement
}

// oslcFolder resolves a folder path (creating missing folders).
func (s *Server) oslcFolder(ctx context.Context, p *Principal, pr *model.Project, kind string, in *oslcInput) error {
	if in.folder == nil {
		return nil
	}
	if *in.folder == "" {
		in.body["folder_id"] = nil
		return nil
	}
	fr, err := s.newFolderResolver(ctx, p, pr.ID, kind)
	if err != nil {
		return err
	}
	fid, err := fr.resolve(nil, reqimport.FolderParts(*in.folder))
	if err != nil {
		return err
	}
	in.body["folder_id"] = fid
	return nil
}

func (s *Server) handleOSLCCreate(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		ctx := r.Context()
		ids, err := s.loadOSLCIDs(ctx, p, pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		_, u := s.oslcGraph(r)
		in, err := s.readOSLCInput(r, p, pr, kind, u.collection(pr.ID, kind), ids)
		if err != nil {
			s.failOSLC(w, r, err)
			return
		}
		if t, _ := in.body["title"].(string); t == "" {
			s.writeOSLCError(w, r, http.StatusBadRequest, "bad_request", "dcterms:title is required")
			return
		}
		if err := s.oslcFolder(ctx, p, pr, kind, in); err != nil {
			s.failOSLC(w, r, err)
			return
		}
		if kind == model.KindRequirement {
			in.body["create_test"] = false
		}
		create, _, _ := s.oslcHandlers(kind)
		rec := s.subCall(r, p, create, http.MethodPost, map[string]string{"pid": pr.ID}, in.body, 0)
		if rec.status() >= 400 {
			s.failOSLC(w, r, subFailure(rec))
			return
		}
		var out struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(rec.body.Bytes(), &out)
		if err := s.applyOSLCLinks(ctx, p, pr, kind, out.ID, in, u.vocab()); err != nil {
			s.failOSLC(w, r, err)
			return
		}
		s.writeItem(w, r, p, pr, kind, out.ID, http.StatusCreated)
	}
}

// oslcCurrent finds the item of the request and checks If-Match.
func (s *Server) oslcCurrent(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project, kind string, ids *oslcIDs) (id string, version int, ok bool) {
	id, found := ids.find(kind, r.PathValue("item"))
	if !found {
		s.writeOSLCError(w, r, http.StatusNotFound, "not_found", kind+" not found")
		return "", 0, false
	}
	var cur int
	if kind == model.KindTest {
		t, err := s.Store.GetTest(r.Context(), p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return "", 0, false
		}
		cur = t.Version
	} else {
		rq, err := s.Store.GetRequirement(r.Context(), p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return "", 0, false
		}
		cur = rq.Version
	}
	want, has, err := ifMatchVersion(r)
	if err != nil {
		s.failOSLC(w, r, oslcBad(http.StatusBadRequest, "If-Match must be the ETag of the resource"))
		return "", 0, false
	}
	if has && want != cur {
		setVersionTag(w, cur)
		s.writeOSLCError(w, r, http.StatusPreconditionFailed, "stale", "the resource was changed since it was read (ETag \""+strconv.Itoa(cur)+"\"); read it again")
		return "", 0, false
	}
	return id, cur, true
}

func (s *Server) handleOSLCUpdate(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		ctx := r.Context()
		ids, err := s.loadOSLCIDs(ctx, p, pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		id, version, ok := s.oslcCurrent(w, r, p, pr, kind, ids)
		if !ok {
			return
		}
		_, u := s.oslcGraph(r)
		in, err := s.readOSLCInput(r, p, pr, kind, u.item(pr.ID, kind, id), ids)
		if err != nil {
			s.failOSLC(w, r, err)
			return
		}
		if t, ok := in.body["title"].(string); ok && t == "" {
			s.writeOSLCError(w, r, http.StatusBadRequest, "bad_request", "dcterms:title cannot be empty")
			return
		}
		if err := s.oslcFolder(ctx, p, pr, kind, in); err != nil {
			s.failOSLC(w, r, err)
			return
		}
		if len(in.body) > 0 {
			_, update, _ := s.oslcHandlers(kind)
			rec := s.subCall(r, p, update, http.MethodPatch, map[string]string{"id": id}, in.body, version)
			if rec.status() >= 400 {
				s.failOSLC(w, r, subFailure(rec))
				return
			}
		}
		if err := s.applyOSLCLinks(ctx, p, pr, kind, id, in, u.vocab()); err != nil {
			s.failOSLC(w, r, err)
			return
		}
		s.writeItem(w, r, p, pr, kind, id, http.StatusOK)
	}
}

func (s *Server) handleOSLCDelete(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		ids, err := s.loadOSLCIDs(r.Context(), p, pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		id, _, ok := s.oslcCurrent(w, r, p, pr, kind, ids)
		if !ok {
			return
		}
		_, _, del := s.oslcHandlers(kind)
		rec := s.subCall(r, p, del, http.MethodDelete, map[string]string{"id": id}, nil, 0)
		if rec.status() >= 400 {
			s.failOSLC(w, r, subFailure(rec))
			return
		}
		w.Header().Set("OSLC-Core-Version", "2.0")
		w.WriteHeader(http.StatusNoContent)
	}
}
