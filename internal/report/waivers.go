package report

import (
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// Waivers and deviations in the reports and documents: the waiver section
// of the compliance and release reports, the release readiness report and
// the appendix of the requirements specification. Self-contained: it
// registers its template, names and translations here.

// ReleaseReadiness is the id of the release readiness report.
const ReleaseReadiness = "release"

// Enumeration kinds of waivers.
const (
	EnumWaiverStatus = "waiver_status"
	EnumWaiverType   = "waiver_type"
	EnumCriterion    = "exit_criterion"
	// EnumResidualSource: where a residual score comes from.
	EnumResidualSource = "residual_source"
)

func init() {
	templates = append(templates, Template{ID: ReleaseReadiness, Name: "Release readiness",
		Description: "One release: go / no-go, every exit criterion with what keeps it from being met and what a waiver accepts, the waivers in force and the risks whose residual level is above the acceptance threshold.",
		Params:      []Param{{Name: "release_id", Type: "release", Description: "the release (default: the newest release that is not released or cancelled)"}},
		build:       buildReleaseReadiness})
	templateNames[Swedish][ReleaseReadiness] = "Releaseberedskap"
	for k, v := range waiverTexts {
		if _, ok := texts[Swedish][k]; !ok {
			texts[Swedish][k] = v
		}
	}
	enums[Swedish][EnumWaiverStatus] = map[string]string{"draft": "Utkast", "requested": "Begärt", "approved": "Godkänt", "rejected": "Avslaget", "expired": "Utgånget", "revoked": "Återkallat"}
	enums[Swedish][EnumWaiverType] = map[string]string{"waiver": "Undantag", "deviation": "Avvikelse"}
	enums[English][EnumCriterion] = criterionNames
	enums[English][EnumResidualSource] = map[string]string{model.ResidualInherent: "Inherent (mitigation not verified)", model.ResidualVerified: "Verified mitigation", model.ResidualManual: "Entered by hand"}
	enums[Swedish][EnumResidualSource] = map[string]string{model.ResidualInherent: "Ursprunglig (åtgärden ej verifierad)", model.ResidualVerified: "Verifierad åtgärd", model.ResidualManual: "Angiven för hand"}
	enums[Swedish][EnumCriterion] = map[string]string{"coverage": "Täckning", "passed": "Godkända krav", "mandatory_passed": "MUST-krav godkända",
		"executed": "Utförda exekveringar", "no_failed": "Inga underkända testfall", "approved": "Godkända krav (status)", "no_suspect": "Inga misstänkta länkar",
		"no_open_defects": "Inga öppna defekter", "risks_mitigated": "Risker åtgärdade", "trace_rules": "Spårbarhetsregler", "residual_risk": "Restrisk inom gränsen",
		"no_blocking_defects": "Inga blockerande defekter", "cycle_executed": "Testcykel genomförd"}
}

var criterionNames = map[string]string{"coverage": "Coverage", "passed": "Requirements passed", "mandatory_passed": "MUST requirements passed",
	"executed": "Executions done", "no_failed": "No failed tests", "approved": "Requirements approved", "no_suspect": "No suspect links",
	"no_open_defects": "No open defects", "risks_mitigated": "Risks mitigated", "trace_rules": "Traceability rules", "residual_risk": "Residual risk within threshold",
	"no_blocking_defects": "No blocking defects", "cycle_executed": "Test cycle executed"}

// waiverTexts are the Swedish texts of waivers, residual risk and the SoA.
var waiverTexts = map[string]string{
	"Waivers and deviations":             "Undantag och avvikelser",
	"Type":                               "Typ",
	"Scope":                              "Omfattning",
	"Whole project":                      "Hela projektet",
	"Expires":                            "Går ut",
	"Approved by":                        "Godkänt av",
	"Justification":                      "Motivering",
	"Compensating measures":              "Kompenserande åtgärder",
	"Conditions":                         "Villkor",
	"Waived":                             "Undantagna",
	"state|Waived":                       "Undantaget",
	"Waived by":                          "Undantaget genom",
	"Release":                            "Release",
	"Version":                            "Version",
	"Target date":                        "Måldatum",
	"Verdict":                            "Beslut",
	"Go":                                 "Go",
	"No-go":                              "No-go",
	"Requirements in scope":              "Krav i omfattningen",
	"Waived requirements":                "Undantagna krav",
	"Risk acceptance threshold":          "Gräns för riskacceptans",
	"Residual risks above the threshold": "Restrisker över gränsen",
	"Residual risks above the threshold, accepted": "Restrisker över gränsen, accepterade",
	"Exit criteria":                        "Slutkriterier",
	"Criterion":                            "Kriterium",
	"Threshold or level":                   "Gräns eller nivå",
	"Actual":                               "Utfall",
	"Met":                                  "Uppfyllt",
	"Not met because of":                   "Ej uppfyllt på grund av",
	"Accepted by waiver or sign-off":       "Accepterat genom undantag eller signering",
	"Yes":                                  "Ja",
	"No":                                   "Nej",
	"Not evaluated":                        "Ej utvärderat",
	"Risks above the acceptance threshold": "Risker över acceptansgränsen",
	"Residual level":                       "Restnivå",
	"Accepted":                             "Accepterad",
	"No release is selected or planned.":   "Ingen release är vald eller planerad.",
	"Waivers in force":                     "Gällande undantag",
	"The waivers and deviations of the project: requirements that are accepted although they are not, or differently, met.":                "Projektets undantag och avvikelser: krav som accepteras trots att de inte uppfylls, eller uppfylls på annat sätt.",
	"Requirements waived for a release or the project are marked in the requirement; an expired or revoked waiver no longer accepts them.": "Krav som är undantagna för en release eller för projektet är markerade vid kravet; ett utgånget eller återkallat undantag accepterar dem inte längre.",
	"{key} ({type}, until {date})": "{key} ({type}, till {date})",
	"{key} ({type})":               "{key} ({type})",
	"Risk":                         "Risk",
	"Approvals":                    "Godkännanden",
	"Heat map after mitigation (residual, risks not closed)": "Värmekarta efter åtgärder (restrisk, risker som inte är stängda)",
	"Heat map before mitigation (risks not closed)":          "Värmekarta före åtgärder (risker som inte är stängda)",
	"Risks above the acceptance threshold after mitigation":  "Risker över acceptansgränsen efter åtgärder",
	"Residual likelihood":    "Sannolikhet efter åtgärder",
	"Residual impact":        "Konsekvens efter åtgärder",
	"Residual score":         "Restriskvärde",
	"Residual from":          "Restrisk enligt",
	"Residual justification": "Motivering av restrisk",
	"Above threshold":        "Över gränsen",
	"Accepted by":            "Accepterad av",
	"Review date":            "Granskningsdatum",
	"Review owner":           "Ansvarig för granskning",
	"Risks above the threshold after mitigation, not accepted":                                                                        "Risker över gränsen efter åtgärder, ej accepterade",
	"Risks above the threshold after mitigation, accepted":                                                                            "Risker över gränsen efter åtgärder, accepterade",
	"No risk linked to the scope has a residual level above {level} unless its acceptance is signed.":                                 "Ingen risk kopplad till omfattningen har en restnivå över {level} utan att accepteringen är signerad.",
	"No risk linked to the scope has a residual level above the project's risk acceptance threshold unless its acceptance is signed.": "Ingen risk kopplad till omfattningen har en restnivå över projektets gräns för riskacceptans utan att accepteringen är signerad.",
	"Risks due for review": "Risker som ska granskas",
}

// WaiverInForce reports whether a waiver is approved and not expired on
// day (YYYY-MM-DD).
func WaiverInForce(w *model.Waiver, day string) bool {
	return w.Status == "approved" && (w.ExpiresOn == "" || w.ExpiresOn >= day)
}

// waivedRequirements maps the requirements accepted today by a
// project-wide waiver in force to the waiver keys.
func (d *Data) waivedRequirements() map[string][]string {
	today := d.now().Format("2006-01-02")
	out := map[string][]string{}
	for i := range d.Waivers {
		w := &d.Waivers[i]
		if w.ReleaseID == nil && WaiverInForce(w, today) {
			for _, id := range w.RequirementIDs {
				out[id] = append(out[id], w.Key)
			}
		}
	}
	return out
}

// approvedBy lists who approved a waiver (valid approvals).
func approvedBy(w *model.Waiver) string {
	var out []string
	for _, a := range w.Approvals {
		if a.Valid && a.Decision == "approve" {
			name := a.UserName
			if name == "" {
				name = a.UserEmail
			}
			out = append(out, name+" "+a.SignedAt.UTC().Format("2006-01-02"))
		}
	}
	return strings.Join(out, "; ")
}

func waiverReqKeys(w *model.Waiver) string {
	keys := make([]string, len(w.Requirements))
	for i, r := range w.Requirements {
		keys[i] = r.Key
	}
	return strings.Join(keys, ", ")
}

func (l loc) waiverScope(w *model.Waiver) string {
	if w.ReleaseID == nil {
		return l.t("Whole project")
	}
	if w.ReleaseName != "" {
		return w.ReleaseName
	}
	return l.t("Release")
}

// waiverSection tabulates waivers.
func (l loc) waiverSection(name, title string, list []model.Waiver) export.Section {
	s := export.Section{Name: name, Title: l.t(title), Columns: l.cols("Key", "Type", "Title", "Status", "Scope", "Requirements", "Expires", "Approved by",
		"Justification", "Compensating measures", "Conditions", "Risk")}
	for i := range list {
		w := &list[i]
		s.Rows = append(s.Rows, []string{w.Key, l.enum(EnumWaiverType, w.Type), w.Title, l.enum(EnumWaiverStatus, w.Status), l.waiverScope(w), waiverReqKeys(w),
			w.ExpiresOn, approvedBy(w), w.Justification, w.Compensating, w.Conditions, w.RiskKey})
	}
	return s
}

// relevantWaivers are the waivers worth reporting: in force, requested,
// expired or revoked (drafts and rejected ones are left out).
func relevantWaivers(list []model.Waiver) []model.Waiver {
	out := []model.Waiver{}
	for _, w := range list {
		if w.Status != "draft" && w.Status != "rejected" {
			out = append(out, w)
		}
	}
	return out
}

// ---------------------------------------------------------------- release readiness

// ReleaseRef is an item an exit criterion refers to.
type ReleaseRef struct {
	Kind, Key, Title, Note string
}

// ReleaseCriterion is an evaluated exit criterion.
type ReleaseCriterion struct {
	Type      string
	Threshold float64
	Level     string
	Met       bool
	Available bool
	Actual    float64
	Total     int
	Items     []ReleaseRef
	Waived    []ReleaseRef
	Accepted  []ReleaseRef
}

// ReleaseReport is the evaluated release of the release readiness report.
type ReleaseReport struct {
	Release       model.Release
	Go            bool
	Criteria      []ReleaseCriterion
	Summary       map[string]int
	Waivers       []model.Waiver
	RiskThreshold string
	ResidualRisks []ReleaseRef
}

func refKeys(rs []ReleaseRef) string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Key
		if r.Note != "" {
			out[i] += " (" + r.Note + ")"
		}
	}
	return strings.Join(out, ", ")
}

