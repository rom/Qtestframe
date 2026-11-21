package report

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/tracerules"
)

var (
	coverageOrder   = []string{"passed", "partial", "failed", "not_run", "none"}
	coverageLabels  = enums[English][EnumCoverage]
	obligationOrder = append(append([]string{}, model.Obligations...), obligationUnset)
	execTypes       = []string{model.ExecManual, model.ExecAutomated}
)

// cols translates column headers.
func (l loc) cols(names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = l.t(n)
	}
	return out
}

// testIndex maps test ids to tests.
func (d *Data) testIndex() map[string]*model.TestCase {
	m := make(map[string]*model.TestCase, len(d.Tests))
	for i := range d.Tests {
		m[d.Tests[i].ID] = &d.Tests[i]
	}
	return m
}

// linksByRequirement returns requirement id → linked test ids in link order.
func (d *Data) linksByRequirement() map[string][]string {
	m := map[string][]string{}
	for _, l := range d.Links {
		m[l.RequirementID] = append(m[l.RequirementID], l.TestCaseID)
	}
	return m
}

func (d *Data) linkedRequirementIDs() map[string]bool {
	m := map[string]bool{}
	for _, l := range d.Links {
		m[l.TestCaseID] = true
	}
	return m
}

func lastResult(t *model.TestCase) string {
	if t.LastResult == "" {
		return model.ResultNotRun
	}
	return t.LastResult
}

// ---------------------------------------------------------------- project status

func buildProjectStatus(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	status, obl, cov := map[string]int{}, map[string]int{}, map[string]int{}
	linked := 0
	for _, r := range d.Requirements {
		status[r.Status]++
		o := r.Obligation
		if o == "" {
			o = obligationUnset
		}
		obl[o]++
		cov[r.Coverage]++
		if r.LinkedTests > 0 {
			linked++
		}
	}
	results := map[string]int{}
	automated, withResult, passed := 0, 0, 0
	for i := range d.Tests {
		t := &d.Tests[i]
		res := lastResult(t)
		results[res]++
		if t.ExecutionType == model.ExecAutomated {
			automated++
		}
		if executed(res) {
			withResult++
			if res == model.ResultPassed {
				passed++
			}
		}
	}
	active := 0
	for _, r := range d.Runs {
		if r.Status == "in_progress" {
			active++
		}
	}
	nr := len(d.Requirements)
	ds := &export.Dataset{Sections: []export.Section{
		l.kpiSection("kpis", "Key figures",
			kv{"Requirements", itoa(nr)},
			kv{"Requirements with linked tests", itoa(linked)},
			kv{"Requirements without tests", itoa(nr - linked)},
			kv{"Test coverage (requirements with tests)", l.pct(linked, nr)},
			kv{"Requirements verified (all linked tests passed)", itoa(cov["passed"])},
			kv{"Test cases", itoa(len(d.Tests))},
			kv{"Automated test cases", itoa(automated)},
			kv{"Manual test cases", itoa(len(d.Tests) - automated)},
			kv{"Pass rate (last results)", l.pct(passed, withResult)},
			kv{"Test runs", itoa(len(d.Runs))},
			kv{"Runs in progress", itoa(active)},
		),
		l.countSection("requirements_by_status", "Requirements by status", "Status", status, model.RequirementStatuses, l.enumLabels(EnumStatus, model.RequirementStatuses)),
		l.countSection("requirements_by_obligation", "Requirements by obligation", "Obligation", obl, obligationOrder, l.enumLabels(EnumObligation, obligationOrder)),
		l.countSection("coverage", "Requirement coverage", "Coverage", cov, coverageOrder, l.enumLabels(EnumCoverage, coverageOrder)),
		l.countSection("test_results", "Last test results", "Result", results, model.ResultStatuses, l.enumLabels(EnumResult, model.ResultStatuses)),
		l.recentRuns(d.Runs, 10),
	}}
	return ds, nil
}

