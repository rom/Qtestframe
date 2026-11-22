package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// Test cycles and planning (see docs/api.md "Test cycles"): cycles group
// the executions planned for a goal, their plan (tests per configuration
// with tester and estimate), the runs the cycle creates, the progress
// (totals, workload per tester, burndown) and recurring schedules. Reading
// needs run.read; planning needs cycle.plan (test leads).

func (s *Server) routesCycles(h routeFunc) {
	h("GET /api/v1/projects/{pid}/cycles", s.handleListCycles, "run.read")
	h("POST /api/v1/projects/{pid}/cycles", s.handleCreateCycle, "cycle.plan")
	h("GET /api/v1/cycles/{id}", s.handleGetCycle, "run.read")
	h("PATCH /api/v1/cycles/{id}", s.handleUpdateCycle, "cycle.plan")
	h("DELETE /api/v1/cycles/{id}", s.handleDeleteCycle, "cycle.plan")
	h("GET /api/v1/cycles/{id}/tests", s.handleCycleTests, "run.read")
	h("POST /api/v1/cycles/{id}/tests", s.atomic(s.handleAddCycleTests), "cycle.plan")
	h("PATCH /api/v1/cycles/{id}/tests", s.atomic(s.handleUpdateCycleTests), "cycle.plan")
	h("DELETE /api/v1/cycles/{id}/tests", s.atomic(s.handleRemoveCycleTests), "cycle.plan")
	h("POST /api/v1/cycles/{id}/runs", s.handleCreateCycleRun, "cycle.plan")
	h("GET /api/v1/cycles/{id}/progress", s.handleCycleProgress, "run.read")
	h("GET /api/v1/cycles/{id}/history", s.handleCycleHistory, "run.read")
	h("GET /api/v1/projects/{pid}/cycle-schedules", s.handleListCycleSchedules, "run.read")
	h("POST /api/v1/projects/{pid}/cycle-schedules", s.handleCreateCycleSchedule, "cycle.plan")
	h("PATCH /api/v1/cycle-schedules/{id}", s.handleUpdateCycleSchedule, "cycle.plan")
	h("DELETE /api/v1/cycle-schedules/{id}", s.handleDeleteCycleSchedule, "cycle.plan")
	h("POST /api/v1/cycle-schedules/{id}/run", s.handleRunCycleSchedule, "cycle.plan")
}

// ---------------------------------------------------------------- cycles

// cycleReq is the body of POST /projects/{pid}/cycles and PATCH /cycles/{id}.
type cycleReq struct {
	Name             *string    `json:"name"`
	Goal             *string    `json:"goal"`
	Description      *string    `json:"description"`
	Status           *string    `json:"status"`
	OwnerID          *string    `json:"owner_id"`
	ReleaseID        *string    `json:"release_id"`
	VariantID        *string    `json:"variant_id"`
	BuildID          *string    `json:"build_id"`
	Environment      *string    `json:"environment"`
	ConfigurationIDs *[]string  `json:"configuration_ids"`
	StartDate        *string    `json:"start_date"`
	EndDate          *string    `json:"end_date"`
	ScheduledStartAt *time.Time `json:"scheduled_start_at"`
	// ClearScheduledStart removes the scheduled start (PATCH).
	ClearScheduledStart bool `json:"clear_scheduled_start"`
	// CopyFrom (create): a cycle whose plan, testers and estimates the new
	// cycle gets (and its release, variant, build and configurations when
	// not given).
	CopyFrom string `json:"copy_from"`
}

func optID(v *string) *string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil
	}
	x := strings.TrimSpace(*v)
	return &x
}

