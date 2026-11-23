package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/ai"
	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
)

// The optional AI assistance (internal/ai): drafts of tests, improved
// requirements, run summaries with defect drafts, framework mappings and
// acceptance criteria. Every endpoint only returns suggestions; the client
// shows them for review and saves what the user accepts through the
// regular endpoints. No service call runs inside a database transaction.
//
// Availability: the service is configured and on (system settings, section
// ai), the feature is on, the organisation turned the assistance on
// (tenant settings ai_enabled) and the project did not opt out
// (projects.ai_disabled). Each call is rate limited per user and written to
// the audit log (action ai.request: feature, item ids, tokens; never the
// content).

func (s *Server) routesAI(h routeFunc) {
	h("GET /api/v1/ai", s.handleAIOrgStatus)
	h("GET /api/v1/projects/{pid}/ai", s.handleAIStatus, "project.read")
	h("PUT /api/v1/projects/{pid}/ai", s.handleAIProject, "project.update")
	h("POST /api/v1/projects/{pid}/ai/draft-tests", s.handleAIDraftTests, "test.write")
	h("POST /api/v1/projects/{pid}/ai/improve-requirement", s.handleAIImprove, "requirement.write")
	h("POST /api/v1/projects/{pid}/ai/summarize-run", s.handleAISummarizeRun, "run.read")
	h("POST /api/v1/projects/{pid}/ai/suggest-mappings", s.handleAIMappings, "requirement.write")
	h("POST /api/v1/projects/{pid}/ai/suggest-criteria", s.handleAICriteria, "requirement.write")
	h("POST /api/v1/admin/settings/test/ai", s.handleTestAI)
}

// Reasons the assistance is not available.
const (
	aiOff           = "off"            // not enabled in the system settings
	aiNotConfigured = "not_configured" // enabled without a usable service
	aiOrganisation  = "organisation"   // the organisation has not turned it on
	aiProject       = "project"        // the project opted out
	aiPolicy        = "policy"         // a super administrator forbade it for the organisation
	aiFeatureOff    = "feature_off"    // the feature is turned off
)

// aiFeatureOn reports whether a feature is on in c.
func aiFeatureOn(c config.AIFeatures, f string) bool {
	switch f {
	case ai.DraftTests:
		return c.DraftTests
	case ai.ImproveRequirement:
		return c.ImproveRequirement
	case ai.SummarizeRun:
		return c.SummarizeRun
	case ai.SuggestMappings:
		return c.SuggestMappings
	case ai.SuggestCriteria:
		return c.SuggestCriteria
	}
	return false
}

// aiProvider returns the provider of a configuration.
func (s *Server) aiProvider(c config.AI) ai.Provider {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	hc := &http.Client{Timeout: timeout}
	if s.HTTPClient != nil {
		cp := *s.HTTPClient
		cp.Timeout = timeout
		hc = &cp
	}
	if c.ProviderName() == config.AIOpenAI {
		return &ai.OpenAI{URL: c.URL, Key: c.APIKey, Model: c.ModelName(), HTTP: hc}
	}
	return &ai.Anthropic{URL: c.URL, Key: c.APIKey, Model: c.ModelName(), HTTP: hc}
}

// aiLimiter returns the per-user limiter with the configured limit.
func (s *Server) aiLimiter(perHour int) *rateLimiter {
	s.aiOnce.Do(func() { s.aiLimit = newRateLimiter(30, time.Hour) })
	if perHour > 0 {
		s.aiLimit.setLimits(perHour, time.Hour)
	}
	return s.aiLimit
}

// aiRemaining is how many requests the user has left this hour.
func (s *Server) aiRemaining(userID string, perHour int) int {
	l := s.aiLimiter(perHour)
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-l.window)
	n := 0
	for _, t := range l.hits["ai:"+userID] {
		if t.After(cut) {
			n++
		}
	}
	return max(0, l.max-n)
}

