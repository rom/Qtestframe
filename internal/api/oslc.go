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

// An OSLC provider: Requirements Management 2.1 and Quality Management 2.1
// (see docs/api.md "OSLC" and the administrator guide):
//
//	GET    /api/v1/oslc/rootservices                       Jazz-style discovery (public)
//	GET    /api/v1/oslc/catalog                            ServiceProviderCatalog: one provider per project
//	GET    /api/v1/oslc/projects/{pid}/catalog             the catalog of one project
//	GET    /api/v1/oslc/projects/{pid}                     ServiceProvider: RM and QM services
//	GET    /api/v1/oslc/projects/{pid}/shapes/{kind}       ResourceShape (requirement | test)
//	GET    /api/v1/oslc/projects/{pid}/requirements        query capability (oslc_rm:Requirement)
//	POST   /api/v1/oslc/projects/{pid}/requirements        creation factory
//	GET    /api/v1/oslc/projects/{pid}/requirements/{item} a requirement (ETag = version)
//	PUT    /api/v1/oslc/projects/{pid}/requirements/{item} update (If-Match)
//	DELETE /api/v1/oslc/projects/{pid}/requirements/{item}
//	…/tests, …/tests/{item}                                the same for oslc_qm:TestCase
//	GET    /api/v1/oslc/projects/{pid}/dialogs/{kind}/{select|create}  delegated dialogs (oslc_dialog.go)
//
// Resources are RDF/XML (default), Turtle or JSON-LD by the Accept header;
// request bodies by their Content-Type (oslc_write.go). Link properties
// (oslc_rm:validatedBy, oslc_qm:validatesRequirement, oslc_rm:elaborates
// …) are the trace links and requirement relations; links to resources of
// other tools (oslc_rm:implementedBy …) are item links to the URL.
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
	h("GET /api/v1/oslc/projects/{pid}/catalog", s.handleOSLCProjectCatalog, "project.read")
	h("GET /api/v1/oslc/projects/{pid}", s.handleOSLCProvider, "project.read")
	h("GET /api/v1/oslc/projects/{pid}/shapes/{kind}", s.handleOSLCShape, "project.read")
	h("GET /api/v1/oslc/projects/{pid}/requirements", s.handleOSLCQuery(model.KindRequirement), "requirement.read")
	h("POST /api/v1/oslc/projects/{pid}/requirements", s.atomic(s.handleOSLCCreate(model.KindRequirement)), "requirement.write")
	h("GET /api/v1/oslc/projects/{pid}/requirements/{item}", s.handleOSLCResource(model.KindRequirement), "requirement.read")
	h("PUT /api/v1/oslc/projects/{pid}/requirements/{item}", s.atomic(s.handleOSLCUpdate(model.KindRequirement)), "requirement.write")
	h("DELETE /api/v1/oslc/projects/{pid}/requirements/{item}", s.handleOSLCDelete(model.KindRequirement), "requirement.write")
	h("GET /api/v1/oslc/projects/{pid}/tests", s.handleOSLCQuery(model.KindTest), "test.read")
	h("POST /api/v1/oslc/projects/{pid}/tests", s.atomic(s.handleOSLCCreate(model.KindTest)), "test.write")
	h("GET /api/v1/oslc/projects/{pid}/tests/{item}", s.handleOSLCResource(model.KindTest), "test.read")
	h("PUT /api/v1/oslc/projects/{pid}/tests/{item}", s.atomic(s.handleOSLCUpdate(model.KindTest)), "test.write")
	h("DELETE /api/v1/oslc/projects/{pid}/tests/{item}", s.handleOSLCDelete(model.KindTest), "test.write")
	s.routesOSLCDialogs(pub)
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
func (u oslcURLs) projectCatalog(pid string) string { return u.provider(pid) + "/catalog" }
func (u oslcURLs) requirements(pid string) string   { return u.provider(pid) + "/requirements" }
func (u oslcURLs) tests(pid string) string          { return u.provider(pid) + "/tests" }
func (u oslcURLs) collection(pid, kind string) string {
	if kind == model.KindTest {
		return u.tests(pid)
	}
	return u.requirements(pid)
}
func (u oslcURLs) requirement(pid, id string) string {
	return u.requirements(pid) + "/" + url.PathEscape(id)
}
func (u oslcURLs) test(pid, id string) string { return u.tests(pid) + "/" + url.PathEscape(id) }
func (u oslcURLs) item(pid, kind, id string) string {
	if kind == model.KindTest {
		return u.test(pid, id)
	}
	return u.requirement(pid, id)
}
func (u oslcURLs) shape(pid, kind string) string { return u.provider(pid) + "/shapes/" + kind }
func (u oslcURLs) dialog(pid, kind, mode string) string {
	return u.provider(pid) + "/dialogs/" + kind + "/" + mode
}
func (u oslcURLs) vocab() string         { return u.base + oslcPrefix + "vocab#" }
func (u oslcURLs) web(key string) string { return u.base + "/go/" + url.PathEscape(key) }

