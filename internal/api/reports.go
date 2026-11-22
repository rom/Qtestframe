package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// routesReports registers the report endpoints (see docs/api.md "Reports").
func (s *Server) routesReports(h routeFunc) {
	h("GET /api/v1/reports/templates", s.handleReportTemplates)
	h("GET /api/v1/projects/{pid}/reports/fields", s.handleReportFields, "export")
	h("POST /api/v1/projects/{pid}/reports/generate", s.handleGenerateReport, "export")
	h("GET /api/v1/projects/{pid}/reports/definitions", s.handleListReportDefinitions, "export")
	h("POST /api/v1/projects/{pid}/reports/definitions", s.handleCreateReportDefinition)
	h("GET /api/v1/reports/definitions/{id}", s.handleGetReportDefinition, "export")
	h("PATCH /api/v1/reports/definitions/{id}", s.handleUpdateReportDefinition)
	h("DELETE /api/v1/reports/definitions/{id}", s.handleDeleteReportDefinition)
	h("POST /api/v1/projects/{pid}/reports/batch", s.handleReportBatch, "export")
	h("DELETE /api/v1/projects/{pid}/reports/definitions/bulk", s.atomic(s.handleBulkDeleteReportDefinitions))
	h("POST /api/v1/reports/definitions/{id}/generate", s.handleGenerateReportDefinition, "export")
}

// reportLang is the language a report requested by r is produced in: the
// explicit language of the request (the body's "lang", else the "lang"
// query parameter) when it is supported, else the user's preferred
// language, else the browser's (Accept-Language), else English.
func reportLang(r *http.Request, p *Principal, explicit string) string {
	pref := ""
	if p != nil && p.User != nil {
		pref = p.User.Preferences.Language
	}
	return report.ResolveLang(explicit, r.URL.Query().Get("lang"), pref, report.LangFromAcceptLanguage(r.Header.Get("Accept-Language")))
}

// reportError turns report.ErrInvalid into a 400.
func reportError(err error) error {
	if errors.Is(err, report.ErrInvalid) {
		return fmt.Errorf("%w: %s", errBadRequest, strings.TrimPrefix(err.Error(), report.ErrInvalid.Error()+": "))
	}
	return err
}

// canWriteReports: saved reports are managed by requirement or test authors.
func canWriteReports(w http.ResponseWriter, p *Principal) bool {
	if p.Can(rbac.RequirementWr) || p.Can(rbac.TestWrite) {
		return true
	}
	writeError(w, http.StatusForbidden, "forbidden", "permission requirement.write or test.write required")
	return false
}

func (s *Server) handleReportTemplates(w http.ResponseWriter, r *http.Request, p *Principal) {
	writeJSON(w, http.StatusOK, report.Templates())
}

