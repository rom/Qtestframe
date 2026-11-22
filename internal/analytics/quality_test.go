package analytics

import (
	"testing"
	"time"
)

func day(d, h int) time.Time { return time.Date(2026, 3, d, h, 0, 0, 0, time.UTC) }

func tp(t time.Time) *time.Time { return &t }

// qualityInput: now 2026-03-31, range from 03-02 (four weeks and a bit).
//
//	R1 (folder A, cat "Risk: High") changed 3 times, R2 (folder A) once, R3 (root) never, R4 rejected
//	R1 verified by T1, R2 by T2; T3 never run, T4 deprecated
//	D1 critical on R1, created 03-03, resolved 03-04 (24 h), found in test
//	D2 major on T2 (→ R2) found in production, created 03-20, open
//	D3 minor on execution E1 of T1 (→ R1), created 03-11 after release 1.0 (released 03-10, scope R1), resolved 03-13 (48 h)
//	D4 rejected, no links
//	executions: E1 T1 passed 03-30 (1 h), E2 T2 failed 03-01 (before the range, 2 h), E3 T1 passed 03-05 (untimed)
//	reviews: decided after 2 h and 10 h, one pending since 03-29, one cancelled
func qualityInput() QualityInput {
	return QualityInput{
		Now: day(31, 12), From: day(2, 0), StaleDays: 20,
		Requirements: []QualityRequirement{
			{ID: "r1", Key: "R-1", Title: "Login", Description: "The system MUST lock an account after 5 failed attempts.", Status: "approved", Folder: "A", Categories: []string{"Risk: High"}, CreatedAt: day(1, 0), Tests: []string{"t1"}},
			{ID: "r2", Key: "R-2", Title: "Fast", Description: "The system SHOULD be fast and user-friendly.", Status: "draft", Folder: "A", CreatedAt: day(1, 0), Tests: []string{"t2"}},
			{ID: "r3", Key: "R-3", Title: "Export", Description: "The system MUST export a report as PDF.", Status: "approved", CreatedAt: day(20, 0)},
			{ID: "r4", Key: "R-4", Title: "Old", Description: "The system MAY do TBD.", Status: "rejected", CreatedAt: day(1, 0)},
		},
		Tests: []QualityTest{{ID: "t1", Key: "T-1", Title: "T1", Status: "ready"}, {ID: "t2", Key: "T-2", Title: "T2", Status: "ready"}, {ID: "t3", Key: "T-3", Title: "T3"}, {ID: "t4", Key: "T-4", Status: "deprecated"}},
		Revisions: []QualityRevision{
			{RequirementID: "r1", Action: "create", At: day(1, 0), HasText: true, Title: "Login", Description: "The system SHOULD be fast.", Status: "draft"},
			{RequirementID: "r1", Action: "update", At: day(3, 0), HasText: true, Title: "Login", Description: "The system SHOULD be fast.", Status: "draft"},
			{RequirementID: "r1", Action: "update", At: day(10, 0), HasText: true, Title: "Login", Description: "The system MUST lock an account after 5 failed attempts.", Status: "approved"},
			{RequirementID: "r1", Action: "bulk_update", At: day(24, 0)},
			{RequirementID: "r1", Action: "move", At: day(25, 0)},
			{RequirementID: "r2", Action: "update", At: day(17, 0)},
			{RequirementID: "gone", Action: "update", At: day(17, 0)},
		},
		Defects: []QualityDefect{
			{ID: "d1", Key: "D-1", Title: "Crash", Severity: "critical", Status: "resolved", FoundIn: "test", CreatedAt: day(3, 0), ResolvedAt: tp(day(4, 0)), Requirements: []string{"r1"}},
			{ID: "d2", Key: "D-2", Title: "Slow", Severity: "major", Status: "open", FoundIn: "production", CreatedAt: day(20, 0), Tests: []string{"t2"}},
			{ID: "d3", Key: "D-3", Title: "Typo", Severity: "minor", Status: "closed", CreatedAt: day(11, 0), ResolvedAt: tp(day(13, 0)), Executions: []string{"e1"}},
			{ID: "d4", Key: "D-4", Title: "Not a bug", Severity: "minor", Status: "rejected", CreatedAt: day(5, 0), ResolvedAt: tp(day(6, 0))},
		},
		Executions: []QualityExecution{
			{ID: "e1", TestID: "t1", RunID: "run1", Status: "passed", At: day(30, 0), DurationMS: 3600000},
			{ID: "e2", TestID: "t2", RunID: "run2", Status: "failed", At: day(1, 0), DurationMS: 7200000},
			{ID: "e3", TestID: "t1", RunID: "run1", Status: "passed", At: day(5, 0)},
		},
		Releases: []QualityRelease{{ID: "rel", Name: "1.0", Status: "released", ReleasedAt: tp(day(10, 0)), RequirementIDs: []string{"r1"}}, {ID: "rel2", Name: "2.0", Status: "planned"}},
		Reviews: []QualityReview{
			{Status: "approved", CreatedAt: day(5, 0), DecidedAt: tp(day(5, 2))},
			{Status: "changes_requested", CreatedAt: day(12, 0), DecidedAt: tp(day(12, 10))},
			{Status: "pending", CreatedAt: day(29, 12)},
			{Status: "cancelled", CreatedAt: day(6, 0), DecidedAt: tp(day(6, 1))},
		},
	}
}

