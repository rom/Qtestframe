package analytics

import (
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/reqlang"
)

// Quality metrics of a project, computed from its history:
//
//   - requirements volatility: changes per requirement per period (the
//     revisions that changed a requirement after its creation, moves not
//     counted) and the most-changed requirements;
//   - defect density: defects per requirement, overall, per folder and per
//     category. A defect counts for a requirement when it is linked to it,
//     to a test that verifies it or to an execution of such a test;
//   - escaped defects: defects found in production (found_in) or created
//     after a release was released whose scope they concern (linked to its
//     requirements, their tests, or its runs);
//   - test effectiveness: defects found by tests (linked to a test or to
//     one of its executions) per test executed and per execution hour;
//   - result age: how old the latest result of every test is;
//   - review turnaround: the time from a review request to its decision;
//   - mean time to fix: the time from a defect's creation to its
//     resolution (rejected defects not counted);
//   - requirement quality: the share of active requirements with warnings
//     or errors of the language analysis (internal/reqlang).
//
// Trends are per period (week or month) over the range; the period of a
// time is the Monday (week) or first day (month) it falls in, in UTC.

// Periods of the quality trends.
const (
	PeriodWeek  = "week"
	PeriodMonth = "month"
)

// Result age buckets (days).
var AgeBuckets = []string{"0-7", "8-30", "31-90", "91-180", "181+"}

// QualityTopItems bounds the item lists of the quality report.
var QualityTopItems = 10

// QualityRequirement is a requirement as the quality metrics see it.
type QualityRequirement struct {
	ID, Key, Title, Description, Status string
	// Folder is the folder path ("" root); Categories "dimension: name".
	Folder     string
	Categories []string
	CreatedAt  time.Time
	// Tests: ids of the tests that verify it.
	Tests []string
}

// QualityTest is a test.
type QualityTest struct {
	ID, Key, Title, Status string
}

// QualityRevision is a revision of a requirement (Text set when known:
// the title and description of the snapshot).
type QualityRevision struct {
	RequirementID string
	Action        string
	At            time.Time
	// HasText: Title, Description and Status of the snapshot are known.
	HasText            bool
	Title, Description string
	Status             string
}

// QualityDefect is a defect with what it is linked to.
type QualityDefect struct {
	ID, Key, Title, Severity, Status, FoundIn string
	CreatedAt                                 time.Time
	ResolvedAt                                *time.Time
	Requirements, Tests, Executions, Runs     []string
}

// QualityExecution is an execution with a result.
type QualityExecution struct {
	ID, TestID, RunID, Status string
	At                        time.Time
	DurationMS                int64
}

// QualityRelease is a release with its scope and runs.
type QualityRelease struct {
	ID, Name, Status string
	ReleasedAt       *time.Time
	RequirementIDs   []string
	RunIDs           []string
}

// QualityReview is a review request.
type QualityReview struct {
	Status    string
	CreatedAt time.Time
	DecidedAt *time.Time
}

// QualityInput is what Quality measures.
type QualityInput struct {
	Now  time.Time
	From time.Time
	// Period: week or month ("" chooses: week up to 120 days, else month).
	Period string
	// StaleDays: a latest result older than this is stale (default 30).
	StaleDays    int
	Lang         string
	Requirements []QualityRequirement
	Tests        []QualityTest
	Revisions    []QualityRevision
	Defects      []QualityDefect
	Executions   []QualityExecution
	Releases     []QualityRelease
	Reviews      []QualityReview
	// EscapedPhase is the found_in phase of escaped defects (default production).
	EscapedPhase string
}

// PeriodValue is one period of a trend.
type PeriodValue struct {
	Period string  `json:"period"`
	Value  float64 `json:"value"`
	Count  int     `json:"count"`
	Total  int     `json:"total"`
}

// QualityItem is an item of a list with its count (or value).
type QualityItem struct {
	Kind  string  `json:"kind"`
	ID    string  `json:"id"`
	Key   string  `json:"key"`
	Title string  `json:"title"`
	Count int     `json:"count"`
	Value float64 `json:"value,omitempty"`
	Note  string  `json:"note,omitempty"`
}