func (l loc) yesNo(b bool) string {
	if b {
		return l.t("Yes")
	}
	return l.t("No")
}

func buildReleaseReadiness(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	rr := d.Release
	if rr == nil {
		return &export.Dataset{Sections: []export.Section{l.kpiSection("summary", "Key figures", kv{"Release", l.t("No release is selected or planned.")})}}, nil
	}
	rel := rr.Release
	verdict := l.t("No-go")
	if rr.Go {
		verdict = l.t("Go")
	}
	accepted := 0
	for _, r := range rr.ResidualRisks {
		if strings.HasSuffix(r.Note, ", accepted") {
			accepted++
		}
	}
	summary := l.kpiSection("summary", "Key figures",
		kv{"Release", rel.Name},
		kv{"Version", rel.Version},
		kv{"Status", l.enum(EnumStatus, rel.Status)},
		kv{"Target date", rel.TargetDate},
		kv{"Verdict", verdict},
		kv{"Requirements in scope", itoa(rr.Summary["requirements"])},
		kv{"Waived requirements", itoa(rr.Summary["waived"])},
		kv{"Risk acceptance threshold", l.enum(EnumRiskLevel, rr.RiskThreshold)},
		kv{"Residual risks above the threshold", itoa(len(rr.ResidualRisks) - accepted)},
		kv{"Residual risks above the threshold, accepted", itoa(accepted)},
	)
	crit := export.Section{Name: "exit_criteria", Title: l.t("Exit criteria"),
		Columns: l.cols("Criterion", "Threshold or level", "Actual", "Met", "Not met because of", "Accepted by waiver or sign-off")}
	for _, c := range rr.Criteria {
		lim := ""
		switch {
		case c.Threshold > 0:
			lim = strconv.FormatFloat(c.Threshold, 'f', -1, 64) + "%"
		case c.Type == "no_open_defects":
			lim = l.enum(EnumSeverity, c.Level)
		case c.Level != "":
			lim = l.enum(EnumRiskLevel, c.Level)
		}
		met := l.yesNo(c.Met)
		if !c.Available && c.Met {
			met = l.t("Not evaluated")
		}
		actual := strconv.FormatFloat(c.Actual, 'f', -1, 64)
		if c.Type == "coverage" || c.Type == "passed" || c.Type == "executed" || c.Type == "approved" || c.Type == "cycle_executed" {
			actual += "%"
		}
		crit.Rows = append(crit.Rows, []string{l.enum(EnumCriterion, c.Type), lim, actual, met, refKeys(c.Items), refKeys(append(append([]ReleaseRef{}, c.Waived...), c.Accepted...))})
	}
	risks := export.Section{Name: "residual_risks", Title: l.t("Risks above the acceptance threshold"), Columns: l.cols("Key", "Title", "Residual level", "Accepted")}
	for _, r := range rr.ResidualRisks {
		level, acc := strings.CutSuffix(r.Note, ", accepted")
		risks.Rows = append(risks.Rows, []string{r.Key, r.Title, l.enum(EnumRiskLevel, level), l.yesNo(acc)})
	}
	ds := &export.Dataset{Sections: []export.Section{summary, crit, l.waiverSection("waivers", "Waivers in force", rr.Waivers), risks}}
	ds.Subtitle = d.subtitle() + " · " + rel.Name
	return ds, nil
}

