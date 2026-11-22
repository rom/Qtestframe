package report

import (
	"sort"
	"strconv"
	"strings"
)

// Report languages. Every text a report adds (titles, section names,
// column headers, labels and enumeration values) is translated; the
// project's own content (titles, names, notes, …) never is. English is the
// default and the fallback for unknown languages.
const (
	English = "en"
	Swedish = "sv"
)

// Languages lists the supported report languages.
var Languages = []string{English, Swedish}

// NormalizeLang returns the supported language of a language tag ("sv",
// "SV", "sv-SE", "sv_FI" → "sv") or "" when it is not supported.
func NormalizeLang(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		tag = tag[:i]
	}
	for _, l := range Languages {
		if tag == l {
			return l
		}
	}
	return ""
}

// LangFromAcceptLanguage returns the supported language the client
// prefers most in an Accept-Language header ("" when none is supported).
func LangFromAcceptLanguage(header string) string {
	type cand struct {
		lang string
		q    float64
		i    int
	}
	var cs []cand
	for i, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		l := NormalizeLang(fields[0])
		if l == "" {
			continue
		}
		q := 1.0
		for _, p := range fields[1:] {
			p = strings.TrimSpace(p)
			if v, ok := strings.CutPrefix(p, "q="); ok {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					q = f
				}
			}
		}
		if q > 0 {
			cs = append(cs, cand{l, q, i})
		}
	}
	if len(cs) == 0 {
		return ""
	}
	sort.SliceStable(cs, func(a, b int) bool { return cs[a].q > cs[b].q })
	return cs[0].lang
}

// ResolveLang returns the first supported language of the candidates
// (in order of precedence), or English.
func ResolveLang(candidates ...string) string {
	for _, c := range candidates {
		if l := NormalizeLang(c); l != "" {
			return l
		}
	}
	return English
}

// loc translates the texts of a report into one language.
type loc struct{ lang string }

func newLoc(lang string) loc { return loc{lang: ResolveLang(lang)} }

// missingTranslation is called (by tests) for texts without a translation.
var missingTranslation func(lang, key string)

// t translates an English text.
func (l loc) t(en string) string {
	return l.tc("", en)
}

// tc translates an English text in a context (for texts translated
// differently depending on where they are used).
func (l loc) tc(ctx, en string) string {
	if l.lang == English || en == "" {
		return en
	}
	key := en
	if ctx != "" {
		key = ctx + "|" + en
	}
	if s, ok := texts[l.lang][key]; ok {
		return s
	}
	if missingTranslation != nil {
		missingTranslation(l.lang, key)
	}
	return en
}

// T translates an English report text (unknown texts and languages are
// returned unchanged).
func T(lang, en string) string { return newLoc(lang).t(en) }

// Enumeration kinds of EnumLabel.
const (
	EnumStatus      = "status"
	EnumResult      = "result"
	EnumCoverage    = "coverage"
	EnumObligation  = "obligation"
	EnumPriority    = "priority"
	EnumReqType     = "requirement_type"
	EnumExecution   = "execution_type"
	EnumSeverity    = "severity"
	EnumScope       = "scope"
	EnumEvalMethod  = "evaluation_method"
	EnumRiskLevel   = "risk_level"
	EnumRiskStatus  = "risk_status"
	EnumMitigation  = "mitigation_state"
	obligationUnset = "unspecified"
)

// EnumLabel returns the label of an enumeration value of kind in lang.
// Values without a label (custom statuses, …) are humanized.
func EnumLabel(lang, kind, value string) string { return newLoc(lang).enum(kind, value) }

// f translates an English text and fills its {name} placeholders (pairs
// of name, value).
func (l loc) f(en string, kv ...string) string {
	s := l.t(en)
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}

func (l loc) enum(kind, v string) string {
	if v == "" {
		return ""
	}
	if s, ok := enums[l.lang][kind][v]; ok {
		return s
	}
	if kind == EnumObligation {
		return strings.ToUpper(strings.ReplaceAll(v, "_", " "))
	}
	return humanize(v)
}

func (l loc) enumLabels(kind string, vals []string) map[string]string {
	m := map[string]string{}
	for _, v := range vals {
		m[v] = l.enum(kind, v)
	}
	return m
}

// pct formats a/b as a percentage ("–" when b is 0); Swedish uses a
// decimal comma and a space before the percent sign.
func (l loc) pct(a, b int) string {
	s := pct(a, b)
	if l.lang == Swedish && b != 0 {
		s = strings.Replace(strings.TrimSuffix(s, "%"), ".", ",", 1) + " %"
	}
	return s
}