// GroupDensity is the defect density of a folder or category.
type GroupDensity struct {
	Name         string  `json:"name"`
	Requirements int     `json:"requirements"`
	Defects      int     `json:"defects"`
	Density      float64 `json:"density"`
}

// Volatility of requirements.
type Volatility struct {
	Changes      int `json:"changes"`
	Requirements int `json:"requirements"`
	// Changed: requirements changed at least once in the range.
	Changed int `json:"changed"`
	// PerRequirement: changes per requirement per period (average).
	PerRequirement float64       `json:"per_requirement"`
	Trend          []PeriodValue `json:"trend"`
	MostChanged    []QualityItem `json:"most_changed"`
}

// DefectDensity of requirements.
type DefectDensity struct {
	Defects        int            `json:"defects"`
	Requirements   int            `json:"requirements"`
	PerRequirement float64        `json:"per_requirement"`
	BySeverity     []Count        `json:"by_severity"`
	ByRequirement  []QualityItem  `json:"by_requirement"`
	ByFolder       []GroupDensity `json:"by_folder"`
	ByCategory     []GroupDensity `json:"by_category"`
}

// ReleaseEscapes counts the escaped defects of a released release.
type ReleaseEscapes struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ReleasedAt time.Time `json:"released_at"`
	Escaped    int       `json:"escaped"`
}

// Escaped defects.
type Escaped struct {
	Defects int     `json:"defects"`
	Escaped int     `json:"escaped"`
	Percent float64 `json:"percent"`
	// Production: defects found in production; AfterRelease: created after
	// a release they concern was released.
	Production   int              `json:"production"`
	AfterRelease int              `json:"after_release"`
	ByPhase      []Count          `json:"by_phase"`
	ByRelease    []ReleaseEscapes `json:"by_release"`
	Trend        []PeriodValue    `json:"trend"`
	Items        []QualityItem    `json:"items"`
}

// Effectiveness of tests.
type Effectiveness struct {
	// Defects found by tests in the range; Tests executed in the range.
	Defects        int           `json:"defects"`
	Tests          int           `json:"tests"`
	Executions     int           `json:"executions"`
	Hours          float64       `json:"hours"`
	PerTest        float64       `json:"per_test"`
	PerHour        float64       `json:"per_hour"`
	TopTests       []QualityItem `json:"top_tests"`
	Trend          []PeriodValue `json:"trend"`
	UntimedPercent float64       `json:"untimed_percent"`
}

// ResultAge of the latest result per test.
type ResultAge struct {
	Tests        int           `json:"tests"`
	WithResult   int           `json:"with_result"`
	Never        int           `json:"never"`
	Buckets      []Count       `json:"buckets"`
	StaleDays    int           `json:"stale_days"`
	Stale        int           `json:"stale"`
	StalePercent float64       `json:"stale_percent"`
	MedianDays   float64       `json:"median_days"`
	Oldest       []QualityItem `json:"oldest"`
}

// Durations summarises time spans in hours.
type Durations struct {
	Count       int     `json:"count"`
	MeanHours   float64 `json:"mean_hours"`
	MedianHours float64 `json:"median_hours"`
	P90Hours    float64 `json:"p90_hours"`
}

// ReviewTurnaround of review requests.
type ReviewTurnaround struct {
	Durations
	Pending           int           `json:"pending"`
	OldestPendingDays float64       `json:"oldest_pending_days"`
	Trend             []PeriodValue `json:"trend"`
}

// SeverityDurations is the fix time of one severity.
type SeverityDurations struct {
	Severity string `json:"severity"`
	Durations
}

// FixTime of defects.
type FixTime struct {
	Durations
	Open       int                 `json:"open"`
	BySeverity []SeverityDurations `json:"by_severity"`
	Trend      []PeriodValue       `json:"trend"`
}

