package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/runner"
	"github.com/rom/qtestframe/internal/store"
)

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), runSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	runs, err := s.Store.ListRuns(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.TestRun{}
	for i := range runs {
		if q.Match(runRecord(&runs[i])) {
			out = append(out, runs[i])
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Environment string   `json:"environment"`
		Build       string   `json:"build"`
		TestIDs     []string `json:"test_ids"`
		FolderIDs   []string `json:"folder_ids"`
		Query       string   `json:"query"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ids := append([]string{}, req.TestIDs...)
	if len(req.FolderIDs) > 0 || req.Query != "" {
		pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, f := range req.FolderIDs {
			ts, err := s.filterTests(r, p, pr, pc, "", f, true)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for _, t := range ts {
				ids = append(ids, t.ID)
			}
		}
		if req.Query != "" {
			ts, err := s.filterTests(r, p, pr, pc, req.Query, "", true)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for _, t := range ts {
				ids = append(ids, t.ID)
			}
		}
	}
	run := &model.TestRun{ProjectID: pr.ID, Name: req.Name, Description: req.Description,
		Environment: req.Environment, Build: req.Build, CreatedBy: p.User.Email}
	if err := s.Store.CreateRun(r.Context(), p.TenantID(), run, ids); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.create", "run", run.ID, run.Name)
	out, err := s.Store.GetRun(r.Context(), p.TenantID(), run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleUpdateRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Environment *string `json:"environment"`
		Build       *string `json:"build"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	for _, f := range []struct {
		src *string
		dst *string
	}{{req.Name, &run.Name}, {req.Description, &run.Description}, {req.Status, &run.Status}, {req.Environment, &run.Environment}, {req.Build, &run.Build}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if err := s.Store.UpdateRun(r.Context(), p.TenantID(), run); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.update", "run", run.ID, run.Status)
	out, err := s.Store.GetRun(r.Context(), p.TenantID(), run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.Store.DeleteRun(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.delete", "run", id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if _, err := s.Store.GetRun(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), executionSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	es, err := s.Store.ListExecutions(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rc, err := s.loadReadiness(r.Context(), p.TenantID(), run.ProjectID, run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type withReadiness struct {
		model.Execution
		Readiness *Readiness `json:"readiness"`
	}
	out := []withReadiness{}
	for i := range es {
		if q.Match(executionRecord(&es[i])) {
			out = append(out, withReadiness{Execution: es[i], Readiness: rc.check(es[i].TestCaseID)})
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleAddExecutions(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		TestIDs []string `json:"test_ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	n, err := s.Store.AddExecutions(r.Context(), p.TenantID(), r.PathValue("id"), req.TestIDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.add_tests", "run", r.PathValue("id"), "")
	writeJSON(w, http.StatusOK, map[string]int{"added": n})
}

func (s *Server) handleImportResults(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Results []store.ImportResult `json:"results"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rc, err := s.loadReadiness(r.Context(), p.TenantID(), run.ProjectID, run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byKey := map[string]string{}
	for id, t := range rc.tests {
		byKey[strings.ToUpper(t.Key)] = id
	}
	// results of tests that may not be executed are not recorded
	type rejected struct {
		TestKey   string     `json:"test_key"`
		Readiness *Readiness `json:"readiness"`
	}
	rejects := []rejected{}
	accepted := make([]store.ImportResult, 0, len(req.Results))
	for _, res := range req.Results {
		if id, ok := byKey[strings.ToUpper(strings.TrimSpace(res.TestKey))]; ok && activeResult(res.Status) {
			if rd := rc.check(id); !rd.Allowed {
				rejects = append(rejects, rejected{TestKey: res.TestKey, Readiness: rd})
				continue
			}
		}
		accepted = append(accepted, res)
	}
	n, unknown, err := s.Store.ImportResults(r.Context(), p.TenantID(), run.ID, p.User.Email, accepted)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "run.import", "run", run.ID, fmt.Sprintf("%d updated, %d rejected", n, len(rejects)))
	writeJSON(w, http.StatusOK, map[string]any{"updated": n, "unknown": unknown, "rejected": rejects})
}

type executionDetail struct {
	*model.Execution
	Test *model.TestCase `json:"test"`
	// Readiness of the test in this run (organisation settings, test and
	// requirement status, prerequisites).
	Readiness *Readiness `json:"readiness"`
}

func (s *Server) executionDetail(r *http.Request, p *Principal, id string) (*executionDetail, error) {
	e, err := s.Store.GetExecution(r.Context(), p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	t, err := s.Store.GetTest(r.Context(), p.TenantID(), e.TestCaseID)
	if err != nil {
		return nil, err
	}
	rd, err := s.executionReadiness(r.Context(), p.TenantID(), e)
	if err != nil {
		return nil, err
	}
	return &executionDetail{Execution: e, Test: t, Readiness: rd}, nil
}

func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.executionDetail(r, p, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleUpdateExecution(w http.ResponseWriter, r *http.Request, p *Principal) {
	var u store.ExecutionUpdate
	if err := decode(r, &u); err != nil {
		s.fail(w, r, err)
		return
	}
	cur, err := s.Store.GetExecution(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if starts(u, cur) {
		rd, err := s.executionReadiness(r.Context(), p.TenantID(), cur)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !rd.Allowed {
			notReady(w, cur.TestKey, rd)
			return
		}
	}
	e, err := s.Store.UpdateExecution(r.Context(), p.TenantID(), r.PathValue("id"), p.User.Email, u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "execution.update", "execution", e.ID, e.TestKey+" "+e.Status)
	d, err := s.executionDetail(r, p, e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleRunScript(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.executionDetail(r, p, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Test.ExecutionType != model.ExecAutomated || d.Test.Script == "" {
		s.fail(w, r, badRequest("test has no automated script"))
		return
	}
	if !s.Runner.Enabled() {
		s.fail(w, r, runner.ErrDisabled)
		return
	}
	if !d.Readiness.Allowed {
		notReady(w, d.TestKey, d.Readiness)
		return
	}
	run, err := s.Store.GetRun(r.Context(), p.TenantID(), d.RunID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	inProgress := model.ResultInProgress
	if _, err := s.Store.UpdateExecution(r.Context(), p.TenantID(), d.ID, p.User.Email, store.ExecutionUpdate{Status: &inProgress}); err != nil {
		s.fail(w, r, err)
		return
	}
	lang := d.Test.ScriptLanguage
	if lang == "" {
		lang = "sh"
	}
	res, err := s.Runner.Run(r.Context(), lang, d.Test.Script, map[string]string{
		"test_key": d.Test.Key, "run_id": run.ID, "run_name": run.Name,
		"environment": run.Environment, "build": run.Build,
	})
	if err != nil {
		prev := d.Status
		if prev == model.ResultInProgress {
			prev = model.ResultNotRun
		}
		_, _ = s.Store.UpdateExecution(r.Context(), p.TenantID(), d.ID, p.User.Email, store.ExecutionUpdate{Status: &prev})
		s.fail(w, r, err)
		return
	}
	status := model.ResultFailed
	if res.Passed {
		status = model.ResultPassed
	}
	dur := res.Duration.Milliseconds()
	out := res.Output
	if _, err := s.Store.UpdateExecution(r.Context(), p.TenantID(), d.ID, p.User.Email, store.ExecutionUpdate{Status: &status, Output: &out, DurationMS: &dur}); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "execution.script", "execution", d.ID, d.Test.Key+" "+status)
	nd, err := s.executionDetail(r, p, d.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nd)
}
