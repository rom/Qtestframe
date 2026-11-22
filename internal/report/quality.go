package report

import (
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/analytics"
	"github.com/rom/qtestframe/internal/export"
)

// The quality metrics report (analytics.Quality): requirements volatility,
// defect density, escaped defects, test effectiveness, result age, review
// turnaround, time to fix defects and requirement quality, with their
// trends. Self-contained: it registers its template, name and translations
// here. The caller computes the metrics into Data.Quality.

// QualityMetrics is the id of the quality metrics report.
const QualityMetrics = "quality_metrics"

func init() {
	templates = append(templates, Template{ID: QualityMetrics, Name: "Quality metrics",
		Description: "Requirements volatility, defect density, escaped defects, test effectiveness, result age, review turnaround, time to fix defects and requirement quality, with their trends.",
		Params: []Param{
			{Name: "range", Type: "range", Description: "days of history: 30, 90 (default), 180 or 365"},
			{Name: "stale_days", Type: "number", Description: "a latest result older than this many days is stale (default 30)"},
		},
		build: buildQuality})
	templateNames[Swedish][QualityMetrics] = "Kvalitetsmått"
	for k, v := range qualityTexts {
		if _, ok := texts[Swedish][k]; !ok { // keep the shared translations
			texts[Swedish][k] = v
		}
	}
}

var qualityTexts = map[string]string{
	"Quality summary":                        "Sammanfattning av kvalitet",
	"Range":                                  "Period",
	"Changes per requirement and period":     "Ändringar per krav och period",
	"Requirements changed":                   "Ändrade krav",
	"Defects per requirement":                "Defekter per krav",
	"Escaped defects":                        "Läckta defekter",
	"Defects found per test":                 "Hittade defekter per test",
	"Defects found per execution hour":       "Hittade defekter per körtimme",
	"Latest results older than the limit":    "Senaste resultat äldre än gränsen",
	"Median review turnaround (hours)":       "Mediantid för granskning (timmar)",
	"Mean time to fix (hours)":               "Medeltid till åtgärd (timmar)",
	"Requirements with analysis warnings":    "Krav med analysvarningar",
	"Requirements volatility":                "Kravens föränderlighet",
	"Period":                                 "Period",
	"Changes":                                "Ändringar",
	"Requirements":                           "Krav",
	"Per requirement":                        "Per krav",
	"Most changed requirements":              "Mest ändrade krav",
	"Key":                                    "Nyckel",
	"Title":                                  "Titel",
	"Defect density by folder":               "Defekttäthet per mapp",
	"Defect density by category":             "Defekttäthet per kategori",
	"Defects by requirement":                 "Defekter per krav",
	"Folder":                                 "Mapp",
	"Category":                               "Kategori",
	"Defects":                                "Defekter",
	"Density":                                "Täthet",
	"(root)":                                 "(rot)",
	"Escaped defects by phase found":         "Defekter per upptäcktsfas",
	"Phase":                                  "Fas",
	"Escaped defects by release":             "Läckta defekter per release",
	"Release":                                "Release",
	"Released":                               "Släppt",
	"Escaped defects over time":              "Läckta defekter över tid",
	"Share":                                  "Andel",
	"Escaped":                                "Läckta",
	"Defect":                                 "Defekt",
	"Where":                                  "Var",
	"Test effectiveness":                     "Testeffektivitet",
	"Tests executed":                         "Körda test",
	"Executions":                             "Körningar",
	"Execution hours":                        "Körtimmar",
	"Defects found by tests":                 "Defekter hittade av test",
	"Tests that found the most defects":      "Test som hittat flest defekter",
	"Test":                                   "Test",
	"Defects found over time":                "Hittade defekter över tid",
	"Per hour":                               "Per timme",
	"Result age":                             "Resultatens ålder",
	"Age (days)":                             "Ålder (dagar)",
	"Tests":                                  "Testfall",
	"Never executed":                         "Aldrig körda",
	"Oldest latest results":                  "Äldsta senaste resultat",
	"Review turnaround":                      "Granskningstid",
	"Decided reviews":                        "Avgjorda granskningar",
	"Mean (hours)":                           "Medel (timmar)",
	"Median (hours)":                         "Median (timmar)",
	"90th percentile (hours)":                "90:e percentilen (timmar)",
	"Pending reviews":                        "Väntande granskningar",
	"Oldest pending (days)":                  "Äldsta väntande (dagar)",
	"Review turnaround over time":            "Granskningstid över tid",
	"Time to fix defects":                    "Tid till åtgärd av defekter",
	"Severity":                               "Allvarlighet",
	"Resolved":                               "Lösta",
	"Time to fix over time":                  "Tid till åtgärd över tid",
	"Requirement quality":                    "Kravkvalitet",
	"Analysis rule":                          "Analysregel",
	"Requirement quality over time":          "Kravkvalitet över tid",
	"Requirements with the most warnings":    "Krav med flest varningar",
	"Rules":                                  "Regler",
	"Warnings":                               "Varningar",
	"Unknown":                                "Okänd",
	"phase|requirements":                     "Krav",
	"phase|design":                           "Design",
	"phase|development":                      "Utveckling",
	"phase|test":                             "Test",
	"phase|acceptance":                       "Acceptans",
	"phase|production":                       "Produktion",
	"phase|unknown":                          "Okänd",
	"defectSeverity|critical":                "Kritisk",
	"defectSeverity|major":                   "Allvarlig",
	"defectSeverity|minor":                   "Mindre",
	"defectSeverity|trivial":                 "Trivial",
	" days":                                  " dagar",
	"Count":                                  "Antal",
	"Value":                                  "Värde",
	"Metric":                                 "Mått",
	"Defects found in production":            "Defekter hittade i produktion",
	"Defects created after their release":    "Defekter skapade efter sin release",
	"Median age of the latest result (days)": "Medianålder för senaste resultat (dagar)",
}

