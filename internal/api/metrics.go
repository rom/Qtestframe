package api

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Trends over time, flaky tests and release burn-up (see docs/api.md
// "Dashboard and metrics").

func (s *Server) routesMetrics(h routeFunc) {
	h("GET /api/v1/projects/{pid}/metrics/trends", s.handleTrends, "project.read")
	h("GET /api/v1/projects/{pid}/flaky-tests", s.handleFlakyTests, "project.read")
	h("GET /api/v1/releases/{id}/burnup", s.handleReleaseBurnup, "project.read")
}

// TrendRanges are the accepted values of the range parameter (days).
var TrendRanges = []int{30, 90, 365}

// recordSnapshot records today's snapshot of a project (and backfills a
// first one); failures are logged, never shown to the caller.
func (s *Server) recordSnapshot(ctx context.Context, tenantID, projectID string, refresh bool) {
	if _, err := s.Store.RecordSnapshot(ctx, tenantID, projectID, refresh); err != nil {
		s.Log.Warn("metric snapshot failed", "project", projectID, "err", err)
	}
}

// trendPoint is one day of the trends response.
type trendPoint struct {
	Day          string           `json:"day"`
	Source       string           `json:"source"`
	Requirements int              `json:"requirements"`
	Covered      int              `json:"covered"`
	CoveragePct  float64          `json:"coverage_pct"`
	Passed       int              `json:"passed"`
	Tests        int              `json:"tests"`
	Executed     int              `json:"executed"`
	PassRate     float64          `json:"pass_rate"`
	OpenDefects  int              `json:"open_defects"`
	Data         store.MetricData `json:"data"`
}

func (s *Server) handleTrends(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	today, _ := time.Parse(time.DateOnly, store.Day(s.Store.Now()))
	days := 30
	q := r.URL.Query()
	var from time.Time
	if since := q.Get("since"); since != "" {
		t, err := time.Parse(time.DateOnly, since)
		if err != nil || t.After(today) {
			s.fail(w, r, badRequest("since must be a date YYYY-MM-DD not in the future"))
			return
		}
		if t.Before(today.AddDate(0, 0, -store.MaxTrendDays)) {
			t = today.AddDate(0, 0, -store.MaxTrendDays)
		}
		from = t
		days = int(today.Sub(t).Hours()/24) + 1
	} else {
		if v := q.Get("range"); v != "" {
			n, err := strconv.Atoi(v)
			valid := false
			for _, d := range TrendRanges {
				valid = valid || d == n
			}
			if err != nil || !valid {
				s.fail(w, r, badRequest("range must be 30, 90 or 365"))
				return
			}
			days = n
		}
		from = today.AddDate(0, 0, 1-days)
	}
	s.recordSnapshot(ctx, p.TenantID(), pr.ID, false)
	snaps, err := s.Store.ListSnapshots(ctx, p.TenantID(), pr.ID, store.Day(from), store.Day(today))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	points := make([]trendPoint, 0, len(snaps))
	for _, sn := range snaps {
		d := sn.Data
		open := 0
		for _, n := range d.OpenDefects {
			open += n
		}
		points = append(points, trendPoint{
			Day: sn.Day, Source: sn.Source, Requirements: d.Requirements, Covered: d.Covered, CoveragePct: d.CoveragePct(),
			Passed: d.Coverage[model.ResultPassed], Tests: d.Tests,
			Executed: d.Tests - d.Results[model.ResultNotRun] - d.Results[model.ResultInProgress], PassRate: d.PassRate(),
			OpenDefects: open, Data: d,
		})
	}
	weeks, err := s.Store.DefectWeeks(ctx, p.TenantID(), pr.ID, from, today)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": store.Day(from), "to": store.Day(today), "days": days,
		"points": points, "defect_weeks": weeks,
	})
}

func (s *Server) handleFlakyTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	fl, err := s.Store.FlakyTests(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < len(fl) {
		fl = fl[:n]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": fl, "window": store.FlakyWindow, "min_flips": store.FlakyMinFlips,
	})
}

func (s *Server) handleReleaseBurnup(w http.ResponseWriter, r *http.Request, p *Principal) {
	b, err := s.Store.ReleaseBurnup(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// ---------------------------------------------------------------- dashboard extras

// groupCoverage is the coverage of the requirements in a folder or
// category.
type groupCoverage struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Dimension string         `json:"dimension,omitempty"`
	Total     int            `json:"total"`
	Coverage  map[string]int `json:"coverage"`
}

// MaxCoverageGroups bounds the folders / categories on the dashboard (the
// largest first).
const MaxCoverageGroups = 12

func sortGroups(m map[string]*groupCoverage) []groupCoverage {
	out := make([]groupCoverage, 0, len(m))
	for _, g := range m {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > MaxCoverageGroups {
		out = out[:MaxCoverageGroups]
	}
	return out
}

// coverageGroups returns the coverage by top-level folder ("" = no folder)
// and by requirement category of decorated requirements.
func (pc *projectContext) coverageGroups(reqs []model.Requirement) (folders, categories []groupCoverage) {
	parent := map[string]*string{}
	names := map[string]string{}
	for _, f := range pc.folders {
		parent[f.ID] = f.ParentID
		names[f.ID] = f.Name
	}
	top := func(id *string) string {
		if id == nil {
			return ""
		}
		cur := *id
		for i := 0; i < 100; i++ {
			p := parent[cur]
			if p == nil {
				break
			}
			cur = *p
		}
		return cur
	}
	fm, cm := map[string]*groupCoverage{}, map[string]*groupCoverage{}
	add := func(m map[string]*groupCoverage, id, name, dim, cov string) {
		g := m[id]
		if g == nil {
			g = &groupCoverage{ID: id, Name: name, Dimension: dim, Coverage: map[string]int{}}
			m[id] = g
		}
		g.Total++
		g.Coverage[cov]++
	}
	for _, r := range reqs {
		f := top(r.FolderID)
		add(fm, f, names[f], "", r.Coverage)
		for _, cid := range r.CategoryIDs {
			if c, ok := pc.categories[cid]; ok {
				add(cm, cid, c.Name, c.Dimension, r.Coverage)
			}
		}
	}
	return sortGroups(fm), sortGroups(cm)
}

// MaxActiveRuns bounds the active runs on the dashboard (newest first).
const MaxActiveRuns = 8

// activeRuns returns the planned and in-progress runs, newest first.
func activeRuns(runs []model.TestRun) []model.TestRun {
	out := []model.TestRun{}
	for _, rn := range runs {
		if rn.Status == "in_progress" || rn.Status == "planned" {
			out = append(out, rn)
			if len(out) == MaxActiveRuns {
				break
			}
		}
	}
	return out
}
