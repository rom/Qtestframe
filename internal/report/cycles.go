package report

import (
	"strconv"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// The test cycle report: one cycle's progress (planned against executed,
// passed, failed and blocked, estimated against actual time), the workload
// per tester, the burndown and the plan. Self-contained: it registers its
// template, names and translations here.

// CycleReportID is the id of the test cycle report.
const CycleReportID = "cycle"

// EnumCycleStatus: the states of a test cycle.
const EnumCycleStatus = "cycle_status"

// CycleData is the evaluated cycle of the cycle report: its progress,
// plan, and the names of the people (user id → name or e-mail).
type CycleData struct {
	Report *model.CycleReport
	Plan   []model.TestCycleTest
	People map[string]string
}

func init() {
	templates = append(templates, Template{ID: CycleReportID, Name: "Test cycle report",
		Description: "One test cycle: planned against executed, passed, failed and blocked tests, estimated against actual time, the workload per tester, the burndown and the plan with every result.",
		Params:      []Param{{Name: "cycle_id", Type: "cycle", Description: "the test cycle (default: the newest active or planned cycle)"}},
		build:       buildCycleReport})
	templateNames[Swedish][CycleReportID] = "Testcykelrapport"
	for k, v := range cycleTexts {
		if _, ok := texts[Swedish][k]; !ok {
			texts[Swedish][k] = v
		}
	}
	enums[Swedish][EnumCycleStatus] = map[string]string{model.CyclePlanned: "Planerad", model.CycleActive: "Pågår", model.CycleClosed: "Avslutad"}
	enums[English][EnumCycleStatus] = map[string]string{model.CyclePlanned: "Planned", model.CycleActive: "Active", model.CycleClosed: "Closed"}
}

var cycleTexts = map[string]string{
	"Test cycle":                             "Testcykel",
	"No test cycle is selected or planned.":  "Ingen testcykel är vald eller planerad.",
	"Goal":                                   "Mål",
	"Start date":                             "Startdatum",
	"End date":                               "Slutdatum",
	"Planned tests":                          "Planerade testfall",
	"Executed tests":                         "Utförda testfall",
	"Not executed":                           "Ej utförda",
	"Unassigned tests":                       "Testfall utan testare",
	"Progress":                               "Framsteg",
	"Pass rate (of planned tests)":           "Andel godkända (av planerade)",
	"Estimated time (min)":                   "Uppskattad tid (min)",
	"Remaining estimated time (min)":         "Återstående uppskattad tid (min)",
	"Estimated time of executed tests (min)": "Uppskattad tid för utförda testfall (min)",
	"Actual time":                            "Faktisk tid",
	"Workload per tester":                    "Arbetsbelastning per testare",
	"Tester":                                 "Testare",
	"Planned":                                "Planerade",
	"Executed":                               "Utförda",
	"Remaining (min)":                        "Återstår (min)",
	"Estimate (min)":                         "Uppskattning (min)",
	"Not assigned":                           "Ej tilldelad",
	"Burndown":                               "Nedbränning",
	"Day":                                    "Dag",
	"Remaining tests":                        "Återstående testfall",
	"Ideal":                                  "Idealt",
	"Ideal (min)":                            "Idealt (min)",
	"Plan":                                   "Plan",
	"Duration":                               "Tid",
	"Variant":                                "Variant",
}

func (cd *CycleData) person(id string) string {
	if id == "" {
		return ""
	}
	if n := cd.People[id]; n != "" {
		return n
	}
	return id
}

func buildCycleReport(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	cd := d.Cycle
	if cd == nil || cd.Report == nil || cd.Report.Cycle == nil {
		return &export.Dataset{Sections: []export.Section{l.kpiSection("summary", "Key figures", kv{"Test cycle", l.t("No test cycle is selected or planned.")})}}, nil
	}
	cy := cd.Report.Cycle
	p := cy.Progress
	summary := l.kpiSection("summary", "Key figures",
		kv{"Test cycle", cy.Name},
		kv{"Goal", cy.Goal},
		kv{"Status", l.enum(EnumCycleStatus, cy.Status)},
		kv{"Owner", cd.person(cy.OwnerID)},
		kv{"Release", cy.ReleaseName},
		kv{"Variant", cy.VariantName},
		kv{"Build", cy.BuildLabel},
		kv{"Environment", cy.Environment},
		kv{"Start date", cy.StartDate},
		kv{"End date", cy.EndDate},
		kv{"Planned tests", itoa(p.Planned)},
		kv{"Executed tests", itoa(p.Executed)},
		kv{"Passed", itoa(p.Passed)},
		kv{"Failed", itoa(p.Failed)},
		kv{"Blocked", itoa(p.Blocked)},
		kv{"Not executed", itoa(p.Planned - p.Executed)},
		kv{"Unassigned tests", itoa(p.Unassigned)},
		kv{"Progress", l.pct(p.Executed, p.Planned)},
		kv{"Pass rate (of planned tests)", l.pct(p.Passed, p.Planned)},
		kv{"Estimated time (min)", itoa(p.EstimatedMin)},
		kv{"Remaining estimated time (min)", itoa(p.RemainingMin)},
		kv{"Estimated time of executed tests (min)", itoa(p.EstimatedDone)},
		kv{"Actual time", durationText(p.ActualMS)},
	)
	testers := export.Section{Name: "testers", Title: l.t("Workload per tester"),
		Columns: l.cols("Tester", "Planned", "Executed", "Passed", "Failed", "Blocked", "Estimate (min)", "Remaining (min)", "Actual time")}
	for _, t := range cd.Report.Testers {
		who := cd.person(t.AssigneeID)
		if who == "" {
			who = l.t("Not assigned")
		}
		testers.Rows = append(testers.Rows, []string{who, itoa(t.Planned), itoa(t.Executed), itoa(t.Passed), itoa(t.Failed), itoa(t.Blocked),
			itoa(t.EstimatedMin), itoa(t.RemainingMin), durationText(t.ActualMS)})
	}
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	opt := func(v *int) string {
		if v == nil {
			return ""
		}
		return itoa(*v)
	}
	burn := export.Section{Name: "burndown", Title: l.t("Burndown"), Columns: l.cols("Day", "Remaining tests", "Ideal", "Remaining (min)", "Ideal (min)")}
	for _, b := range cd.Report.Burndown {
		burn.Rows = append(burn.Rows, []string{b.Day, opt(b.Remaining), num(b.Ideal), opt(b.RemainingMin), num(b.IdealMin)})
	}
	plan := export.Section{Name: "plan", Title: l.t("Plan"),
		Columns: l.cols("#", "Test", "Title", "Configuration", "Tester", "Estimate (min)", "Result", "Executed by", "Finished", "Duration")}
	for i, x := range cd.Plan {
		dur := ""
		if x.DurationMS > 0 {
			dur = durationText(x.DurationMS)
		}
		plan.Rows = append(plan.Rows, []string{itoa(i + 1), x.TestKey, x.TestTitle, x.ConfigurationName, cd.person(x.AssigneeID), itoa(x.EstimateMinutes),
			l.enum(EnumResult, x.Status), x.ExecutedBy, tsPtr(x.FinishedAt), dur})
	}
	return &export.Dataset{
		Subtitle: d.Project.Key + " – " + d.Project.Name + " · " + l.t("Test cycle") + ": " + cy.Name,
		Sections: []export.Section{summary, testers, burn, plan},
	}, nil
}