// itemOf returns the item id (or key) of one of our resource IRIs in the
// project, and its kind.
func (u oslcURLs) itemOf(pid, iri string) (kind, id string, ok bool) {
	for _, k := range []string{model.KindRequirement, model.KindTest} {
		if rest, found := strings.CutPrefix(iri, u.collection(pid, k)+"/"); found && rest != "" && !strings.Contains(rest, "/") {
			if v, err := url.PathUnescape(rest); err == nil {
				rest = v
			}
			return k, rest, true
		}
	}
	return "", "", false
}

func (s *Server) oslcGraph(r *http.Request) (*oslc.Graph, oslcURLs) {
	u := oslcURLs{base: s.oslcBase(r)}
	return oslc.NewGraph(map[string]string{"qtf": u.vocab()}), u
}

func writeOSLC(w http.ResponseWriter, r *http.Request, g *oslc.Graph) {
	writeOSLCStatus(w, r, g, http.StatusOK)
}

func writeOSLCStatus(w http.ResponseWriter, r *http.Request, g *oslc.Graph, status int) {
	mt := oslc.Negotiate(r.Header.Get("Accept"))
	var buf bytes.Buffer
	if err := g.Write(&buf, mt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "rendering failed")
		return
	}
	w.Header().Set("Content-Type", mt+"; charset=utf-8")
	w.Header().Set("OSLC-Core-Version", "2.0")
	w.Header().Set("Vary", "Accept")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// writeOSLCError answers an oslc:Error resource in the negotiated format
// (JSON clients that ask for nothing else get the usual JSON error).
func (s *Server) writeOSLCError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	accept := strings.ToLower(r.Header.Get("Accept"))
	if !strings.Contains(accept, "rdf") && !strings.Contains(accept, "turtle") && !strings.Contains(accept, "ld+json") {
		writeError(w, status, code, msg)
		return
	}
	g, _ := s.oslcGraph(r)
	e := oslc.Blank("error")
	g.Type(e, oslc.NSOSLC+"Error")
	g.Add(e, oslc.NSOSLC+"statusCode", oslc.Lit(strconv.Itoa(status)))
	g.Add(e, oslc.NSOSLC+"message", oslc.Lit(msg))
	writeOSLCStatus(w, r, g, status)
}

func (s *Server) handleOSLCRootServices(w http.ResponseWriter, r *http.Request) {
	g, u := s.oslcGraph(r)
	jd := "http://jazz.net/xmlns/prod/jazz/discovery/1.0/"
	g.Prefixes["jd"] = jd
	g.Prefixes["oslc_rm_v1"] = "http://open-services.net/xmlns/rm/1.0/"
	g.Prefixes["oslc_qm_v1"] = "http://open-services.net/xmlns/qm/1.0/"
	root := oslc.IRI(s.oslcBase(r) + oslcPrefix + "rootservices")
	g.Add(root, oslc.NSDCTerms+"title", oslc.Lit("Qtestframe"))
	g.Add(root, "http://open-services.net/xmlns/rm/1.0/rmServiceProviders", oslc.IRI(u.catalog()))
	g.Add(root, "http://open-services.net/xmlns/qm/1.0/qmServiceProviders", oslc.IRI(u.catalog()))
	g.Add(root, jd+"oslcCatalogs", oslc.IRI(u.catalog()))
	g.Add(root, jd+"friends", oslc.IRI(u.catalog()))
	g.Add(root, oslc.NSOSLC+"serviceProviderCatalog", oslc.IRI(u.catalog()))
	writeOSLC(w, r, g)
}

