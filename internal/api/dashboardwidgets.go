package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// The widget registry and the widget data providers. A dashboard asks for
// the data of its (visible) widgets in one request, POST /dashboards/data;
// each provider reads the API's own GET endpoints as the caller (an
// in-process request through the router: the same permission checks, so
// a widget only shows what the caller may see there) and trims or
// summarises the answer. Within one data request every GET is made once
// (the dashboard and its insights feed many widgets).

// widgetType describes one widget type.
type widgetType struct {
	// Project: the widget is about one project (on the home dashboard its
	// settings name it: project_id); Home: it may be placed on the home
	// dashboard.
	Project, Home bool
	// W and H: the default size in grid units.
	W, H int
	// fetch returns the widget's data (nil: the widget loads nothing, or
	// its own data in the client).
	fetch func(c *widgetCall) (any, error)
}

// widgetTypes is the registry. The client has the matching one in
// web/src/lib/widgets.ts (titles, settings and renderers).
var widgetTypes = map[string]widgetType{
	"key_figures":         {Project: true, Home: true, W: 12, H: 2, fetch: fetchKeyFigures},
	"coverage":            {Project: true, Home: true, W: 6, H: 4, fetch: pickDashboard("coverage")},
	"execution_results":   {Project: true, Home: true, W: 6, H: 4, fetch: pickDashboard("results")},
	"active_runs":         {Project: true, Home: true, W: 6, H: 4, fetch: pickDashboard("active_runs")},
	"recent_runs":         {Project: true, Home: true, W: 6, H: 4, fetch: pickDashboard("recent_runs")},
	"breakdown":           {Project: true, Home: true, W: 6, H: 4, fetch: fetchBreakdown},
	"test_types":          {Project: true, Home: true, W: 6, H: 4, fetch: fetchTestTypes},
	"verification_status": {Project: true, Home: true, W: 6, H: 4, fetch: fetchVerification},
	"test_results_trend":  {Project: true, Home: true, W: 12, H: 5, fetch: fetchTrends("points")},
	"defects_per_week":    {Project: true, Home: true, W: 6, H: 4, fetch: fetchTrends("defect_weeks")},
	"recent_activity":     {Project: true, Home: true, W: 12, H: 5, fetch: pickInsights("activity")},
	"flaky_tests":         {Project: true, Home: true, W: 12, H: 5, fetch: pickDashboard("flaky")},
	"coverage_by_group":   {Project: true, Home: true, W: 6, H: 4, fetch: pickDashboard("coverage_by_folder", "coverage_by_category")},
	"risk_heat":           {Project: true, Home: true, W: 6, H: 4, fetch: pickInsightsRisks},
	"defect_age":          {Project: true, Home: true, W: 6, H: 4, fetch: pickInsights("defects")},
	"trace_rules":         {Project: true, Home: true, W: 6, H: 4, fetch: pickInsights("trace_rules")},
	"correlations":        {Project: true, Home: true, W: 12, H: 2, fetch: pickDashboard("correlations")},
	"release_progress":    {Project: true, Home: true, W: 6, H: 4, fetch: pickInsights("release")},
	"release_readiness":   {Project: true, Home: true, W: 6, H: 5, fetch: fetchReleaseReadiness},
	"release_burnup":      {Project: true, Home: true, W: 6, H: 4, fetch: fetchBurnup},
	"cycle_burndown":      {Project: true, Home: true, W: 6, H: 4, fetch: fetchCycleBurndown},
	"quality_metrics":     {Project: true, Home: true, W: 12, H: 9},
	"consistency":         {Project: true, Home: true, W: 12, H: 5},
	"my_work":             {Home: true, W: 12, H: 3, fetch: fetchMyWork},
	"discussions":         {Home: true, W: 6, H: 5, fetch: fetchDiscussions},
	"saved_query":         {Project: true, Home: true, W: 6, H: 4, fetch: fetchSavedQuery},
	"traceability_gaps":   {Project: true, Home: true, W: 6, H: 4, fetch: fetchTraceGaps},
	"exploratory_metrics": {Project: true, Home: true, W: 6, H: 4, fetch: fetchExploratory},
	"framework_coverage":  {Project: true, Home: true, W: 6, H: 4, fetch: fetchFrameworkCoverage},
	"note":                {Home: true, W: 6, H: 3},
}

