package api

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/oslc"
	"github.com/rom/qtestframe/internal/rbac"
)

// A minimal, read-only OSLC provider: Requirements Management 2.0 and
// Quality Management 2.0 (see docs/api.md "OSLC"):
//
//	GET /api/v1/oslc/rootservices                    discovery (public)
//	GET /api/v1/oslc/catalog                         ServiceProviderCatalog: one provider per project
//	GET /api/v1/oslc/projects/{pid}                  ServiceProvider: RM and QM services
//	GET /api/v1/oslc/projects/{pid}/requirements     query capability (oslc_rm:Requirement)
//	GET /api/v1/oslc/projects/{pid}/requirements/{item}
//	GET /api/v1/oslc/projects/{pid}/tests            query capability (oslc_qm:TestCase)
//	GET /api/v1/oslc/projects/{pid}/tests/{item}
//
// Resources are RDF/XML (default), Turtle or JSON-LD by the Accept header.
// Authentication: an API token as Bearer, or as the password of Basic
// authentication (for OSLC clients that only offer Basic), or the session.
// Restricted projects are not listed and answer 404 without access.

const oslcPrefix = "/api/v1/oslc/"

// oslcDefaultPage is the page size of query results (oslc.pageSize caps at
// oslcMaxPage).
const (
	oslcDefaultPage = 100
	oslcMaxPage     = 1000
)

func (s *Server) routesOSLC(pub func(string, http.HandlerFunc), h routeFunc) {
	pub("GET /api/v1/oslc/rootservices", s.handleOSLCRootServices)
	h("GET /api/v1/oslc/catalog", s.handleOSLCCatalog)
	h("GET /api/v1/oslc/projects/{pid}", s.handleOSLCProvider, "project.read")
	h("GET /api/v1/oslc/projects/{pid}/requirements", s.handleOSLCQuery(model.KindRequirement), "requirement.read")
	h("GET /api/v1/oslc/projects/{pid}/requirements/{item}", s.handleOSLCResource(model.KindRequirement), "requirement.read")
	h("GET /api/v1/oslc/projects/{pid}/tests", s.handleOSLCQuery(model.KindTest), "test.read")
	h("GET /api/v1/oslc/projects/{pid}/tests/{item}", s.handleOSLCResource(model.KindTest), "test.read")
}

// basicTokenAuth turns "Authorization: Basic user:<API token>" on the OSLC
// paths into a Bearer token (the user name is ignored).
func basicTokenAuth(r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, oslcPrefix) {
		return
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Basic ") {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(h, "Basic ")))
	if err != nil {
		return
	}
	_, pw, ok := strings.Cut(string(raw), ":")
	if ok && auth.IsAPIToken(pw) {
		r.Header.Set("Authorization", "Bearer "+pw)
	}
}