// templateNames are the titles of the standard reports (as in the UI).
var templateNames = map[string]map[string]string{
	Swedish: {
		ProjectStatus:        "Projektstatus",
		RequirementsCoverage: "Kravtäckning",
		Traceability:         "Spårbarhet",
		RunSummary:           "Sammanfattning av körning",
		Defects:              "Defekter",
		TestInventory:        "Testförteckning",
		Risks:                "Riskrapport",
	},
}

func (l loc) templateName(t Template) string {
	if s, ok := templateNames[l.lang][t.ID]; ok {
		return s
	}
	return t.Name
}

// enums are the labels of enumeration values; the Swedish ones follow the
// web client (web/src/i18n/locales/sv.ts, "enums").
var enums = map[string]map[string]map[string]string{
	English: {
		EnumCoverage: {"passed": "Passed", "partial": "Partially passed", "failed": "Failed", "not_run": "Not run", "none": "No linked tests"},
		EnumObligation: {"must": "MUST", "must_not": "MUST NOT", "shall": "SHALL", "shall_not": "SHALL NOT", "should": "SHOULD",
			"should_not": "SHOULD NOT", "may": "MAY", "optional": "OPTIONAL", "will": "WILL", "describe": "DESCRIBE", obligationUnset: "Unspecified"},
		EnumScope: {"requirement": "Requirements", "test": "Tests", "risk": "Risks"},
		EnumMitigation: {MitigationNone: "No requirements", MitigationUntested: "Untested", MitigationPartly: "Partly tested",
			MitigationTested: "Tested, not all passed", MitigationVerified: "Verified"},
	},
	Swedish: {
		EnumStatus: {"draft": "Utkast", "review": "Granskning", "approved": "Godkänt", "implemented": "Implementerat", "verified": "Verifierat",
			"rejected": "Avvisat", "obsolete": "Inaktuellt", "ready": "Klart", "deprecated": "Utfasat", "planned": "Planerad",
			"in_progress": "Pågår", "completed": "Slutförd", "aborted": "Avbruten"},
		EnumResult:   {"not_run": "Ej körd", "in_progress": "Pågår", "passed": "Godkänd", "failed": "Underkänd", "blocked": "Blockerad", "skipped": "Överhoppad"},
		EnumCoverage: {"none": "Ingen täckning", "not_run": "Ej körd", "passed": "Godkänd", "failed": "Underkänd", "partial": "Delvis"},
		EnumObligation: {"must": "MUST (SKA)", "must_not": "MUST NOT (SKA INTE)", "shall": "SHALL (SKA)", "shall_not": "SHALL NOT (SKA INTE)",
			"should": "SHOULD (BÖR)", "should_not": "SHOULD NOT (BÖR INTE)", "may": "MAY (FÅR)", "optional": "OPTIONAL (VALFRITT)",
			"will": "WILL (KOMMER ATT)", "describe": "Beskrivande", obligationUnset: "Ej angiven"},
		EnumPriority: {"critical": "Kritisk", "high": "Hög", "medium": "Medel", "low": "Låg"},
		EnumReqType: {"functional": "Funktionellt", "non_functional": "Icke-funktionellt", "security": "Säkerhet", "performance": "Prestanda",
			"usability": "Användbarhet", "interface": "Gränssnitt", "constraint": "Begränsning", "business": "Verksamhet", "regulatory": "Regelverk"},
		EnumExecution:  {"manual": "Manuellt", "automated": "Automatiserat"},
		EnumSeverity:   {"error": "Fel", "warning": "Varning"},
		EnumScope:      {"requirement": "Krav", "test": "Testfall", "risk": "Risker"},
		EnumEvalMethod: {"test": "Test", "inspection": "Inspektion", "analysis": "Analys", "demonstration": "Demonstration", "review": "Granskning"},
		EnumRiskLevel:  {"low": "Låg", "medium": "Medel", "high": "Hög", "critical": "Kritisk"},
		EnumRiskStatus: {"identified": "Identifierad", "mitigating": "Åtgärdas", "mitigated": "Åtgärdad", "accepted": "Accepterad", "closed": "Stängd"},
		EnumMitigation: {MitigationNone: "Inga krav", MitigationUntested: "Otestad", MitigationPartly: "Delvis testad",
			MitigationTested: "Testad, inte alla godkända", MitigationVerified: "Verifierad"},
	},
}