func (s *Server) handleReportFields(w http.ResponseWriter, r *http.Request, p *Principal) {
	if _, ok := s.project(w, r, p); !ok {
		return
	}
	entity := r.URL.Query().Get("entity")
	fs := report.Fields(entity)
	if fs == nil {
		s.fail(w, r, badRequest("entity must be one of %s", strings.Join(report.Entities, ", ")))
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

// ---------------------------------------------------------------- data

var entitySchemas = map[string]*query.Schema{
	report.EntityRequirement: requirementSchema,
	report.EntityTest:        testSchema,
	report.EntityRun:         runSchema,
	report.EntityExecution:   executionSchema,
}

// projectRun returns run id of project pr (400 otherwise).
func (s *Server) projectRun(r *http.Request, p *Principal, pr *model.Project, id string) (*model.TestRun, error) {
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && run.ProjectID != pr.ID) {
		return nil, badRequest("run %q does not belong to project %s", id, pr.Key)
	}
	return run, err
}

// validateCustom checks a custom configuration against the project.
func (s *Server) validateCustom(r *http.Request, p *Principal, pr *model.Project, c *report.CustomConfig) error {
	if err := c.Validate(); err != nil {
		return reportError(err)
	}
	if _, err := query.Parse(c.Query, entitySchemas[c.Entity]); err != nil {
		return err
	}
	if c.RunID != "" {
		if _, err := s.projectRun(r, p, pr, c.RunID); err != nil {
			return err
		}
	}
	return nil
}

// reportData loads what template id needs.
func (s *Server) reportData(r *http.Request, p *Principal, pr *model.Project, id string, params map[string]string) (*report.Data, error) {
	if _, ok := report.Lookup(id); !ok {
		return nil, badRequest("unknown report template %q", id)
	}
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	expr := strings.TrimSpace(params["query"])
	d := &report.Data{Project: *pr, Links: pc.links, Categories: pc.categories, Executions: map[string][]model.Execution{}, Now: time.Now().UTC()}
	reqQ, testQ := "", ""
	switch id {
	case report.RequirementsCoverage, report.Verification:
		reqQ = expr
	case report.Traceability:
		reqQ, testQ = reqExpr(expr), testExpr(expr)
	case report.TestInventory:
		testQ = expr
	}
	if reqQ != "" || testQ != "" {
		d.Query = expr
	}
	if id != report.RunSummary && id != report.Defects && id != report.TestInventory {
		if d.Requirements, err = s.filterRequirements(r, p, pr, pc, reqQ, "", true); err != nil {
			return nil, err
		}
	}
	if id != report.RunSummary && id != report.Defects {
		if d.Tests, err = s.filterTests(r, p, pr, pc, testQ, "", true); err != nil {
			return nil, err
		}
	}
	if id == report.Traceability {
		in := map[string]bool{}
		for _, rq := range d.Requirements {
			in[model.KindRequirement+":"+rq.ID] = true
		}
		for _, t := range d.Tests {
			in[model.KindTest+":"+t.ID] = true
		}
		if d.TraceRules, err = s.evaluateTraceRules(ctx, p.TenantID(), pr, false, func(k, x string) bool { return in[k+":"+x] }); err != nil {
			return nil, err
		}
	}
	switch id {
	case report.RunSummary:
		run, err := s.projectRun(r, p, pr, strings.TrimSpace(params["run_id"]))
		if err != nil {
			return nil, err
		}
		d.Runs = []model.TestRun{*run}
	default:
		if d.Runs, err = s.Store.ListRuns(ctx, p.TenantID(), pr.ID); err != nil {
			return nil, err
		}
	}
	if id == report.Risks {
		if d.Risks, err = s.Store.ListRisks(ctx, p.TenantID(), pr.ID); err != nil {
			return nil, err
		}
	}
	if id == report.Verification {
		if d.Verification, err = s.verificationData(ctx, p, pc, d.Requirements); err != nil {
			return nil, err
		}
	}
	if id == report.RunSummary || id == report.Defects {
		for _, run := range d.Runs {
			if d.Executions[run.ID], err = s.Store.ListExecutions(ctx, p.TenantID(), run.ID); err != nil {
				return nil, err
			}
		}
	}
	return d, nil
}

// customRecords returns the records of a custom report, filtered by its
// query, with report-friendly values for multi-part fields (in lang).
func (s *Server) customRecords(r *http.Request, p *Principal, pr *model.Project, c *report.CustomConfig, lang string) ([]query.MapRecord, error) {
	ctx := r.Context()
	var out []query.MapRecord
	switch c.Entity {
	case report.EntityRequirement, report.EntityTest:
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			return nil, err
		}
		if c.Entity == report.EntityRequirement {
			reqs, err := s.filterRequirements(r, p, pr, pc, c.Query, "", true)
			if err != nil {
				return nil, err
			}
			for i := range reqs {
				rec := pc.requirementRecord(&reqs[i])
				var crit []string
				for j, cr := range reqs[i].Criteria {
					method := cr.Method
					if lang != report.English {
						method = report.EnumLabel(lang, report.EnumEvalMethod, cr.Method)
					}
					line := fmt.Sprintf("%d. %s (%s", j+1, cr.Description, method)
					if cr.Threshold != "" {
						line += "; " + cr.Threshold
					}
					crit = append(crit, line+")")
				}
				rec["criteria"] = crit
				out = append(out, rec)
			}
			return out, nil
		}
		tests, err := s.filterTests(r, p, pr, pc, c.Query, "", true)
		if err != nil {
			return nil, err
		}
		for i := range tests {
			rec := pc.testRecord(&tests[i])
			var steps []string
			for j, st := range tests[i].Steps {
				line := fmt.Sprintf("%d. %s", j+1, st.Action)
				if st.CallTestKey != "" {
					line = fmt.Sprintf("%d. [%s %s]", j+1, report.T(lang, "call"), st.CallTestKey)
				}
				if st.Expected != "" {
					line += " => " + st.Expected
				}
				steps = append(steps, line)
			}
			rec["step"] = steps
			out = append(out, rec)
		}
		return out, nil
	}
	q, err := query.Parse(c.Query, entitySchemas[c.Entity])
	if err != nil {
		return nil, err
	}
	var runs []model.TestRun
	if c.Entity == report.EntityExecution && c.RunID != "" {
		run, err := s.projectRun(r, p, pr, c.RunID)
		if err != nil {
			return nil, err
		}
		runs = []model.TestRun{*run}
	} else if runs, err = s.Store.ListRuns(ctx, p.TenantID(), pr.ID); err != nil {
		return nil, err
	}
	for i := range runs {
		run := &runs[i]
		if c.Entity == report.EntityRun {
			rec := runRecord(run)
			if !q.Match(rec) {
				continue
			}
			rec["started_at"], rec["completed_at"] = optTime(run.StartedAt), optTime(run.CompletedAt)
			for _, k := range []string{"total", model.ResultPassed, model.ResultFailed, model.ResultBlocked, model.ResultSkipped, model.ResultNotRun} {
				rec[k] = []string{strconv.Itoa(run.Summary[k])}
			}
			rec["pass_rate"] = []string{passRate(run.Summary)}
			out = append(out, rec)
			continue
		}
		es, err := s.Store.ListExecutions(ctx, p.TenantID(), run.ID)
		if err != nil {
			return nil, err
		}
		for j := range es {
			e := &es[j]
			rec := executionRecord(e)
			if !q.Match(rec) {
				continue
			}
			rec["run"] = []string{run.Name}
			rec["started_at"], rec["finished_at"] = optTime(e.StartedAt), optTime(e.FinishedAt)
			rec["duration_ms"] = []string{strconv.FormatInt(e.DurationMS, 10)}
			out = append(out, rec)
		}
	}
	return out, nil
}