// aiStatus is GET /projects/{pid}/ai.
type aiStatus struct {
	// Available: the assistance can be used in the project (by someone
	// with the permissions of the features).
	Available bool `json:"available"`
	// Reason when not available: off, not_configured, policy, organisation, project.
	Reason   string          `json:"reason,omitempty"`
	Provider string          `json:"provider,omitempty"`
	Model    string          `json:"model,omitempty"`
	Features map[string]bool `json:"features"`
	// The three switches.
	SystemEnabled       bool `json:"system_enabled"`
	OrganisationEnabled bool `json:"organisation_enabled"`
	ProjectDisabled     bool `json:"project_disabled"`
	// PolicyDisabled: a super administrator forbade AI assistance for the
	// organisation (Administration → Organisations).
	PolicyDisabled bool `json:"policy_disabled"`
	// RateLimit per user and hour, Remaining for the caller.
	RateLimit int `json:"rate_limit"`
	Remaining int `json:"remaining"`
	// Prices per million tokens (0: not set), for the cost estimate.
	PriceInput  float64 `json:"price_input"`
	PriceOutput float64 `json:"price_output"`
	// PromptVersion of the embedded prompt templates.
	PromptVersion string `json:"prompt_version"`
}

// aiState computes the availability of the assistance in a project.
func (s *Server) aiState(ctx context.Context, p *Principal, projectID string) (*aiStatus, config.AI, error) {
	c := s.conf().AI
	st := &aiStatus{Features: map[string]bool{}, SystemEnabled: c.Enabled, RateLimit: c.RateLimit, PromptVersion: ai.PromptVersion}
	ts, err := s.Store.TenantSettings(ctx, p.TenantID())
	if err != nil {
		return nil, c, err
	}
	st.OrganisationEnabled = ts.AIEnabled
	pol, err := s.Store.TenantPolicy(ctx, p.TenantID())
	if err != nil {
		return nil, c, err
	}
	st.PolicyDisabled = pol.AIDisabled
	if projectID != "" {
		if st.ProjectDisabled, err = s.Store.ProjectAIDisabled(ctx, p.TenantID(), projectID); err != nil {
			return nil, c, err
		}
	}
	for _, f := range ai.Features {
		st.Features[f] = aiFeatureOn(c.Features, f)
	}
	st.PriceInput, _ = config.ParsePrice(c.PriceInput)
	st.PriceOutput, _ = config.ParsePrice(c.PriceOutput)
	switch {
	case !c.Enabled:
		st.Reason = aiOff
	case !c.On():
		st.Reason = aiNotConfigured
	case pol.AIDisabled:
		st.Reason = aiPolicy
	case !ts.AIEnabled:
		st.Reason = aiOrganisation
	case st.ProjectDisabled:
		st.Reason = aiProject
	default:
		st.Available = true
		st.Provider, st.Model = c.ProviderName(), c.ModelName()
	}
	st.Remaining = s.aiRemaining(p.User.ID, c.RateLimit)
	return st, c, nil
}