// oslcBase is the absolute URL of the server.
func (s *Server) oslcBase(r *http.Request) string {
	if b := s.baseURL(); b != "" {
		return b
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

type oslcURLs struct{ base string }

func (u oslcURLs) catalog() string { return u.base + oslcPrefix + "catalog" }
func (u oslcURLs) provider(pid string) string {
	return u.base + oslcPrefix + "projects/" + url.PathEscape(pid)
}
func (u oslcURLs) requirements(pid string) string { return u.provider(pid) + "/requirements" }
func (u oslcURLs) tests(pid string) string        { return u.provider(pid) + "/tests" }
func (u oslcURLs) requirement(pid, id string) string {
	return u.requirements(pid) + "/" + url.PathEscape(id)
}
func (u oslcURLs) test(pid, id string) string { return u.tests(pid) + "/" + url.PathEscape(id) }
func (u oslcURLs) vocab() string              { return u.base + oslcPrefix + "vocab#" }
func (u oslcURLs) web(key string) string      { return u.base + "/go/" + url.PathEscape(key) }

func (s *Server) oslcGraph(r *http.Request) (*oslc.Graph, oslcURLs) {
	u := oslcURLs{base: s.oslcBase(r)}
	return oslc.NewGraph(map[string]string{"qtf": u.vocab()}), u
}

func writeOSLC(w http.ResponseWriter, r *http.Request, g *oslc.Graph) {
	mt := oslc.Negotiate(r.Header.Get("Accept"))
	var buf bytes.Buffer
	if err := g.Write(&buf, mt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "rendering failed")
		return
	}
	w.Header().Set("Content-Type", mt+"; charset=utf-8")
	w.Header().Set("OSLC-Core-Version", "2.0")
	w.Header().Set("Vary", "Accept")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleOSLCRootServices(w http.ResponseWriter, r *http.Request) {
	g, u := s.oslcGraph(r)
	g.Prefixes["jd"] = "http://jazz.net/xmlns/prod/jazz/discovery/1.0/"
	root := oslc.IRI(s.oslcBase(r) + oslcPrefix + "rootservices")
	g.Add(root, oslc.NSDCTerms+"title", oslc.Lit("Qtestframe"))
	g.Add(root, "http://open-services.net/xmlns/rm/1.0/rmServiceProviders", oslc.IRI(u.catalog()))
	g.Add(root, "http://open-services.net/xmlns/qm/1.0/qmServiceProviders", oslc.IRI(u.catalog()))
	writeOSLC(w, r, g)
}

func (s *Server) handleOSLCCatalog(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	ps, err := s.Store.ListProjects(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vis, err := s.visibleProjects(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	g, u := s.oslcGraph(r)
	cat := oslc.IRI(u.catalog())
	g.Type(cat, oslc.NSOSLC+"ServiceProviderCatalog")
	g.Add(cat, oslc.NSDCTerms+"title", oslc.Lit("Qtestframe – "+p.Tenant.Name))
	g.Add(cat, oslc.NSDCTerms+"description", oslc.Lit("Requirements (OSLC RM 2.0) and test cases (OSLC QM 2.0), read-only"))
	g.Add(cat, oslc.NSOSLC+"domain", oslc.IRI(oslc.NSRM))
	g.Add(cat, oslc.NSOSLC+"domain", oslc.IRI(oslc.NSQM))
	sort.Slice(ps, func(i, j int) bool { return ps[i].Key < ps[j].Key })
	for _, pr := range ps {
		if !visibleIn(vis, p, pr.ID, rbac.ProjectRead) {
			continue
		}
		sp := oslc.IRI(u.provider(pr.ID))
		g.Add(cat, oslc.NSOSLC+"serviceProvider", sp)
		g.Type(sp, oslc.NSOSLC+"ServiceProvider")
		g.Add(sp, oslc.NSDCTerms+"title", oslc.Lit(pr.Key+" – "+pr.Name))
		g.Add(sp, oslc.NSDCTerms+"identifier", oslc.Lit(pr.Key))
	}
	writeOSLC(w, r, g)
}

func (s *Server) handleOSLCProvider(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	g, u := s.oslcGraph(r)
	sp := oslc.IRI(u.provider(pr.ID))
	g.Type(sp, oslc.NSOSLC+"ServiceProvider")
	g.Add(sp, oslc.NSDCTerms+"title", oslc.Lit(pr.Key+" – "+pr.Name))
	g.Add(sp, oslc.NSDCTerms+"identifier", oslc.Lit(pr.Key))
	g.Add(sp, oslc.NSDCTerms+"description", oslc.Lit(pr.Description))
	g.Add(sp, oslc.NSOSLC+"details", oslc.IRI(u.base+"/p/"+url.PathEscape(pr.ID)))
	for _, sv := range []struct {
		kind, domain, title, class, query string
		perm                              rbac.Permission
	}{
		{model.KindRequirement, oslc.NSRM, "Requirements", oslc.NSRM + "Requirement", u.requirements(pr.ID), rbac.RequirementRd},
		{model.KindTest, oslc.NSQM, "Test cases", oslc.NSQM + "TestCase", u.tests(pr.ID), rbac.TestRead},
	} {
		if !p.Can(sv.perm) {
			continue
		}
		svc := oslc.Blank("service-" + sv.kind)
		qc := oslc.Blank("query-" + sv.kind)
		g.Add(sp, oslc.NSOSLC+"service", svc)
		g.Type(svc, oslc.NSOSLC+"Service")
		g.Add(svc, oslc.NSOSLC+"domain", oslc.IRI(sv.domain))
		g.Add(svc, oslc.NSOSLC+"queryCapability", qc)
		g.Type(qc, oslc.NSOSLC+"QueryCapability")
		g.Add(qc, oslc.NSDCTerms+"title", oslc.Lit(sv.title))
		g.Add(qc, oslc.NSOSLC+"label", oslc.Lit(sv.title))
		g.Add(qc, oslc.NSOSLC+"queryBase", oslc.IRI(sv.query))
		g.Add(qc, oslc.NSOSLC+"resourceType", oslc.IRI(sv.class))
	}
	writeOSLC(w, r, g)
}

// oslcData is what the resources of a project need.
type oslcData struct {
	reqs   []model.Requirement
	tests  []model.TestCase
	links  []model.TraceLink
	rels   []model.Relation
	paths  map[string]string
	tpaths map[string]string
}

func (s *Server) loadOSLC(r *http.Request, p *Principal, pr *model.Project, kind string) (*oslcData, error) {
	ctx := r.Context()
	d := &oslcData{}
	var err error
	if d.links, err = s.Store.ListLinks(ctx, p.TenantID(), pr.ID, "", ""); err != nil {
		return nil, err
	}
	folders, err := s.Store.ListFolders(ctx, p.TenantID(), pr.ID, kind)
	if err != nil {
		return nil, err
	}
	d.paths = folderPaths(folders)
	if kind == model.KindRequirement {
		if d.reqs, err = s.Store.ListRequirements(ctx, p.TenantID(), pr.ID); err != nil {
			return nil, err
		}
		if d.rels, err = s.Store.ListRelations(ctx, p.TenantID(), pr.ID, ""); err != nil {
			return nil, err
		}
	} else if d.tests, err = s.Store.ListTests(ctx, p.TenantID(), pr.ID); err != nil {
		return nil, err
	}
	return d, nil
}

func dateTime(t time.Time) oslc.Term {
	return oslc.Typed(t.UTC().Format(time.RFC3339), oslc.NSXSD+"dateTime")
}

func addCreator(g *oslc.Graph, s oslc.Term, who, id string) {
	if who == "" {
		return
	}
	b := oslc.Blank("creator-" + id)
	g.Add(s, oslc.NSDCTerms+"creator", b)
	g.Type(b, oslc.NSFOAF+"Person")
	g.Add(b, oslc.NSFOAF+"name", oslc.Lit(who))
	if strings.Contains(who, "@") {
		g.Add(b, oslc.NSFOAF+"mbox", oslc.IRI("mailto:"+who))
	}
}

// oslcRelation maps a relation type to an OSLC RM property (also always
// written as qtf:<type>).
var oslcRelation = map[string]string{"refines": oslc.NSRM + "elaborates", "depends_on": oslc.NSRM + "constrainedBy",
	"derived_from": oslc.NSRM + "satisfies", "conflicts_with": oslc.NSDCTerms + "relation", "duplicates": oslc.NSDCTerms + "relation",
	"provides": oslc.NSDCTerms + "relation", "relates_to": oslc.NSDCTerms + "relation"}

func (s *Server) addRequirement(g *oslc.Graph, u oslcURLs, pr *model.Project, rq *model.Requirement, d *oslcData) {
	res := oslc.IRI(u.requirement(pr.ID, rq.ID))
	q := u.vocab()
	g.Type(res, oslc.NSRM+"Requirement")
	g.Add(res, oslc.NSDCTerms+"identifier", oslc.Lit(rq.Key))
	g.Add(res, oslc.NSOSLC+"shortTitle", oslc.Lit(rq.Key))
	g.Add(res, oslc.NSDCTerms+"title", oslc.Lit(rq.Title))
	g.Add(res, oslc.NSDCTerms+"description", oslc.Lit(rq.Description))
	g.Add(res, oslc.NSDCTerms+"created", dateTime(rq.CreatedAt))
	g.Add(res, oslc.NSDCTerms+"modified", dateTime(rq.UpdatedAt))
	addCreator(g, res, rq.CreatedBy, rq.ID)
	g.Add(res, oslc.NSOSLC+"serviceProvider", oslc.IRI(u.provider(pr.ID)))
	g.Add(res, oslc.NSRDFS+"seeAlso", oslc.IRI(u.web(rq.Key)))
	for _, t := range rq.Tags {
		g.Add(res, oslc.NSDCTerms+"subject", oslc.Lit(t))
	}
	g.Add(res, q+"status", oslc.Lit(rq.Status))
	g.Add(res, q+"obligation", oslc.Lit(rq.Obligation))
	g.Add(res, q+"type", oslc.Lit(rq.Type))
	g.Add(res, q+"priority", oslc.Lit(rq.Priority))
	g.Add(res, q+"source", oslc.Lit(rq.Source))
	g.Add(res, q+"owner", oslc.Lit(rq.Owner))
	g.Add(res, q+"version", oslc.Typed(strconv.Itoa(rq.Version), oslc.NSXSD+"integer"))
	if rq.FolderID != nil {
		g.Add(res, q+"folder", oslc.Lit(d.paths[*rq.FolderID]))
	}
	for _, l := range d.links {
		if l.RequirementID == rq.ID {
			g.Add(res, oslc.NSRM+"validatedBy", oslc.IRI(u.test(pr.ID, l.TestCaseID)))
		}
	}
	for _, rel := range d.rels {
		if rel.SourceID != rq.ID {
			continue
		}
		target := oslc.IRI(u.requirement(pr.ID, rel.TargetID))
		g.Add(res, q+rel.Type, target)
		if prop := oslcRelation[rel.Type]; prop != "" {
			g.Add(res, prop, target)
		}
	}
}

func (s *Server) addTest(g *oslc.Graph, u oslcURLs, pr *model.Project, t *model.TestCase, d *oslcData) {
	res := oslc.IRI(u.test(pr.ID, t.ID))
	q := u.vocab()
	g.Type(res, oslc.NSQM+"TestCase")
	g.Add(res, oslc.NSDCTerms+"identifier", oslc.Lit(t.Key))
	g.Add(res, oslc.NSOSLC+"shortTitle", oslc.Lit(t.Key))
	g.Add(res, oslc.NSDCTerms+"title", oslc.Lit(t.Title))
	g.Add(res, oslc.NSDCTerms+"description", oslc.Lit(t.Description))
	g.Add(res, oslc.NSDCTerms+"created", dateTime(t.CreatedAt))
	g.Add(res, oslc.NSDCTerms+"modified", dateTime(t.UpdatedAt))
	addCreator(g, res, t.CreatedBy, t.ID)
	g.Add(res, oslc.NSOSLC+"serviceProvider", oslc.IRI(u.provider(pr.ID)))
	g.Add(res, oslc.NSRDFS+"seeAlso", oslc.IRI(u.web(t.Key)))
	for _, tag := range t.Tags {
		g.Add(res, oslc.NSDCTerms+"subject", oslc.Lit(tag))
	}
	g.Add(res, q+"status", oslc.Lit(t.Status))
	g.Add(res, q+"priority", oslc.Lit(t.Priority))
	g.Add(res, q+"executionType", oslc.Lit(t.ExecutionType))
	g.Add(res, q+"preconditions", oslc.Lit(t.Preconditions))
	g.Add(res, q+"expectedResult", oslc.Lit(t.ExpectedResult))
	g.Add(res, q+"version", oslc.Typed(strconv.Itoa(t.Version), oslc.NSXSD+"integer"))
	if t.FolderID != nil {
		g.Add(res, q+"folder", oslc.Lit(d.paths[*t.FolderID]))
	}
	for _, l := range d.links {
		if l.TestCaseID == t.ID {
			g.Add(res, oslc.NSQM+"validatesRequirement", oslc.IRI(u.requirement(pr.ID, l.RequirementID)))
		}
	}
}

func (s *Server) handleOSLCResource(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		d, err := s.loadOSLC(r, p, pr, kind)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		id := r.PathValue("item")
		g, u := s.oslcGraph(r)
		found := false
		if kind == model.KindRequirement {
			for i := range d.reqs {
				if d.reqs[i].ID == id || strings.EqualFold(d.reqs[i].Key, id) {
					s.addRequirement(g, u, pr, &d.reqs[i], d)
					w.Header().Set("ETag", `"`+strconv.Itoa(d.reqs[i].Version)+`"`)
					found = true
				}
			}
		} else {
			for i := range d.tests {
				if d.tests[i].ID == id || strings.EqualFold(d.tests[i].Key, id) {
					s.addTest(g, u, pr, &d.tests[i], d)
					w.Header().Set("ETag", `"`+strconv.Itoa(d.tests[i].Version)+`"`)
					found = true
				}
			}
		}
		if !found {
			writeError(w, http.StatusNotFound, "not_found", kind+" not found")
			return
		}
		writeOSLC(w, r, g)
	}
}

// oslcFields are the properties oslc.where can filter on.
var oslcFields = map[string]bool{oslc.NSDCTerms + "identifier": true, oslc.NSDCTerms + "title": true,
	oslc.NSDCTerms + "modified": true, oslc.NSDCTerms + "created": true}

type oslcRow struct {
	id, key, title string
	created, mod   time.Time
}

func (row oslcRow) value(prop string) (string, bool) {
	switch strings.TrimPrefix(prop, oslc.NSDCTerms) {
	case "identifier":
		return row.key, false
	case "title":
		return row.title, false
	case "modified":
		return row.mod.UTC().Format(time.RFC3339Nano), true
	}
	return row.created.UTC().Format(time.RFC3339Nano), true
}

func (s *Server) handleOSLCQuery(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		q := r.URL.Query()
		g, u := s.oslcGraph(r)
		var where *oslc.Query
		if wv := strings.TrimSpace(q.Get("oslc.where")); wv != "" {
			prefixes, err := oslc.ParsePrefixes(q.Get("oslc.prefix"), g.Prefixes)
			if err != nil {
				s.fail(w, r, badRequest("%v", err))
				return
			}
			if where, err = oslc.ParseWhere(wv, prefixes); err != nil {
				s.fail(w, r, badRequest("%v", err))
				return
			}
			for _, c := range where.Conditions {
				if !oslcFields[c.Property] {
					s.fail(w, r, badRequest("oslc.where: %s cannot be queried (use dcterms:identifier, dcterms:title, dcterms:modified or dcterms:created)", c.Property))
					return
				}
			}
		}
		size := oslcDefaultPage
		if v := q.Get("oslc.pageSize"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				s.fail(w, r, badRequest("oslc.pageSize must be a positive number"))
				return
			}
			size = min(n, oslcMaxPage)
		}
		page := 0
		if v := q.Get("page"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				s.fail(w, r, badRequest("page must be a number"))
				return
			}
			page = n
		}
		d, err := s.loadOSLC(r, p, pr, kind)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var rows []oslcRow
		if kind == model.KindRequirement {
			for _, rq := range d.reqs {
				rows = append(rows, oslcRow{rq.ID, rq.Key, rq.Title, rq.CreatedAt, rq.UpdatedAt})
			}
		} else {
			for _, t := range d.tests {
				rows = append(rows, oslcRow{t.ID, t.Key, t.Title, t.CreatedAt, t.UpdatedAt})
			}
		}
		var match []oslcRow
	rowLoop:
		for _, row := range rows {
			if where != nil {
				for _, c := range where.Conditions {
					v, isTime := row.value(c.Property)
					if !c.Match(v, isTime) {
						continue rowLoop
					}
				}
			}
			match = append(match, row)
		}
		base := u.requirements(pr.ID)
		if kind == model.KindTest {
			base = u.tests(pr.ID)
		}
		qb := oslc.IRI(base)
		from, to := page*size, (page+1)*size
		from, to = min(from, len(match)), min(to, len(match))
		ids := map[string]bool{}
		for _, row := range match[from:to] {
			ids[row.id] = true
		}
		for i := range d.reqs {
			if ids[d.reqs[i].ID] {
				g.Add(qb, oslc.NSRDFS+"member", oslc.IRI(u.requirement(pr.ID, d.reqs[i].ID)))
			}
		}
		for i := range d.tests {
			if ids[d.tests[i].ID] {
				g.Add(qb, oslc.NSRDFS+"member", oslc.IRI(u.test(pr.ID, d.tests[i].ID)))
			}
		}
		info := oslc.IRI(u.base + r.URL.RequestURI())
		g.Type(info, oslc.NSOSLC+"ResponseInfo")
		g.Add(info, oslc.NSDCTerms+"title", oslc.Lit(pr.Key+" "+kind+"s"))
		g.Add(info, oslc.NSOSLC+"totalCount", oslc.Typed(strconv.Itoa(len(match)), oslc.NSXSD+"integer"))
		if to < len(match) {
			next := r.URL.Query()
			next.Set("page", strconv.Itoa(page+1))
			next.Set("oslc.paging", "true")
			next.Set("oslc.pageSize", strconv.Itoa(size))
			g.Add(info, oslc.NSOSLC+"nextPage", oslc.IRI(base+"?"+next.Encode()))
		}
		for i := range d.reqs {
			if ids[d.reqs[i].ID] {
				s.addRequirement(g, u, pr, &d.reqs[i], d)
			}
		}
		for i := range d.tests {
			if ids[d.tests[i].ID] {
				s.addTest(g, u, pr, &d.tests[i], d)
			}
		}
		writeOSLC(w, r, g)
	}
}