func optTime(t *time.Time) []string {
	if t == nil {
		return nil
	}
	return []string{ts(*t)}
}

// passRate is passed / executions with a final result.
func passRate(sum map[string]int) string {
	done := sum["total"] - sum[model.ResultNotRun] - sum[model.ResultInProgress]
	if done <= 0 {
		return ""
	}
	return strconv.FormatFloat(float64(sum[model.ResultPassed])*100/float64(done), 'f', 1, 64) + "%"
}

// ---------------------------------------------------------------- generate

// reportJob is one report to render.
type reportJob struct {
	template string
	params   map[string]string
	custom   *report.CustomConfig
	format   string
	archive  bool
	name     string
	stem     string // default file name stem
	lang     string // language of the report's texts (report.Languages)
}

// renderedReport is a rendered report file.
type renderedReport struct {
	name, contentType, title, details, lang string
	data                                    []byte
}

// render builds a report into memory.
func (s *Server) render(r *http.Request, p *Principal, pr *model.Project, job reportJob) (*renderedReport, error) {
	f, err := export.ParseFormat(job.format)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	var ds *export.Dataset
	if job.custom != nil {
		if err := s.validateCustom(r, p, pr, job.custom); err != nil {
			return nil, err
		}
		recs, err := s.customRecords(r, p, pr, job.custom, job.lang)
		if err != nil {
			return nil, err
		}
		ds = report.BuildCustom(*job.custom, recs, pr.Key+" – "+pr.Name, time.Now().UTC(), job.lang)
	} else {
		d, err := s.reportData(r, p, pr, job.template, job.params)
		if err != nil {
			return nil, err
		}
		d.Lang = job.lang
		if ds, err = report.Build(job.template, d, job.params); err != nil {
			return nil, reportError(err)
		}
	}
	ds.Link = s.itemLink(pr)
	multi := len(ds.Sections) > 1
	ext := f.Extension(multi)
	name := strings.TrimSpace(job.name)
	if name == "" {
		name = fmt.Sprintf("%s-%s.%s", job.stem, time.Now().UTC().Format("20060102-150405"), ext)
	} else if !strings.HasSuffix(strings.ToLower(name), "."+ext) {
		name += "." + ext
	}
	var buf bytes.Buffer
	if err := export.Write(&buf, f, *ds); err != nil {
		return nil, err
	}
	return &renderedReport{name: store.CleanFileName(name), contentType: f.ContentTypeFor(multi), title: ds.Title, details: job.stem + " " + string(f), lang: ds.Lang, data: buf.Bytes()}, nil
}

