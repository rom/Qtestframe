package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Test configurations of a project, coverage per configuration and the
// test data of data-driven tests (see docs/api.md "Configurations" and
// "Test data").

const maxBulkConfigurations = 200

func (s *Server) routesConfigurations(h routeFunc) {
	h("GET /api/v1/projects/{pid}/configurations", s.handleListConfigurations, "project.read")
	h("POST /api/v1/projects/{pid}/configurations", s.handleCreateConfiguration, "project.update")
	h("POST /api/v1/projects/{pid}/configurations/bulk", s.atomic(s.handleBulkCreateConfigurations), "project.update")
	h("DELETE /api/v1/projects/{pid}/configurations/bulk", s.atomic(s.handleBulkDeleteConfigurations), "project.update")
	h("GET /api/v1/configurations/{id}", s.handleGetConfiguration, "project.read")
	h("PATCH /api/v1/configurations/{id}", s.handleUpdateConfiguration, "project.update")
	h("DELETE /api/v1/configurations/{id}", s.handleDeleteConfiguration, "project.update")
	h("GET /api/v1/requirements/{id}/coverage", s.handleRequirementCoverage, "requirement.read")
	h("GET /api/v1/tests/{id}/data", s.handleGetTestData, "test.read")
	h("PUT /api/v1/tests/{id}/data", s.handlePutTestData, "test.write")
}