// num formats a number with up to two decimals (decimal comma in Swedish).
func (l loc) num(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if i := strings.IndexByte(s, '.'); i >= 0 && len(s)-i > 3 {
		s = strconv.FormatFloat(f, 'f', 2, 64)
	}
	if l.lang == Swedish {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// pctf formats a percentage given as 0–100.
func (l loc) pctf(p float64) string {
	s := l.num(p)
	if l.lang == Swedish {
		return s + " %"
	}
	return s + "%"
}

func (l loc) phase(p string) string {
	if p == "" {
		p = "unknown"
	}
	if l.lang == English {
		return humanize(p)
	}
	return l.tc("phase", p)
}

func (l loc) defectSeverity(s string) string {
	if l.lang == English {
		return humanize(s)
	}
	return l.tc("defectSeverity", s)
}

// QualityInput builds the quality metrics input from report data (the
// current requirements and tests; no history, defects or executions).
func (d *Data) QualityInput() analytics.QualityInput {
	byReq := d.linksByRequirement()
	in := analytics.QualityInput{Now: d.now(), From: d.now().AddDate(0, 0, -89), Lang: ""}
	for _, r := range d.Requirements {
		in.Requirements = append(in.Requirements, analytics.QualityRequirement{ID: r.ID, Key: r.Key, Title: r.Title, Description: r.Description,
			Status: r.Status, Folder: r.FolderPath, CreatedAt: r.CreatedAt, Tests: byReq[r.ID]})
	}
	for _, t := range d.Tests {
		in.Tests = append(in.Tests, analytics.QualityTest{ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status})
	}
	return in
}

func buildQuality(d *Data, params map[string]string) (*export.Dataset, error) {
	l := d.loc()
	q := d.Quality
	if q == nil { // not computed by the caller: the current state, without history
		r := analytics.Quality(d.QualityInput())
		q = &r
	}
	ds := &export.Dataset{}
	ds.Subtitle = d.subtitle() + " · " + l.t("Range") + ": " + q.From + " – " + q.To
	v, dd, e, ef, ra, rt, ft, rq := q.Volatility, q.DefectDensity, q.Escaped, q.Effectiveness, q.ResultAge, q.ReviewTurnaround, q.FixTime, q.RequirementQuality
	ds.Sections = append(ds.Sections, l.kpiSection("summary", "Quality summary",
		kv{"Changes per requirement and period", l.num(v.PerRequirement)},
		kv{"Requirements changed", itoa(v.Changed)},
		kv{"Defects per requirement", l.num(dd.PerRequirement)},
		kv{"Escaped defects", itoa(e.Escaped) + " (" + l.pctf(e.Percent) + ")"},
		kv{"Defects found per test", l.num(ef.PerTest)},
		kv{"Defects found per execution hour", l.num(ef.PerHour)},
		kv{"Latest results older than the limit", itoa(ra.Stale) + " (" + l.pctf(ra.StalePercent) + ", > " + itoa(ra.StaleDays) + l.t(" days") + ")"},
		kv{"Median review turnaround (hours)", l.num(rt.MedianHours)},
		kv{"Mean time to fix (hours)", l.num(ft.MeanHours)},
		kv{"Requirements with analysis warnings", itoa(rq.WithWarnings) + " (" + l.pctf(rq.Percent) + ")"},
	))

	// trend tabulates a trend: Count, Total (when three columns are
	// named) and Value per period
	trend := func(name, title string, tr []analytics.PeriodValue, cols ...string) export.Section {
		s := export.Section{Name: name, Title: l.t(title), Columns: l.cols(append([]string{"Period"}, cols...)...)}
		for _, p := range tr {
			row := []string{p.Period, itoa(p.Count)}
			if len(cols) == 3 {
				row = append(row, itoa(p.Total))
			}
			s.Rows = append(s.Rows, append(row, l.num(p.Value)))
		}
		return s
	}
	items := func(name, title, count string, list []analytics.QualityItem, note string) export.Section {
		cols := []string{"Key", "Title", count}
		if note != "" {
			cols = append(cols, note)
		}
		s := export.Section{Name: name, Title: l.t(title), Columns: l.cols(cols...)}
		for _, it := range list {
			row := []string{it.Key, it.Title, itoa(it.Count)}
			if note != "" {
				row = append(row, it.Note)
			}
			s.Rows = append(s.Rows, row)
		}
		return s
	}
	groups := func(name, title, label string, gs []analytics.GroupDensity) export.Section {
		s := export.Section{Name: name, Title: l.t(title), Columns: l.cols(label, "Requirements", "Defects", "Density")}
		for _, g := range gs {
			n := g.Name
			if n == "" {
				n = l.t("(root)")
			}
			s.Rows = append(s.Rows, []string{n, itoa(g.Requirements), itoa(g.Defects), l.num(g.Density)})
		}
		return s
	}

	ds.Sections = append(ds.Sections,
		trend("volatility", "Requirements volatility", v.Trend, "Changes", "Requirements", "Per requirement"),
		items("most_changed", "Most changed requirements", "Changes", v.MostChanged, ""),
		groups("density_folders", "Defect density by folder", "Folder", dd.ByFolder),
		groups("density_categories", "Defect density by category", "Category", dd.ByCategory),
		items("density_requirements", "Defects by requirement", "Defects", dd.ByRequirement, ""),
	)

	phases := export.Section{Name: "escaped_phases", Title: l.t("Escaped defects by phase found"), Columns: l.cols("Phase", "Defects", "Share")}
	for _, c := range e.ByPhase {
		phases.Rows = append(phases.Rows, []string{l.phase(c.Value), itoa(c.Count), l.pctf(c.Percent)})
	}
	byRel := export.Section{Name: "escaped_releases", Title: l.t("Escaped defects by release"), Columns: l.cols("Release", "Released", "Escaped")}
	for _, r := range e.ByRelease {
		byRel.Rows = append(byRel.Rows, []string{r.Name, ts(r.ReleasedAt), itoa(r.Escaped)})
	}
	escTrend := export.Section{Name: "escaped_trend", Title: l.t("Escaped defects over time"), Columns: l.cols("Period", "Escaped", "Defects", "Share")}
	for _, p := range e.Trend {
		escTrend.Rows = append(escTrend.Rows, []string{p.Period, itoa(p.Count), itoa(p.Total), l.pctf(p.Value)})
	}
	escItems := export.Section{Name: "escaped_defects", Title: l.t("Escaped defects"), Columns: l.cols("Defect", "Title", "Where")}
	for _, it := range e.Items {
		where := it.Note
		if where == "production" {
			where = l.phase(where)
		}
		escItems.Rows = append(escItems.Rows, []string{it.Key, it.Title, where})
	}
	ds.Sections = append(ds.Sections, phases, byRel, escTrend, escItems)

	ds.Sections = append(ds.Sections,
		l.kpiSection("effectiveness", "Test effectiveness",
			kv{"Tests executed", itoa(ef.Tests)}, kv{"Executions", itoa(ef.Executions)}, kv{"Execution hours", l.num(ef.Hours)},
			kv{"Defects found by tests", itoa(ef.Defects)}, kv{"Defects found per test", l.num(ef.PerTest)}, kv{"Defects found per execution hour", l.num(ef.PerHour)}),
		items("effective_tests", "Tests that found the most defects", "Defects", ef.TopTests, ""),
		trend("effectiveness_trend", "Defects found over time", ef.Trend, "Defects", "Execution hours", "Per hour"),
	)

	ages := export.Section{Name: "result_age", Title: l.t("Result age"), Columns: l.cols("Age (days)", "Tests", "Share")}
	for _, b := range ra.Buckets {
		ages.Rows = append(ages.Rows, []string{b.Value, itoa(b.Count), l.pctf(b.Percent)})
	}
	ages.Rows = append(ages.Rows, []string{l.t("Never executed"), itoa(ra.Never), ""})
	oldest := export.Section{Name: "oldest_results", Title: l.t("Oldest latest results"), Columns: l.cols("Key", "Title", "Age (days)")}
	for _, it := range ra.Oldest {
		oldest.Rows = append(oldest.Rows, []string{it.Key, it.Title, l.num(it.Value)})
	}
	ds.Sections = append(ds.Sections, ages, oldest,
		l.kpiSection("result_age_summary", "Result age",
			kv{"Median age of the latest result (days)", l.num(ra.MedianDays)},
			kv{"Latest results older than the limit", itoa(ra.Stale) + " (" + l.pctf(ra.StalePercent) + ")"}),
	)

	ds.Sections = append(ds.Sections,
		l.kpiSection("review_turnaround", "Review turnaround",
			kv{"Decided reviews", itoa(rt.Count)}, kv{"Mean (hours)", l.num(rt.MeanHours)}, kv{"Median (hours)", l.num(rt.MedianHours)},
			kv{"90th percentile (hours)", l.num(rt.P90Hours)}, kv{"Pending reviews", itoa(rt.Pending)}, kv{"Oldest pending (days)", l.num(rt.OldestPendingDays)}),
		trend("review_trend", "Review turnaround over time", rt.Trend, "Decided reviews", "Median (hours)"),
	)

	fix := export.Section{Name: "fix_time", Title: l.t("Time to fix defects"), Columns: l.cols("Severity", "Resolved", "Mean (hours)", "Median (hours)", "90th percentile (hours)")}
	for _, s := range ft.BySeverity {
		fix.Rows = append(fix.Rows, []string{l.defectSeverity(s.Severity), itoa(s.Count), l.num(s.MeanHours), l.num(s.MedianHours), l.num(s.P90Hours)})
	}
	ds.Sections = append(ds.Sections, fix, trend("fix_trend", "Time to fix over time", ft.Trend, "Resolved", "Mean (hours)"))

	rules := export.Section{Name: "quality_rules", Title: l.t("Requirement quality"), Columns: l.cols("Analysis rule", "Requirements", "Share")}
	for _, c := range rq.ByRule {
		rules.Rows = append(rules.Rows, []string{humanize(c.Value), itoa(c.Count), l.pctf(c.Percent)})
	}
	qTrend := export.Section{Name: "quality_trend", Title: l.t("Requirement quality over time"), Columns: l.cols("Period", "Warnings", "Requirements", "Share")}
	for _, p := range rq.Trend {
		qTrend.Rows = append(qTrend.Rows, []string{p.Period, itoa(p.Count), itoa(p.Total), l.pctf(p.Value)})
	}
	ds.Sections = append(ds.Sections, rules, qTrend, items("quality_items", "Requirements with the most warnings", "Warnings", rq.Items, "Rules"))
	for i := range ds.Sections {
		if ds.Sections[i].Rows == nil {
			ds.Sections[i].Rows = [][]string{}
		}
	}
	return ds, nil
}