func (l loc) recentRuns(runs []model.TestRun, n int) export.Section {
	s := export.Section{Name: "recent_runs", Title: l.t("Recent test runs"),
		Columns: l.cols("Run", "Status", "Environment", "Build", "Created", "Total", "Passed", "Failed", "Blocked", "Skipped", "Not run")}
	rs := append([]model.TestRun{}, runs...)
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].CreatedAt.After(rs[j].CreatedAt) })
	if len(rs) > n {
		rs = rs[:n]
	}
	for _, r := range rs {
		sum := r.Summary
		s.Rows = append(s.Rows, []string{r.Name, l.enum(EnumStatus, r.Status), r.Environment, r.Build, ts(r.CreatedAt), itoa(sum["total"]),
			itoa(sum[model.ResultPassed]), itoa(sum[model.ResultFailed]), itoa(sum[model.ResultBlocked]), itoa(sum[model.ResultSkipped]),
			itoa(sum[model.ResultNotRun])})
	}
	return s
}

// ---------------------------------------------------------------- requirements coverage

func buildRequirementsCoverage(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	tests := d.testIndex()
	r2t := d.linksByRequirement()
	cov := map[string]int{}
	detail := export.Section{Name: "requirements", Title: l.t("Requirements and their tests"),
		Columns: l.cols("Key", "Title", "Status", "Obligation", "Priority", "Linked tests", "Last results", "Coverage")}
	for _, r := range d.Requirements {
		cov[r.Coverage]++
		var keys, results []string
		for _, tid := range r2t[r.ID] {
			t, ok := tests[tid]
			if !ok {
				continue
			}
			keys = append(keys, t.Key)
			results = append(results, t.Key+": "+l.enum(EnumResult, lastResult(t)))
		}
		detail.Rows = append(detail.Rows, []string{r.Key, r.Title, l.enum(EnumStatus, r.Status), l.enum(EnumObligation, r.Obligation),
			l.enum(EnumPriority, r.Priority), strings.Join(keys, ", "), strings.Join(results, "\n"), l.enum(EnumCoverage, r.Coverage)})
	}
	n := len(d.Requirements)
	return &export.Dataset{Sections: []export.Section{
		l.kpiSection("summary", "Summary",
			kv{"Requirements", itoa(n)},
			kv{"With linked tests", itoa(n - cov["none"])},
			kv{"Without linked tests", itoa(cov["none"])},
			kv{"Passed", itoa(cov["passed"])},
			kv{"Partially passed", itoa(cov["partial"])},
			kv{"Failed", itoa(cov["failed"])},
			kv{"Not run", itoa(cov["not_run"])},
			kv{"Verified share", l.pct(cov["passed"], n)},
		),
		detail,
	}}, nil
}

func coverageLabel(c string) string { return newLoc(English).enum(EnumCoverage, c) }

// ---------------------------------------------------------------- traceability

func buildTraceability(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	tests := d.testIndex()
	r2t := d.linksByRequirement()
	matrix := export.Section{Name: "matrix", Title: l.t("Traceability matrix"),
		Columns: l.cols("Requirement", "Requirement title", "Coverage", "Test", "Test title", "Last result")}
	links, uncovered := 0, 0
	for _, r := range d.Requirements {
		n := 0
		for _, tid := range r2t[r.ID] {
			t, ok := tests[tid]
			if !ok {
				continue
			}
			n++
			matrix.Rows = append(matrix.Rows, []string{r.Key, r.Title, l.enum(EnumCoverage, r.Coverage), t.Key, t.Title, l.enum(EnumResult, lastResult(t))})
		}
		if n == 0 {
			uncovered++
			matrix.Rows = append(matrix.Rows, []string{r.Key, r.Title, l.enum(EnumCoverage, r.Coverage), "", "", ""})
		}
		links += n
	}
	linked := d.linkedRequirementIDs()
	orphans := export.Section{Name: "tests_without_requirements", Title: l.t("Tests without requirements"),
		Columns: l.cols("Key", "Title", "Status", "Last result")}
	for i := range d.Tests {
		t := &d.Tests[i]
		if !linked[t.ID] {
			orphans.Rows = append(orphans.Rows, []string{t.Key, t.Title, l.enum(EnumStatus, t.Status), l.enum(EnumResult, lastResult(t))})
		}
	}
	sections := []export.Section{
		l.kpiSection("summary", "Summary",
			kv{"Requirements", itoa(len(d.Requirements))},
			kv{"Tests", itoa(len(d.Tests))},
			kv{"Links", itoa(links)},
			kv{"Requirements without tests", itoa(uncovered)},
			kv{"Tests without requirements", itoa(len(orphans.Rows))},
		),
		matrix, orphans,
	}
	if len(d.TraceRules) > 0 {
		sections = append(sections, l.traceRuleSections(d.TraceRules)...)
	}
	return &export.Dataset{Sections: sections}, nil
}