func addProviderRef(g *oslc.Graph, u oslcURLs, cat oslc.Term, pr *model.Project) {
	sp := oslc.IRI(u.provider(pr.ID))
	g.Add(cat, oslc.NSOSLC+"serviceProvider", sp)
	g.Type(sp, oslc.NSOSLC+"ServiceProvider")
	g.Add(sp, oslc.NSDCTerms+"title", oslc.Lit(pr.Key+" – "+pr.Name))
	g.Add(sp, oslc.NSDCTerms+"identifier", oslc.Lit(pr.Key))
	g.Add(sp, oslc.NSOSLC+"details", oslc.IRI(u.base+"/p/"+url.PathEscape(pr.ID)))
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
	g.Add(cat, oslc.NSDCTerms+"description", oslc.Lit("Requirements (OSLC RM 2.1) and test cases (OSLC QM 2.1)"))
	g.Add(cat, oslc.NSOSLC+"domain", oslc.IRI(oslc.NSRM))
	g.Add(cat, oslc.NSOSLC+"domain", oslc.IRI(oslc.NSQM))
	sort.Slice(ps, func(i, j int) bool { return ps[i].Key < ps[j].Key })
	for i := range ps {
		if !visibleIn(vis, p, ps[i].ID, rbac.ProjectRead) {
			continue
		}
		addProviderRef(g, u, cat, &ps[i])
		g.Add(cat, oslc.NSOSLC+"serviceProviderCatalog", oslc.IRI(u.projectCatalog(ps[i].ID)))
	}
	writeOSLC(w, r, g)
}

// handleOSLCProjectCatalog is a catalog holding one project's provider,
// for tools that connect one of their projects to one of ours.
func (s *Server) handleOSLCProjectCatalog(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	g, u := s.oslcGraph(r)
	cat := oslc.IRI(u.projectCatalog(pr.ID))
	g.Type(cat, oslc.NSOSLC+"ServiceProviderCatalog")
	g.Add(cat, oslc.NSDCTerms+"title", oslc.Lit("Qtestframe – "+pr.Key+" – "+pr.Name))
	g.Add(cat, oslc.NSOSLC+"domain", oslc.IRI(oslc.NSRM))
	g.Add(cat, oslc.NSOSLC+"domain", oslc.IRI(oslc.NSQM))
	addProviderRef(g, u, cat, pr)
	writeOSLC(w, r, g)
}

type oslcService struct {
	kind, domain, title, class string
	read, write                rbac.Permission
}

func oslcServices() []oslcService {
	return []oslcService{
		{model.KindRequirement, oslc.NSRM, "Requirements", oslc.NSRM + "Requirement", rbac.RequirementRd, rbac.RequirementWr},
		{model.KindTest, oslc.NSQM, "Test cases", oslc.NSQM + "TestCase", rbac.TestRead, rbac.TestWrite},
	}
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
	pub := oslc.Blank("publisher")
	g.Add(sp, oslc.NSDCTerms+"publisher", pub)
	g.Type(pub, oslc.NSOSLC+"Publisher")
	g.Add(pub, oslc.NSDCTerms+"title", oslc.Lit("Qtestframe"))
	g.Add(pub, oslc.NSDCTerms+"identifier", oslc.Lit("qtestframe"))
	for _, sv := range oslcServices() {
		if !p.Can(sv.read) {
			continue
		}
		svc := oslc.Blank("service-" + sv.kind)
		g.Add(sp, oslc.NSOSLC+"service", svc)
		g.Type(svc, oslc.NSOSLC+"Service")
		g.Add(svc, oslc.NSOSLC+"domain", oslc.IRI(sv.domain))
		qc := oslc.Blank("query-" + sv.kind)
		g.Add(svc, oslc.NSOSLC+"queryCapability", qc)
		g.Type(qc, oslc.NSOSLC+"QueryCapability")
		g.Add(qc, oslc.NSDCTerms+"title", oslc.Lit(sv.title))
		g.Add(qc, oslc.NSOSLC+"label", oslc.Lit(sv.title))
		g.Add(qc, oslc.NSOSLC+"queryBase", oslc.IRI(u.collection(pr.ID, sv.kind)))
		g.Add(qc, oslc.NSOSLC+"resourceType", oslc.IRI(sv.class))
		g.Add(qc, oslc.NSOSLC+"resourceShape", oslc.IRI(u.shape(pr.ID, sv.kind)))
		g.Add(qc, oslc.NSOSLC+"usage", oslc.IRI(oslc.NSOSLC+"default"))
		dialog := func(id, mode, title string, w, h int) {
			d := oslc.Blank(id + "-" + sv.kind)
			g.Type(d, oslc.NSOSLC+"Dialog")
			g.Add(d, oslc.NSDCTerms+"title", oslc.Lit(title))
			g.Add(d, oslc.NSOSLC+"label", oslc.Lit(title))
			g.Add(d, oslc.NSOSLC+"dialog", oslc.IRI(u.dialog(pr.ID, sv.kind, mode)))
			g.Add(d, oslc.NSOSLC+"hintWidth", oslc.Lit(strconv.Itoa(w)+"px"))
			g.Add(d, oslc.NSOSLC+"hintHeight", oslc.Lit(strconv.Itoa(h)+"px"))
			g.Add(d, oslc.NSOSLC+"resourceType", oslc.IRI(sv.class))
			g.Add(d, oslc.NSOSLC+"usage", oslc.IRI(oslc.NSOSLC+"default"))
			g.Add(svc, oslc.NSOSLC+id, d)
		}
		singular := strings.TrimSuffix(strings.ToLower(sv.title), "s")
		dialog("selectionDialog", "select", "Select "+singular+"s", 620, 520)
		if !p.Can(sv.write) {
			continue
		}
		cf := oslc.Blank("factory-" + sv.kind)
		g.Add(svc, oslc.NSOSLC+"creationFactory", cf)
		g.Type(cf, oslc.NSOSLC+"CreationFactory")
		g.Add(cf, oslc.NSDCTerms+"title", oslc.Lit("New "+singular))
		g.Add(cf, oslc.NSOSLC+"label", oslc.Lit("New "+singular))
		g.Add(cf, oslc.NSOSLC+"creation", oslc.IRI(u.collection(pr.ID, sv.kind)))
		g.Add(cf, oslc.NSOSLC+"resourceType", oslc.IRI(sv.class))
		g.Add(cf, oslc.NSOSLC+"resourceShape", oslc.IRI(u.shape(pr.ID, sv.kind)))
		g.Add(cf, oslc.NSOSLC+"usage", oslc.IRI(oslc.NSOSLC+"default"))
		dialog("creationDialog", "create", "New "+singular, 620, 560)
	}
	writeOSLC(w, r, g)
}

