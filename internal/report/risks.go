package report

import (
	"slices"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// The risk report: key figures, risks by level and status, the
// likelihood × impact heat map, the register, the traceability of every
// risk to its requirements, their tests and latest results, the open risks
// whose mitigation is not (fully) tested, the top risks and the open risks
// that have not been updated for a long time.
//
// A risk has one likelihood × impact score (there is no separate residual
// score after mitigation); how far its mitigation is verified is derived
// from the requirements that address it ("mitigation state").

// Mitigation states of a risk (EnumMitigation).
const (
	MitigationNone     = "no_requirements" // no requirement addresses it
	MitigationUntested = "untested"        // none of its requirements has tests
	MitigationPartly   = "partly_tested"   // some of its requirements have tests
	MitigationTested   = "tested"          // all have tests, not all of them passed
	MitigationVerified = "verified"        // all have tests and all of them passed
)

// MitigationStates in the order of the report.
var MitigationStates = []string{MitigationNone, MitigationUntested, MitigationPartly, MitigationTested, MitigationVerified}

// StaleRiskDays: open risks not updated for this many days are listed as
// due for review.
const StaleRiskDays = 90

// TopRisks bounds the top risks section.
const TopRisks = 10

// MitigationState derives the state of a risk's mitigation from the
// coverage of the requirements that address it (requirement id →
// coverage: none, not_run, failed, partial, passed; missing = none).
func MitigationState(reqIDs []string, coverage map[string]string) string {
	if len(reqIDs) == 0 {
		return MitigationNone
	}
	tested, passed := 0, 0
	for _, id := range reqIDs {
		c := coverage[id]
		if c != "" && c != "none" {
			tested++
		}
		if c == model.ResultPassed {
			passed++
		}
	}
	switch {
	case tested == 0:
		return MitigationUntested
	case tested < len(reqIDs):
		return MitigationPartly
	case passed < len(reqIDs):
		return MitigationTested
	}
	return MitigationVerified
}

func riskOpen(status string) bool { return slices.Contains(model.RiskOpenStatuses, status) }

func buildRisks(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	reqs := map[string]*model.Requirement{}
	coverage := map[string]string{}
	for i := range d.Requirements {
		r := &d.Requirements[i]
		reqs[r.ID] = r
		coverage[r.ID] = r.Coverage
	}
	tests := d.testIndex()
	r2t := d.linksByRequirement()
	now := d.now()

	risks := append([]model.Risk{}, d.Risks...)
	for i := range risks {
		k := &risks[i]
		k.Score = k.Likelihood * k.Impact
		k.Level = model.RiskLevel(k.Likelihood, k.Impact)
	}
	sort.SliceStable(risks, func(i, j int) bool {
		if risks[i].Score != risks[j].Score {
			return risks[i].Score > risks[j].Score
		}
		return naturalLess(risks[i].Key, risks[j].Key)
	})

	byStatus, openByLevel, states := map[string]int{}, map[string]int{}, map[string]int{}
	cross := map[string]map[string]int{}
	open, stale := 0, 0
	for _, k := range risks {
		byStatus[k.Status]++
		if cross[k.Level] == nil {
			cross[k.Level] = map[string]int{}
		}
		cross[k.Level][k.Status]++
		if riskOpen(k.Status) {
			open++
			openByLevel[k.Level]++
			states[MitigationState(k.RequirementIDs, coverage)]++
			if now.Sub(k.UpdatedAt).Hours() >= 24*StaleRiskDays {
				stale++
			}
		}
	}
	levelsHigh := append([]string{}, model.RiskLevels...)
	slices.Reverse(levelsHigh)

	// level × status
	cols := []string{l.t("Level")}
	for _, st := range model.RiskStatuses {
		cols = append(cols, l.enum(EnumRiskStatus, st))
	}
	cols = append(cols, l.t("Total"))
	byLevel := export.Section{Name: "risks_by_level_and_status", Title: l.t("Risks by level and status"), Columns: cols}
	totals := map[string]int{}
	for _, lv := range levelsHigh {
		row := []string{l.enum(EnumRiskLevel, lv)}
		sum := 0
		for _, st := range model.RiskStatuses {
			n := cross[lv][st]
			row = append(row, itoa(n))
			sum += n
			totals[st] += n
		}
		byLevel.Rows = append(byLevel.Rows, append(row, itoa(sum)))
	}
	row := []string{l.t("Total")}
	for _, st := range model.RiskStatuses {
		row = append(row, itoa(totals[st]))
	}
	byLevel.Rows = append(byLevel.Rows, append(row, itoa(len(risks))))

	// heat map: likelihood 5 (top) to 1, impact 1 to 5; closed risks are left out
	heat := export.Section{Name: "heat_map", Title: l.t("Heat map (risks not closed)"), Columns: []string{l.t("Likelihood \\ impact")}}
	for i := 1; i <= 5; i++ {
		heat.Columns = append(heat.Columns, itoa(i))
	}
	var grid [6][6]int
	for _, k := range risks {
		if k.Status != "closed" && k.Likelihood >= 1 && k.Likelihood <= 5 && k.Impact >= 1 && k.Impact <= 5 {
			grid[k.Likelihood][k.Impact]++
		}
	}
	for li := 5; li >= 1; li-- {
		row := []string{itoa(li)}
		for im := 1; im <= 5; im++ {
			row = append(row, itoa(grid[li][im])+" · "+l.enum(EnumRiskLevel, model.RiskLevel(li, im)))
		}
		heat.Rows = append(heat.Rows, row)
	}

	keysOf := func(ids []string) string {
		var out []string
		for _, id := range ids {
			if r, ok := reqs[id]; ok {
				out = append(out, r.Key)
			}
		}
		return strings.Join(out, ", ")
	}

	register := export.Section{Name: "register", Title: l.t("Risk register"),
		Columns: l.cols("Key", "Title", "Category", "Owner", "Likelihood", "Impact", "Score", "Level", "Status", "Mitigation", "Requirements", "Mitigation state", "Updated")}
	for _, k := range risks {
		register.Rows = append(register.Rows, []string{k.Key, k.Title, k.Category, k.Owner, itoa(k.Likelihood), itoa(k.Impact), itoa(k.Score),
			l.enum(EnumRiskLevel, k.Level), l.enum(EnumRiskStatus, k.Status), k.Mitigation, keysOf(k.RequirementIDs),
			l.enum(EnumMitigation, MitigationState(k.RequirementIDs, coverage)), ts(k.UpdatedAt)})
	}

	trace := export.Section{Name: "traceability", Title: l.t("Traceability: risks, requirements and tests"),
		Columns: l.cols("Risk", "Level", "Status", "Requirement", "Requirement title", "Coverage", "Test", "Test title", "Last result")}
	untested := export.Section{Name: "untested_mitigations", Title: l.t("Open risks with untested mitigation"),
		Columns: l.cols("Key", "Title", "Level", "Status", "Mitigation state", "Requirements without tests", "Requirements not passed")}
	for _, k := range risks {
		lv, st := l.enum(EnumRiskLevel, k.Level), l.enum(EnumRiskStatus, k.Status)
		if len(k.RequirementIDs) == 0 {
			trace.Rows = append(trace.Rows, []string{k.Key, lv, st, "", "", "", "", "", ""})
		}
		var noTests, notPassed []string
		for _, id := range k.RequirementIDs {
			r, ok := reqs[id]
			if !ok {
				continue
			}
			n := 0
			for _, tid := range r2t[id] {
				t, ok := tests[tid]
				if !ok {
					continue
				}
				n++
				trace.Rows = append(trace.Rows, []string{k.Key, lv, st, r.Key, r.Title, l.enum(EnumCoverage, r.Coverage), t.Key, t.Title, l.enum(EnumResult, lastResult(t))})
			}
			if n == 0 {
				noTests = append(noTests, r.Key)
				trace.Rows = append(trace.Rows, []string{k.Key, lv, st, r.Key, r.Title, l.enum(EnumCoverage, r.Coverage), "", "", ""})
			} else if r.Coverage != model.ResultPassed {
				notPassed = append(notPassed, r.Key)
			}
		}
		if state := MitigationState(k.RequirementIDs, coverage); riskOpen(k.Status) && state != MitigationVerified && state != MitigationTested {
			untested.Rows = append(untested.Rows, []string{k.Key, k.Title, lv, st, l.enum(EnumMitigation, state), strings.Join(noTests, ", "), strings.Join(notPassed, ", ")})
		}
	}

	top := export.Section{Name: "top_risks", Title: l.t("Top open risks"),
		Columns: l.cols("#", "Key", "Title", "Score", "Level", "Status", "Owner", "Mitigation state")}
	for _, k := range risks {
		if len(top.Rows) == TopRisks {
			break
		}
		if riskOpen(k.Status) {
			top.Rows = append(top.Rows, []string{itoa(len(top.Rows) + 1), k.Key, k.Title, itoa(k.Score), l.enum(EnumRiskLevel, k.Level),
				l.enum(EnumRiskStatus, k.Status), k.Owner, l.enum(EnumMitigation, MitigationState(k.RequirementIDs, coverage))})
		}
	}

	due := export.Section{Name: "due_for_review", Title: l.t("Open risks not updated for 90 days or more"),
		Columns: l.cols("Key", "Title", "Level", "Status", "Owner", "Updated", "Days since update")}
	for _, k := range risks {
		if days := int(now.Sub(k.UpdatedAt).Hours() / 24); riskOpen(k.Status) && days >= StaleRiskDays {
			due.Rows = append(due.Rows, []string{k.Key, k.Title, l.enum(EnumRiskLevel, k.Level), l.enum(EnumRiskStatus, k.Status), k.Owner, ts(k.UpdatedAt), itoa(days)})
		}
	}

	summary := l.kpiSection("summary", "Key figures",
		kv{"Risks", itoa(len(risks))},
		kv{"Open risks (identified or mitigating)", itoa(open)},
		kv{"Mitigated risks", itoa(byStatus["mitigated"])},
		kv{"Accepted risks", itoa(byStatus["accepted"])},
		kv{"Closed risks", itoa(byStatus["closed"])},
		kv{"Open critical risks", itoa(openByLevel["critical"])},
		kv{"Open high risks", itoa(openByLevel["high"])},
		kv{"Open medium risks", itoa(openByLevel["medium"])},
		kv{"Open low risks", itoa(openByLevel["low"])},
		kv{"Open risks without requirements", itoa(states[MitigationNone])},
		kv{"Open risks with untested mitigation", itoa(states[MitigationNone] + states[MitigationUntested] + states[MitigationPartly])},
		kv{"Open risks with verified mitigation", itoa(states[MitigationVerified])},
		kv{"Share of open risks verified", l.pct(states[MitigationVerified], open)},
		kv{"Open risks not updated for 90 days or more", itoa(stale)},
	)
	return &export.Dataset{Sections: []export.Section{
		summary,
		byLevel,
		l.countSection("open_by_mitigation_state", "Open risks by mitigation state", "Mitigation state", states, MitigationStates, l.enumLabels(EnumMitigation, MitigationStates)),
		heat,
		top,
		register,
		trace,
		untested,
		due,
	}}, nil
}