// traceRuleSections lists the traceability rules with their results and
// the items that break them.
func (l loc) traceRuleSections(results []tracerules.Result) []export.Section {
	rules := export.Section{Name: "trace_rules", Title: l.t("Traceability rules"),
		Columns: l.cols("Rule", "Severity", "Applies to", "Selector", "Matching items", "Violations", "Result")}
	viol := export.Section{Name: "trace_rule_violations", Title: l.t("Traceability rule violations"),
		Columns: l.cols("Rule", "Severity", "Item", "Title", "Found", "Required")}
	for _, res := range results {
		result := l.tc("rule", "Passed")
		switch {
		case res.Error != "":
			result = l.tc("rule", "Invalid: ") + res.Error
		case !res.Passed:
			result = l.tc("rule", "Failed")
		}
		rules.Rows = append(rules.Rows, []string{res.Rule.Name, l.enum(EnumSeverity, res.Rule.Severity), l.enum(EnumScope, res.Rule.Scope), res.Rule.Selector,
			itoa(res.Matched), itoa(res.ViolationCount), result})
		for _, v := range res.Violations {
			viol.Rows = append(viol.Rows, []string{res.Rule.Name, l.enum(EnumSeverity, res.Rule.Severity), v.Key, v.Title, itoa(v.Found), itoa(v.Required)})
		}
	}
	return []export.Section{rules, viol}
}

// ---------------------------------------------------------------- run summary