// oslcData is what the resources of a project need.
type oslcData struct {
	reqs      []model.Requirement
	tests     []model.TestCase
	links     []model.TraceLink
	rels      []model.Relation
	itemLinks map[string][]model.ItemLink // by source id: links to URLs
	paths     map[string]string
}

func (s *Server) loadOSLC(r *http.Request, p *Principal, pr *model.Project, kind string) (*oslcData, error) {
	ctx := r.Context()
	d := &oslcData{itemLinks: map[string][]model.ItemLink{}}
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
	ils, err := s.Store.ListProjectItemLinks(ctx, p.TenantID(), pr.ID, kind)
	if err != nil {
		return nil, err
	}
	for _, l := range ils {
		if l.TargetType == "url" && l.SourceType == kind {
			d.itemLinks[l.SourceID] = append(d.itemLinks[l.SourceID], l)
		}
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

// oslcRelation maps a relation type to the OSLC RM properties of its
// source (out) and of its target (in); every relation is also written as
// qtf:<type> on its source.
var oslcRelation = map[string]struct{ out, in string }{
	"refines":      {oslc.NSRM + "elaborates", oslc.NSRM + "elaboratedBy"},
	"depends_on":   {oslc.NSRM + "constrainedBy", oslc.NSRM + "constrains"},
	"derived_from": {oslc.NSRM + "satisfies", oslc.NSRM + "satisfiedBy"},
	"relates_to":   {oslc.NSDCTerms + "relation", ""},
}

// oslcURLLinkProps are the link properties whose values may be resources
// of other tools: they are kept as item links to the URL, labelled with
// the property's prefixed name.
var oslcURLLinkProps = map[string][]string{
	model.KindRequirement: {oslc.NSRM + "validatedBy", oslc.NSRM + "implementedBy", oslc.NSRM + "trackedBy", oslc.NSRM + "affectedBy",
		oslc.NSRM + "elaborates", oslc.NSRM + "elaboratedBy", oslc.NSRM + "satisfies", oslc.NSRM + "satisfiedBy", oslc.NSRM + "specifiedBy",
		oslc.NSRM + "specifies", oslc.NSRM + "decomposes", oslc.NSRM + "decomposedBy", oslc.NSRM + "constrains", oslc.NSRM + "constrainedBy",
		oslc.NSDCTerms + "references"},
	model.KindTest: {oslc.NSQM + "validatesRequirement", oslc.NSQM + "testsChangeRequest", oslc.NSQM + "relatedChangeRequest",
		oslc.NSQM + "usesTestScript", oslc.NSQM + "affectedByChangeRequest", oslc.NSQM + "blockedByChangeRequest", oslc.NSDCTerms + "references"},
}

// linkLabel is the item link label of an OSLC link property.
func linkLabel(g *oslc.Graph, prop string) string {
	for _, pre := range []string{"oslc_rm", "oslc_qm", "dcterms"} {
		if local, ok := strings.CutPrefix(prop, g.Prefixes[pre]); ok {
			return pre + ":" + local
		}
	}
	return prop
}

func addURLLinks(g *oslc.Graph, res oslc.Term, kind string, links []model.ItemLink) {
	props := map[string]string{}
	for _, p := range oslcURLLinkProps[kind] {
		props[linkLabel(g, p)] = p
	}
	for _, l := range links {
		prop, ok := props[l.Label]
		if !ok {
			prop = oslc.NSDCTerms + "references"
		}
		g.Add(res, prop, oslc.IRI(l.URL))
	}
}

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
	g.Add(res, oslc.NSOSLC+"instanceShape", oslc.IRI(u.shape(pr.ID, model.KindRequirement)))
	g.Add(res, oslc.NSRDFS+"seeAlso", oslc.IRI(u.web(rq.Key)))
	for _, t := range rq.Tags {
		g.Add(res, oslc.NSDCTerms+"subject", oslc.Lit(t))
	}
	g.Add(res, q+"status", oslc.Lit(rq.Status))
	g.Add(res, q+"obligation", oslc.Lit(rq.Obligation))
	g.Add(res, q+"type", oslc.Lit(rq.Type))
	g.Add(res, q+"priority", oslc.Lit(rq.Priority))
	g.Add(res, q+"source", oslc.Lit(rq.Source))
	g.Add(res, q+"rationale", oslc.Lit(rq.Rationale))
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
		switch rq.ID {
		case rel.SourceID:
			target := oslc.IRI(u.requirement(pr.ID, rel.TargetID))
			g.Add(res, q+rel.Type, target)
			if prop := oslcRelation[rel.Type].out; prop != "" {
				g.Add(res, prop, target)
			}
		case rel.TargetID:
			if prop := oslcRelation[rel.Type].in; prop != "" {
				g.Add(res, prop, oslc.IRI(u.requirement(pr.ID, rel.SourceID)))
			}
		}
	}
	addURLLinks(g, res, model.KindRequirement, d.itemLinks[rq.ID])
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
	g.Add(res, oslc.NSOSLC+"instanceShape", oslc.IRI(u.shape(pr.ID, model.KindTest)))
	g.Add(res, oslc.NSRDFS+"seeAlso", oslc.IRI(u.web(t.Key)))
	for _, tag := range t.Tags {
		g.Add(res, oslc.NSDCTerms+"subject", oslc.Lit(tag))
	}
	g.Add(res, q+"status", oslc.Lit(t.Status))
	g.Add(res, q+"priority", oslc.Lit(t.Priority))
	g.Add(res, q+"executionType", oslc.Lit(t.ExecutionType))
	g.Add(res, q+"preconditions", oslc.Lit(t.Preconditions))
	g.Add(res, q+"expectedResult", oslc.Lit(t.ExpectedResult))
	if t.EstimatedMin > 0 {
		g.Add(res, q+"estimatedMinutes", oslc.Typed(strconv.Itoa(t.EstimatedMin), oslc.NSXSD+"integer"))
	}
	g.Add(res, q+"version", oslc.Typed(strconv.Itoa(t.Version), oslc.NSXSD+"integer"))
	if t.FolderID != nil {
		g.Add(res, q+"folder", oslc.Lit(d.paths[*t.FolderID]))
	}
	for _, l := range d.links {
		if l.TestCaseID == t.ID {
			g.Add(res, oslc.NSQM+"validatesRequirement", oslc.IRI(u.requirement(pr.ID, l.RequirementID)))
		}
	}
	addURLLinks(g, res, model.KindTest, d.itemLinks[t.ID])
}