// apply copies the given fields of the request onto cy; variants may be
// given by name.
func (s *Server) applyCycleReq(ctx context.Context, tenantID string, cy *model.TestCycle, in *cycleReq) error {
	for _, f := range []struct{ src, dst *string }{{in.Name, &cy.Name}, {in.Goal, &cy.Goal}, {in.Description, &cy.Description}, {in.Status, &cy.Status},
		{in.OwnerID, &cy.OwnerID}, {in.Environment, &cy.Environment}, {in.StartDate, &cy.StartDate}, {in.EndDate, &cy.EndDate}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if in.ReleaseID != nil {
		cy.ReleaseID = optID(in.ReleaseID)
	}
	if in.BuildID != nil {
		cy.BuildID = optID(in.BuildID)
	}
	if in.VariantID != nil {
		cy.VariantID = optID(in.VariantID)
		if cy.VariantID != nil {
			v, err := s.Store.FindVariant(ctx, tenantID, cy.ProjectID, *cy.VariantID)
			if err != nil {
				return badRequest("unknown variant %q", *cy.VariantID)
			}
			cy.VariantID = &v.ID
		}
	}
	if in.ConfigurationIDs != nil {
		cy.ConfigurationIDs = *in.ConfigurationIDs
	}
	if in.ScheduledStartAt != nil {
		t := in.ScheduledStartAt.UTC()
		cy.ScheduledStartAt = &t
	}
	if in.ClearScheduledStart {
		cy.ScheduledStartAt = nil
	}
	return nil
}

// cycleRecord is a cycle for the query language; owners and testers are
// matched by e-mail and name, tests by key.
func (s *Server) cycleRecord(cy *model.TestCycle, plan []model.TestCycleTest, people func(id string) []string) query.MapRecord {
	cfgs := append([]string{}, cy.ConfigurationIDs...)
	var tests, testers []string
	seen := map[string]bool{}
	for _, x := range plan {
		tests = append(tests, x.TestKey)
		if x.ConfigurationName != "" && !seen["c"+x.ConfigurationName] {
			seen["c"+x.ConfigurationName] = true
			cfgs = append(cfgs, x.ConfigurationName)
		}
		if x.AssigneeID != "" && !seen[x.AssigneeID] {
			seen[x.AssigneeID] = true
			testers = append(testers, people(x.AssigneeID)...)
		}
	}
	p := cy.Progress
	return query.MapRecord{"id": {cy.ID}, "name": {cy.Name}, "goal": {cy.Goal}, "description": {cy.Description}, "status": {cy.Status},
		"owner": people(cy.OwnerID), "release": {cy.ReleaseName}, "variant": {cy.VariantName}, "build": {cy.BuildLabel}, "environment": {cy.Environment},
		"configuration": cfgs, "start": {cy.StartDate}, "end": {cy.EndDate}, "created_by": {cy.CreatedBy}, "created_at": {ts(cy.CreatedAt)},
		"planned": {strconv.Itoa(p.Planned)}, "executed": {strconv.Itoa(p.Executed)}, "progress": {strconv.FormatFloat(p.PercentDone, 'f', -1, 64)},
		"passed": {strconv.Itoa(p.Passed)}, "failed": {strconv.Itoa(p.Failed)}, "blocked": {strconv.Itoa(p.Blocked)}, "unassigned": {strconv.Itoa(p.Unassigned)},
		"test": tests, "assignee": testers}
}

// peopleLookup returns the e-mail and name of users by id (cached).
func (s *Server) peopleLookup(ctx context.Context) func(id string) []string {
	cache := map[string][]string{}
	return func(id string) []string {
		if id == "" {
			return nil
		}
		if v, ok := cache[id]; ok {
			return v
		}
		var v []string
		if u, err := s.Store.GetUser(ctx, id); err == nil {
			v = []string{u.Email}
			if u.DisplayName != "" {
				v = append(v, u.DisplayName)
			}
		}
		cache[id] = v
		return v
	}
}

func (s *Server) handleListCycles(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	expr := r.URL.Query().Get("q")
	q, err := query.Parse(expr, cycleSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListCycles(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(expr) == "" {
		writeJSON(w, http.StatusOK, listOf(list))
		return
	}
	people := s.peopleLookup(r.Context())
	out := []model.TestCycle{}
	var recs []query.MapRecord
	for i := range list {
		plan, err := s.Store.CycleTests(r.Context(), p.TenantID(), list[i].ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if rec := s.cycleRecord(&list[i], plan, people); q.Match(rec) {
			out = append(out, list[i])
			recs = append(recs, rec)
		}
	}
	sortByQuery(q, out, recs)
	writeJSON(w, http.StatusOK, listOf(out))
}

// recordCycle stores a revision of a cycle (fields and plan) for its
// history; failures are only logged.
func (s *Server) recordCycle(ctx context.Context, tenantID, user, id, action string) {
	cy, err := s.Store.GetCycle(ctx, tenantID, id)
	if err == nil {
		var plan []model.TestCycleTest
		if plan, err = s.Store.CycleTests(ctx, tenantID, id); err == nil {
			err = s.Store.AddRevision(ctx, tenantID, "cycle", id, cy.Version, action, user, store.SnapshotCycle(cy, plan))
		}
	}
	if err != nil {
		s.Log.Warn("revision not recorded", "entity", "cycle", "id", id, "err", err)
	}
}

// checkTester verifies that a tester (or owner) may see the project.
func (s *Server) checkTester(ctx context.Context, tenantID, projectID, userID, what string) error {
	if userID == "" {
		return nil
	}
	if !s.userCanSee(ctx, tenantID, projectID, userID, rbac.RunRead) {
		return badRequest("the %s may not see the runs of this project", what)
	}
	return nil
}

func (s *Server) handleCreateCycle(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	var in cycleReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	cy := &model.TestCycle{ProjectID: pr.ID, OwnerID: p.User.ID, CreatedBy: p.User.Email}
	var src *model.TestCycle
	if in.CopyFrom != "" {
		var err error
		if src, err = s.Store.GetCycle(ctx, p.TenantID(), in.CopyFrom); err != nil || src.ProjectID != pr.ID {
			s.fail(w, r, badRequest("cycle %q to copy not found in the project", in.CopyFrom))
			return
		}
		cy.Goal, cy.Description, cy.ReleaseID, cy.VariantID, cy.BuildID = src.Goal, src.Description, src.ReleaseID, src.VariantID, src.BuildID
		cy.Environment, cy.ConfigurationIDs = src.Environment, src.ConfigurationIDs
	}
	if err := s.applyCycleReq(ctx, p.TenantID(), cy, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if cy.Status != "" && cy.Status != model.CyclePlanned {
		s.fail(w, r, badRequest("create the cycle first, then start it"))
		return
	}
	if err := s.checkTester(ctx, p.TenantID(), pr.ID, cy.OwnerID, "owner"); err != nil {
		s.fail(w, r, err)
		return
	}
	err := s.Store.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateCycle(ctx, p.TenantID(), cy); err != nil {
			return err
		}
		if src != nil {
			_, err := s.Store.CopyCyclePlan(ctx, p.TenantID(), src.ID, cy.ID, p.User.Email)
			return err
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	detail := cy.Name
	if src != nil {
		detail += " (copy of " + src.Name + ")"
	}
	s.audit(ctx, p, "cycle.create", "cycle", cy.ID, detail)
	s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "create")
	out, err := s.Store.GetCycle(ctx, p.TenantID(), cy.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleGetCycle(w http.ResponseWriter, r *http.Request, p *Principal) {
	cy, err := s.Store.GetCycle(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cy)
}

// handleUpdateCycle changes a cycle. Starting it (status active) creates
// its run of the planned tests.
func (s *Server) handleUpdateCycle(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cy, err := s.Store.GetCycle(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in cycleReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	before := cy.Status
	owner := cy.OwnerID
	if err := s.applyCycleReq(ctx, p.TenantID(), cy, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if cy.OwnerID != owner {
		if err := s.checkTester(ctx, p.TenantID(), cy.ProjectID, cy.OwnerID, "owner"); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := s.Store.UpdateCycle(ctx, p.TenantID(), cy); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "cycle.update", "cycle", cy.ID, cy.Name+" "+cy.Status)
	if cy.Status == model.CycleActive && before == model.CyclePlanned {
		if _, err := s.startCycleRun(ctx, p, cy); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "update")
	out, err := s.Store.GetCycle(ctx, p.TenantID(), cy.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// startCycleRun creates the run of the cycle's planned tests that are in
// no run yet (nil when there are none), audits it and tells the testers.
func (s *Server) startCycleRun(ctx context.Context, p *Principal, cy *model.TestCycle) (*model.TestRun, error) {
	plan, err := s.Store.CycleTests(ctx, p.TenantID(), cy.ID)
	if err != nil {
		return nil, err
	}
	emails := map[string]string{}
	var testers []string
	for _, x := range plan {
		if x.AssigneeID == "" || x.ExecutionID != nil {
			continue
		}
		if _, ok := emails[x.AssigneeID]; !ok {
			if u, err := s.Store.GetUser(ctx, x.AssigneeID); err == nil {
				emails[x.AssigneeID] = u.Email
				testers = append(testers, x.AssigneeID)
			}
		}
	}
	run, err := s.Store.CreateCycleRun(ctx, p.TenantID(), cy.ID, p.User.Email, emails)
	if err != nil || run == nil {
		return run, err
	}
	s.audit(ctx, p, "run.create", "run", run.ID, fmt.Sprintf("%s (test cycle %s, %d executions)", run.Name, cy.Name, run.Summary["total"]))
	s.notifyUsers(ctx, p, testers, model.Notification{ProjectID: cy.ProjectID, Event: "assigned", Title: cy.Name, Body: run.Name, EntityType: "cycle", EntityID: cy.ID})
	return run, nil
}

func (s *Server) handleCreateCycleRun(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cy, err := s.Store.GetCycle(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if cy.Status == model.CycleClosed {
		s.fail(w, r, store.Conflictf("the cycle is closed"))
		return
	}
	if !p.Can(rbac.RunWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission run.write required")
		return
	}
	run, err := s.startCycleRun(ctx, p, cy)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if run == nil {
		s.fail(w, r, badRequest("every planned test is already in a run of the cycle"))
		return
	}
	if cy.Status == model.CyclePlanned {
		cy.Status = model.CycleActive
		if err := s.Store.UpdateCycle(ctx, p.TenantID(), cy); err != nil {
			s.fail(w, r, err)
			return
		}
		s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "update")
	}
	s.notifyEvent(r, p, run.ProjectID, "run.created", map[string]any{"run": run})
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) handleDeleteCycle(w http.ResponseWriter, r *http.Request, p *Principal) {
	cy, err := s.Store.GetCycle(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteCycle(r.Context(), p.TenantID(), cy.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "cycle.delete", "cycle", cy.ID, cy.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCycleHistory(w http.ResponseWriter, r *http.Request, p *Principal) {
	cy, err := s.Store.GetCycle(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	revs, err := s.Store.ListRevisions(r.Context(), p.TenantID(), "cycle", cy.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(revs))
}

// ---------------------------------------------------------------- plan

func (s *Server) handleCycleTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	cy, err := s.Store.GetCycle(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	plan, err := s.Store.CycleTests(r.Context(), p.TenantID(), cy.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// ?assignee=<user id> | "none": one tester's part of the plan
	if a := r.URL.Query().Get("assignee"); a != "" {
		if a == "none" {
			a = ""
		}
		out := []model.TestCycleTest{}
		for _, x := range plan {
			if x.AssigneeID == a {
				out = append(out, x)
			}
		}
		plan = out
	}
	writeJSON(w, http.StatusOK, listOf(plan))
}

// addCycleTestsReq: the tests to plan — by id, query, folder, the
// regression selection or the plan of another cycle — with a tester and
// estimate for all of them (assignee_id, estimate_minutes; nil: the
// default estimate) or per test (tests).
type addCycleTestsReq struct {
	TestIDs          []string               `json:"test_ids"`
	Tests            []store.CycleTestInput `json:"tests"`
	Query            string                 `json:"query"`
	FolderID         string                 `json:"folder_id"`
	Recursive        *bool                  `json:"recursive"`
	Regression       *regressionReq         `json:"regression"`
	CopyFrom         string                 `json:"copy_from"`
	AssigneeID       string                 `json:"assignee_id"`
	EstimateMinutes  *int                   `json:"estimate_minutes"`
	ConfigurationIDs []string               `json:"configuration_ids"`
}

func (s *Server) handleAddCycleTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cy, err := s.Store.GetCycle(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in addCycleTestsReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	pr, err := s.Store.GetProject(ctx, p.TenantID(), cy.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := append([]string{}, in.TestIDs...)
	var how []string
	if len(in.TestIDs) > 0 || len(in.Tests) > 0 {
		how = append(how, "selection")
	}
	if in.Query != "" || in.FolderID != "" || in.Regression != nil {
		if !p.Can(rbac.TestRead) {
			writeError(w, http.StatusForbidden, "forbidden", "permission test.read required")
			return
		}
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if in.Query != "" || in.FolderID != "" {
			rec := in.Recursive == nil || *in.Recursive
			list, err := s.filterTests(r, p, pr, pc, in.Query, in.FolderID, rec)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for _, t := range list {
				ids = append(ids, t.ID)
			}
			if in.Query != "" {
				how = append(how, "query "+in.Query)
			}
			if in.FolderID != "" {
				how = append(how, "folder")
			}
		}
		if in.Regression != nil {
			sel, err := s.regression(r, p, pr, in.Regression)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for _, t := range sel.Tests {
				ids = append(ids, t.ID)
			}
			how = append(how, "regression selection")
		}
	}
	people := map[string]bool{}
	check := func(id string) error {
		if id == "" || people[id] {
			return nil
		}
		people[id] = true
		return s.checkTester(ctx, p.TenantID(), cy.ProjectID, id, "tester")
	}
	if err := check(in.AssigneeID); err != nil {
		s.fail(w, r, err)
		return
	}
	var inputs []store.CycleTestInput
	for _, t := range in.Tests {
		if err := check(t.AssigneeID); err != nil {
			s.fail(w, r, err)
			return
		}
		inputs = append(inputs, t)
	}
	for _, id := range ids {
		base := store.CycleTestInput{TestID: id, AssigneeID: in.AssigneeID, EstimateMinutes: in.EstimateMinutes}
		if len(in.ConfigurationIDs) == 0 {
			inputs = append(inputs, base)
			continue
		}
		for _, c := range in.ConfigurationIDs {
			x := base
			c := c
			x.ConfigurationID = &c
			inputs = append(inputs, x)
		}
	}
	added := 0
	if len(inputs) > 0 {
		if added, err = s.Store.AddCycleTests(ctx, p.TenantID(), cy.ID, p.User.Email, inputs); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if in.CopyFrom != "" {
		n, err := s.Store.CopyCyclePlan(ctx, p.TenantID(), in.CopyFrom, cy.ID, p.User.Email)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		added += n
		how = append(how, "copy")
	}
	if len(how) == 0 {
		s.fail(w, r, badRequest("choose tests: test_ids, tests, query, folder_id, regression or copy_from"))
		return
	}
	s.audit(ctx, p, "cycle.plan", "cycle", cy.ID, fmt.Sprintf("%s: %d tests added (%s)", cy.Name, added, strings.Join(how, ", ")))
	if added > 0 {
		s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "plan")
		testers := []string{}
		for id := range people {
			if id != "" && id != p.User.ID {
				testers = append(testers, id)
			}
		}
		s.notifyUsers(ctx, p, testers, model.Notification{ProjectID: cy.ProjectID, Event: "assigned", Title: cy.Name, EntityType: "cycle", EntityID: cy.ID})
	}
	out, err := s.Store.GetCycle(ctx, p.TenantID(), cy.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "cycle": out})
}

// handleUpdateCycleTests: {"ids": [row ids] (or "all": true), "assignee_id"?, "estimate_minutes"?}.
func (s *Server) handleUpdateCycleTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cy, err := s.Store.GetCycle(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
		store.CycleTestUpdate
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(in.IDs) == 0 && !in.All {
		s.fail(w, r, badRequest("choose rows: ids or all"))
		return
	}
	ids := in.IDs
	if in.All {
		ids = nil
	}
	email := ""
	if in.AssigneeID != nil && *in.AssigneeID != "" {
		if err := s.checkTester(ctx, p.TenantID(), cy.ProjectID, *in.AssigneeID, "tester"); err != nil {
			s.fail(w, r, err)
			return
		}
		u, err := s.Store.GetUser(ctx, *in.AssigneeID)
		if err != nil {
			s.fail(w, r, badRequest("unknown tester"))
			return
		}
		email = u.Email
	}
	n, err := s.Store.UpdateCycleTests(ctx, p.TenantID(), cy.ID, ids, in.CycleTestUpdate, email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	detail := fmt.Sprintf("%s: %d tests changed", cy.Name, n)
	if in.AssigneeID != nil {
		detail += ", tester " + email
		if email == "" {
			detail += "none"
		}
	}
	if in.EstimateMinutes != nil {
		detail += fmt.Sprintf(", estimate %d min", *in.EstimateMinutes)
	}
	s.audit(ctx, p, "cycle.plan", "cycle", cy.ID, detail)
	s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "plan")
	if in.AssigneeID != nil && *in.AssigneeID != "" && n > 0 {
		s.notifyUsers(ctx, p, []string{*in.AssigneeID}, model.Notification{ProjectID: cy.ProjectID, Event: "assigned", Title: cy.Name, EntityType: "cycle", EntityID: cy.ID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n})
}

// handleRemoveCycleTests: {"ids": [row ids]}.
func (s *Server) handleRemoveCycleTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cy, err := s.Store.GetCycle(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(in.IDs) == 0 {
		s.fail(w, r, badRequest("choose rows: ids"))
		return
	}
	n, err := s.Store.RemoveCycleTests(ctx, p.TenantID(), cy.ID, in.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "cycle.plan", "cycle", cy.ID, fmt.Sprintf("%s: %d tests removed", cy.Name, n))
	s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "plan")
	writeJSON(w, http.StatusOK, map[string]any{"removed": n})
}

func (s *Server) handleCycleProgress(w http.ResponseWriter, r *http.Request, p *Principal) {
	rep, err := s.Store.CycleReport(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// ---------------------------------------------------------------- schedules

type cycleScheduleReq struct {
	NamePattern     *string `json:"name_pattern"`
	TemplateCycleID *string `json:"template_cycle_id"`
	Query           *string `json:"query"`
	OwnerID         *string `json:"owner_id"`
	Frequency       *string `json:"frequency"`
	Weekday         *int    `json:"weekday"`
	DayOfMonth      *int    `json:"day_of_month"`
	TimeOfDay       *string `json:"time"`
	Timezone        *string `json:"timezone"`
	DurationDays    *int    `json:"duration_days"`
	AutoStart       *bool   `json:"auto_start"`
	Enabled         *bool   `json:"enabled"`
}

func (in *cycleScheduleReq) apply(x *model.CycleSchedule) {
	for _, f := range []struct{ src, dst *string }{{in.NamePattern, &x.NamePattern}, {in.Query, &x.Query}, {in.OwnerID, &x.OwnerID},
		{in.Frequency, &x.Frequency}, {in.TimeOfDay, &x.TimeOfDay}, {in.Timezone, &x.Timezone}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	for _, f := range []struct{ src, dst *int }{{in.Weekday, &x.Weekday}, {in.DayOfMonth, &x.DayOfMonth}, {in.DurationDays, &x.DurationDays}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if in.TemplateCycleID != nil {
		x.TemplateCycleID = optID(in.TemplateCycleID)
	}
	if in.AutoStart != nil {
		x.AutoStart = *in.AutoStart
	}
	if in.Enabled != nil {
		x.Enabled = *in.Enabled
	}
}

func (s *Server) handleListCycleSchedules(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListCycleSchedules(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// checkScheduleQuery parses the query of a schedule.
func checkScheduleQuery(x *model.CycleSchedule) error {
	_, err := query.Parse(x.Query, testSchema)
	return err
}

func (s *Server) handleCreateCycleSchedule(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in cycleScheduleReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	x := &model.CycleSchedule{ProjectID: pr.ID, OwnerID: p.User.ID, Enabled: true, CreatedBy: p.User.Email,
		Schedule: model.Schedule{Frequency: model.FrequencyWeekly, Weekday: 1, DayOfMonth: 1, TimeOfDay: "06:00", Timezone: "UTC"}}
	if tz := p.User.Preferences.Timezone; tz != "" {
		x.Timezone = tz
	}
	in.apply(x)
	if err := checkScheduleQuery(x); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.checkTester(r.Context(), p.TenantID(), pr.ID, x.OwnerID, "owner"); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateCycleSchedule(r.Context(), p.TenantID(), x); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "cycle_schedule.create", "cycle_schedule", x.ID, x.NamePattern+" "+x.Frequency)
	out, err := s.Store.GetCycleSchedule(r.Context(), p.TenantID(), x.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateCycleSchedule(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, err := s.Store.GetCycleSchedule(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in cycleScheduleReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	owner := x.OwnerID
	in.apply(x)
	if err := checkScheduleQuery(x); err != nil {
		s.fail(w, r, err)
		return
	}
	if x.OwnerID != owner {
		if err := s.checkTester(r.Context(), p.TenantID(), x.ProjectID, x.OwnerID, "owner"); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := s.Store.UpdateCycleSchedule(r.Context(), p.TenantID(), x); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "cycle_schedule.update", "cycle_schedule", x.ID, x.NamePattern+" "+x.Frequency)
	out, err := s.Store.GetCycleSchedule(r.Context(), p.TenantID(), x.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteCycleSchedule(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, err := s.Store.GetCycleSchedule(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteCycleSchedule(r.Context(), p.TenantID(), x.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "cycle_schedule.delete", "cycle_schedule", x.ID, x.NamePattern)
	w.WriteHeader(http.StatusNoContent)
}

// handleRunCycleSchedule creates the schedule's next cycle now (the
// schedule keeps its next run).
func (s *Server) handleRunCycleSchedule(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, err := s.Store.GetCycleSchedule(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cy, err := s.cycleFromSchedule(r.Context(), p, x, s.now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.RecordCycleScheduleRun(r.Context(), x.ID, s.now(), &cy.ID, "", x.NextRunAt); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, cy)
}

// cycleName fills the name pattern of a schedule.
func cycleName(pattern, day string, n int) string {
	name := strings.ReplaceAll(pattern, "{date}", day)
	name = strings.ReplaceAll(name, "{n}", strconv.Itoa(n))
	if name == pattern && !strings.Contains(pattern, "{") {
		name = pattern + " " + day
	}
	return name
}

// cycleFromSchedule creates a cycle from a schedule as principal p (the
// schedule's owner, or whoever runs it now): the template's fields and
// plan, the tests matching the query, starting today (in the schedule's
// time zone) for duration_days; with auto_start it is started.
func (s *Server) cycleFromSchedule(ctx context.Context, p *Principal, x *model.CycleSchedule, now time.Time) (*model.TestCycle, error) {
	pr, err := s.Store.GetProject(ctx, p.TenantID(), x.ProjectID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(x.Timezone)
	if err != nil {
		loc = time.UTC
	}
	start := now.In(loc)
	day := start.Format(time.DateOnly)
	end := start.AddDate(0, 0, x.DurationDays-1).Format(time.DateOnly)
	cy := &model.TestCycle{ProjectID: pr.ID, OwnerID: x.OwnerID, StartDate: day, EndDate: end, CreatedBy: p.User.Email, ScheduleID: &x.ID}
	var tpl *model.TestCycle
	if x.TemplateCycleID != nil {
		if tpl, err = s.Store.GetCycle(ctx, p.TenantID(), *x.TemplateCycleID); err != nil {
			return nil, fmt.Errorf("the template cycle: %w", err)
		}
		cy.Goal, cy.Description, cy.ReleaseID, cy.VariantID, cy.BuildID = tpl.Goal, tpl.Description, tpl.ReleaseID, tpl.VariantID, tpl.BuildID
		cy.Environment, cy.ConfigurationIDs = tpl.Environment, tpl.ConfigurationIDs
	}
	var ids []string
	if x.Query != "" {
		if !p.Can(rbac.TestRead) {
			return nil, errors.New("the owner may no longer read the tests")
		}
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			return nil, err
		}
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		if err != nil {
			return nil, err
		}
		list, err := s.filterTests(r, p, pr, pc, x.Query, "", true)
		if err != nil {
			return nil, err
		}
		for _, t := range list {
			ids = append(ids, t.ID)
		}
	}
	// a unique name: the pattern, then with a counter
	base := cycleName(x.NamePattern, day, x.Runs+1)
	err = s.Store.InTx(ctx, func(ctx context.Context) error {
		for i := 0; ; i++ {
			cy.Name = base
			if i > 0 {
				cy.Name = fmt.Sprintf("%s (%d)", base, i+1)
			}
			err := s.Store.Savepoint(ctx, func() error { return s.Store.CreateCycle(ctx, p.TenantID(), cy) })
			if errors.Is(err, store.ErrConflict) && i < 20 {
				continue
			}
			if err != nil {
				return err
			}
			break
		}
		if tpl != nil {
			if _, err := s.Store.CopyCyclePlan(ctx, p.TenantID(), tpl.ID, cy.ID, p.User.Email); err != nil {
				return err
			}
		}
		if len(ids) > 0 {
			in := make([]store.CycleTestInput, len(ids))
			for i, id := range ids {
				in[i] = store.CycleTestInput{TestID: id}
			}
			if _, err := s.Store.AddCycleTests(ctx, p.TenantID(), cy.ID, p.User.Email, in); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.audit(ctx, p, "cycle.create", "cycle", cy.ID, cy.Name+" (schedule "+x.NamePattern+")")
	s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "create")
	if x.AutoStart {
		if err := s.startCycle(ctx, p, cy); err != nil {
			return nil, err
		}
	}
	return s.Store.GetCycle(ctx, p.TenantID(), cy.ID)
}

// startCycle makes a planned cycle active and creates its run.
func (s *Server) startCycle(ctx context.Context, p *Principal, cy *model.TestCycle) error {
	cy.Status = model.CycleActive
	if err := s.Store.UpdateCycle(ctx, p.TenantID(), cy); err != nil {
		return err
	}
	s.audit(ctx, p, "cycle.update", "cycle", cy.ID, cy.Name+" active")
	if p.Can(rbac.RunWrite) {
		if _, err := s.startCycleRun(ctx, p, cy); err != nil {
			return err
		}
	}
	s.recordCycle(ctx, p.TenantID(), p.User.Email, cy.ID, "update")
	return nil
}

// cyclePrincipal is the user a background job acts as for a cycle or
// schedule: the owner, with the permissions to plan in the project.
func (s *Server) cyclePrincipal(ctx context.Context, tenantID, userID, projectID string) (*Principal, error) {
	p, err := s.principalFor(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("the owner: %w", err)
	}
	acc, err := s.projectAccess(ctx, p, projectID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, errors.New("the owner may no longer see the project")
	}
	p.scope(projectID, acc)
	if !p.Can(rbac.CyclePlan) {
		return nil, errors.New("the owner may no longer plan test cycles")
	}
	return p, nil
}

// RunCycleJobs starts the cycles whose scheduled start has come and
// creates the cycles of the recurring schedules that are due. It returns
// how many cycles it started or created. A schedule that missed several
// runs (server down) creates one cycle; its next run is computed from now.
func (s *Server) RunCycleJobs(ctx context.Context) (int, error) {
	now := s.now()
	n := 0
	due, err := s.Store.DueCycleStarts(ctx, now)
	if err != nil {
		return 0, err
	}
	for i := range due {
		d := &due[i]
		owner := d.Cycle.OwnerID
		if owner == "" {
			u, err := s.Store.GetUserByEmail(ctx, d.Cycle.CreatedBy)
			if err == nil {
				owner = u.ID
			}
		}
		p, err := s.cyclePrincipal(ctx, d.TenantID, owner, d.Cycle.ProjectID)
		if err == nil {
			err = s.startCycle(ctx, p, &d.Cycle)
		}
		if err != nil {
			s.Log.Warn("scheduled cycle start", "cycle", d.Cycle.ID, "err", err)
			// do not try again every minute: the planner starts it by hand
			d.Cycle.ScheduledStartAt = nil
			_ = s.Store.UpdateCycle(ctx, d.TenantID, &d.Cycle)
			_ = s.Store.Audit(ctx, model.AuditEntry{TenantID: d.TenantID, Action: "cycle.start_failed", EntityType: "cycle", EntityID: d.Cycle.ID, Details: d.Cycle.Name + ": " + err.Error()})
			continue
		}
		n++
	}
	sched, err := s.Store.DueCycleSchedules(ctx, now)
	if err != nil {
		return n, err
	}
	for i := range sched {
		x := &sched[i].Schedule
		var next *time.Time
		if t, err := x.Schedule.NextRun(now); err == nil {
			next = &t
		}
		// record the next run first, so that a crash does not create twice
		if err := s.Store.RecordCycleScheduleRun(ctx, x.ID, now, nil, x.LastError, next); err != nil {
			return n, err
		}
		var cy *model.TestCycle
		p, err := s.cyclePrincipal(ctx, sched[i].TenantID, x.OwnerID, x.ProjectID)
		if err == nil {
			cy, err = s.cycleFromSchedule(ctx, p, x, now)
		}
		msg := ""
		var id *string
		if err != nil {
			msg = err.Error()
			s.Log.Warn("cycle schedule", "schedule", x.ID, "err", err)
		} else {
			id = &cy.ID
			n++
		}
		if err := s.Store.RecordCycleScheduleRun(ctx, x.ID, now, id, msg, next); err != nil {
			return n, err
		}
	}
	return n, nil
}

// cycleReportData evaluates a cycle of pr for the cycle report: id, else
// the first active or planned cycle (as listed), else the newest one (nil
// when the project has none).
func (s *Server) cycleReportData(ctx context.Context, p *Principal, pr *model.Project, id string) (*report.CycleData, error) {
	if id == "" {
		list, err := s.Store.ListCycles(ctx, p.TenantID(), pr.ID)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			if c.Status != model.CycleClosed {
				id = c.ID
				break
			}
		}
		if id == "" && len(list) > 0 {
			id = list[0].ID
		}
		if id == "" {
			return nil, nil
		}
	}
	cy, err := s.Store.GetCycle(ctx, p.TenantID(), id)
	if err != nil || cy.ProjectID != pr.ID {
		return nil, badRequest("cycle %q does not belong to project %s", id, pr.Key)
	}
	rep, err := s.Store.CycleReport(ctx, p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	plan, err := s.Store.CycleTests(ctx, p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	people := map[string]string{}
	look := s.peopleLookup(ctx)
	add := func(uid string) {
		if v := look(uid); len(v) > 0 {
			people[uid] = v[len(v)-1] // the name, else the e-mail
		}
	}
	add(cy.OwnerID)
	for _, x := range plan {
		if _, ok := people[x.AssigneeID]; !ok {
			add(x.AssigneeID)
		}
	}
	return &report.CycleData{Report: rep, Plan: plan, People: people}, nil
}
