package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/interop"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqimport"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracker"
)

// Importing requirements from Jira and Azure DevOps (internal/tracker
// source.go, azure.go): a JQL or WIQL query picks the issues or work items,
// which are put into a ReqIF model (every field an attribute, every link
// a relation) and go through the ReqIF import with re-import
// (exchange_reqif.go): the same field mapping, diff preview (new, changed,
// unchanged, conflicts), external references (system "jira" or
// "azure-devops", the issue key or work item ID) and relations.
//
//   - GET/PUT /projects/{pid}/import-sources: the connections (tokens are
//     write-only); Jira falls back to the defect tracker when it is Jira.
//   - POST /projects/{pid}/import-sources/{source}/check
//   - POST /projects/{pid}/requirements/import/{source}/preview and …/apply
//     (JSON: query, or area_path and work_item_types for Azure DevOps;
//     mapping, folder_id, relations; apply also overwrite_conflicts,
//     create and exclude_refs).
//
// Reading the source happens before the transaction of apply: a tracker
// request must not hold the database (see atomic.go).

func (s *Server) routesRequirementSources(h routeFunc) {
	h("GET /api/v1/projects/{pid}/import-sources", s.handleGetImportSources, "project.read")
	h("PUT /api/v1/projects/{pid}/import-sources", s.handlePutImportSources, "project.update")
	h("POST /api/v1/projects/{pid}/import-sources/{source}/check", s.handleCheckImportSource, "project.update")
	h("POST /api/v1/projects/{pid}/requirements/import/{source}/preview", s.handleExternalImportPreview, "requirement.write")
	h("POST /api/v1/projects/{pid}/requirements/import/{source}/apply", s.handleExternalImportApply, "requirement.write")
}

// sourceNames name the systems in previews, locations and the audit log.
var sourceNames = map[string]string{tracker.SourceJira: "Jira", tracker.SourceAzure: "Azure DevOps"}

type importSourcesView struct {
	model.ImportSources
	// JiraFromTracker: the Jira import uses the defect tracker's
	// connection (no Jira URL of its own; the tracker is Jira).
	JiraFromTracker bool   `json:"jira_from_tracker"`
	TrackerJiraURL  string `json:"tracker_jira_url,omitempty"`
	TrackerJiraKey  string `json:"tracker_jira_key,omitempty"`
}

func (s *Server) importSourcesView(ctx context.Context, p *Principal, pid string, src *model.ImportSources) (*importSourcesView, error) {
	v := &importSourcesView{ImportSources: *src}
	v.Jira.Token, v.Azure.Token = "", ""
	t, err := s.Store.DefectTracker(ctx, p.TenantID(), pid)
	if err != nil {
		return nil, err
	}
	if t.Type == "jira" {
		v.TrackerJiraURL, v.TrackerJiraKey = t.URL, t.ProjectKey
		v.JiraFromTracker = src.Jira.URL == ""
	}
	return v, nil
}