func buildRunSummary(d *Data, params map[string]string) (*export.Dataset, error) {
	l := d.loc()
	id := strings.TrimSpace(params["run_id"])
	var run *model.TestRun
	for i := range d.Runs {
		if d.Runs[i].ID == id {
			run = &d.Runs[i]
		}
	}
	if run == nil {
		return nil, invalidf("run %q does not belong to project %s", id, d.Project.Key)
	}
	es := d.Executions[run.ID]
	counts := map[string]int{}
	var total, timed int64
	done, passed := 0, 0
	defects := map[string][]string{}
	var defectOrder []string
	execs := export.Section{Name: "executions", Title: l.t("Executions"),
		Columns: l.cols("#", "Test", "Title", "Result", "Assignee", "Executed by", "Started", "Finished", "Duration (ms)", "Defects", "Notes")}
	for i, e := range es {
		counts[e.Status]++
		if executed(e.Status) {
			done++
			if e.Status == model.ResultPassed {
				passed++
			}
		}
		if e.DurationMS > 0 {
			total += e.DurationMS
			timed++
		}
		for _, df := range e.Defects {
			if df = strings.TrimSpace(df); df == "" {
				continue
			}
			if _, ok := defects[df]; !ok {
				defectOrder = append(defectOrder, df)
			}
			defects[df] = append(defects[df], e.TestKey)
		}
		execs.Rows = append(execs.Rows, []string{itoa(i + 1), e.TestKey, e.TestTitle, l.enum(EnumResult, e.Status), e.Assignee, e.ExecutedBy,
			tsPtr(e.StartedAt), tsPtr(e.FinishedAt), strconv.FormatInt(e.DurationMS, 10), strings.Join(e.Defects, ", "), e.Notes})
	}
	avg := "–"
	if timed > 0 {
		avg = durationText(total / timed)
	}
	kpis := []kv{
		{"Run", run.Name}, {"Description", run.Description}, {"Status", l.enum(EnumStatus, run.Status)},
		{"Environment", run.Environment}, {"Build", run.Build}, {"Created by", run.CreatedBy},
		{"Created", ts(run.CreatedAt)}, {"Started", tsPtr(run.StartedAt)}, {"Completed", tsPtr(run.CompletedAt)},
		{"Executions", itoa(len(es))},
	}
	for _, st := range model.ResultStatuses {
		kpis = append(kpis, kv{humanize(st), itoa(counts[st])}) // translated as counts ("Passed" → "Godkända")
	}
	kpis = append(kpis,
		kv{"Progress (executions with a result)", l.pct(done, len(es))},
		kv{"Pass rate (of executions with a result)", l.pct(passed, done)},
		kv{"Total duration", durationText(total)},
		kv{"Average duration", avg},
		kv{"Defects", itoa(len(defectOrder))},
	)
	sort.SliceStable(defectOrder, func(i, j int) bool { return naturalLess(defectOrder[i], defectOrder[j]) })
	ds := export.Section{Name: "defects", Title: l.t("Defects"), Columns: l.cols("Defect", "Link", "Tests")}
	for _, df := range defectOrder {
		ds.Rows = append(ds.Rows, []string{df, defectLink(d.Project.DefectURLTemplate, df), strings.Join(uniqSorted(defects[df]), ", ")})
	}
	return &export.Dataset{
		Subtitle: d.Project.Key + " – " + d.Project.Name + l.t(" · Run: ") + run.Name,
		Sections: []export.Section{l.kpiSection("summary", "Summary", kpis...), execs, ds},
	}, nil
}

// ---------------------------------------------------------------- defects

func buildDefects(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	type agg struct {
		id          string
		occurrences int
		tests, runs []string
		latest      string
		lastSeen    time.Time
	}
	byID := map[string]*agg{}
	occ := export.Section{Name: "occurrences", Title: l.t("Occurrences"),
		Columns: l.cols("Defect", "Run", "Build", "Test", "Title", "Result", "Executed by", "Finished")}
	runs := append([]model.TestRun{}, d.Runs...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt.Before(runs[j].CreatedAt) })
	tests, allRuns := map[string]bool{}, map[string]bool{}
	occurrences := 0
	for _, run := range runs {
		for _, e := range d.Executions[run.ID] {
			at := run.CreatedAt
			if e.StartedAt != nil {
				at = *e.StartedAt
			}
			if e.FinishedAt != nil {
				at = *e.FinishedAt
			}
			for _, df := range e.Defects {
				df = strings.TrimSpace(df)
				if df == "" {
					continue
				}
				a := byID[df]
				if a == nil {
					a = &agg{id: df}
					byID[df] = a
				}
				a.occurrences++
				occurrences++
				a.tests = append(a.tests, e.TestKey)
				a.runs = append(a.runs, run.Name)
				tests[e.TestCaseID], allRuns[run.ID] = true, true
				if !at.Before(a.lastSeen) {
					a.lastSeen, a.latest = at, e.Status
				}
				occ.Rows = append(occ.Rows, []string{df, run.Name, run.Build, e.TestKey, e.TestTitle, l.enum(EnumResult, e.Status), e.ExecutedBy, tsPtr(e.FinishedAt)})
			}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return naturalLess(ids[i], ids[j]) })
	list := export.Section{Name: "defects", Title: l.t("Defects"),
		Columns: l.cols("Defect", "Link", "Occurrences", "Tests", "Runs", "Latest result", "Last seen")}
	for _, id := range ids {
		a := byID[id]
		list.Rows = append(list.Rows, []string{a.id, defectLink(d.Project.DefectURLTemplate, a.id), itoa(a.occurrences),
			strings.Join(uniqSorted(a.tests), ", "), strings.Join(uniqSorted(a.runs), ", "), l.enum(EnumResult, a.latest), ts(a.lastSeen)})
	}
	return &export.Dataset{Sections: []export.Section{
		l.kpiSection("summary", "Summary",
			kv{"Distinct defects", itoa(len(ids))},
			kv{"Occurrences", itoa(occurrences)},
			kv{"Affected tests", itoa(len(tests))},
			kv{"Affected runs", itoa(len(allRuns))},
		),
		list, occ,
	}}, nil
}