// archiveReport saves a rendered report in the project's document archive.
func (s *Server) archiveReport(r *http.Request, p *Principal, pr *model.Project, rep *renderedReport) (*model.FileInfo, error) {
	fi := &model.FileInfo{ProjectID: &pr.ID, Kind: model.FileReport, Name: rep.name, ContentType: rep.contentType,
		Description: rep.title, Tags: []string{"report"}, UploadedBy: p.User.Email}
	if err := s.Store.CreateFile(r.Context(), p.TenantID(), fi, rep.data); err != nil {
		return nil, err
	}
	s.audit(r.Context(), p, "report.archive", "file", fi.ID, rep.details+" "+fi.Name)
	return fi, nil
}

func (s *Server) generate(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project, job reportJob) {
	rep, err := s.render(r, p, pr, job)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !job.archive {
		w.Header().Set("Content-Type", rep.contentType)
		w.Header().Set("Content-Disposition", contentDisposition(rep.name))
		_, _ = w.Write(rep.data)
		s.audit(r.Context(), p, "report.generate", "project", pr.ID, rep.details)
		return
	}
	fi, err := s.archiveReport(r, p, pr, rep)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, fi)
}

// itemLink maps the key of a requirement, test case or defect of pr to its
// /go/{key} link (Markdown documents link the keys). It is nil when
// server.base_url is not configured.
func (s *Server) itemLink(pr *model.Project) func(string) string {
	base := s.baseURL()
	if base == "" {
		return nil
	}
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(pr.Key) + `-(?:REQ|TC|BUG)-\d+$`)
	return func(v string) string {
		if !re.MatchString(v) {
			return ""
		}
		return base + "/go/" + url.PathEscape(v)
	}
}

// contentDisposition builds an attachment header with an ASCII fallback
// and the UTF-8 name (RFC 6266).
func contentDisposition(name string) string {
	ascii := []rune(name)
	plain := true
	for i, c := range ascii {
		if c < 0x20 || c > 0x7e || c == '"' || c == '\\' {
			ascii[i] = '_'
			plain = false
		}
	}
	h := `attachment; filename="` + string(ascii) + `"`
	if !plain {
		h += "; filename*=UTF-8''" + url.PathEscape(name)
	}
	return h
}

type generateReq struct {
	Template string            `json:"template"`
	Params   map[string]string `json:"params"`
	Custom   json.RawMessage   `json:"custom"`
	Format   string            `json:"format"`
	Archive  bool              `json:"archive"`
	Name     string            `json:"name"`
	Lang     string            `json:"lang"`
}