func (s *Server) handleGetImportSources(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	src, err := s.Store.ImportSources(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.importSourcesView(r.Context(), p, pr.ID, src)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handlePutImportSources(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		model.ImportSources
		ClearTokens store.ClearTokens `json:"clear_tokens"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	src := req.ImportSources
	if err := s.Store.SaveImportSources(r.Context(), p.TenantID(), pr.ID, &src, req.ClearTokens); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "import_sources.update", "project", pr.ID, strings.TrimSpace("jira "+src.Jira.URL+"; azure devops "+src.Azure.URL+" "+src.Azure.Project))
	v, err := s.importSourcesView(r.Context(), p, pr.ID, &src)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// importSource is a configured source of a project.
type importSource struct {
	system string
	jira   *model.DefectTracker
	azure  *model.AzureDevOpsSource
	// defaults of the query
	jql             string
	areaPath, types string
}

func (s *Server) importSource(r *http.Request, p *Principal, projectID, system string) (*importSource, error) {
	src, err := s.Store.ImportSources(r.Context(), p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	switch system {
	case tracker.SourceJira:
		out := &importSource{system: system, jql: src.Jira.JQL}
		if src.Jira.URL != "" {
			out.jira = &model.DefectTracker{Type: "jira", URL: src.Jira.URL, User: src.Jira.User, Token: src.Jira.Token}
			return out, nil
		}
		t, err := s.Store.DefectTracker(r.Context(), p.TenantID(), projectID)
		if err != nil {
			return nil, err
		}
		if t.Type != "jira" {
			return nil, badRequest("no Jira connection: set the Jira URL in the project settings (Import sources) or use Jira as the defect tracker")
		}
		out.jira = t
		if out.jql == "" {
			out.jql = "project = " + t.ProjectKey + " ORDER BY key"
		}
		return out, nil
	case tracker.SourceAzure:
		if src.Azure.URL == "" {
			return nil, badRequest("no Azure DevOps connection: set the organization URL and project in the project settings (Import sources)")
		}
		return &importSource{system: system, azure: &src.Azure, areaPath: src.Azure.AreaPath, types: src.Azure.WorkItemTypes}, nil
	}
	return nil, badRequest("unknown import source %q (want jira or azure-devops)", system)
}

func (s *Server) handleCheckImportSource(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	src, err := s.importSource(r, p, pr.ID, r.PathValue("source"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if src.azure != nil {
		err = tracker.NewAzure(src.azure, s.HTTPClient).Check(r.Context())
	} else {
		_, err = tracker.SearchJira(r.Context(), src.jira, s.HTTPClient, firstNonBlank(src.jql, "order by key"), 1)
	}
	if err != nil {
		s.trackerFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// externalImportReq is the body of a preview or apply.
type externalImportReq struct {
	Query         string            `json:"query"`
	AreaPath      string            `json:"area_path"`
	WorkItemTypes []string          `json:"work_item_types"`
	Mapping       map[string]string `json:"mapping"`
	FolderID      string            `json:"folder_id"`
	Relations     *bool             `json:"relations"`
	// Limit bounds the items read (default and at most tracker.MaxSourceItems).
	Limit int `json:"limit"`
	// apply
	OverwriteConflicts bool     `json:"overwrite_conflicts"`
	Create             *bool    `json:"create"`
	ExcludeRefs        []string `json:"exclude_refs"`
}

type externalPreview struct {
	reqifPreview
	Source    string `json:"source"`
	Query     string `json:"query"`
	Truncated bool   `json:"truncated"`
}

// fetchSource runs the query of a request.
func (s *Server) fetchSource(r *http.Request, p *Principal, pr *model.Project, system string, req *externalImportReq) (*tracker.SourceResult, string, error) {
	src, err := s.importSource(r, p, pr.ID, system)
	if err != nil {
		return nil, "", err
	}
	q := strings.TrimSpace(req.Query)
	if src.azure != nil {
		if q == "" {
			area, types := strings.TrimSpace(req.AreaPath), req.WorkItemTypes
			if area == "" && types == nil {
				area, types = src.areaPath, strings.Split(src.types, ",")
			}
			q = tracker.AzureWIQL(area, types)
		}
		res, err := tracker.NewAzure(src.azure, s.HTTPClient).Search(r.Context(), q, req.Limit)
		return res, q, err
	}
	if q == "" {
		q = src.jql
	}
	if q == "" {
		return nil, "", badRequest("query is required: a JQL query such as project = SHOP AND issuetype = Story")
	}
	res, err := tracker.SearchJira(r.Context(), src.jira, s.HTTPClient, q, req.Limit)
	return res, q, err
}

var criteriaFieldRe = regexp.MustCompile(`(?i)^\s*(acceptance\s*criteria|acceptanskriterier|definition of done)\s*$`)

// bulletRe matches a list marker at the start of a line.
var bulletRe = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)

// sourceModel puts a query result into a ReqIF model: attribute names are
// the fields' names (made unique), objects the items. It returns the
// default mapping by attribute name.
func sourceModel(res *tracker.SourceResult, query string, relations bool) (*reqimport.ReqIFModel, map[string]string, map[string]string) {
	name := sourceNames[res.System]
	m := &reqimport.ReqIFModel{Title: query, ToolID: res.Base, SourceTool: name, Attributes: []reqimport.ReqIFAttribute{}}
	attr := map[string]string{} // field id → attribute name
	used := map[string]bool{}
	for _, f := range res.Fields {
		n := strings.TrimSpace(f.Name)
		if n == "" || used[strings.ToLower(n)] {
			n = strings.TrimSpace(f.Name + " (" + f.ID + ")")
		}
		used[strings.ToLower(n)] = true
		attr[f.ID] = n
	}
	defaults := map[string]string{}
	for id, field := range tracker.DefaultSourceMapping(res.System) {
		if n, ok := attr[id]; ok {
			defaults[n] = field
		}
	}
	for _, f := range res.Fields {
		if criteriaFieldRe.MatchString(f.Name) && !hasValue(defaults, "criteria") {
			defaults[attr[f.ID]] = "criteria"
		}
	}
	for _, f := range res.Fields {
		a := reqimport.ReqIFAttribute{Name: attr[f.ID], Type: f.Type, ObjectTypes: []string{}}
		for _, it := range res.Items {
			if v, ok := it.Values[f.ID]; ok {
				if a.Objects == 0 {
					a.Sample = v
				}
				a.Objects++
			}
		}
		m.Attributes = append(m.Attributes, a)
	}
	keys := map[string]bool{}
	for i, it := range res.Items {
		o := reqimport.ReqIFObject{Identifier: it.Key, LongName: it.Key, Order: i, Values: map[string]string{}}
		for id, v := range it.Values {
			o.Values[attr[id]] = v
		}
		m.Objects = append(m.Objects, o)
		keys[it.Key] = true
	}
	names := map[string]string{} // relation ref → the source's link name
	if relations {
		seen := map[string]bool{}
		types := map[string]bool{}
		for _, it := range res.Items {
			for _, l := range it.Links {
				if seen[l.Ref] || !keys[l.Source] || !keys[l.Target] || l.Source == l.Target {
					continue
				}
				seen[l.Ref] = true
				typ := l.Type
				if typ == "" {
					typ = interop.RelationType(l.Name)
				}
				names[l.Ref] = l.Name
				types[typ] = true
				m.Relations = append(m.Relations, reqimport.ReqIFRelation{Identifier: l.Ref, Type: typ, Source: l.Source, Target: l.Target})
			}
		}
		for t := range types {
			m.RelationTypes = append(m.RelationTypes, t)
		}
		sort.Strings(m.RelationTypes)
	}
	return m, defaults, names
}

func hasValue(m map[string]string, v string) bool {
	for _, x := range m {
		if x == v {
			return true
		}
	}
	return false
}

// sourceStatus maps a status or state of Jira or Azure DevOps to a
// requirement status: by name (reqimport.ParseStatus and the usual
// workflow states), else by the Jira status category.
func sourceStatus(raw, category string) (string, bool) {
	if v, ok := reqimport.ParseStatus(raw); ok && v != "" {
		return v, true
	}
	switch strings.ToLower(strings.Join(strings.Fields(raw), " ")) {
	case "to do", "backlog", "proposed", "funnel":
		return "draft", true
	case "active", "approved", "committed", "in progress", "doing", "selected for development", "ready":
		return "approved", true
	case "resolved", "closed", "completed", "complete":
		return "implemented", true
	case "removed", "cut":
		return "obsolete", true
	}
	switch category {
	case "new":
		return "draft", true
	case "indeterminate":
		return "approved", true
	case "done":
		return "implemented", true
	}
	return "", false
}

// adjustSourcePlan finishes the rows of a source: locations, the source
// field and folder of new requirements, values that fit the target
// fields, and the link names of the relations.
func adjustSourcePlan(res *tracker.SourceResult, defaults, linkNames map[string]string, folderID string) func(d *exchangeData, plan *interop.ReqIFPlan) {
	name := sourceNames[res.System]
	items := map[string]*tracker.SourceItem{}
	for i := range res.Items {
		items[res.Items[i].Key] = &res.Items[i]
	}
	return func(d *exchangeData, plan *interop.ReqIFPlan) {
		for i := range plan.Attributes {
			if g, ok := defaults[plan.Attributes[i].Name]; ok {
				plan.Attributes[i].Guess = g
			}
		}
		root := d.paths[folderID]
		for i := range plan.Rows {
			in := &plan.Rows[i]
			it := items[in.Ref]
			in.Location = name + " " + in.Ref
			for key, c := range in.Cells {
				f := interop.FieldByKey(d.fields, key)
				if f == nil {
					continue
				}
				switch {
				case key == "criteria":
					var ls []string
					for _, l := range strings.Split(c.Raw, "\n") {
						if l = strings.TrimSpace(bulletRe.ReplaceAllString(l, "")); l != "" {
							ls = append(ls, l)
						}
					}
					c.Raw = strings.Join(ls, "\n")
					in.Cells[key] = c
				case f.Parse != nil:
					if _, err := f.Parse(c.Raw); err == nil {
						continue
					}
					if key == "status" && it != nil {
						if v, ok := sourceStatus(c.Raw, it.Category); ok {
							c.Raw = v
							in.Cells[key] = c
							continue
						}
					}
					delete(in.Cells, key)
					in.Warnings = append(in.Warnings, fmt.Sprintf("%s %q has no matching value in Qtestframe and is left out", f.Label, c.Raw))
				}
			}
			if in.ID == "" {
				in.Cells["folder"] = interop.Cell{Raw: root, Ref: "target folder"}
				if strings.TrimSpace(in.Cells["source"].Raw) == "" {
					in.Cells["source"] = interop.Cell{Raw: name + " " + in.Ref, Ref: "key"}
				}
			}
		}
		for i := range plan.Relations {
			if n := linkNames[plan.Relations[i].Ref]; n != "" {
				plan.Relations[i].FileType = n
			}
		}
		plan.RelTypes = map[string]string{}
		for _, rel := range plan.Relations {
			plan.RelTypes[rel.FileType] = rel.Type
		}
	}
}

// planSource plans the import of a query result.
func (s *Server) planSource(r *http.Request, p *Principal, pr *model.Project, res *tracker.SourceResult, query string, req *externalImportReq) (*reqifUpload, *externalPreview, error) {
	relations := req.Relations == nil || *req.Relations
	m, defaults, linkNames := sourceModel(res, query, relations)
	mapping := map[string]string{}
	for k, v := range defaults {
		mapping[k] = v
	}
	for k, v := range req.Mapping {
		mapping[k] = v
	}
	name := sourceNames[res.System]
	defs, err := s.Store.ListCustomFields(r.Context(), p.TenantID(), pr.ID, model.KindRequirement)
	if err != nil {
		return nil, nil, err
	}
	u, err := s.planReqIF(r, p, pr, name+": "+query, m, reqifPlanOpts{system: res.System, format: res.System, origin: name, mapping: mapping,
		folderID: strings.TrimSpace(req.FolderID), noRelations: !relations, moreFields: customInteropFields(defs),
		adjust: adjustSourcePlan(res, defaults, linkNames, strings.TrimSpace(req.FolderID))})
	if err != nil {
		return nil, nil, err
	}
	pv := &externalPreview{reqifPreview: *u.preview, Source: res.System, Query: query, Truncated: res.Truncated}
	pv.Tool = name + " " + res.Base
	// the roles of ReqIF documents do not apply
	pv.Targets = slicesFilter(pv.Targets, func(t reqifTarget) bool { return t.Type != "role" })
	if res.Truncated {
		pv.Warnings = append(pv.Warnings, fmt.Sprintf("the query finds more than %d items: only the first %d are imported; narrow the query", len(res.Items), len(res.Items)))
	}
	if len(pv.Relations.Changes) > 500 {
		pv.Relations.Changes = pv.Relations.Changes[:500]
	}
	return u, pv, nil
}

// customInteropFields are the requirement custom fields as fields of the
// import ("cf.<key>"): values are read with customfield.Parse and
// compared as text; user fields are left out (their values must be
// members, which a tracker's user names are not).
func customInteropFields(defs []model.CustomField) []interop.Field {
	var out []interop.Field
	for i := range defs {
		f := defs[i]
		if f.Type == model.FieldUser {
			continue
		}
		typ := interop.Text
		if f.Type == model.FieldLongText {
			typ = interop.LongText
		}
		out = append(out, interop.Field{Key: customFieldPrefix[0] + f.Key, Label: f.Label, Type: typ,
			Get: func(it *interop.Item) string {
				if it.Req == nil {
					return ""
				}
				return customfield.Plain(it.Req.CustomFields[f.Key])
			},
			Parse: func(raw string) (string, error) {
				v, err := customfield.Parse(&f, raw)
				if err != nil {
					return "", fmt.Errorf("%s", strings.TrimPrefix(err.Error(), customfield.ErrInvalid.Error()+": "))
				}
				return customfield.Plain(v), nil
			},
			Put: func(b map[string]any, raw string, _ *interop.Item) error {
				v, err := customfield.Parse(&f, raw)
				if err != nil {
					return err
				}
				cf, _ := b["custom_fields"].(map[string]any)
				if cf == nil {
					cf = map[string]any{}
					b["custom_fields"] = cf
				}
				cf[f.Key] = v
				return nil
			}})
	}
	return out
}

func slicesFilter[T any](in []T, keep func(T) bool) []T {
	out := make([]T, 0, len(in))
	for _, x := range in {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func decodeExternalImport(r *http.Request) (*externalImportReq, error) {
	var req externalImportReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if len(req.Query) > 20000 {
		return nil, badRequest("the query may have at most 20000 characters")
	}
	if req.Limit < 0 || req.Limit > tracker.MaxSourceItems {
		return nil, badRequest("limit must be between 1 and %d", tracker.MaxSourceItems)
	}
	return &req, nil
}

func (s *Server) handleExternalImportPreview(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	req, err := decodeExternalImport(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res, q, err := s.fetchSource(r, p, pr, r.PathValue("source"), req)
	if err != nil {
		s.trackerFail(w, r, err)
		return
	}
	_, pv, err := s.planSource(r, p, pr, res, q, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

func (s *Server) handleExternalImportApply(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	req, err := decodeExternalImport(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// the source is read before the transaction
	res, q, err := s.fetchSource(r, p, pr, r.PathValue("source"), req)
	if err != nil {
		s.trackerFail(w, r, err)
		return
	}
	s.atomic(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		u, _, err := s.planSource(r, p, pr, res, q, req)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		opt := applyOptions{overwrite: req.OverwriteConflicts, create: req.Create == nil || *req.Create, exclude: map[int]bool{}}
		skip := map[string]bool{}
		for _, ref := range req.ExcludeRefs {
			skip[ref] = true
		}
		for i, row := range u.preview.Rows {
			if skip[row.Ref] {
				opt.exclude[i] = true
			}
		}
		s.applyReqIF(w, r, p, pr, u, opt)
	})(w, r, p)
}