// ---------------------------------------------------------------- test inventory

func buildTestInventory(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	folders, types, status, prio := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	type dimCat struct{ dim, name string }
	cats := map[dimCat]int{}
	withScript, linked, estimate := 0, 0, 0
	list := export.Section{Name: "tests", Title: l.t("Test cases"),
		Columns: l.cols("Key", "Title", "Folder", "Execution", "Status", "Priority", "Categories", "Linked requirements", "Last result")}
	for i := range d.Tests {
		t := &d.Tests[i]
		f := t.FolderPath
		if f == "" {
			f = l.t("(no folder)")
		}
		folders[f]++
		types[t.ExecutionType]++
		status[t.Status]++
		prio[t.Priority]++
		for _, id := range t.CategoryIDs {
			if c, ok := d.Categories[id]; ok {
				cats[dimCat{c.Dimension, c.Name}]++
			}
		}
		if strings.TrimSpace(t.Script) != "" {
			withScript++
		}
		if t.LinkedReqs > 0 {
			linked++
		}
		estimate += t.EstimatedMin
		list.Rows = append(list.Rows, []string{t.Key, t.Title, t.FolderPath, l.enum(EnumExecution, t.ExecutionType), l.enum(EnumStatus, t.Status), l.enum(EnumPriority, t.Priority),
			d.categoryNames(t.CategoryIDs), itoa(t.LinkedReqs), l.enum(EnumResult, lastResult(t))})
	}
	folderNames := make([]string, 0, len(folders))
	for f := range folders {
		folderNames = append(folderNames, f)
	}
	sort.Slice(folderNames, func(i, j int) bool { return naturalLess(folderNames[i], folderNames[j]) })
	byCat := export.Section{Name: "by_category", Title: l.t("Tests by category"), Columns: l.cols("Dimension", "Category", "Count")}
	keys := make([]dimCat, 0, len(cats))
	for k := range cats {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].dim != keys[j].dim {
			return naturalLess(keys[i].dim, keys[j].dim)
		}
		return naturalLess(keys[i].name, keys[j].name)
	})
	for _, k := range keys {
		byCat.Rows = append(byCat.Rows, []string{k.dim, k.name, itoa(cats[k])})
	}
	n := len(d.Tests)
	return &export.Dataset{Sections: []export.Section{
		l.kpiSection("summary", "Summary",
			kv{"Test cases", itoa(n)},
			kv{"Manual", itoa(types[model.ExecManual])},
			kv{"Automated", itoa(types[model.ExecAutomated])},
			kv{"With script", itoa(withScript)},
			kv{"Linked to requirements", itoa(linked)},
			kv{"Not linked to requirements", itoa(n - linked)},
			kv{"Total estimate (minutes)", itoa(estimate)},
		),
		l.countSection("by_folder", "Tests by folder", "Folder", folders, folderNames, nil),
		l.countSection("by_execution_type", "Tests by execution type", "Execution", types, execTypes, l.enumLabels(EnumExecution, execTypes)),
		l.countSection("by_status", "Tests by status", "Status", status, model.TestStatuses, l.enumLabels(EnumStatus, model.TestStatuses)),
		l.countSection("by_priority", "Tests by priority", "Priority", prio, model.Priorities, l.enumLabels(EnumPriority, model.Priorities)),
		byCat,
		list,
	}}, nil
}