// find returns the index of the item with the id or key ("" kind's list).
func (d *oslcData) find(kind, id string) int {
	if kind == model.KindRequirement {
		for i := range d.reqs {
			if d.reqs[i].ID == id || strings.EqualFold(d.reqs[i].Key, id) {
				return i
			}
		}
		return -1
	}
	for i := range d.tests {
		if d.tests[i].ID == id || strings.EqualFold(d.tests[i].Key, id) {
			return i
		}
	}
	return -1
}

// writeItem renders one resource with its ETag.
func (s *Server) writeItem(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project, kind, id string, status int) {
	d, err := s.loadOSLC(r, p, pr, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	i := d.find(kind, id)
	if i < 0 {
		s.writeOSLCError(w, r, http.StatusNotFound, "not_found", kind+" not found")
		return
	}
	g, u := s.oslcGraph(r)
	if kind == model.KindRequirement {
		s.addRequirement(g, u, pr, &d.reqs[i], d)
		setVersionTag(w, d.reqs[i].Version)
		if status == http.StatusCreated {
			w.Header().Set("Location", u.requirement(pr.ID, d.reqs[i].ID))
		}
	} else {
		s.addTest(g, u, pr, &d.tests[i], d)
		setVersionTag(w, d.tests[i].Version)
		if status == http.StatusCreated {
			w.Header().Set("Location", u.test(pr.ID, d.tests[i].ID))
		}
	}
	writeOSLCStatus(w, r, g, status)
}

func (s *Server) handleOSLCResource(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		s.writeItem(w, r, p, pr, kind, r.PathValue("item"), http.StatusOK)
	}
}