// texts translate the English texts of the reports ("context|text" for
// texts that depend on where they are used). The terms follow the web
// client's Swedish texts (krav, testfall, testkörning, utfall, täckning,
// kravnivå, …).
var texts = map[string]map[string]string{
	Swedish: {
		// shared
		"Metric": "Mått", "Value": "Värde", "Count": "Antal", "Share": "Andel", "Total": "Totalt", "Summary": "Sammanfattning",
		" · Filter: ": " · Filter: ", " · Run: ": " · Testkörning: ", "Filter": "Filter",
		// project status
		"Key figures":                                     "Nyckeltal",
		"Requirements":                                    "Krav",
		"Requirements with linked tests":                  "Krav med länkade testfall",
		"Requirements without tests":                      "Krav utan testfall",
		"Test coverage (requirements with tests)":         "Testtäckning (krav med testfall)",
		"Requirements verified (all linked tests passed)": "Verifierade krav (alla länkade testfall godkända)",
		"Test cases":                                      "Testfall",
		"Automated test cases":                            "Automatiserade testfall",
		"Manual test cases":                               "Manuella testfall",
		"Pass rate (last results)":                        "Andel godkända (senaste utfall)",
		"Test runs":                                       "Testkörningar",
		"Runs in progress":                                "Pågående testkörningar",
		"Requirements by status":                          "Krav per status",
		"Status":                                          "Status",
		"Requirements by obligation":                      "Krav per kravnivå",
		"Obligation":                                      "Kravnivå",
		"Requirement coverage":                            "Kravtäckning",
		"Coverage":                                        "Täckning",
		"Last test results":                               "Senaste testutfall",
		"Result":                                          "Utfall",
		"stale":                                           "inaktuellt",
		"Recent test runs":                                "Senaste testkörningar",
		"Run":                                             "Testkörning",
		"Environment":                                     "Miljö",
		"Build":                                           "Bygge",
		"Created":                                         "Skapad",
		"Passed":                                          "Godkända",
		"Failed":                                          "Underkända",
		"Blocked":                                         "Blockerade",
		"Skipped":                                         "Överhoppade",
		"Not run":                                         "Ej körda",
		"Requirements and their tests":                    "Krav och deras testfall",
		"Key":                                             "Nyckel",
		"Title":                                           "Titel",
		"Priority":                                        "Prioritet",
		"Linked tests":                                    "Länkade testfall",
		"Last results":                                    "Senaste utfall",
		"With linked tests":                               "Med länkade testfall",
		"Without linked tests":                            "Utan länkade testfall",
		"Partially passed":                                "Delvis godkända",
		"Verified share":                                  "Andel verifierade",
		"Traceability matrix":                             "Spårbarhetsmatris",
		"Requirement":                                     "Krav",
		"Requirement title":                               "Kravtitel",
		"Test":                                            "Testfall",
		"Test title":                                      "Testfallstitel",
		"Last result":                                     "Senaste utfall",
		"Tests without requirements":                      "Testfall utan krav",
		"Tests":                                           "Testfall",
		"Links":                                           "Länkar",
		"Traceability rules":                              "Spårbarhetsregler",
		"Traceability rule violations":                    "Överträdelser av spårbarhetsregler",
		"Rule":                                            "Regel",
		"Severity":                                        "Allvarlighet",
		"Applies to":                                      "Gäller",
		"Selector":                                        "Urval",
		"Matching items":                                  "Matchande objekt",
		"Violations":                                      "Överträdelser",
		"Item":                                            "Objekt",
		"Found":                                           "Hittade",
		"Required":                                        "Krävs",
		"Involved items":                                  "Berörda objekt",
		"rule|Passed":                                     "Uppfylld",
		"rule|Failed":                                     "Bryts",
		"rule|Invalid: ":                                  "Ogiltig: ",
		"Executions":                                      "Exekveringar",
		"Assignee":                                        "Tilldelad",
		"Executed by":                                     "Utförd av",
		"Started":                                         "Startad",
		"Finished":                                        "Klar",
		"Duration (ms)":                                   "Tid (ms)",
		"Defects":                                         "Defekter",
		"Notes":                                           "Anteckningar",
		"Description":                                     "Beskrivning",
		"Created by":                                      "Skapad av",
		"Completed":                                       "Avslutad",
		"Progress (executions with a result)":             "Framsteg (exekveringar med utfall)",
		"Pass rate (of executions with a result)": "Andel godkända (av exekveringar med utfall)",
		"Total duration":             "Total tid",
		"Average duration":           "Genomsnittlig tid",
		"Defect":                     "Defekt",
		"Link":                       "Länk",
		"Occurrences":                "Förekomster",
		"Latest result":              "Senaste utfall",
		"Last seen":                  "Senast sedd",
		"Distinct defects":           "Unika defekter",
		"Affected tests":             "Berörda testfall",
		"Affected runs":              "Berörda testkörningar",
		"Folder":                     "Mapp",
		"Execution":                  "Exekveringstyp",
		"Categories":                 "Kategorier",
		"Linked requirements":        "Länkade krav",
		"(no folder)":                "(ingen mapp)",
		"Tests by category":          "Testfall per kategori",
		"Dimension":                  "Dimension",
		"Category":                   "Kategori",
		"Manual":                     "Manuella",
		"Automated":                  "Automatiserade",
		"With script":                "Med skript",
		"Linked to requirements":     "Länkade till krav",
		"Not linked to requirements": "Inte länkade till krav",
		"Total estimate (minutes)":   "Total uppskattning (minuter)",
		"Tests by folder":            "Testfall per mapp",
		"Tests by execution type":    "Testfall per exekveringstyp",
		"Tests by status":            "Testfall per status",
		"Tests by priority":          "Testfall per prioritet",
		// custom reports (field labels as in the web client's "reportFields")
		"Custom report – ":          "Anpassad rapport – ",
		"Entity":                    "Rapport över",
		"(none)":                    "(ingen)",
		"Summary by ":               "Sammanfattning per ",
		"Total ":                    "Totalt ",
		"Type":                      "Typ",
		"Source":                    "Källa",
		"Rationale":                 "Motivering",
		"Owner":                     "Ägare",
		"Tags":                      "Taggar",
		"Version":                   "Version",
		"Categories with dimension": "Kategorier med dimension",
		"Updated":                   "Uppdaterad",
		"Evaluation criteria":       "Utvärderingskriterier",
		"ID":                        "ID",
		"Related requirements":      "Relaterade krav",
		"Parent requirements":       "Överordnade krav",
		"Child requirements":        "Underordnade krav",
		"Comments":                  "Kommentarer",
		"Preconditions":             "Förutsättningar",
		"Execution type":            "Exekveringstyp",
		"Script language":           "Skriptspråk",
		"Script":                    "Skript",
		"Expected result":           "Förväntat resultat",
		"Steps":                     "Steg",
		"Estimate (minutes)":        "Uppskattning (minuter)",
		"Called tests":              "Anropade testfall",
		"Name":                      "Namn",
		"Pass rate":                 "Andel godkända",
		"Test key":                  "Testfallsnyckel",
		"Output":                    "Utdata",
		"call":                      "anropa",
		"In progress":               "Pågående",
		"#":                         "#",
		"Runs":                      "Testkörningar",
		// risk report (terms as in the web client's "risksView")
		"Level":                       "Nivå",
		"Risks by level and status":   "Risker per nivå och status",
		"Heat map (risks not closed)": "Värmekarta (risker som inte är stängda)",
		"Likelihood \\ impact":        "Sannolikhet \\ konsekvens",
		"Risk register":               "Riskregister",
		"Likelihood":                  "Sannolikhet",
		"Impact":                      "Konsekvens",
		"Score":                       "Poäng",
		"Mitigation":                  "Åtgärd",
		"Mitigation state":            "Åtgärdens status",
		"Traceability: risks, requirements and tests": "Spårbarhet: risker, krav och testfall",
		"Risk":                                "Risk",
		"Open risks with untested mitigation": "Öppna risker med otestad åtgärd",
		"Requirements not passed":             "Krav som inte är godkända",
		"Top open risks":                      "Största öppna risker",
		"Open risks not updated for 90 days or more": "Öppna risker som inte uppdaterats på 90 dagar eller mer",
		"Days since update":                          "Dagar sedan uppdatering",
		"Risks":                                      "Risker",
		"Open risks (identified or mitigating)":      "Öppna risker (identifierade eller under åtgärd)",
		"Mitigated risks":                            "Åtgärdade risker",
		"Accepted risks":                             "Accepterade risker",
		"Closed risks":                               "Stängda risker",
		"Open critical risks":                        "Öppna kritiska risker",
		"Open high risks":                            "Öppna höga risker",
		"Open medium risks":                          "Öppna medelhöga risker",
		"Open low risks":                             "Öppna låga risker",
		"Open risks without requirements":            "Öppna risker utan krav",
		"Open risks with verified mitigation":        "Öppna risker med verifierad åtgärd",
		"Share of open risks verified":               "Andel verifierade öppna risker",
		"Open risks by mitigation state":             "Öppna risker per åtgärdsstatus",
		"ATT&CK techniques":                          "ATT&CK-tekniker",
		"MITRE ATT&CK techniques of the risks":       "MITRE ATT&CK-tekniker för riskerna",
		"Technique":                                  "Teknik",
		"Tactics":                                    "Taktiker",
		"Matrix":                                     "Matris",
		"Open risks":                                 "Öppna risker",
		"Risks with ATT&CK techniques":               "Risker med ATT&CK-tekniker",
	},
}
