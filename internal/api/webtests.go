package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/runner"
	"github.com/rom/qtestframe/internal/store"
)

// Web tests (script language "playwright"): the project settings (base
// URLs and test variables) and how a test's target is derived from them,
// the run's environment and the execution's configuration.

func (s *Server) routesWebTests(h routeFunc) {
	h("GET /api/v1/projects/{pid}/web-tests", s.handleGetWebTests, "project.read")
	h("PUT /api/v1/projects/{pid}/web-tests", s.handlePutWebTests, "project.update")
	h("POST /api/v1/executions/{id}/run-script/cancel", s.atomic(s.handleCancelExecutionJob), "script.execute")
	h("POST /api/v1/runs/{id}/run-automated", s.atomic(s.handleRunAutomated), "script.execute")
	h("GET /api/v1/runs/{id}/jobs", s.handleListRunJobs, "run.read")
	h("POST /api/v1/runs/{id}/jobs/cancel", s.atomic(s.handleCancelRunJobs), "script.execute")
}

// webTestsResponse is the settings without secret values, with what the
// server can run.
type webTestsResponse struct {
	*model.WebTestSettings
	// Enabled: the server runs web tests (runner.web.enabled).
	Enabled bool `json:"enabled"`
	// Browser is the server's default browser.
	Browser string `json:"browser"`
}

func (s *Server) webTestsResponse(w *model.WebTestSettings) webTestsResponse {
	return webTestsResponse{WebTestSettings: store.RedactWebTestSettings(w), Enabled: s.Runner.WebEnabled(), Browser: s.Cfg.Runner.Web.Browser}
}

func (s *Server) handleGetWebTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	set, err := s.Store.WebTestSettings(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.webTestsResponse(set))
}

func (s *Server) handlePutWebTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req model.WebTestSettings
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveWebTestSettings(r.Context(), p.TenantID(), pr.ID, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	names := make([]string, 0, len(req.Variables))
	for _, v := range req.Variables {
		names = append(names, v.Name)
	}
	// variable names only: values (and secrets) stay out of the audit log
	s.audit(r.Context(), p, "web_tests.update", "project", pr.ID, req.BaseURL+" variables: "+strings.Join(names, ", "))
	writeJSON(w, http.StatusOK, s.webTestsResponse(&req))
}

// webTarget is where and in which browser a web test runs.
type webTarget struct {
	BaseURL string
	Browser string
	Channel string
	// Notes explain the choice (shown in the output).
	Notes []string
}

// Configuration dimensions that choose the browser or the base URL.
var (
	browserDimensions = map[string]bool{"browser": true, "webbläsare": true, "webblasare": true}
	urlDimensions     = map[string]bool{"base url": true, "base_url": true, "baseurl": true, "url": true, "bas-url": true, "basadress": true, "bas-adress": true}
)

// targetFor derives the target of a web test: the base URL is the
// configuration's "Base URL" value, else the URL of the run's environment,
// else the project's base URL; the browser is the configuration's Browser
// value, else the server's default.
func targetFor(w config.WebRunner, set *model.WebTestSettings, environment string, values []model.DimensionValue) webTarget {
	t := webTarget{BaseURL: set.BaseURL}
	if env := strings.TrimSpace(environment); env != "" {
		for _, e := range set.EnvironmentURLs {
			if strings.EqualFold(e.Environment, env) {
				t.BaseURL = e.BaseURL
				t.Notes = append(t.Notes, "base URL of environment "+e.Environment)
			}
		}
	}
	for _, v := range values {
		dim := strings.ToLower(strings.TrimSpace(v.Dimension))
		switch {
		case urlDimensions[dim]:
			if store.ValidBaseURL(strings.TrimSpace(v.Value)) {
				t.BaseURL = strings.TrimSpace(v.Value)
				t.Notes = append(t.Notes, "base URL of the configuration ("+v.Dimension+")")
			} else {
				t.Notes = append(t.Notes, "ignored "+v.Dimension+" "+v.Value+": not an http(s) URL")
			}
		case browserDimensions[dim]:
			if b, c, ok := runner.BrowserFor(v.Value, w.BrowserChannels); ok {
				t.Browser, t.Channel = b, c
				t.Notes = append(t.Notes, v.Dimension+" "+v.Value+" runs in "+b)
			} else {
				t.Notes = append(t.Notes, "unknown "+v.Dimension+" "+v.Value+": the default browser is used")
			}
		}
	}
	if t.Browser == "" {
		t.Browser, t.Channel = w.Browser, w.Channel
	}
	return t
}

// scriptEnv returns the QTF_* variables of an automated test and the
// secret values to mask.
func scriptEnv(t *model.TestCase, run *model.TestRun, set *model.WebTestSettings, baseURL string) (map[string]string, []string) {
	env := map[string]string{
		"test_key": t.Key, "run_id": run.ID, "run_name": run.Name,
		"environment": run.Environment, "build": run.Build, "base_url": baseURL,
	}
	var secrets []string
	for _, v := range set.Variables {
		env["var_"+v.Name] = v.Value
		if v.Secret && v.Value != "" {
			secrets = append(secrets, v.Value)
		}
	}
	return env, secrets
}

// executionTarget loads what a job needs to know about where it runs.
func (s *Server) executionTarget(ctx context.Context, tenantID string, run *model.TestRun, e *model.Execution, set *model.WebTestSettings) webTarget {
	var values []model.DimensionValue
	if e.ConfigurationID != nil {
		if c, err := s.Store.GetConfiguration(ctx, tenantID, *e.ConfigurationID); err == nil {
			values = c.Values
		}
	}
	return targetFor(s.Cfg.Runner.Web, set, run.Environment, values)
}