func (s *Server) handleGenerateReport(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req generateReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	hasCustom := len(req.Custom) > 0 && string(req.Custom) != "null"
	req.Template = strings.TrimSpace(req.Template)
	if (req.Template == "") == !hasCustom {
		s.fail(w, r, badRequest("exactly one of template or custom is required"))
		return
	}
	job := reportJob{template: req.Template, params: req.Params, format: req.Format, archive: req.Archive, name: req.Name, stem: req.Template,
		lang: reportLang(r, p, req.Lang)}
	if hasCustom {
		c, err := report.ParseCustomConfig(req.Custom)
		if err != nil {
			s.fail(w, r, reportError(err))
			return
		}
		job.custom, job.stem = c, "custom_report"
	}
	if job.params == nil {
		job.params = map[string]string{}
	}
	s.generate(w, r, p, pr, job)
}

// ---------------------------------------------------------------- definitions

type definitionReq struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Config      json.RawMessage `json:"config"`
}

// definitionConfig validates and normalises a definition's config.
func (s *Server) definitionConfig(r *http.Request, p *Principal, pr *model.Project, raw json.RawMessage) (json.RawMessage, error) {
	c, err := report.ParseCustomConfig(raw)
	if err != nil {
		return nil, reportError(err)
	}
	if err := s.validateCustom(r, p, pr, c); err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

func (s *Server) handleListReportDefinitions(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ds, err := s.Store.ListReportDefinitions(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(ds))
}

func (s *Server) handleCreateReportDefinition(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !canWriteReports(w, p) {
		return
	}
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req definitionReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	cfg, err := s.definitionConfig(r, p, pr, req.Config)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := &model.ReportDefinition{ProjectID: pr.ID, Config: cfg, CreatedBy: p.User.Email}
	if req.Name != nil {
		d.Name = *req.Name
	}
	if req.Description != nil {
		d.Description = strings.TrimSpace(*req.Description)
	}
	if err := s.Store.CreateReportDefinition(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "report_definition.create", "report_definition", d.ID, d.Name)
	writeJSON(w, http.StatusCreated, d)
}

// definition loads {id} and its project.
func (s *Server) definition(w http.ResponseWriter, r *http.Request, p *Principal) (*model.ReportDefinition, *model.Project, bool) {
	d, err := s.Store.GetReportDefinition(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), d.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	return d, pr, true
}

func (s *Server) handleGetReportDefinition(w http.ResponseWriter, r *http.Request, p *Principal) {
	if d, _, ok := s.definition(w, r, p); ok {
		writeJSON(w, http.StatusOK, d)
	}
}

func (s *Server) handleUpdateReportDefinition(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !canWriteReports(w, p) {
		return
	}
	d, pr, ok := s.definition(w, r, p)
	if !ok {
		return
	}
	var req definitionReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		d.Name = *req.Name
	}
	if req.Description != nil {
		d.Description = strings.TrimSpace(*req.Description)
	}
	if len(req.Config) > 0 {
		cfg, err := s.definitionConfig(r, p, pr, req.Config)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		d.Config = cfg
	}
	if err := s.Store.UpdateReportDefinition(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "report_definition.update", "report_definition", d.ID, d.Name)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteReportDefinition(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !canWriteReports(w, p) {
		return
	}
	d, _, ok := s.definition(w, r, p)
	if !ok {
		return
	}
	if err := s.Store.DeleteReportDefinition(r.Context(), p.TenantID(), d.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "report_definition.delete", "report_definition", d.ID, d.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGenerateReportDefinition(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, pr, ok := s.definition(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Format  string `json:"format"`
		Archive bool   `json:"archive"`
		Name    string `json:"name"`
		Lang    string `json:"lang"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	c, err := report.ParseCustomConfig(d.Config)
	if err != nil {
		s.fail(w, r, reportError(err))
		return
	}
	if c.Title == "" {
		c.Title = d.Name
	}
	s.generate(w, r, p, pr, reportJob{custom: c, format: req.Format, archive: req.Archive, name: req.Name, stem: d.Name, lang: reportLang(r, p, req.Lang)})
}