// handleAIOrgStatus: GET /ai — the availability in the organisation
// (without a project's opt-out), for the organisation settings.
func (s *Server) handleAIOrgStatus(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	st, _, err := s.aiState(r.Context(), p, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	st, _, err := s.aiState(r.Context(), p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleAIProject: PUT /projects/{pid}/ai {"disabled": bool} — the
// project's opt-out.
func (s *Server) handleAIProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Disabled *bool `json:"disabled"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Disabled == nil {
		s.fail(w, r, badRequest("disabled is required"))
		return
	}
	if err := s.Store.SetProjectAIDisabled(r.Context(), p.TenantID(), pr.ID, *req.Disabled); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.ai_settings", "project", pr.ID, fmt.Sprintf("ai_disabled=%v", *req.Disabled))
	st, _, err := s.aiState(r.Context(), p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ---------------------------------------------------------------- the call

// aiMeta describes a completed (or failed) call.
type aiMeta struct {
	ai.Result
	DurationMS int64 `json:"duration_ms"`
	// Cost is the estimate from the configured prices (0: no prices).
	Cost float64 `json:"cost"`
}

// aiEnvelope is the answer of a feature endpoint.
type aiEnvelope struct {
	Meta   aiMeta `json:"meta"`
	Result any    `json:"result"`
}

// aiJob is one feature request: what is asked, about which items.
type aiJob struct {
	call  ai.Call
	out   ai.Answer
	pid   string
	kind  string   // entity type of the audit entry
	items []string // item ids (audit)
	// finish turns the validated answer into the result (may be nil: out).
	finish func() (any, error)
}

// sseWriter writes server-sent events.
type sseWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
	mu sync.Mutex
}

func (e *sseWriter) send(event string, v any) {
	b, _ := json.Marshal(v)
	e.mu.Lock()
	defer e.mu.Unlock()
	fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, b)
	_ = e.rc.Flush()
}

func wantsStream(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// aiError maps a failed call to a status, code and message.
func aiError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ai.ErrRefused):
		return http.StatusUnprocessableEntity, "ai_refused", "The AI service declined to answer this request."
	case errors.Is(err, ai.ErrInvalidAnswer):
		return http.StatusBadGateway, "ai_invalid_answer", err.Error()
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(err.Error(), "Client.Timeout"):
		return http.StatusGatewayTimeout, "ai_timeout", "The AI service did not answer in time."
	case errors.Is(err, ai.ErrService):
		return http.StatusBadGateway, "ai_service", err.Error()
	}
	return http.StatusBadGateway, "ai_service", err.Error()
}

// runAI checks availability and the rate limit, calls the service (streamed
// to the client as server-sent events when it asks for text/event-stream:
// progress, then result or error), writes the audit entry and answers.
func (s *Server) runAI(w http.ResponseWriter, r *http.Request, p *Principal, job aiJob) {
	ctx := r.Context()
	st, c, err := s.aiState(ctx, p, job.pid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !st.Available {
		writeError(w, http.StatusConflict, "ai_unavailable", "AI assistance is not available here: "+st.Reason)
		return
	}
	if !st.Features[job.call.Feature] {
		writeError(w, http.StatusConflict, "ai_unavailable", "AI assistance is not available here: "+aiFeatureOff)
		return
	}
	if !s.aiLimiter(c.RateLimit).allow("ai:" + p.User.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("at most %d AI requests per hour", c.RateLimit))
		return
	}
	job.call.Lang = ai.Lang(job.call.Lang)
	job.call.MaxTokens = c.MaxTokens
	job.call.Temperature, _ = config.ParseTemperature(c.Temperature)
	var ev *sseWriter
	if wantsStream(r) {
		rc := http.NewResponseController(w)
		// a long answer may outlast the server's write timeout
		_ = rc.SetWriteDeadline(time.Now().Add(2*max(c.Timeout, 30*time.Second) + time.Minute))
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		ev = &sseWriter{w: w, rc: rc}
		ev.send("start", map[string]any{"feature": job.call.Feature, "provider": st.Provider, "model": st.Model})
		chars, last := 0, time.Time{}
		job.call.OnDelta = func(t string) {
			chars += len([]rune(t))
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				ev.send("progress", map[string]int{"chars": chars})
			}
		}
	}
	start := time.Now()
	res, err := ai.Run(ctx, s.aiProvider(c), job.call, job.out)
	meta := aiMeta{DurationMS: time.Since(start).Milliseconds()}
	if res != nil {
		meta.Result = *res
		meta.Cost = (float64(res.Usage.InputTokens)*st.PriceInput + float64(res.Usage.OutputTokens)*st.PriceOutput) / 1e6
	}
	var result any
	if err == nil {
		result = job.out
		if job.finish != nil {
			result, err = job.finish()
		}
	}
	status := "ok"
	switch {
	case err != nil && ctx.Err() != nil:
		status = "cancelled"
	case err != nil:
		_, code, _ := aiError(err)
		status = code
	}
	// the audit entry: what was asked about, never the content
	s.audit(context.WithoutCancel(ctx), p, "ai.request", job.kind, firstNonBlank(append(job.items, job.pid)...),
		fmt.Sprintf("feature=%s status=%s provider=%s model=%s prompt=%s items=%s input_tokens=%d output_tokens=%d retried=%v duration_ms=%d",
			job.call.Feature, status, st.Provider, meta.Model, ai.PromptVersion, strings.Join(job.items, ","),
			meta.Usage.InputTokens, meta.Usage.OutputTokens, meta.Retried, meta.DurationMS))
	if err != nil {
		if ctx.Err() != nil {
			return // the client went away (cancelled)
		}
		code, name, msg := aiError(err)
		s.Log.Warn("ai: request failed", "feature", job.call.Feature, "err", err)
		if ev != nil {
			ev.send("error", map[string]any{"status": code, "code": name, "error": msg, "meta": meta})
			return
		}
		writeError(w, code, name, msg)
		return
	}
	env := aiEnvelope{Meta: meta, Result: result}
	if ev != nil {
		ev.send("result", env)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

// aiRequest is the body of the feature endpoints.
type aiRequest struct {
	// Lang of the prompt and the suggestions ("en", "sv").
	Lang          string `json:"lang"`
	RequirementID string `json:"requirement_id"`
	RiskID        string `json:"risk_id"`
	RunID         string `json:"run_id"`
	// Kind "risk" with risk_id empty: suggest mappings for a risk not saved yet.
	Kind string `json:"kind"`
	// The editor's current (maybe unsaved) text; empty: the stored one.
	Title       *string        `json:"title"`
	Description *string        `json:"description"`
	Criteria    []ai.Criterion `json:"criteria"`
	// AnalysisLanguage of the requirement checks ("" or "auto": detect).
	AnalysisLanguage string `json:"analysis_language"`
}

// aiRequirement returns the requirement data of a request: the stored
// requirement (it must be in the project) with the editor's text over it.
func (s *Server) aiRequirement(ctx context.Context, p *Principal, pid string, req *aiRequest, needID bool) (ai.RequirementData, []string, error) {
	d := ai.RequirementData{Criteria: []ai.Criterion{}}
	var items []string
	if req.RequirementID != "" {
		rq, err := s.Store.GetRequirement(ctx, p.TenantID(), req.RequirementID)
		if err != nil {
			return d, nil, err
		}
		if rq.ProjectID != pid {
			return d, nil, badRequest("the requirement is not in this project")
		}
		items = []string{rq.ID}
		d.Key, d.Title, d.Description, d.Type, d.Obligation, d.Rationale = rq.Key, rq.Title, rq.Description, rq.Type, rq.Obligation, rq.Rationale
		for i, c := range rq.Criteria {
			d.Criteria = append(d.Criteria, ai.Criterion{Number: i + 1, Description: c.Description, Method: c.Method, Threshold: c.Threshold})
		}
	} else if needID {
		return d, nil, badRequest("requirement_id is required")
	}
	if req.Title != nil {
		d.Title = *req.Title
	}
	if req.Description != nil {
		d.Description = *req.Description
	}
	if req.Criteria != nil {
		d.Criteria = []ai.Criterion{}
		for i, c := range req.Criteria {
			if strings.TrimSpace(c.Description) != "" {
				d.Criteria = append(d.Criteria, ai.Criterion{Number: i + 1, Description: c.Description, Method: c.Method, Threshold: c.Threshold})
			}
		}
	}
	if strings.TrimSpace(d.Title+d.Description) == "" {
		return d, nil, badRequest("the requirement has no text")
	}
	return d, items, nil
}

func (s *Server) aiDecode(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Project, *aiRequest, bool) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return nil, nil, false
	}
	var req aiRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	return pr, &req, true
}

// ---------------------------------------------------------------- 1. draft tests

// draftedTest is a drafted test with the ids of the criteria it covers.
type draftedTest struct {
	ai.DraftTest
	CriterionIDs []string `json:"criterion_ids"`
}

func (s *Server) handleAIDraftTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, req, ok := s.aiDecode(w, r, p)
	if !ok {
		return
	}
	d, items, err := s.aiRequirement(r.Context(), p, pr.ID, req, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rq, err := s.Store.GetRequirement(r.Context(), p.TenantID(), req.RequirementID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := &ai.DraftTestsAnswer{}
	s.runAI(w, r, p, aiJob{call: ai.Call{Feature: ai.DraftTests, Lang: req.Lang, Data: d}, out: out, pid: pr.ID, kind: "requirement", items: items,
		finish: func() (any, error) {
			tests := make([]draftedTest, 0, len(out.Tests))
			for _, t := range out.Tests {
				dt := draftedTest{DraftTest: t, CriterionIDs: []string{}}
				for _, n := range t.Criteria {
					if n >= 1 && n <= len(rq.Criteria) && !slices.Contains(dt.CriterionIDs, rq.Criteria[n-1].ID) {
						dt.CriterionIDs = append(dt.CriterionIDs, rq.Criteria[n-1].ID)
					}
				}
				if dt.Criteria == nil {
					dt.Criteria = []int{}
				}
				tests = append(tests, dt)
			}
			return map[string]any{"requirement_id": rq.ID, "requirement_key": rq.Key, "tests": tests}, nil
		}})
}

// ---------------------------------------------------------------- 2. improve a requirement

// ruleDiff returns the rules of a whose count drops in b (fixed), as a
// multiset difference.
func ruleDiff(a, b []reqlang.Issue) []string {
	count := map[string]int{}
	for _, i := range b {
		count[i.Rule]++
	}
	out := []string{}
	for _, i := range a {
		if count[i.Rule] > 0 {
			count[i.Rule]--
			continue
		}
		out = append(out, i.Rule)
	}
	return out
}

func (s *Server) handleAIImprove(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, req, ok := s.aiDecode(w, r, p)
	if !ok {
		return
	}
	d, items, err := s.aiRequirement(r.Context(), p, pr.ID, req, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	lang := req.AnalysisLanguage
	if lang != "" && lang != "auto" && !reqlang.Supported(lang) {
		s.fail(w, r, badRequest("analysis_language must be auto or one of %s", strings.Join(reqlang.Languages(), ", ")))
		return
	}
	before := reqlang.AnalyzeLang(d.Description, lang)
	data := ai.ImproveData{Requirement: d, Findings: []ai.Finding{}}
	for _, i := range before.Issues {
		data.Findings = append(data.Findings, ai.Finding{Rule: i.Rule, Message: i.Message})
	}
	out := &ai.ImproveAnswer{}
	s.runAI(w, r, p, aiJob{call: ai.Call{Feature: ai.ImproveRequirement, Lang: req.Lang, Data: data}, out: out, pid: pr.ID, kind: "requirement", items: items,
		finish: func() (any, error) {
			after := reqlang.AnalyzeLang(out.Description, lang)
			if out.Addresses == nil {
				out.Addresses = []string{}
			}
			return map[string]any{
				"original":    map[string]string{"title": d.Title, "description": d.Description},
				"title":       out.Title,
				"description": out.Description,
				"explanation": out.Explanation,
				// checked again on the server: what the rewrite fixes for real
				"before":  before,
				"after":   after,
				"fixed":   ruleDiff(before.Issues, after.Issues),
				"new":     ruleDiff(after.Issues, before.Issues),
				"claimed": out.Addresses,
			}, nil
		}})
}

// ---------------------------------------------------------------- 3. summarise a run

// maxRunExecutions bounds the failed executions sent; maxOutput the error
// output of one (its end is kept).
const (
	maxRunExecutions = 40
	maxOutput        = 3000
	maxNotes         = 1500
)

func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n:])
}

func head(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// draftTestRef is a test a defect draft is about, with its execution.
type draftTestRef struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	TestCaseID  string `json:"test_case_id"`
	ExecutionID string `json:"execution_id"`
}

type runDefectDraft struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Severity    string         `json:"severity"`
	Tests       []draftTestRef `json:"tests"`
}

func (s *Server) handleAISummarizeRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, req, ok := s.aiDecode(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	if req.RunID == "" {
		s.fail(w, r, badRequest("run_id is required"))
		return
	}
	run, err := s.Store.GetRun(ctx, p.TenantID(), req.RunID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if run.ProjectID != pr.ID {
		s.fail(w, r, badRequest("the run is not in this project"))
		return
	}
	execs, err := s.Store.ListExecutions(ctx, p.TenantID(), run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var data ai.RunData
	data.Run.Name, data.Run.Status, data.Run.Environment, data.Run.Build = run.Name, run.Status, run.Environment, run.Build
	data.Run.Results = map[string]int{}
	byKey := map[string]draftTestRef{}
	inRun := map[string]string{} // entity id → test key
	for _, e := range execs {
		data.Run.Results[e.Status]++
		inRun[e.ID], inRun[e.TestCaseID] = e.TestKey, e.TestKey
		if e.Status != model.ResultFailed && e.Status != model.ResultBlocked {
			continue
		}
		if _, seen := byKey[e.TestKey]; !seen {
			byKey[e.TestKey] = draftTestRef{Key: e.TestKey, Title: e.TestTitle, TestCaseID: e.TestCaseID, ExecutionID: e.ID}
		}
		if len(data.Executions) >= maxRunExecutions {
			continue
		}
		x := ai.ExecutionData{Test: e.TestKey, Title: e.TestTitle, Status: e.Status, Configuration: e.ConfigurationName,
			Notes: head(e.Notes, maxNotes), Output: tail(e.Output, maxOutput), Defects: e.Defects}
		if full, err := s.Store.GetExecution(ctx, p.TenantID(), e.ID); err == nil {
			for _, sr := range full.StepResults {
				if sr.Status == model.ResultFailed || sr.Status == model.ResultBlocked {
					x.Steps = append(x.Steps, ai.FailedStep{Number: sr.Position + 1, Action: head(sr.Action, 500), Expected: head(sr.Expected, 500), Status: sr.Status, Actual: head(sr.Actual, 1000)})
				}
			}
		}
		data.Executions = append(data.Executions, x)
	}
	if len(data.Executions) == 0 {
		s.fail(w, r, badRequest("the run has no failed or blocked executions"))
		return
	}
	defects, err := s.Store.ListDefects(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data.Defects = []ai.DefectData{}
	for _, d := range defects {
		dd := ai.DefectData{Key: d.Key, Title: d.Title, Status: d.Status, Severity: d.Severity}
		linked := false
		for _, l := range d.Links {
			if l.EntityID == run.ID {
				linked = true
			}
			if k, ok := inRun[l.EntityID]; ok {
				linked = true
				if !slices.Contains(dd.Tests, k) {
					dd.Tests = append(dd.Tests, k)
				}
			}
		}
		if linked {
			data.Defects = append(data.Defects, dd)
		}
	}
	out := &ai.RunSummaryAnswer{}
	s.runAI(w, r, p, aiJob{call: ai.Call{Feature: ai.SummarizeRun, Lang: req.Lang, Data: data}, out: out, pid: pr.ID, kind: "run", items: []string{run.ID},
		finish: func() (any, error) {
			drafts := make([]runDefectDraft, 0, len(out.Defects))
			for _, d := range out.Defects {
				dd := runDefectDraft{Title: d.Title, Description: d.Description, Severity: d.Severity, Tests: []draftTestRef{}}
				for _, k := range d.Tests {
					if ref, ok := byKey[k]; ok && !slices.ContainsFunc(dd.Tests, func(x draftTestRef) bool { return x.Key == k }) {
						dd.Tests = append(dd.Tests, ref)
					}
				}
				drafts = append(drafts, dd)
			}
			causes := out.Causes
			if causes == nil {
				causes = []ai.Cause{}
			}
			for i := range causes {
				// only tests of the run
				var keep []string
				for _, k := range causes[i].Tests {
					if _, ok := byKey[k]; ok {
						keep = append(keep, k)
					}
				}
				causes[i].Tests = append([]string{}, keep...)
			}
			return map[string]any{"run_id": run.ID, "summary": out.Summary, "causes": causes, "defects": drafts,
				"failed": len(byKey), "linked_defects": data.Defects}, nil
		}})
}

// ---------------------------------------------------------------- 4. framework mappings

// Frameworks of the mappings.
const (
	fwAttack = "attack"
	fwD3fend = "d3fend"
	fwEmb3d  = "emb3d"
	fwCWE    = "cwe"
)

var frameworkText = map[string]map[string]string{
	"en": {
		fwAttack:                 "attack: MITRE ATT&CK or ATLAS techniques that realise the risk (T1110, T1110.004, AML.T0051)",
		fwD3fend:                 "d3fend: MITRE D3FEND defensive techniques the requirement implements (D3-MFA)",
		fwEmb3d + ".requirement": "emb3d: MITRE EMB3D mitigations the requirement implements or threats it addresses (MID-026, TID-201)",
		fwEmb3d + ".risk":        "emb3d: MITRE EMB3D threats that realise the risk (TID-201)",
		fwCWE:                    "cwe: CWE weaknesses the requirement prevents (CWE-79)",
	},
	"sv": {
		fwAttack:                 "attack: MITRE ATT&CK- eller ATLAS-tekniker som förverkligar risken (T1110, T1110.004, AML.T0051)",
		fwD3fend:                 "d3fend: MITRE D3FEND-försvarstekniker som kravet implementerar (D3-MFA)",
		fwEmb3d + ".requirement": "emb3d: MITRE EMB3D-mitigeringar som kravet implementerar eller hot som det hanterar (MID-026, TID-201)",
		fwEmb3d + ".risk":        "emb3d: MITRE EMB3D-hot som förverkligar risken (TID-201)",
		fwCWE:                    "cwe: CWE-svagheter som kravet förhindrar (CWE-79)",
	},
}

var kindText = map[string]map[string]string{
	"en": {"requirement": "requirement", "risk": "risk"},
	"sv": {"requirement": "ett krav", "risk": "en risk"},
}

// suggestedMapping is a validated mapping with the name from the list.
type suggestedMapping struct {
	Framework string `json:"framework"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
}

// validMapping checks a suggested id against the embedded lists and
// returns its canonical form and name ("" id: not valid).
func validMapping(c *catalog.Catalog, cwes map[string]string, kind, fw, id string) (string, string) {
	id = catalog.NormalizeFrameworkID(id)
	switch fw {
	case fwAttack:
		if kind == "risk" {
			if t := c.Technique(id); t != nil {
				return t.ID, t.Name
			}
		}
	case fwD3fend:
		if kind == "requirement" {
			if t := c.D3fendTechnique(id); t != nil {
				return t.ID, t.Name
			}
		}
	case fwEmb3d:
		if t := c.Emb3dThreat(id); t != nil {
			return t.ID, t.Name
		}
		if kind == "requirement" {
			if m := c.Emb3dMitigation(id); m != nil {
				return m.ID, m.Name
			}
		}
	case fwCWE:
		if kind == "requirement" {
			if name, ok := cwes[id]; ok {
				return id, name
			}
		}
	}
	return "", ""
}

func (s *Server) handleAIMappings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, req, ok := s.aiDecode(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	cat, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	lang := ai.Lang(req.Lang)
	data := ai.MappingData{Mapped: map[string][]string{}}
	var items, frameworks []string
	switch {
	case req.RiskID != "":
		k, err := s.Store.GetRisk(ctx, p.TenantID(), req.RiskID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if k.ProjectID != pr.ID {
			s.fail(w, r, badRequest("the risk is not in this project"))
			return
		}
		data.Kind, data.Key, data.Title, data.Description = "risk", k.Key, k.Title, k.Description
		data.Extra = strings.TrimSpace(k.Category + "\n" + k.Mitigation)
		data.Mapped[fwAttack], data.Mapped[fwEmb3d] = append([]string{}, k.Attack...), append([]string{}, k.Emb3d...)
		items, frameworks = []string{k.ID}, []string{fwAttack, fwEmb3d}
	case req.Kind == "risk":
		if req.Title == nil || strings.TrimSpace(*req.Title) == "" {
			s.fail(w, r, badRequest("the risk has no text"))
			return
		}
		data.Kind, frameworks = "risk", []string{fwAttack, fwEmb3d}
	default:
		d, ids, err := s.aiRequirement(ctx, p, pr.ID, req, false)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data.Kind, data.Key, data.Title, data.Description = "requirement", d.Key, d.Title, d.Description
		if len(ids) > 0 {
			rq, err := s.Store.GetRequirement(ctx, p.TenantID(), ids[0])
			if err != nil {
				s.fail(w, r, err)
				return
			}
			data.Mapped[fwD3fend], data.Mapped[fwEmb3d] = append([]string{}, rq.D3fend...), append([]string{}, rq.Emb3d...)
			for _, t := range rq.Tags {
				if strings.HasPrefix(strings.ToUpper(t), "CWE-") {
					data.Mapped[fwCWE] = append(data.Mapped[fwCWE], strings.ToUpper(t))
				}
			}
		}
		items, frameworks = ids, []string{fwD3fend, fwEmb3d, fwCWE}
	}
	if req.Title != nil {
		data.Title = *req.Title
	}
	if req.Description != nil {
		data.Description = *req.Description
	}
	var lines []string
	for _, f := range frameworks {
		t := frameworkText[lang][f]
		if f == fwEmb3d {
			t = frameworkText[lang][f+"."+data.Kind]
		}
		lines = append(lines, t)
	}
	cwes := cat.CWEs()
	out := &ai.MappingsAnswer{}
	s.runAI(w, r, p, aiJob{call: ai.Call{Feature: ai.SuggestMappings, Lang: lang, Data: data, Kind: kindText[lang][data.Kind], Frameworks: lines, FrameworkIDs: frameworks},
		out: out, pid: pr.ID, kind: data.Kind, items: items,
		finish: func() (any, error) {
			keep := []suggestedMapping{}
			dropped := []ai.Mapping{}
			seen := map[string]bool{}
			for _, m := range out.Mappings {
				fw := strings.ToLower(strings.TrimSpace(m.Framework))
				id, name := "", ""
				if slices.Contains(frameworks, fw) {
					id, name = validMapping(cat, cwes, data.Kind, fw, m.ID)
				}
				if id == "" {
					dropped = append(dropped, ai.Mapping{Framework: m.Framework, ID: m.ID})
					continue
				}
				if seen[fw+" "+id] || slices.Contains(data.Mapped[fw], id) || len(keep) >= ai.MaxMappings {
					continue
				}
				seen[fw+" "+id] = true
				keep = append(keep, suggestedMapping{Framework: fw, ID: id, Name: name, Reason: m.Reason})
			}
			return map[string]any{"kind": data.Kind, "mappings": keep, "dropped": dropped}, nil
		}})
}

// ---------------------------------------------------------------- 5. acceptance criteria

func (s *Server) handleAICriteria(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, req, ok := s.aiDecode(w, r, p)
	if !ok {
		return
	}
	d, items, err := s.aiRequirement(r.Context(), p, pr.ID, req, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := &ai.CriteriaAnswer{}
	s.runAI(w, r, p, aiJob{call: ai.Call{Feature: ai.SuggestCriteria, Lang: req.Lang, Data: d}, out: out, pid: pr.ID, kind: "requirement", items: items,
		finish: func() (any, error) {
			have := map[string]bool{}
			for _, c := range d.Criteria {
				have[strings.ToLower(strings.TrimSpace(c.Description))] = true
			}
			keep := []ai.Criterion{}
			for _, c := range out.Criteria {
				k := strings.ToLower(strings.TrimSpace(c.Description))
				if have[k] {
					continue
				}
				have[k] = true
				keep = append(keep, c)
			}
			return map[string]any{"criteria": keep}, nil
		}})
}

// ---------------------------------------------------------------- settings test

// handleTestAI: POST /admin/settings/test/ai {values} — asks the service
// of the form (unsaved values; the stored key when none is typed) for a
// short answer and reports the model and tokens. No item data is sent.
func (s *Server) handleTestAI(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	var req struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := decodeOptional(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	cfg, err := s.candidate(r.Context(), req.Values, "ai.")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c := cfg.AI
	if c.ProviderName() == config.AIOpenAI && c.URL == "" {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: "no service URL"})
		return
	}
	if c.ModelName() == "" {
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: "no model"})
		return
	}
	prov := s.aiProvider(c)
	resp, err := prov.Complete(r.Context(), ai.Request{System: "You are a connection test. Answer with the single word OK.",
		Messages: []ai.Message{{Role: "user", Text: "Connection test: answer OK."}}, MaxTokens: 256}, nil)
	target := c.ProviderName() + " " + firstNonBlank(c.URL, ai.DefaultAnthropicURL)
	if err != nil {
		s.audit(r.Context(), p, "settings.test_ai", "system_settings", "ai", target+": "+err.Error())
		writeJSON(w, http.StatusOK, testResult{OK: false, Error: err.Error()})
		return
	}
	s.audit(r.Context(), p, "settings.test_ai", "system_settings", "ai", target)
	writeJSON(w, http.StatusOK, testResult{OK: true, Message: fmt.Sprintf("%s: %d input and %d output tokens", firstNonBlank(resp.Model, c.ModelName()), resp.Usage.InputTokens, resp.Usage.OutputTokens),
		Details: map[string]any{"model": resp.Model, "input_tokens": resp.Usage.InputTokens, "output_tokens": resp.Usage.OutputTokens}})
}
