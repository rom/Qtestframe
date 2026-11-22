package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/rom/qtestframe/internal/analytics"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
)

// Quality metrics (GET /projects/{pid}/analytics/quality): volatility,
// defect density, escaped defects, test effectiveness, result age, review
// turnaround, time to fix and requirement quality, see analytics.Quality.

// QualityRanges are the accepted range values (days).
var QualityRanges = []int{30, 90, 180, 365}

// qualityParams are the parameters of the quality metrics.
type qualityParams struct {
	Days, StaleDays int
	Period, Lang    string
}

func parseQualityParams(q interface{ Get(string) string }) (qualityParams, error) {
	out := qualityParams{Days: 90, StaleDays: 30, Period: q.Get("period"), Lang: q.Get("lang")}
	if v := q.Get("range"); v != "" {
		n, err := strconv.Atoi(v)
		valid := false
		for _, d := range QualityRanges {
			valid = valid || d == n
		}
		if err != nil || !valid {
			return out, badRequest("range must be 30, 90, 180 or 365")
		}
		out.Days = n
	}
	if v := q.Get("stale_days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 3650 {
			return out, badRequest("stale_days must be between 1 and 3650")
		}
		out.StaleDays = n
	}
	if out.Period != "" && out.Period != analytics.PeriodWeek && out.Period != analytics.PeriodMonth {
		return out, badRequest("period must be week or month")
	}
	if out.Lang == "auto" {
		out.Lang = ""
	}
	if out.Lang != "" && !reqlang.Supported(out.Lang) {
		return out, badRequest("lang must be auto or one of the analysis languages")
	}
	return out, nil
}

func (s *Server) handleQuality(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	qp, err := parseQualityParams(r.URL.Query())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rep, err := s.qualityReport(r.Context(), p.TenantID(), pr, qp)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// qualityReport loads the project's history and computes the metrics.
func (s *Server) qualityReport(ctx context.Context, tenantID string, pr *model.Project, qp qualityParams) (*analytics.QualityReport, error) {
	now := s.Store.Now().UTC()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1-qp.Days)
	in := analytics.QualityInput{Now: now, From: from, Period: qp.Period, StaleDays: qp.StaleDays, Lang: qp.Lang, EscapedPhase: model.DefectEscapedPhase}
	pc, err := s.loadContext(ctx, tenantID, pr)
	if err != nil {
		return nil, err
	}
	reqs, err := s.Store.ListRequirements(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	for _, rq := range reqs {
		x := analytics.QualityRequirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, Description: rq.Description, Status: rq.Status,
			Folder: pc.folderPath(rq.FolderID), CreatedAt: rq.CreatedAt, Tests: pc.r2t[rq.ID]}
		for _, cid := range rq.CategoryIDs {
			if c, ok := pc.categories[cid]; ok {
				name := c.Name
				if c.Dimension != "" {
					name = c.Dimension + ": " + c.Name
				}
				x.Categories = append(x.Categories, name)
			}
		}
		in.Requirements = append(in.Requirements, x)
	}
	tests, err := s.Store.ListTests(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	for _, t := range tests {
		in.Tests = append(in.Tests, analytics.QualityTest{ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status})
	}
	revs, err := s.Store.QualityRevisions(ctx, tenantID, pr.ID, &from)
	if err != nil {
		return nil, err
	}
	for _, rv := range revs {
		in.Revisions = append(in.Revisions, analytics.QualityRevision{RequirementID: rv.RequirementID, Action: rv.Action, At: rv.At,
			HasText: rv.Title != "" || rv.Description != "", Title: rv.Title, Description: rv.Description, Status: rv.Status})
	}
	defects, err := s.Store.ListDefects(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	for _, d := range defects {
		x := analytics.QualityDefect{ID: d.ID, Key: d.Key, Title: d.Title, Severity: d.Severity, Status: d.Status, FoundIn: d.FoundIn,
			CreatedAt: d.CreatedAt.UTC(), ResolvedAt: d.ResolvedAt}
		for _, l := range d.Links {
			switch l.EntityType {
			case model.KindRequirement:
				x.Requirements = append(x.Requirements, l.EntityID)
			case model.KindTest:
				x.Tests = append(x.Tests, l.EntityID)
			case "execution":
				x.Executions = append(x.Executions, l.EntityID)
			case "run":
				x.Runs = append(x.Runs, l.EntityID)
			}
		}
		in.Defects = append(in.Defects, x)
	}
	execs, err := s.Store.QualityExecutions(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	for _, x := range execs {
		in.Executions = append(in.Executions, analytics.QualityExecution{ID: x.ID, TestID: x.TestID, RunID: x.RunID, Status: x.Status, At: x.At, DurationMS: x.DurationMS})
	}
	rels, err := s.Store.ListReleases(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	scopes, err := s.Store.ReleaseScopes(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	runs, err := s.Store.ListRuns(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	runsOf := map[string][]string{}
	for _, rn := range runs {
		if rn.ReleaseID != nil {
			runsOf[*rn.ReleaseID] = append(runsOf[*rn.ReleaseID], rn.ID)
		}
	}
	for _, rl := range rels {
		in.Releases = append(in.Releases, analytics.QualityRelease{ID: rl.ID, Name: rl.Name, Status: rl.Status, ReleasedAt: rl.ReleasedAt,
			RequirementIDs: scopes[rl.ID], RunIDs: runsOf[rl.ID]})
	}
	reviews, err := s.Store.QualityReviews(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	for _, v := range reviews {
		in.Reviews = append(in.Reviews, analytics.QualityReview{Status: v.Status, CreatedAt: v.CreatedAt, DecidedAt: v.DecidedAt})
	}
	rep := analytics.Quality(in)
	return &rep, nil
}