// RequirementQuality is the share with analysis warnings.
type RequirementQuality struct {
	Requirements int           `json:"requirements"`
	WithWarnings int           `json:"with_warnings"`
	Percent      float64       `json:"percent"`
	ByRule       []Count       `json:"by_rule"`
	Trend        []PeriodValue `json:"trend"`
	Items        []QualityItem `json:"items"`
}

// QualityReport is the result of Quality.
type QualityReport struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Period    string   `json:"period"`
	Periods   []string `json:"periods"`
	StaleDays int      `json:"stale_days"`

	Volatility         Volatility         `json:"volatility"`
	DefectDensity      DefectDensity      `json:"defect_density"`
	Escaped            Escaped            `json:"escaped"`
	Effectiveness      Effectiveness      `json:"effectiveness"`
	ResultAge          ResultAge          `json:"result_age"`
	ReviewTurnaround   ReviewTurnaround   `json:"review_turnaround"`
	FixTime            FixTime            `json:"fix_time"`
	RequirementQuality RequirementQuality `json:"requirement_quality"`
}

// PeriodStart is the start of the period t falls in.
func PeriodStart(t time.Time, period string) time.Time {
	t = t.UTC()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if period == PeriodMonth {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	wd := (int(d.Weekday()) + 6) % 7 // Monday 0
	return d.AddDate(0, 0, -wd)
}

func nextPeriod(t time.Time, period string) time.Time {
	if period == PeriodMonth {
		return t.AddDate(0, 1, 0)
	}
	return t.AddDate(0, 0, 7)
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

func hours(d time.Duration) float64 { return d.Hours() }

func durations(hs []float64) Durations {
	out := Durations{Count: len(hs)}
	if len(hs) == 0 {
		return out
	}
	s := append([]float64{}, hs...)
	sort.Float64s(s)
	sum := 0.0
	for _, h := range s {
		sum += h
	}
	out.MeanHours = round2(sum / float64(len(s)))
	out.MedianHours = round2(quantile(s, 0.5))
	out.P90Hours = round2(quantile(s, 0.9))
	return out
}

// quantile of sorted values (linear interpolation).
func quantile(s []float64, q float64) float64 {
	if len(s) == 0 {
		return 0
	}
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

// volatile: a revision that changed the requirement (not its creation or a move).
func volatile(action string) bool { return action != "create" && action != "move" }

func activeStatus(s string) bool { return s != "rejected" && s != "obsolete" }

// warned: the analysis of a text has warnings or errors; the rules returned.
func warned(text, lang string) []string {
	a := reqlang.AnalyzeLang(text, lang)
	var rules []string
	for _, is := range a.Issues {
		if is.Severity == reqlang.SeverityWarning || is.Severity == reqlang.SeverityError {
			if !slices.Contains(rules, is.Rule) {
				rules = append(rules, is.Rule)
			}
		}
	}
	return rules
}

func reqText(title, description string) string {
	if d := strings.TrimSpace(description); d != "" {
		return d
	}
	return title
}

// Quality computes the report.
func Quality(in QualityInput) QualityReport {
	now := in.Now.UTC()
	from := in.From.UTC()
	if !from.Before(now) {
		from = now.AddDate(0, 0, -90)
	}
	period := in.Period
	if period != PeriodWeek && period != PeriodMonth {
		period = PeriodWeek
		if now.Sub(from) > 120*24*time.Hour {
			period = PeriodMonth
		}
	}
	stale := in.StaleDays
	if stale <= 0 {
		stale = 30
	}
	escapedPhase := in.EscapedPhase
	if escapedPhase == "" {
		escapedPhase = "production"
	}
	rep := QualityReport{From: from.Format(time.DateOnly), To: now.Format(time.DateOnly), Period: period, StaleDays: stale}
	var starts []time.Time
	for p := PeriodStart(from, period); !p.After(now); p = nextPeriod(p, period) {
		starts = append(starts, p)
		rep.Periods = append(rep.Periods, p.Format(time.DateOnly))
	}
	periodIndex := func(t time.Time) int {
		if t.Before(from) || t.After(now) {
			return -1
		}
		ps := PeriodStart(t, period)
		for i, s := range starts {
			if s.Equal(ps) {
				return i
			}
		}
		return -1
	}
	trend := func() []PeriodValue {
		out := make([]PeriodValue, len(starts))
		for i := range starts {
			out[i].Period = rep.Periods[i]
		}
		return out
	}
	inRange := func(t time.Time) bool { return !t.Before(from) && !t.After(now) }

	reqByID := map[string]*QualityRequirement{}
	for i := range in.Requirements {
		reqByID[in.Requirements[i].ID] = &in.Requirements[i]
	}
	testByID := map[string]*QualityTest{}
	for i := range in.Tests {
		testByID[in.Tests[i].ID] = &in.Tests[i]
	}

	// ---------------------------------------------------------- volatility
	vol := Volatility{Trend: trend(), MostChanged: []QualityItem{}}
	changes := map[string]int{}
	for _, rv := range in.Revisions {
		if !volatile(rv.Action) || reqByID[rv.RequirementID] == nil {
			continue
		}
		if i := periodIndex(rv.At); i >= 0 {
			vol.Trend[i].Count++
			vol.Changes++
			changes[rv.RequirementID]++
		}
	}
	for i, s := range starts {
		end := nextPeriod(s, period)
		n := 0
		for _, rq := range in.Requirements {
			if rq.CreatedAt.Before(end) {
				n++
			}
		}
		vol.Trend[i].Total = n
		if n > 0 {
			vol.Trend[i].Value = round2(float64(vol.Trend[i].Count) / float64(n))
		}
	}
	vol.Requirements = len(in.Requirements)
	vol.Changed = len(changes)
	if vol.Requirements > 0 && len(starts) > 0 {
		vol.PerRequirement = round2(float64(vol.Changes) / float64(vol.Requirements) / float64(len(starts)))
	}
	for id, n := range changes {
		rq := reqByID[id]
		vol.MostChanged = append(vol.MostChanged, QualityItem{Kind: "requirement", ID: id, Key: rq.Key, Title: rq.Title, Count: n})
	}
	vol.MostChanged = topItems(vol.MostChanged)
	rep.Volatility = vol

	// ------------------------------------------------------ defect density
	testReqs := map[string][]string{}
	for _, rq := range in.Requirements {
		for _, t := range rq.Tests {
			testReqs[t] = append(testReqs[t], rq.ID)
		}
	}
	execTest := map[string]string{}
	for _, x := range in.Executions {
		execTest[x.ID] = x.TestID
	}
	// defect id → requirements and tests it concerns
	defReqs := make([]map[string]bool, len(in.Defects))
	defTests := make([]map[string]bool, len(in.Defects))
	for i, d := range in.Defects {
		rs, ts := map[string]bool{}, map[string]bool{}
		for _, id := range d.Requirements {
			rs[id] = true
		}
		for _, id := range d.Tests {
			ts[id] = true
		}
		for _, id := range d.Executions {
			if t := execTest[id]; t != "" {
				ts[t] = true
			}
		}
		for t := range ts {
			for _, r := range testReqs[t] {
				rs[r] = true
			}
		}
		defReqs[i], defTests[i] = rs, ts
	}
	dd := DefectDensity{ByRequirement: []QualityItem{}, ByFolder: []GroupDensity{}, ByCategory: []GroupDensity{}, BySeverity: []Count{}}
	perReq := map[string]int{}
	sev := map[string]int{}
	folderDef := map[string]map[string]bool{}
	catDef := map[string]map[string]bool{}
	for i, d := range in.Defects {
		counted := false
		for id := range defReqs[i] {
			rq := reqByID[id]
			if rq == nil {
				continue
			}
			counted = true
			perReq[id]++
			if folderDef[rq.Folder] == nil {
				folderDef[rq.Folder] = map[string]bool{}
			}
			folderDef[rq.Folder][d.ID] = true
			for _, c := range rq.Categories {
				if catDef[c] == nil {
					catDef[c] = map[string]bool{}
				}
				catDef[c][d.ID] = true
			}
		}
		if counted {
			dd.Defects++
			sev[d.Severity]++
		}
	}
	folderReqs, catReqs := map[string]int{}, map[string]int{}
	for _, rq := range in.Requirements {
		if !activeStatus(rq.Status) {
			continue
		}
		dd.Requirements++
		folderReqs[rq.Folder]++
		for _, c := range rq.Categories {
			catReqs[c]++
		}
	}
	if dd.Requirements > 0 {
		dd.PerRequirement = round2(float64(dd.Defects) / float64(dd.Requirements))
	}
	for id, n := range perReq {
		rq := reqByID[id]
		dd.ByRequirement = append(dd.ByRequirement, QualityItem{Kind: "requirement", ID: id, Key: rq.Key, Title: rq.Title, Count: n})
	}
	dd.ByRequirement = topItems(dd.ByRequirement)
	group := func(reqs map[string]int, defs map[string]map[string]bool) []GroupDensity {
		out := []GroupDensity{}
		names := map[string]bool{}
		for k := range reqs {
			names[k] = true
		}
		for k := range defs {
			names[k] = true
		}
		for k := range names {
			g := GroupDensity{Name: k, Requirements: reqs[k], Defects: len(defs[k])}
			if g.Requirements > 0 {
				g.Density = round2(float64(g.Defects) / float64(g.Requirements))
			}
			out = append(out, g)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Density != out[j].Density {
				return out[i].Density > out[j].Density
			}
			if out[i].Defects != out[j].Defects {
				return out[i].Defects > out[j].Defects
			}
			return out[i].Name < out[j].Name
		})
		return out
	}
	dd.ByFolder = group(folderReqs, folderDef)
	dd.ByCategory = group(catReqs, catDef)
	for _, s := range []string{"critical", "major", "minor", "trivial"} {
		if sev[s] > 0 {
			dd.BySeverity = append(dd.BySeverity, Count{Value: s, Count: sev[s], Percent: pct(sev[s], dd.Defects)})
		}
	}
	rep.DefectDensity = dd

	// ----------------------------------------------------- escaped defects
	esc := Escaped{Trend: trend(), ByPhase: []Count{}, ByRelease: []ReleaseEscapes{}, Items: []QualityItem{}}
	type relScope struct {
		rel  QualityRelease
		reqs map[string]bool
		runs map[string]bool
	}
	var released []relScope
	for _, rl := range in.Releases {
		if rl.ReleasedAt == nil || rl.Status == "cancelled" {
			continue
		}
		sc := relScope{rel: rl, reqs: map[string]bool{}, runs: map[string]bool{}}
		for _, id := range rl.RequirementIDs {
			sc.reqs[id] = true
		}
		for _, id := range rl.RunIDs {
			sc.runs[id] = true
		}
		released = append(released, sc)
	}
	byRelease := map[string]int{}
	phase := map[string]int{}
	execRun := map[string]string{}
	for _, x := range in.Executions {
		execRun[x.ID] = x.RunID
	}
	for i, d := range in.Defects {
		esc.Defects++
		ph := d.FoundIn
		if ph == "" {
			ph = "unknown"
		}
		phase[ph]++
		prod := d.FoundIn == escapedPhase
		after := false
		var rels []string
		for _, sc := range released {
			if !d.CreatedAt.After(*sc.rel.ReleasedAt) {
				continue
			}
			hit := false
			for id := range defReqs[i] {
				if sc.reqs[id] {
					hit = true
					break
				}
			}
			for _, id := range d.Runs {
				hit = hit || sc.runs[id]
			}
			for _, id := range d.Executions {
				hit = hit || sc.runs[execRun[id]]
			}
			if hit {
				after = true
				byRelease[sc.rel.ID]++
				rels = append(rels, sc.rel.Name)
			}
		}
		if !prod && !after {
			if j := periodIndex(d.CreatedAt); j >= 0 {
				esc.Trend[j].Total++
			}
			continue
		}
		esc.Escaped++
		if prod {
			esc.Production++
		}
		if after {
			esc.AfterRelease++
		}
		if j := periodIndex(d.CreatedAt); j >= 0 {
			esc.Trend[j].Count++
			esc.Trend[j].Total++
		}
		note := strings.Join(rels, ", ")
		if prod && note == "" {
			note = escapedPhase
		}
		esc.Items = append(esc.Items, QualityItem{Kind: "defect", ID: d.ID, Key: d.Key, Title: d.Title, Count: 1, Note: note})
	}
	for i := range esc.Trend {
		if esc.Trend[i].Total > 0 {
			esc.Trend[i].Value = pct(esc.Trend[i].Count, esc.Trend[i].Total)
		}
	}
	esc.Percent = pct(esc.Escaped, esc.Defects)
	for _, ph := range append(append([]string{}, defectPhases...), "unknown") {
		if phase[ph] > 0 {
			esc.ByPhase = append(esc.ByPhase, Count{Value: ph, Count: phase[ph], Percent: pct(phase[ph], esc.Defects)})
		}
	}
	for _, sc := range released {
		esc.ByRelease = append(esc.ByRelease, ReleaseEscapes{ID: sc.rel.ID, Name: sc.rel.Name, ReleasedAt: sc.rel.ReleasedAt.UTC(), Escaped: byRelease[sc.rel.ID]})
	}
	sort.Slice(esc.ByRelease, func(i, j int) bool { return esc.ByRelease[i].ReleasedAt.Before(esc.ByRelease[j].ReleasedAt) })
	if len(esc.Items) > QualityTopItems*5 {
		esc.Items = esc.Items[:QualityTopItems*5]
	}
	rep.Escaped = esc

	// ---------------------------------------------------- test effectiveness
	eff := Effectiveness{Trend: trend(), TopTests: []QualityItem{}}
	executed := map[string]bool{}
	var ms int64
	untimed := 0
	periodMS := make([]int64, len(starts))
	for _, x := range in.Executions {
		if !inRange(x.At) {
			continue
		}
		eff.Executions++
		executed[x.TestID] = true
		if x.DurationMS > 0 {
			ms += x.DurationMS
			if j := periodIndex(x.At); j >= 0 {
				periodMS[j] += x.DurationMS
			}
		} else {
			untimed++
		}
	}
	eff.Tests = len(executed)
	eff.Hours = round2(float64(ms) / 3.6e6)
	if eff.Executions > 0 {
		eff.UntimedPercent = pct(untimed, eff.Executions)
	}
	perTest := map[string]int{}
	for i, d := range in.Defects {
		if len(defTests[i]) == 0 || !inRange(d.CreatedAt) {
			continue
		}
		eff.Defects++
		if j := periodIndex(d.CreatedAt); j >= 0 {
			eff.Trend[j].Count++
		}
		for t := range defTests[i] {
			perTest[t]++
		}
	}
	for j := range eff.Trend {
		h := float64(periodMS[j]) / 3.6e6
		eff.Trend[j].Total = int(math.Round(h))
		if h > 0 {
			eff.Trend[j].Value = round2(float64(eff.Trend[j].Count) / h)
		}
	}
	if eff.Tests > 0 {
		eff.PerTest = round2(float64(eff.Defects) / float64(eff.Tests))
	}
	if eff.Hours > 0 {
		eff.PerHour = round2(float64(eff.Defects) / (float64(ms) / 3.6e6))
	}
	for id, n := range perTest {
		t := testByID[id]
		if t == nil {
			continue
		}
		eff.TopTests = append(eff.TopTests, QualityItem{Kind: "test", ID: id, Key: t.Key, Title: t.Title, Count: n})
	}
	eff.TopTests = topItems(eff.TopTests)
	rep.Effectiveness = eff

	// ------------------------------------------------------------ result age
	age := ResultAge{StaleDays: stale, Buckets: []Count{}, Oldest: []QualityItem{}}
	latest := map[string]time.Time{}
	for _, x := range in.Executions {
		if x.Status == "in_progress" {
			continue
		}
		if t, ok := latest[x.TestID]; !ok || x.At.After(t) {
			latest[x.TestID] = x.At
		}
	}
	bucket := make([]int, len(AgeBuckets))
	var ages []float64
	for _, t := range in.Tests {
		if t.Status == "deprecated" {
			continue
		}
		age.Tests++
		at, ok := latest[t.ID]
		if !ok {
			age.Never++
			continue
		}
		age.WithResult++
		days := now.Sub(at).Hours() / 24
		if days < 0 {
			days = 0
		}
		ages = append(ages, days)
		switch {
		case days <= 7:
			bucket[0]++
		case days <= 30:
			bucket[1]++
		case days <= 90:
			bucket[2]++
		case days <= 180:
			bucket[3]++
		default:
			bucket[4]++
		}
		if days > float64(stale) {
			age.Stale++
		}
		age.Oldest = append(age.Oldest, QualityItem{Kind: "test", ID: t.ID, Key: t.Key, Title: t.Title, Count: int(days), Value: round2(days)})
	}
	for i, b := range AgeBuckets {
		age.Buckets = append(age.Buckets, Count{Value: b, Count: bucket[i], Percent: pct(bucket[i], age.WithResult)})
	}
	age.StalePercent = pct(age.Stale, age.WithResult)
	sort.Float64s(ages)
	age.MedianDays = round2(quantile(ages, 0.5))
	sort.SliceStable(age.Oldest, func(i, j int) bool {
		if age.Oldest[i].Value != age.Oldest[j].Value {
			return age.Oldest[i].Value > age.Oldest[j].Value
		}
		return age.Oldest[i].Key < age.Oldest[j].Key
	})
	if len(age.Oldest) > QualityTopItems {
		age.Oldest = age.Oldest[:QualityTopItems]
	}
	rep.ResultAge = age

	// ----------------------------------------------------- review turnaround
	rt := ReviewTurnaround{Trend: trend()}
	var rh []float64
	ph := make([][]float64, len(starts))
	for _, v := range in.Reviews {
		switch {
		case v.Status == "pending":
			rt.Pending++
			if d := now.Sub(v.CreatedAt).Hours() / 24; d > rt.OldestPendingDays {
				rt.OldestPendingDays = round2(d)
			}
		case v.DecidedAt != nil && v.Status != "cancelled" && inRange(*v.DecidedAt):
			h := hours(v.DecidedAt.Sub(v.CreatedAt))
			rh = append(rh, h)
			if j := periodIndex(*v.DecidedAt); j >= 0 {
				ph[j] = append(ph[j], h)
			}
		}
	}
	rt.Durations = durations(rh)
	for j := range rt.Trend {
		d := durations(ph[j])
		rt.Trend[j].Count, rt.Trend[j].Value = d.Count, d.MedianHours
	}
	rep.ReviewTurnaround = rt

	// ----------------------------------------------------------- fix time
	ft := FixTime{Trend: trend(), BySeverity: []SeverityDurations{}}
	var fh []float64
	bySev := map[string][]float64{}
	pf := make([][]float64, len(starts))
	for _, d := range in.Defects {
		if d.Status == "open" || d.Status == "in_progress" {
			ft.Open++
			continue
		}
		if d.Status == "rejected" || d.ResolvedAt == nil || !inRange(*d.ResolvedAt) {
			continue
		}
		h := hours(d.ResolvedAt.Sub(d.CreatedAt))
		if h < 0 {
			h = 0
		}
		fh = append(fh, h)
		bySev[d.Severity] = append(bySev[d.Severity], h)
		if j := periodIndex(*d.ResolvedAt); j >= 0 {
			pf[j] = append(pf[j], h)
		}
	}
	ft.Durations = durations(fh)
	for _, s := range []string{"critical", "major", "minor", "trivial"} {
		if len(bySev[s]) > 0 {
			ft.BySeverity = append(ft.BySeverity, SeverityDurations{Severity: s, Durations: durations(bySev[s])})
		}
	}
	for j := range ft.Trend {
		d := durations(pf[j])
		ft.Trend[j].Count, ft.Trend[j].Value = d.Count, d.MeanHours
	}
	rep.FixTime = ft

	// --------------------------------------------------- requirement quality
	rq := RequirementQuality{Trend: trend(), ByRule: []Count{}, Items: []QualityItem{}}
	ruleCount := map[string]int{}
	for _, r := range in.Requirements {
		if !activeStatus(r.Status) {
			continue
		}
		rq.Requirements++
		rules := warned(reqText(r.Title, r.Description), in.Lang)
		if len(rules) == 0 {
			continue
		}
		rq.WithWarnings++
		for _, ru := range rules {
			ruleCount[ru]++
		}
		rq.Items = append(rq.Items, QualityItem{Kind: "requirement", ID: r.ID, Key: r.Key, Title: r.Title, Count: len(rules), Note: strings.Join(rules, ", ")})
	}
	rq.Percent = pct(rq.WithWarnings, rq.Requirements)
	for ru, n := range ruleCount {
		rq.ByRule = append(rq.ByRule, Count{Value: ru, Count: n, Percent: pct(n, rq.Requirements)})
	}
	sort.Slice(rq.ByRule, func(i, j int) bool {
		if rq.ByRule[i].Count != rq.ByRule[j].Count {
			return rq.ByRule[i].Count > rq.ByRule[j].Count
		}
		return rq.ByRule[i].Value < rq.ByRule[j].Value
	})
	rq.Items = topItems(rq.Items)
	rq.Trend = qualityTrend(in, starts, period, rep.Periods)
	rep.RequirementQuality = rq
	return rep
}

var defectPhases = []string{"requirements", "design", "development", "test", "acceptance", "production"}

// qualityTrend replays the requirement texts from their revisions: at the
// end of each period the share of the requirements existing then (and
// active) whose text had analysis warnings.
func qualityTrend(in QualityInput, starts []time.Time, period string, labels []string) []PeriodValue {
	out := make([]PeriodValue, len(starts))
	byReq := map[string][]QualityRevision{}
	for _, rv := range in.Revisions {
		if rv.HasText {
			byReq[rv.RequirementID] = append(byReq[rv.RequirementID], rv)
		}
	}
	cache := map[string][]string{}
	analyse := func(text string) bool {
		r, ok := cache[text]
		if !ok {
			r = warned(text, in.Lang)
			cache[text] = r
		}
		return len(r) > 0
	}
	now := in.Now.UTC()
	for i, s := range starts {
		end := nextPeriod(s, period)
		if end.After(now) {
			end = now.Add(time.Nanosecond)
		}
		out[i].Period = labels[i]
		for _, r := range in.Requirements {
			if !r.CreatedAt.Before(end) {
				continue
			}
			text, status := reqText(r.Title, r.Description), r.Status
			revs := byReq[r.ID]
			// the latest revision before the end of the period, if any
			// revision is known; else the current text
			k := sort.Search(len(revs), func(j int) bool { return !revs[j].At.Before(end) })
			if k > 0 {
				text, status = reqText(revs[k-1].Title, revs[k-1].Description), revs[k-1].Status
			}
			if !activeStatus(status) {
				continue
			}
			out[i].Total++
			if analyse(text) {
				out[i].Count++
			}
		}
		if out[i].Total > 0 {
			out[i].Value = pct(out[i].Count, out[i].Total)
		}
	}
	return out
}

// topItems sorts by count (then key) and keeps QualityTopItems.
func topItems(items []QualityItem) []QualityItem {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Key < items[j].Key
	})
	if len(items) > QualityTopItems {
		items = items[:QualityTopItems]
	}
	return items
}
