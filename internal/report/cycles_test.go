package report

import (
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

func cycleSample() *CycleData {
	fin := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	exec := "e1"
	two, one := 2, 1
	cy := &model.TestCycle{ID: "c1", Name: "Sprint 14 regression", Goal: "Regression", Status: model.CycleActive, OwnerID: "u1",
		StartDate: "2026-03-01", EndDate: "2026-03-03", ReleaseName: "1.0",
		Progress: model.CycleProgress{Planned: 2, Executed: 1, Passed: 1, EstimatedMin: 30, RemainingMin: 20, ActualMS: 600000, EstimatedDone: 10}}
	plan := []model.TestCycleTest{
		{ID: "x1", TestKey: "T-1", TestTitle: "Login", AssigneeID: "u1", EstimateMinutes: 10, Status: model.ResultPassed, ExecutionID: &exec, FinishedAt: &fin, DurationMS: 600000, ExecutedBy: "ann@example.com"},
		{ID: "x2", TestKey: "T-2", TestTitle: "Logout", EstimateMinutes: 20, Status: model.ResultNotRun},
	}
	return &CycleData{
		Report: &model.CycleReport{Cycle: cy, Testers: []model.CycleTesterLoad{{AssigneeID: "u1", Planned: 1, Executed: 1, Passed: 1, EstimatedMin: 10, ActualMS: 600000}, {Planned: 1, EstimatedMin: 20, RemainingMin: 20}},
			Burndown: []model.CycleBurndownPoint{{Day: "2026-03-01", Remaining: &two, Ideal: 2}, {Day: "2026-03-02", Remaining: &one, Ideal: 1}, {Day: "2026-03-03", Ideal: 0}}},
		Plan:   plan,
		People: map[string]string{"u1": "Ann Tester"},
	}
}

func TestCycleReport(t *testing.T) {
	d := sampleData()
	ds, err := Build(CycleReportID, d, nil)
	if err != nil || len(ds.Sections) != 1 || !strings.Contains(ds.Sections[0].Rows[0][1], "No test cycle") {
		t.Fatal(err, ds)
	}
	d.Cycle = cycleSample()
	ds, err = Build(CycleReportID, d, map[string]string{"cycle_id": "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Sections) != 4 || ds.Sections[1].Rows[0][0] != "Ann Tester" || ds.Sections[1].Rows[1][0] != "Not assigned" {
		t.Fatal(ds.Sections)
	}
	if ds.Sections[2].Rows[2][1] != "" || ds.Sections[3].Rows[0][6] != "Passed" || ds.Sections[3].Rows[0][9] != "10m0s" {
		t.Fatal(ds.Sections[2:])
	}
	renderAll(t, ds)

	failOnMissing(t)
	d.Lang = Swedish
	ds, err = Build(CycleReportID, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkSwedish(t, ds)
	if ds.Title != "Testcykelrapport" || ds.Sections[1].Rows[1][0] != "Ej tilldelad" {
		t.Fatal(ds.Title, ds.Sections[1].Rows)
	}
}