// ---------------------------------------------------------------- specification

// waiversOf returns the waivers of a requirement worth mentioning in a
// specification, in force first.
func (g *docGen) waiversOf(id string) []*model.Waiver {
	var out []*model.Waiver
	for i := range g.d.Waivers {
		w := &g.d.Waivers[i]
		if (w.Status == "approved" || w.Status == "requested") && containsID(w.RequirementIDs, id) {
			out = append(out, w)
		}
	}
	return out
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// waiverField describes the waivers of a requirement ("" none).
func (g *docGen) waiverField(id string) string {
	var out []string
	for _, w := range g.waiversOf(id) {
		typ := g.enum(EnumWaiverType, w.Type)
		if w.Status != "approved" {
			typ += ", " + g.enum(EnumWaiverStatus, w.Status)
		}
		if w.ExpiresOn != "" {
			out = append(out, g.f("{key} ({type}, until {date})", "key", w.Key, "type", typ, "date", w.ExpiresOn))
		} else {
			out = append(out, g.f("{key} ({type})", "key", w.Key, "type", typ))
		}
	}
	return strings.Join(out, "; ")
}

// specWaivers reports whether the specification has waivers to list.
func (g *docGen) specWaivers() bool {
	for _, w := range relevantWaivers(g.d.Waivers) {
		for _, id := range w.RequirementIDs {
			if g.reqByID[id] != nil {
				return true
			}
		}
	}
	return false
}

// waiverAppendix lists the waivers and deviations of the specification's
// requirements.
func (g *docGen) waiverAppendix() {
	l := g.l
	var list []model.Waiver
	for _, w := range relevantWaivers(g.d.Waivers) {
		for _, id := range w.RequirementIDs {
			if g.reqByID[id] != nil {
				list = append(list, w)
				break
			}
		}
	}
	if len(list) == 0 {
		return
	}
	g.add(export.PageBreak())
	g.h(1, l.t("Waivers and deviations"))
	g.add(export.Paragraph(l.t("The waivers and deviations of the project: requirements that are accepted although they are not, or differently, met.")))
	g.add(export.Paragraph(l.t("Requirements waived for a release or the project are marked in the requirement; an expired or revoked waiver no longer accepts them.")))
	rows := make([][]string, len(list))
	for i := range list {
		w := &list[i]
		rows[i] = []string{w.Key, g.enum(EnumWaiverType, w.Type), w.Title, g.enum(EnumWaiverStatus, w.Status), l.waiverScope(w), waiverReqKeys(w), w.ExpiresOn}
	}
	g.add(export.Table(l.cols("Key", "Type", "Title", "Status", "Scope", "Requirements", "Expires"), rows, 1.2, 1, 2.4, 1, 1.2, 1.6, 1))
	for i := range list {
		w := &list[i]
		g.itemHeading(2, w.Key, w.Title)
		fs := []export.Item{{Label: l.t("Justification"), Value: w.Justification}}
		if w.Compensating != "" {
			fs = append(fs, export.Item{Label: l.t("Compensating measures"), Value: w.Compensating})
		}
		if w.Conditions != "" {
			fs = append(fs, export.Item{Label: l.t("Conditions"), Value: w.Conditions})
		}
		if a := approvedBy(w); a != "" {
			fs = append(fs, export.Item{Label: l.t("Approved by"), Value: a})
		}
		g.add(export.Fields(fs...))
	}
}