func (s *Server) handleListConfigurations(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListConfigurations(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

type configurationInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Values      []model.DimensionValue `json:"values"`
}

func (s *Server) handleCreateConfiguration(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in configurationInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	c := &model.Configuration{ProjectID: pr.ID, Name: in.Name, Description: in.Description, Values: in.Values, CreatedBy: p.User.Email}
	if err := s.Store.CreateConfiguration(r.Context(), p.TenantID(), c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "configuration.create", "configuration", c.ID, c.Name)
	writeJSON(w, http.StatusCreated, c)
}

// handleBulkCreateConfigurations: POST /projects/{pid}/configurations/bulk
// {"configurations": [...], "skip_existing": bool} creates several
// configurations at once (e.g. every combination of some dimensions); with
// skip_existing, names that already exist are skipped instead of failing.
func (s *Server) handleBulkCreateConfigurations(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Configurations []configurationInput `json:"configurations"`
		SkipExisting   bool                 `json:"skip_existing"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Configurations) == 0 || len(req.Configurations) > maxBulkConfigurations {
		s.fail(w, r, badRequest("give 1 to %d configurations", maxBulkConfigurations))
		return
	}
	existing, err := s.Store.ListConfigurations(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	have := map[string]bool{}
	for _, c := range existing {
		have[strings.ToLower(c.Name)] = true
	}
	created := []model.Configuration{}
	skipped := []string{}
	for _, in := range req.Configurations {
		c := &model.Configuration{ProjectID: pr.ID, Name: in.Name, Description: in.Description, Values: in.Values, CreatedBy: p.User.Email}
		if err := store.ValidateConfiguration(c); err != nil {
			s.fail(w, r, err)
			return
		}
		if req.SkipExisting && have[strings.ToLower(c.Name)] {
			skipped = append(skipped, c.Name)
			continue
		}
		if err := s.Store.CreateConfiguration(r.Context(), p.TenantID(), c); err != nil {
			s.fail(w, r, err)
			return
		}
		have[strings.ToLower(c.Name)] = true
		created = append(created, *c)
	}
	s.audit(r.Context(), p, "configuration.bulk_create", "project", pr.ID, fmt.Sprintf("%d created, %d skipped", len(created), len(skipped)))
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "skipped": skipped})
}

// handleBulkDeleteConfigurations: DELETE /projects/{pid}/configurations/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteConfigurations(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > maxBulkConfigurations {
		s.fail(w, r, badRequest("give 1 to %d ids", maxBulkConfigurations))
		return
	}
	n := 0
	for _, id := range uniqStrings(req.IDs) {
		c, err := s.Store.GetConfiguration(r.Context(), p.TenantID(), id)
		if err == nil && c.ProjectID != pr.ID {
			err = fmt.Errorf("configuration %s: %w", id, store.ErrNotFound)
		}
		if err == nil {
			err = s.Store.DeleteConfiguration(r.Context(), p.TenantID(), id)
		}
		if err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", configLabel(c, id), err))
			return
		}
		n++
	}
	s.audit(r.Context(), p, "configuration.bulk_delete", "project", pr.ID, fmt.Sprintf("%d deleted", n))
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

func configLabel(c *model.Configuration, id string) string {
	if c != nil {
		return c.Name
	}
	return id
}

func (s *Server) handleGetConfiguration(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.Store.GetConfiguration(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleUpdateConfiguration(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.Store.GetConfiguration(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Name        *string                 `json:"name"`
		Description *string                 `json:"description"`
		Values      *[]model.DimensionValue `json:"values"`
		Position    *int                    `json:"position"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.Description != nil {
		c.Description = *req.Description
	}
	if req.Values != nil {
		c.Values = *req.Values
	}
	if req.Position != nil {
		c.Position = *req.Position
	}
	if err := s.Store.UpdateConfiguration(r.Context(), p.TenantID(), c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "configuration.update", "configuration", c.ID, c.Name)
	out, err := s.Store.GetConfiguration(r.Context(), p.TenantID(), c.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteConfiguration(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.Store.DeleteConfiguration(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "configuration.delete", "configuration", id, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- coverage

type coverageTest struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
	// Results: configuration id ("" without configuration) → latest result
	// ("" when only planned there).
	Results map[string]string `json:"results"`
	// Result over all configurations (see ConfigResults.Combined).
	Result string `json:"result"`
}

type coverageConfig struct {
	// ID is "" for results without configuration.
	ID       string `json:"id"`
	Name     string `json:"name"`
	Coverage string `json:"coverage"`
}

// handleRequirementCoverage: GET /requirements/{id}/coverage — the
// coverage of a requirement overall and per configuration, with the latest
// result of every linked test in every configuration it is planned in.
func (s *Server) handleRequirementCoverage(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rq, err := s.Store.GetRequirement(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	links, err := s.Store.ListLinks(ctx, p.TenantID(), rq.ProjectID, rq.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.Store.LastConfigResults(ctx, p.TenantID(), rq.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cfgs, err := s.Store.ListConfigurations(ctx, p.TenantID(), rq.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var linked []string
	tests := []coverageTest{}
	used := map[string]bool{}
	for _, l := range links {
		t, err := s.Store.GetTest(ctx, p.TenantID(), l.TestCaseID)
		if err != nil {
			continue
		}
		linked = append(linked, t.ID)
		units := map[string]string{}
		for cfg, st := range res[t.ID] {
			units[cfg] = st
			used[cfg] = true
		}
		tests = append(tests, coverageTest{ID: t.ID, Key: t.Key, Title: t.Title, Results: units, Result: res.Combined(t.ID)})
	}
	byConfig := []coverageConfig{}
	for _, c := range cfgs {
		if used[c.ID] {
			byConfig = append(byConfig, coverageConfig{ID: c.ID, Name: c.Name, Coverage: store.CoverageInConfiguration(linked, res, c.ID)})
		}
	}
	if used[store.NoConfiguration] {
		byConfig = append(byConfig, coverageConfig{ID: store.NoConfiguration, Coverage: store.CoverageInConfiguration(linked, res, store.NoConfiguration)})
	}
	sort.SliceStable(tests, func(i, j int) bool { return naturalKeyLess(tests[i].Key, tests[j].Key) })
	writeJSON(w, http.StatusOK, map[string]any{
		"requirement_id": rq.ID,
		"coverage":       store.CoverageOf(linked, res),
		"configurations": byConfig,
		"tests":          tests,
	})
}

// naturalKeyLess orders keys like P-TC-2 before P-TC-10.
func naturalKeyLess(a, b string) bool {
	ia, ib := strings.LastIndexByte(a, '-'), strings.LastIndexByte(b, '-')
	if ia >= 0 && ib >= 0 && a[:ia] == b[:ib] {
		na, nb := a[ia+1:], b[ib+1:]
		if len(na) != len(nb) {
			return len(na) < len(nb)
		}
		return na < nb
	}
	return a < b
}

// ---------------------------------------------------------------- test data

type testData struct {
	Parameters []string        `json:"parameters"`
	DataRows   []model.DataRow `json:"data_rows"`
}

func (s *Server) handleGetTestData(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.Store.GetTest(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, testData{Parameters: t.Parameters, DataRows: t.DataRows})
}

// handlePutTestData: PUT /tests/{id}/data {"parameters", "data_rows"}
// replaces the test data of a test (a new version of the test).
func (s *Server) handlePutTestData(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	t, err := s.Store.GetTest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in testData
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	before := store.SnapshotTest(t)
	s.ensureInitialRevision(ctx, p, model.KindTest, t.ID)
	t.Parameters, t.DataRows = in.Parameters, in.DataRows
	if err := s.Store.UpdateTest(ctx, p.TenantID(), t, false); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "test.update", "test", t.ID, t.Key+" test data")
	s.recordTest(ctx, p, t.ID, "update")
	s.afterContentChange(ctx, p, model.KindTest, t.ID, t.Key, t.ProjectID, before)
	writeJSON(w, http.StatusOK, testData{Parameters: t.Parameters, DataRows: t.DataRows})
}