func TestQuality(t *testing.T) {
	q := Quality(qualityInput())
	if q.Period != PeriodWeek || q.From != "2026-03-02" || q.To != "2026-03-31" || len(q.Periods) != 5 || q.Periods[0] != "2026-03-02" {
		t.Fatalf("periods %+v", q.Periods)
	}
	v := q.Volatility
	if v.Changes != 4 || v.Changed != 2 || v.Requirements != 4 || v.MostChanged[0].ID != "r1" || v.MostChanged[0].Count != 3 || v.Trend[0].Count != 1 || v.Trend[0].Total != 3 {
		t.Fatalf("volatility %+v", v)
	}
	if v.PerRequirement != 0.2 {
		t.Fatal(v.PerRequirement)
	}
	d := q.DefectDensity
	if d.Defects != 3 || d.Requirements != 3 || d.PerRequirement != 1 || d.ByRequirement[0].ID != "r1" || d.ByRequirement[0].Count != 2 {
		t.Fatalf("density %+v", d)
	}
	if d.ByFolder[0].Name != "A" || d.ByFolder[0].Defects != 3 || d.ByFolder[0].Requirements != 2 || d.ByFolder[0].Density != 1.5 || len(d.ByCategory) != 1 || d.ByCategory[0].Defects != 2 {
		t.Fatalf("groups %+v %+v", d.ByFolder, d.ByCategory)
	}
	e := q.Escaped
	if e.Defects != 4 || e.Escaped != 2 || e.Production != 1 || e.AfterRelease != 1 || e.Percent != 50 || len(e.ByRelease) != 1 || e.ByRelease[0].Escaped != 1 {
		t.Fatalf("escaped %+v", e)
	}
	if e.ByPhase[0].Value != "test" || e.ByPhase[len(e.ByPhase)-1].Value != "unknown" || e.Items[0].ID != "d2" && e.Items[1].ID != "d2" {
		t.Fatalf("phases %+v", e.ByPhase)
	}
	f := q.Effectiveness
	if f.Executions != 2 || f.Tests != 1 || f.Hours != 1 || f.Defects != 2 || f.PerTest != 2 || f.PerHour != 2 || f.UntimedPercent != 50 || f.TopTests[0].Count != 1 {
		t.Fatalf("effectiveness %+v", f)
	}
	a := q.ResultAge
	if a.Tests != 3 || a.WithResult != 2 || a.Never != 1 || a.Stale != 1 || a.StalePercent != 50 || a.Buckets[0].Count != 1 || a.Buckets[2].Count != 1 || a.Oldest[0].ID != "t2" {
		t.Fatalf("age %+v", a)
	}
	r := q.ReviewTurnaround
	if r.Count != 2 || r.MeanHours != 6 || r.MedianHours != 6 || r.Pending != 1 || r.OldestPendingDays != 2 {
		t.Fatalf("reviews %+v", r)
	}
	x := q.FixTime
	if x.Count != 2 || x.MeanHours != 36 || x.Open != 1 || len(x.BySeverity) != 2 || x.BySeverity[0].Severity != "critical" || x.BySeverity[0].MeanHours != 24 {
		t.Fatalf("fix %+v", x)
	}
	rq := q.RequirementQuality
	if rq.Requirements != 3 || rq.WithWarnings != 1 || rq.Items[0].ID != "r2" || len(rq.ByRule) == 0 {
		t.Fatalf("quality %+v", rq)
	}
	// trend: in the first week R1's text was vague (SHOULD be fast)
	if tr := rq.Trend; tr[0].Total != 2 || tr[0].Count != 2 || tr[len(tr)-1].Total != 3 || tr[len(tr)-1].Count != 1 {
		t.Fatalf("quality trend %+v", tr)
	}
}

func TestQualityPeriods(t *testing.T) {
	in := qualityInput()
	in.From = day(31, 12).AddDate(0, 0, -200)
	q := Quality(in)
	if q.Period != PeriodMonth || q.Periods[0] != "2025-09-01" || q.Periods[len(q.Periods)-1] != "2026-03-01" {
		t.Fatal(q.Period, q.Periods)
	}
	in.Period = PeriodWeek
	in.From = in.Now // invalid: 90 days
	in.StaleDays = 0
	q = Quality(in)
	if q.Period != PeriodWeek || q.StaleDays != 30 || q.From != "2025-12-31" {
		t.Fatal(q.Period, q.From)
	}
	empty := Quality(QualityInput{Now: day(31, 0)})
	if empty.Volatility.Changes != 0 || empty.ResultAge.Tests != 0 || empty.Escaped.Percent != 0 || empty.FixTime.Count != 0 {
		t.Fatalf("%+v", empty)
	}
	if PeriodStart(day(1, 5), PeriodWeek).Weekday() != time.Monday || quantile(nil, 0.5) != 0 {
		t.Fatal("helpers")
	}
}