// ---------------------------------------------------------------- shapes

func (s *Server) handleOSLCShape(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	if kind != model.KindRequirement && kind != model.KindTest {
		s.writeOSLCError(w, r, http.StatusNotFound, "not_found", "no such resource shape")
		return
	}
	g, u := s.oslcGraph(r)
	q := u.vocab()
	sh := oslc.IRI(u.shape(pr.ID, kind))
	g.Type(sh, oslc.NSOSLC+"ResourceShape")
	class, title := oslc.NSRM+"Requirement", "Requirement"
	if kind == model.KindTest {
		class, title = oslc.NSQM+"TestCase", "Test case"
	}
	g.Add(sh, oslc.NSDCTerms+"title", oslc.Lit(title))
	g.Add(sh, oslc.NSOSLC+"describes", oslc.IRI(class))
	type prop struct {
		def, name, occurs, value string
		readOnly                 bool
		allowed                  []string
	}
	one, opt, many := oslc.NSOSLC+"Exactly-one", oslc.NSOSLC+"Zero-or-one", oslc.NSOSLC+"Zero-or-many"
	str, xml, res, integer, dt := oslc.NSXSD+"string", oslc.NSRDF+"XMLLiteral", oslc.NSOSLC+"Resource", oslc.NSXSD+"integer", oslc.NSXSD+"dateTime"
	props := []prop{
		{oslc.NSDCTerms + "identifier", "identifier", one, str, true, nil},
		{oslc.NSDCTerms + "title", "title", one, str, false, nil},
		{oslc.NSDCTerms + "description", "description", opt, xml, false, nil},
		{oslc.NSDCTerms + "subject", "subject", many, str, false, nil},
		{oslc.NSDCTerms + "created", "created", one, dt, true, nil},
		{oslc.NSDCTerms + "modified", "modified", one, dt, true, nil},
		{q + "folder", "folder", opt, str, false, nil},
		{q + "version", "version", one, integer, true, nil},
	}
	if kind == model.KindRequirement {
		props = append(props,
			prop{q + "status", "status", opt, str, false, model.RequirementStatuses},
			prop{q + "obligation", "obligation", opt, str, false, model.Obligations},
			prop{q + "type", "type", opt, str, false, model.RequirementTypes},
			prop{q + "priority", "priority", opt, str, false, model.Priorities},
			prop{q + "source", "source", opt, str, false, nil},
			prop{q + "rationale", "rationale", opt, str, false, nil},
			prop{q + "owner", "owner", opt, str, false, nil},
			prop{oslc.NSRM + "validatedBy", "validatedBy", many, res, false, nil},
			prop{oslc.NSRM + "implementedBy", "implementedBy", many, res, false, nil},
			prop{oslc.NSRM + "trackedBy", "trackedBy", many, res, false, nil},
			prop{oslc.NSRM + "affectedBy", "affectedBy", many, res, false, nil},
			prop{oslc.NSRM + "elaborates", "elaborates", many, res, false, nil},
			prop{oslc.NSRM + "elaboratedBy", "elaboratedBy", many, res, false, nil},
			prop{oslc.NSRM + "constrainedBy", "constrainedBy", many, res, false, nil},
			prop{oslc.NSRM + "constrains", "constrains", many, res, false, nil},
			prop{oslc.NSRM + "satisfies", "satisfies", many, res, false, nil},
			prop{oslc.NSRM + "satisfiedBy", "satisfiedBy", many, res, false, nil},
			prop{oslc.NSDCTerms + "relation", "relation", many, res, false, nil})
		for _, rt := range model.RelationTypes {
			props = append(props, prop{q + rt, rt, many, res, false, nil})
		}
	} else {
		props = append(props,
			prop{q + "status", "status", opt, str, false, model.TestStatuses},
			prop{q + "priority", "priority", opt, str, false, model.Priorities},
			prop{q + "executionType", "executionType", opt, str, false, []string{model.ExecManual, model.ExecAutomated}},
			prop{q + "preconditions", "preconditions", opt, str, false, nil},
			prop{q + "expectedResult", "expectedResult", opt, str, false, nil},
			prop{q + "estimatedMinutes", "estimatedMinutes", opt, integer, false, nil},
			prop{oslc.NSQM + "validatesRequirement", "validatesRequirement", many, res, false, nil},
			prop{oslc.NSQM + "testsChangeRequest", "testsChangeRequest", many, res, false, nil},
			prop{oslc.NSQM + "relatedChangeRequest", "relatedChangeRequest", many, res, false, nil})
	}
	props = append(props, prop{oslc.NSDCTerms + "references", "references", many, res, false, nil})
	for i, pp := range props {
		b := oslc.Blank("p" + strconv.Itoa(i))
		g.Add(sh, oslc.NSOSLC+"property", b)
		g.Type(b, oslc.NSOSLC+"Property")
		g.Add(b, oslc.NSOSLC+"name", oslc.Lit(pp.name))
		g.Add(b, oslc.NSOSLC+"propertyDefinition", oslc.IRI(pp.def))
		g.Add(b, oslc.NSOSLC+"occurs", oslc.IRI(pp.occurs))
		g.Add(b, oslc.NSOSLC+"valueType", oslc.IRI(pp.value))
		g.Add(b, oslc.NSOSLC+"readOnly", oslc.Typed(strconv.FormatBool(pp.readOnly), oslc.NSXSD+"boolean"))
		if pp.value == res {
			g.Add(b, oslc.NSOSLC+"representation", oslc.IRI(oslc.NSOSLC+"Reference"))
		}
		for _, v := range pp.allowed {
			g.Add(b, oslc.NSOSLC+"allowedValue", oslc.Lit(v))
		}
	}
	writeOSLC(w, r, g)
}