// checkWidgets checks the widget types of a layout (home: a home dashboard).
func checkWidgets(ws []model.DashboardWidget, home bool) error {
	for i, w := range ws {
		t, ok := widgetTypes[w.Type]
		if !ok {
			return badRequest("widget %d: unknown type %q", i+1, w.Type)
		}
		if home && !t.Home {
			return badRequest("widget %d: %s cannot be placed on the home dashboard", i+1, w.Type)
		}
	}
	return nil
}

// handleListWidgetTypes: GET /dashboards/widgets lists the widget types.
func (s *Server) handleListWidgetTypes(w http.ResponseWriter, r *http.Request, p *Principal) {
	type item struct {
		Type    string `json:"type"`
		Project bool   `json:"project"`
		Home    bool   `json:"home"`
		W       int    `json:"w"`
		H       int    `json:"h"`
	}
	out := []item{}
	for k, t := range widgetTypes {
		out = append(out, item{k, t.Project, t.Home, t.W, t.H})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	writeJSON(w, http.StatusOK, listOf(out))
}

// ---------------------------------------------------------------- data

// MaxWidgetRequests bounds the widgets of one data request.
const MaxWidgetRequests = model.DashboardMaxWidgets

type widgetRequest struct {
	ID        string                     `json:"id"`
	Type      string                     `json:"type"`
	ProjectID string                     `json:"project_id"`
	Settings  map[string]json.RawMessage `json:"settings"`
}

// widgetError is the error of one widget (its data endpoint's answer).
type widgetError struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"error"`
}

func (e *widgetError) Error() string { return e.Message }

type widgetResult struct {
	Data  any          `json:"data"`
	Error *widgetError `json:"error,omitempty"`
}

// handleDashboardData: POST /dashboards/data {widgets: [{id, type,
// project_id, settings}]} → {results: {id: {data} | {error}}}.
func (s *Server) handleDashboardData(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	var req struct {
		Widgets []widgetRequest `json:"widgets"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Widgets) > MaxWidgetRequests {
		s.fail(w, r, badRequest("at most %d widgets per request", MaxWidgetRequests))
		return
	}
	b := s.newWidgetBatch(r, p)
	out := map[string]widgetResult{}
	for _, wr := range req.Widgets {
		data, err := b.fetch(wr)
		res := widgetResult{Data: data}
		if err != nil {
			res.Data = nil
			res.Error = asWidgetError(err)
		}
		out[wr.ID] = res
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

func asWidgetError(err error) *widgetError {
	if we, ok := err.(*widgetError); ok {
		return we
	}
	return &widgetError{Status: http.StatusBadRequest, Code: "bad_request", Message: err.Error()}
}

// subPrincipalKey carries the caller of a data request into the in-process
// requests of its widgets (see principal).
type subPrincipalKey struct{}

// subPrincipal returns the caller of the data request an in-process widget
// request belongs to (a fresh copy: each request scopes its own).
func subPrincipal(ctx context.Context) *Principal {
	sp, _ := ctx.Value(subPrincipalKey{}).(*Principal)
	if sp == nil {
		return nil
	}
	c := *sp
	return &c
}

// widgetBatch makes and caches the GETs of one data request.
type widgetBatch struct {
	s     *Server
	base  *http.Request
	ctx   context.Context
	cache map[string]subResult
}

type subResult struct {
	body json.RawMessage
	err  error
}

func (s *Server) newWidgetBatch(r *http.Request, p *Principal) *widgetBatch {
	// the caller as before any project scoping (each GET scopes itself)
	base := *p
	base.projectID, base.access = "", nil
	if base.orgPerms != nil {
		base.perms = base.orgPerms
	}
	return &widgetBatch{s: s, base: r, ctx: context.WithValue(r.Context(), subPrincipalKey{}, &base), cache: map[string]subResult{}}
}

// memResponse records the answer of an in-process request.
type memResponse struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (m *memResponse) Header() http.Header         { return m.header }
func (m *memResponse) WriteHeader(code int)        { m.code = code }
func (m *memResponse) Write(b []byte) (int, error) { return m.body.Write(b) }

// get makes GET /api/v1<path>?<q> as the caller (cached).
func (b *widgetBatch) get(path string, q url.Values) (json.RawMessage, error) {
	u := url.URL{Path: "/api/v1" + path, RawQuery: q.Encode()}
	key := u.String()
	if r, ok := b.cache[key]; ok {
		return r.body, r.err
	}
	req, err := http.NewRequestWithContext(b.ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	req.Host = b.base.Host
	req.RemoteAddr = b.base.RemoteAddr
	for _, h := range []string{"Accept-Language", "User-Agent", "X-Forwarded-For"} {
		if v := b.base.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	rec := &memResponse{header: http.Header{}, code: http.StatusOK}
	b.s.mux.ServeHTTP(rec, req)
	res := subResult{body: json.RawMessage(rec.body.Bytes())}
	if rec.code >= 300 {
		var e apiError
		_ = json.Unmarshal(rec.body.Bytes(), &e)
		if e.Code == "" {
			e.Code = strings.ToLower(strings.ReplaceAll(http.StatusText(rec.code), " ", "_"))
		}
		res = subResult{err: &widgetError{Status: rec.code, Code: e.Code, Message: e.Error}}
	}
	b.cache[key] = res
	return res.body, res.err
}

// fetch returns the data of one widget.
func (b *widgetBatch) fetch(wr widgetRequest) (any, error) {
	t, ok := widgetTypes[wr.Type]
	if !ok {
		return nil, &widgetError{Status: http.StatusBadRequest, Code: "bad_request", Message: "unknown widget type " + strconv.Quote(wr.Type)}
	}
	c := &widgetCall{b: b, settings: wr.Settings, pid: wr.ProjectID}
	if c.pid == "" {
		c.pid = c.str("project_id")
	}
	if strings.Trim(c.pid, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		return nil, &widgetError{Status: http.StatusBadRequest, Code: "bad_request", Message: "invalid project id"}
	}
	if t.Project && c.pid == "" {
		return nil, &widgetError{Status: http.StatusBadRequest, Code: "bad_request", Message: "the widget needs a project"}
	}
	if t.fetch == nil {
		return nil, nil
	}
	return t.fetch(c)
}

// widgetCall is one widget's request: its project and settings.
type widgetCall struct {
	b        *widgetBatch
	pid      string
	settings map[string]json.RawMessage
}

func (c *widgetCall) str(key string) string {
	var v string
	if raw, ok := c.settings[key]; ok {
		_ = json.Unmarshal(raw, &v)
	}
	return strings.TrimSpace(v)
}

// num returns an integer setting within [lo, hi] (def when unset).
func (c *widgetCall) num(key string, def, lo, hi int) int {
	var v float64
	raw, ok := c.settings[key]
	if !ok || json.Unmarshal(raw, &v) != nil {
		return def
	}
	return max(lo, min(hi, int(v)))
}

func (c *widgetCall) project(path string) string { return "/projects/" + c.pid + path }

func (c *widgetCall) get(path string, q url.Values) (json.RawMessage, error) {
	return c.b.get(path, q)
}

// object decodes a JSON object into its members.
func object(raw json.RawMessage) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &m)
	return m
}

// pick returns the named members of a JSON object.
func pick(raw json.RawMessage, keys ...string) map[string]json.RawMessage {
	all := object(raw)
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	return out
}

func (c *widgetCall) dashboard() (json.RawMessage, error) { return c.get(c.project("/dashboard"), nil) }
func (c *widgetCall) insights() (json.RawMessage, error) {
	return c.get(c.project("/dashboard/insights"), nil)
}

func pickDashboard(keys ...string) func(*widgetCall) (any, error) {
	return func(c *widgetCall) (any, error) {
		d, err := c.dashboard()
		if err != nil {
			return nil, err
		}
		return pick(d, keys...), nil
	}
}

func pickInsights(keys ...string) func(*widgetCall) (any, error) {
	return func(c *widgetCall) (any, error) {
		d, err := c.insights()
		if err != nil {
			return nil, err
		}
		return pick(d, keys...), nil
	}
}

func pickInsightsRisks(c *widgetCall) (any, error) {
	d, err := c.insights()
	if err != nil {
		return nil, err
	}
	return pick(object(d)["risks"], "open", "total", "heat"), nil
}

func fetchKeyFigures(c *widgetCall) (any, error) {
	d, err := c.dashboard()
	if err != nil {
		return nil, err
	}
	out := map[string]any{"dashboard": pick(d, "requirements", "tests", "runs", "results")}
	// the further figures when the caller may see them
	if ins, err := c.insights(); err == nil {
		out["insights"] = pick(ins, "requirements", "risks", "defects", "suspect", "trace_rules", "release")
	}
	return out, nil
}

func fetchTestTypes(c *widgetCall) (any, error) {
	d, err := c.insights()
	if err != nil {
		return nil, err
	}
	return pick(object(d)["tests"], "by_type"), nil
}

// breakdownDims maps the dimensions of the breakdown widget to where
// their counts are: source (dashboard, insights), the path of members.
var breakdownDims = map[string][]string{
	"requirement_status":     {"dashboard", "requirements", "by_status"},
	"requirement_obligation": {"dashboard", "requirements", "by_obligation"},
	"test_status":            {"dashboard", "tests", "by_status"},
	"test_execution_type":    {"dashboard", "tests", "by_execution_type"},
	"run_status":             {"dashboard", "runs", "by_status"},
	"defect_severity":        {"dashboard", "defects_by_severity"},
	"review_state":           {"insights", "requirements", "review"},
	"risk_level":             {"insights", "risks", "by_level"},
	"risk_status":            {"insights", "risks", "by_status"},
	"risk_mitigation":        {"insights", "risks", "mitigation"},
	"defect_status":          {"defects"},
}

// BreakdownDimensions are the dimensions of the breakdown widget.
func BreakdownDimensions() []string {
	out := make([]string, 0, len(breakdownDims))
	for k := range breakdownDims {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fetchBreakdown(c *widgetCall) (any, error) {
	dim := c.str("dimension")
	if dim == "" {
		dim = "requirement_status"
	}
	path, ok := breakdownDims[dim]
	if !ok {
		return nil, &widgetError{Status: http.StatusBadRequest, Code: "bad_request", Message: "unknown dimension " + strconv.Quote(dim)}
	}
	counts := map[string]int{}
	switch path[0] {
	case "defects":
		raw, err := c.get(c.project("/defects"), nil)
		if err != nil {
			return nil, err
		}
		var l struct {
			Items []struct {
				Status string `json:"status"`
			} `json:"items"`
		}
		_ = json.Unmarshal(raw, &l)
		for _, d := range l.Items {
			counts[d.Status]++
		}
	default:
		var raw json.RawMessage
		var err error
		if path[0] == "dashboard" {
			raw, err = c.dashboard()
		} else {
			raw, err = c.insights()
		}
		if err != nil {
			return nil, err
		}
		for _, k := range path[1:] {
			raw = object(raw)[k]
		}
		_ = json.Unmarshal(raw, &counts)
	}
	return map[string]any{"dimension": dim, "counts": counts}, nil
}

func fetchVerification(c *widgetCall) (any, error) {
	raw, err := c.get(c.project("/verification"), nil)
	if err != nil {
		return nil, err
	}
	var v struct {
		Strict           bool              `json:"strict"`
		Counts           map[string]int    `json:"counts"`
		CriteriaVerified int               `json:"criteria_verified"`
		CriteriaTotal    int               `json:"criteria_total"`
		Inconsistent     []json.RawMessage `json:"inconsistent"`
	}
	_ = json.Unmarshal(raw, &v)
	return map[string]any{"strict": v.Strict, "counts": v.Counts, "criteria_verified": v.CriteriaVerified, "criteria_total": v.CriteriaTotal, "inconsistent": len(v.Inconsistent)}, nil
}

func fetchTrends(key string) func(*widgetCall) (any, error) {
	return func(c *widgetCall) (any, error) {
		rng := c.num("range", 30, 1, 365)
		if !slices.Contains(TrendRanges, rng) {
			rng = 30
		}
		raw, err := c.get(c.project("/metrics/trends"), url.Values{"range": {strconv.Itoa(rng)}})
		if err != nil {
			return nil, err
		}
		out := pick(raw, "from", "to", "days", key)
		out["range"], _ = json.Marshal(rng)
		return out, nil
	}
}

// pickItem names a release or a cycle and its state.
type pickItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
}

// choose returns the item with the id of the setting key, else the first
// active, planned or any one (nil: none).
func (c *widgetCall) choose(list []pickItem, key string) *pickItem {
	if id := c.str(key); id != "" {
		if i := slices.IndexFunc(list, func(x pickItem) bool { return x.ID == id }); i >= 0 {
			return &list[i]
		}
	}
	for _, st := range []string{"active", "planned"} {
		if i := slices.IndexFunc(list, func(x pickItem) bool { return x.Status == st }); i >= 0 {
			return &list[i]
		}
	}
	if len(list) > 0 {
		return &list[0]
	}
	return nil
}

func (c *widgetCall) list(path string) ([]pickItem, error) {
	raw, err := c.get(c.project(path), nil)
	if err != nil {
		return nil, err
	}
	var l struct {
		Items []pickItem `json:"items"`
	}
	_ = json.Unmarshal(raw, &l)
	if l.Items == nil {
		l.Items = []pickItem{}
	}
	return l.Items, nil
}

func fetchReleaseReadiness(c *widgetCall) (any, error) {
	rels, err := c.list("/releases")
	if err != nil {
		return nil, err
	}
	rel := c.choose(rels, "release_id")
	out := map[string]any{"releases": rels, "release": rel, "variant": c.str("variant")}
	if rel == nil {
		return out, nil
	}
	q := url.Values{}
	if v := c.str("variant"); v != "" {
		q.Set("variant", v)
	}
	rep, err := c.get("/releases/"+url.PathEscape(rel.ID)+"/status", q)
	if err != nil {
		return nil, err
	}
	out["report"] = rep
	return out, nil
}

func fetchBurnup(c *widgetCall) (any, error) {
	rels, err := c.list("/releases")
	if err != nil {
		return nil, err
	}
	rel := c.choose(rels, "release_id")
	out := map[string]any{"releases": rels, "release": rel}
	if rel != nil {
		b, err := c.get("/releases/"+url.PathEscape(rel.ID)+"/burnup", nil)
		if err != nil {
			return nil, err
		}
		out["burnup"] = b
	}
	return out, nil
}

func fetchCycleBurndown(c *widgetCall) (any, error) {
	cycles, err := c.list("/cycles")
	if err != nil {
		return nil, err
	}
	cy := c.choose(cycles, "cycle_id")
	out := map[string]any{"cycles": cycles, "cycle": cy}
	if cy != nil {
		rep, err := c.get("/cycles/"+url.PathEscape(cy.ID)+"/progress", nil)
		if err != nil {
			return nil, err
		}
		out["report"] = pick(rep, "burndown", "cycle")
	}
	return out, nil
}

// trimItems keeps the first n of a list answer {items, ...}.
func trimItems(raw json.RawMessage, n int) map[string]json.RawMessage {
	m := object(raw)
	var items []json.RawMessage
	_ = json.Unmarshal(m["items"], &items)
	m["total"], _ = json.Marshal(len(items))
	if len(items) > n {
		items = items[:n]
	}
	if items == nil {
		items = []json.RawMessage{}
	}
	m["items"], _ = json.Marshal(items)
	return m
}

func fetchMyWork(c *widgetCall) (any, error) {
	q := url.Values{}
	if c.pid != "" {
		q.Set("project", c.pid)
	}
	raw, err := c.get("/my-work", q)
	if err != nil {
		return nil, err
	}
	out := trimItems(raw, c.num("limit", 5, 1, 50))
	delete(out, "sources")
	return out, nil
}

func fetchDiscussions(c *widgetCall) (any, error) {
	limit := c.num("limit", 8, 1, 50)
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if st := c.str("status"); st != "" && st != "all" {
		q.Set("status", st)
	} else if st == "" {
		q.Set("status", "open")
	}
	path := "/discussions"
	if c.pid != "" {
		path = c.project("/discussions")
	}
	if f := c.str("filter"); f != "" {
		q.Set("filter", f)
	}
	raw, err := c.get(path, q)
	if err != nil {
		return nil, err
	}
	return trimItems(raw, limit), nil
}

// savedQueryEntities are the lists a saved query widget can query, with
// the field shown next to the status.
var savedQueryEntities = map[string]string{
	"requirements": "coverage", "tests": "last_result", "defects": "severity", "risks": "level", "runs": "",
}

// SavedQueryEntities are the item kinds of the saved query widget.
func SavedQueryEntities() []string {
	out := make([]string, 0, len(savedQueryEntities))
	for k := range savedQueryEntities {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type queryRow struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Extra  string `json:"extra,omitempty"`
}

func fetchSavedQuery(c *widgetCall) (any, error) {
	entity := c.str("entity")
	if entity == "" {
		entity = "requirements"
	}
	extra, ok := savedQueryEntities[entity]
	if !ok {
		return nil, &widgetError{Status: http.StatusBadRequest, Code: "bad_request", Message: "unknown entity " + strconv.Quote(entity)}
	}
	q := url.Values{}
	if expr := c.str("query"); expr != "" {
		q.Set("q", expr)
	}
	raw, err := c.get(c.project("/"+entity), q)
	if err != nil {
		return nil, err
	}
	var l struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(raw, &l)
	limit := c.num("limit", 10, 1, 50)
	rows := []queryRow{}
	for i, it := range l.Items {
		if i >= limit {
			break
		}
		s := func(k string) string { v, _ := it[k].(string); return v }
		r := queryRow{ID: s("id"), Key: s("key"), Title: s("title"), Status: s("status")}
		if r.Title == "" {
			r.Title = s("name")
		}
		if extra != "" {
			r.Extra = s(extra)
		}
		rows = append(rows, r)
	}
	return map[string]any{"entity": entity, "total": len(l.Items), "items": rows}, nil
}

func fetchTraceGaps(c *widgetCall) (any, error) {
	limit := c.num("limit", 5, 1, 50)
	out := map[string]any{}
	var firstErr error
	gather := func(path, key string, gap func(map[string]any) bool) {
		raw, err := c.get(c.project(path), nil)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		var l struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal(raw, &l)
		rows := []queryRow{}
		n, total := 0, 0
		for _, it := range l.Items {
			st, _ := it["status"].(string)
			if st == "rejected" || st == "obsolete" || st == "deprecated" {
				continue
			}
			total++
			if !gap(it) {
				continue
			}
			n++
			if len(rows) < limit {
				id, _ := it["id"].(string)
				k, _ := it["key"].(string)
				t, _ := it["title"].(string)
				rows = append(rows, queryRow{ID: id, Key: k, Title: t, Status: st})
			}
		}
		out[key] = map[string]any{"total": total, "gaps": n, "items": rows}
	}
	zero := func(field string) func(map[string]any) bool {
		return func(it map[string]any) bool { v, _ := it[field].(float64); return v == 0 }
	}
	gather("/requirements", "requirements", zero("linked_tests"))
	gather("/tests", "tests", zero("linked_requirements"))
	if len(out) == 0 {
		return nil, firstErr
	}
	return out, nil
}

func fetchExploratory(c *widgetCall) (any, error) {
	raw, err := c.get(c.project("/exploratory-metrics"), nil)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// frameworkRow is one tactic or threat category of a framework coverage.
type frameworkRow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Covered int    `json:"covered"`
	Total   int    `json:"total"`
}

func fetchFrameworkCoverage(c *widgetCall) (any, error) {
	fw := c.str("framework")
	if fw == "" {
		fw = "attack"
	}
	q := url.Values{}
	if cat := c.str("catalogue"); cat != "" {
		q.Set("catalogue", cat)
	}
	rows := []frameworkRow{}
	switch fw {
	case "attack":
		raw, err := c.get("/catalog/attack", q)
		if err != nil {
			return nil, err
		}
		var v struct {
			Matrices []struct {
				Name    string `json:"name"`
				Tactics []struct {
					ID, Name   string
					Techniques []struct {
						Requirements int `json:"requirements"`
					} `json:"techniques"`
				} `json:"tactics"`
			} `json:"matrices"`
		}
		_ = json.Unmarshal(raw, &v)
		for _, m := range v.Matrices {
			for _, t := range m.Tactics {
				r := frameworkRow{ID: t.ID, Name: t.Name, Total: len(t.Techniques)}
				if len(v.Matrices) > 1 {
					r.Name = m.Name + ": " + t.Name
				}
				for _, te := range t.Techniques {
					if te.Requirements > 0 {
						r.Covered++
					}
				}
				rows = append(rows, r)
			}
		}
	case "d3fend":
		raw, err := c.get("/catalog/d3fend/coverage", q)
		if err != nil {
			return nil, err
		}
		var v struct {
			Tactics []struct {
				ID, Name    string
				Used, Total int
			} `json:"tactics"`
		}
		_ = json.Unmarshal(raw, &v)
		for _, t := range v.Tactics {
			rows = append(rows, frameworkRow{t.ID, t.Name, t.Used, t.Total})
		}
	case "emb3d":
		raw, err := c.get("/catalog/emb3d/coverage", q)
		if err != nil {
			return nil, err
		}
		var v struct {
			Categories []struct {
				ID               string
				Addressed, Total int
			} `json:"categories"`
		}
		_ = json.Unmarshal(raw, &v)
		for _, t := range v.Categories {
			rows = append(rows, frameworkRow{t.ID, t.ID, t.Addressed, t.Total})
		}
	default:
		return nil, &widgetError{Status: http.StatusBadRequest, Code: "bad_request", Message: fmt.Sprintf("unknown framework %q", fw)}
	}
	covered, total := 0, 0
	for _, r := range rows {
		covered += r.Covered
		total += r.Total
	}
	return map[string]any{"framework": fw, "rows": rows, "covered": covered, "total": total}, nil
}
