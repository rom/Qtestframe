package report

import (
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// Exploratory testing reports: one session (charter, timer, debrief,
// notes and defects) and the summary of a project's sessions (metrics per
// area and tester, the session list).

// Template ids.
const (
	ExploratorySession  = "exploratory_session"
	ExploratorySessions = "exploratory_sessions"
)

// Enumeration kinds of exploratory testing.
const (
	EnumExploratoryStatus  = "exploratory_status"
	EnumExploratoryNote    = "exploratory_note"
	EnumExploratoryOutcome = "exploratory_outcome"
)

// ExploratoryData is what the exploratory reports need: the session with
// its notes (session report) or the project's sessions and metrics
// (summary).
type ExploratoryData struct {
	Session  *model.ExploratorySession
	Notes    []model.ExploratoryNote
	Sessions []model.ExploratorySession
	Metrics  *model.ExploratoryMetrics
}

func init() {
	templates = append(templates,
		Template{ID: ExploratorySession, Name: "Exploratory session",
			Description: "One exploratory test session: the charter and its scope, the session time, the debrief (summary, coverage, time split, outcome), every note in the order taken and the defects raised.",
			Params:      []Param{{Name: "session_id", Type: "exploratory_session", Description: "the session (default: the newest debriefed session)"}},
			build:       buildExploratorySession},
		Template{ID: ExploratorySessions, Name: "Exploratory testing",
			Description: "The project's exploratory test sessions: key figures, sessions and bugs per session hour by area and by tester, the average time split, and every session with its outcome.",
			Params:      []Param{}, build: buildExploratorySessions})
	templateNames[Swedish][ExploratorySession] = "Utforskande testsession"
	templateNames[Swedish][ExploratorySessions] = "Utforskande test"
	for k, v := range exploratoryTexts {
		if _, ok := texts[Swedish][k]; !ok {
			texts[Swedish][k] = v
		}
	}
	enums[English][EnumExploratoryStatus] = map[string]string{"planned": "Planned", "in_progress": "In progress", "paused": "Paused", "completed": "Completed", "debriefed": "Debriefed"}
	enums[Swedish][EnumExploratoryStatus] = map[string]string{"planned": "Planerad", "in_progress": "Pågår", "paused": "Pausad", "completed": "Avslutad", "debriefed": "Utvärderad"}
	enums[English][EnumExploratoryNote] = map[string]string{"observation": "Observation", "question": "Question", "issue": "Issue", "idea": "Idea", "bug": "Bug"}
	enums[Swedish][EnumExploratoryNote] = map[string]string{"observation": "Observation", "question": "Fråga", "issue": "Problem", "idea": "Idé", "bug": "Bugg"}
	enums[English][EnumExploratoryOutcome] = map[string]string{"passed": "Passed", "failed": "Failed", "inconclusive": "Inconclusive"}
	enums[Swedish][EnumExploratoryOutcome] = map[string]string{"passed": "Godkänd", "failed": "Underkänd", "inconclusive": "Ofullständig"}
}

// exploratoryTexts are the Swedish texts of the exploratory reports.
var exploratoryTexts = map[string]string{
	"Session":                   "Session",
	"Field":                     "Fält",
	"Charter":                   "Uppdrag",
	"Mission":                   "Mål",
	"Tester":                    "Testare",
	"Time box":                  "Tidsram",
	"Session time":              "Sessionstid",
	"Started":                   "Startad",
	"Ended":                     "Avslutad",
	"Areas":                     "Områden",
	"Requirements":              "Krav",
	"Tests":                     "Testfall",
	"Debrief":                   "Utvärdering",
	"Summary":                   "Sammanfattning",
	"Outcome":                   "Utfall",
	"Coverage (estimate)":       "Täckning (uppskattning)",
	"Set-up":                    "Förberedelse",
	"Testing":                   "Testning",
	"Bug investigation":         "Felutredning",
	"Debriefed by":              "Utvärderad av",
	"Reviewer's comment":        "Granskarens kommentar",
	"Not debriefed yet":         "Inte utvärderad än",
	"No session":                "Ingen session",
	"Notes":                     "Anteckningar",
	"Time":                      "Tid",
	"Type":                      "Typ",
	"Note":                      "Anteckning",
	"Defect":                    "Defekt",
	"Files":                     "Filer",
	"By":                        "Av",
	"Defects":                   "Defekter",
	"Key":                       "Nyckel",
	"Title":                     "Titel",
	"Status":                    "Status",
	"Key figures":               "Nyckeltal",
	"Sessions":                  "Sessioner",
	"Debriefed sessions":        "Utvärderade sessioner",
	"Session hours":             "Sessionstimmar",
	"Bugs":                      "Buggar",
	"Bugs per session hour":     "Buggar per sessionstimme",
	"Average set-up":            "Genomsnittlig förberedelse",
	"Average testing":           "Genomsnittlig testning",
	"Average bug investigation": "Genomsnittlig felutredning",
	"Average coverage":          "Genomsnittlig täckning",
	"By area":                   "Per område",
	"By tester":                 "Per testare",
	"Area":                      "Område",
	"Hours":                     "Timmar",
	"No area":                   "Inget område",
	"Not assigned":              "Inte tilldelad",
	"All sessions":              "Alla sessioner",
	"Minutes":                   "Minuter",
	"minutes":                   "minuter",
}

// Clock formats seconds of session time as h:mm:ss (m:ss under an hour).
func Clock(sec int) string {
	h, m, s := sec/3600, sec/60%60, sec%60
	pad := func(n int) string {
		if n < 10 {
			return "0" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	if h > 0 {
		return strconv.Itoa(h) + ":" + pad(m) + ":" + pad(s)
	}
	return strconv.Itoa(m) + ":" + pad(s)
}

func (l loc) percent(v int) string {
	if v < 0 {
		return "–"
	}
	if l.lang == Swedish {
		return strconv.Itoa(v) + " %"
	}
	return strconv.Itoa(v) + "%"
}

func (l loc) avgPercent(v float64) string {
	if v < 0 {
		return "–"
	}
	if l.lang == Swedish {
		return l.num(v) + " %"
	}
	return l.num(v) + "%"
}

func scopeOf(x *model.ExploratorySession, kind string) string {
	var out []string
	for _, it := range x.Scope {
		if it.Type != kind {
			continue
		}
		switch {
		case kind == "folder":
			out = append(out, it.Title)
		case it.Title != "":
			out = append(out, it.Key+" "+it.Title)
		default:
			out = append(out, it.Key)
		}
	}
	return strings.Join(out, "; ")
}

func buildExploratorySession(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	ds := &export.Dataset{}
	if d.Exploratory == nil || d.Exploratory.Session == nil {
		ds.Sections = []export.Section{{Name: "session", Title: l.t("Session"), Columns: l.cols("Field", "Value"), Rows: [][]string{{l.t("Session"), l.t("No session")}}}}
		return ds, nil
	}
	x := d.Exploratory.Session
	ds.Subtitle = d.Project.Key + " – " + d.Project.Name + " · " + x.Key
	tester := x.Tester
	if tester == "" {
		tester = l.t("Not assigned")
	}
	session := export.Section{Name: "session", Title: l.t("Session"), Columns: l.cols("Field", "Value"), Rows: [][]string{
		{l.t("Key"), x.Key},
		{l.t("Charter"), x.Title},
		{l.t("Mission"), x.Mission},
		{l.t("Tester"), tester},
		{l.t("Status"), l.enum(EnumExploratoryStatus, x.Status)},
		{l.t("Time box"), strconv.Itoa(x.TimeboxMinutes) + " " + l.t("minutes")},
		{l.t("Session time"), Clock(x.ElapsedSeconds)},
		{l.t("Started"), tsPtr(x.StartedAt)},
		{l.t("Ended"), tsPtr(x.EndedAt)},
		{l.t("Areas"), scopeOf(x, "folder")},
		{l.t("Requirements"), scopeOf(x, model.KindRequirement)},
		{l.t("Tests"), scopeOf(x, model.KindTest)},
	}}
	debrief := export.Section{Name: "debrief", Title: l.t("Debrief"), Columns: l.cols("Field", "Value")}
	if b := x.Debrief; b != nil {
		by := b.Name
		if by == "" {
			by = b.By
		}
		debrief.Rows = [][]string{
			{l.t("Outcome"), l.enum(EnumExploratoryOutcome, b.Outcome)},
			{l.t("Summary"), b.Summary},
			{l.t("Coverage (estimate)"), l.percent(b.CoveragePct)},
			{l.t("Set-up"), l.percent(b.SetupPct)},
			{l.t("Testing"), l.percent(b.TestingPct)},
			{l.t("Bug investigation"), l.percent(b.BugPct)},
			{l.t("Debriefed by"), by + " · " + ts(b.At)},
		}
		if b.Comment != "" {
			debrief.Rows = append(debrief.Rows, []string{l.t("Reviewer's comment"), b.Comment})
		}
	} else {
		debrief.Rows = [][]string{{l.t("Status"), l.t("Not debriefed yet")}}
	}
	notes := export.Section{Name: "notes", Title: l.t("Notes"), Columns: l.cols("Time", "Type", "Note", "Defect", "Files", "By")}
	for _, n := range d.Exploratory.Notes {
		var files []string
		for _, f := range n.Files {
			files = append(files, f.Name)
		}
		notes.Rows = append(notes.Rows, []string{Clock(n.ElapsedSeconds), l.enum(EnumExploratoryNote, n.Type), n.Body, n.DefectKey, strings.Join(files, ", "), n.CreatedBy})
	}
	defects := export.Section{Name: "defects", Title: l.t("Defects"), Columns: l.cols("Key", "Title")}
	for _, df := range x.Defects {
		defects.Rows = append(defects.Rows, []string{df.Key, df.Title})
	}
	ds.Sections = []export.Section{session, debrief, notes, defects}
	return ds, nil
}

func buildExploratorySessions(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	var list []model.ExploratorySession
	m := &model.ExploratoryMetrics{AvgSetupPct: -1, AvgTestingPct: -1, AvgBugPct: -1, AvgCoveragePct: -1}
	if d.Exploratory != nil {
		list = d.Exploratory.Sessions
		if d.Exploratory.Metrics != nil {
			m = d.Exploratory.Metrics
		}
	}
	debriefed := 0
	for _, x := range list {
		if x.Debrief != nil {
			debriefed++
		}
	}
	kpi := l.kpiSection("key_figures", "Key figures",
		kv{"Sessions", itoa(m.Sessions)},
		kv{"Debriefed sessions", itoa(debriefed)},
		kv{"Session hours", l.num(m.Hours)},
		kv{"Bugs", itoa(m.Bugs)},
		kv{"Bugs per session hour", l.num(m.BugsPerHour)},
		kv{"Average set-up", l.avgPercent(m.AvgSetupPct)},
		kv{"Average testing", l.avgPercent(m.AvgTestingPct)},
		kv{"Average bug investigation", l.avgPercent(m.AvgBugPct)},
		kv{"Average coverage", l.avgPercent(m.AvgCoveragePct)},
	)
	group := func(name, title, col, empty string, rows []model.ExploratoryGroup) export.Section {
		s := export.Section{Name: name, Title: l.t(title), Columns: l.cols(col, "Sessions", "Hours", "Bugs", "Bugs per session hour")}
		for _, g := range rows {
			n := g.Name
			if g.ID == "" {
				n = l.t(empty)
			}
			s.Rows = append(s.Rows, []string{n, itoa(g.Sessions), l.num(g.Hours), itoa(g.Bugs), l.num(g.BugsPerHour)})
		}
		return s
	}
	sessions := export.Section{Name: "sessions", Title: l.t("All sessions"), Columns: l.cols("Key", "Charter", "Tester", "Status", "Outcome", "Session time", "Notes", "Bugs", "Coverage (estimate)")}
	for _, x := range list {
		outcome, cov := "", "–"
		if x.Debrief != nil {
			outcome, cov = l.enum(EnumExploratoryOutcome, x.Debrief.Outcome), l.percent(x.Debrief.CoveragePct)
		}
		sessions.Rows = append(sessions.Rows, []string{x.Key, x.Title, x.Tester, l.enum(EnumExploratoryStatus, x.Status), outcome, Clock(x.ElapsedSeconds),
			itoa(x.NoteCount), itoa(x.BugCount), cov})
	}
	return &export.Dataset{Sections: []export.Section{kpi,
		group("by_area", "By area", "Area", "No area", m.ByArea),
		group("by_tester", "By tester", "Tester", "Not assigned", m.ByTester),
		sessions}}, nil
}