// ---------------------------------------------------------------- query

// oslcSelect is a parsed oslc.select ("*" = everything).
type oslcSelect struct {
	all   bool
	props map[string]bool
}

func parseOSLCSelect(v string, prefixes map[string]string) (*oslcSelect, error) {
	sel := &oslcSelect{props: map[string]bool{}}
	// nested selections (prop{a,b}) select the property itself
	depth := 0
	var b strings.Builder
	for _, r := range v {
		switch r {
		case '{':
			depth++
			continue
		case '}':
			depth--
			continue
		}
		if depth == 0 {
			b.WriteRune(r)
		}
	}
	for _, name := range strings.Split(b.String(), ",") {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
		case name == "*":
			sel.all = true
		default:
			pre, local, ok := strings.Cut(name, ":")
			ns, known := prefixes[pre]
			if !ok || !known {
				return nil, badRequest("oslc.select: unknown property %q (declare its prefix with oslc.prefix)", name)
			}
			sel.props[ns+local] = true
		}
	}
	return sel, nil
}

// matchesWhere evaluates the conditions on the triples of res in g: a
// property with several values matches when one of them does (!= when
// none is equal).
func matchesWhere(g *oslc.Graph, res oslc.Term, where *oslc.Query) bool {
	if where == nil {
		return true
	}
	for _, c := range where.Conditions {
		vals := g.Objects(res, c.Property)
		match := false
		if c.Op == "!=" {
			match = true
			for _, v := range vals {
				if !c.Match(termValue(v), v.Datatype == oslc.NSXSD+"dateTime") {
					match = false
				}
			}
		} else {
			for _, v := range vals {
				if c.Match(termValue(v), v.Datatype == oslc.NSXSD+"dateTime") {
					match = true
				}
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func termValue(t oslc.Term) string {
	if t.IsLiteral() {
		return t.Literal
	}
	return t.IRI
}

// searchTerms parses oslc.searchTerms: quoted strings or words.
func searchTerms(v string) []string {
	var out []string
	for v = strings.TrimSpace(v); v != ""; v = strings.TrimSpace(v) {
		if v[0] == '"' {
			end := strings.IndexByte(v[1:], '"')
			if end < 0 {
				out = append(out, strings.ToLower(v[1:]))
				break
			}
			out = append(out, strings.ToLower(v[1:end+1]))
			v = strings.TrimPrefix(v[end+2:], ",")
			continue
		}
		w, rest, _ := strings.Cut(v, " ")
		out = append(out, strings.ToLower(strings.Trim(w, ",")))
		v = rest
	}
	return out
}

func (s *Server) handleOSLCQuery(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		q := r.URL.Query()
		g, u := s.oslcGraph(r)
		prefixes, err := oslc.ParsePrefixes(q.Get("oslc.prefix"), g.Prefixes)
		if err != nil {
			s.fail(w, r, badRequest("%v", err))
			return
		}
		var where *oslc.Query
		if wv := strings.TrimSpace(q.Get("oslc.where")); wv != "" {
			if where, err = oslc.ParseWhere(wv, prefixes); err != nil {
				s.fail(w, r, badRequest("%v", err))
				return
			}
		}
		var sel *oslcSelect
		if sv := strings.TrimSpace(q.Get("oslc.select")); sv != "" {
			if sel, err = parseOSLCSelect(sv, prefixes); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		type order struct {
			prop string
			desc bool
		}
		var orderBy []order
		for _, o := range strings.Split(q.Get("oslc.orderBy"), ",") {
			if o = strings.TrimSpace(o); o == "" {
				continue
			}
			desc := strings.HasPrefix(o, "-")
			name := strings.TrimLeft(o, "+- ")
			pre, local, ok := strings.Cut(name, ":")
			ns, known := prefixes[pre]
			if !ok || !known {
				s.fail(w, r, badRequest("oslc.orderBy: unknown property %q", name))
				return
			}
			orderBy = append(orderBy, order{ns + local, desc})
		}
		terms := searchTerms(q.Get("oslc.searchTerms"))
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
		// every item as its own graph: the conditions, the order and the
		// selection work on what the resource says
		type member struct {
			res   oslc.Term
			g     *oslc.Graph
			score int
		}
		var all []member
		n := len(d.reqs)
		if kind == model.KindTest {
			n = len(d.tests)
		}
		for i := 0; i < n; i++ {
			ig, _ := s.oslcGraph(r)
			var res oslc.Term
			if kind == model.KindRequirement {
				s.addRequirement(ig, u, pr, &d.reqs[i], d)
				res = oslc.IRI(u.requirement(pr.ID, d.reqs[i].ID))
			} else {
				s.addTest(ig, u, pr, &d.tests[i], d)
				res = oslc.IRI(u.test(pr.ID, d.tests[i].ID))
			}
			if !matchesWhere(ig, res, where) {
				continue
			}
			score := 0
			if len(terms) > 0 {
				var text []string
				for _, prop := range []string{oslc.NSDCTerms + "identifier", oslc.NSDCTerms + "title", oslc.NSDCTerms + "description"} {
					for _, o := range ig.Objects(res, prop) {
						text = append(text, strings.ToLower(o.Literal))
					}
				}
				joined := strings.Join(text, "\n")
				for _, t := range terms {
					if t != "" && strings.Contains(joined, t) {
						score++
					}
				}
				if score == 0 {
					continue
				}
			}
			all = append(all, member{res, ig, score})
		}
		first := func(m member, prop string) string {
			if o := m.g.Objects(m.res, prop); len(o) > 0 {
				return termValue(o[0])
			}
			return ""
		}
		if len(orderBy) > 0 || len(terms) > 0 {
			sort.SliceStable(all, func(i, j int) bool {
				for _, o := range orderBy {
					a, b := first(all[i], o.prop), first(all[j], o.prop)
					if a == b {
						continue
					}
					if o.desc {
						return a > b
					}
					return a < b
				}
				return all[i].score > all[j].score
			})
		}
		base := u.collection(pr.ID, kind)
		qb := oslc.IRI(base)
		from, to := min(page*size, len(all)), min((page+1)*size, len(all))
		for _, m := range all[from:to] {
			g.Add(qb, oslc.NSRDFS+"member", m.res)
		}
		info := oslc.IRI(u.base + r.URL.RequestURI())
		g.Type(info, oslc.NSOSLC+"ResponseInfo")
		g.Add(info, oslc.NSDCTerms+"title", oslc.Lit(pr.Key+" "+kind+"s"))
		g.Add(info, oslc.NSOSLC+"totalCount", oslc.Typed(strconv.Itoa(len(all)), oslc.NSXSD+"integer"))
		if to < len(all) {
			next := r.URL.Query()
			next.Set("page", strconv.Itoa(page+1))
			next.Set("oslc.paging", "true")
			next.Set("oslc.pageSize", strconv.Itoa(size))
			g.Add(info, oslc.NSOSLC+"nextPage", oslc.IRI(base+"?"+next.Encode()))
		}
		for _, m := range all[from:to] {
			keep := map[string]bool{} // blank nodes of selected properties
			for _, t := range m.g.Triples {
				if t.S == m.res && t.O.BNode != "" && (sel == nil || sel.all || sel.props[t.P]) {
					keep[t.O.BNode] = true
				}
			}
			for _, t := range m.g.Triples {
				switch {
				case sel == nil || sel.all:
				case t.S == m.res:
					if t.P != oslc.NSRDF+"type" && !sel.props[t.P] {
						continue
					}
				case !keep[t.S.BNode]:
					continue
				}
				g.Triples = append(g.Triples, t)
			}
		}
		writeOSLC(w, r, g)
	}
}
