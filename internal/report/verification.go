package report

import (
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// The verification report: whether every requirement is verified, and
// why — per criterion with its evidence (tests, test steps, verification
// records), the records themselves, the roll-up along the requirement
// hierarchy, inconsistencies and the coverage per system element.

// Verification is the id of the verification report.
const Verification = "verification"

// Enumeration kinds of the verification report.
const (
	EnumVerification = "verification_state"
	EnumRecordResult = "record_result"
	EnumElementKind  = "element_kind"
)

// VerificationData is what the verification report (and the verification
// columns of other reports and documents) need; nil leaves them out.
type VerificationData struct {
	// Requirements: requirement id → its verification.
	Requirements map[string]*model.RequirementVerification
	Records      []model.VerificationRecord
	Elements     []model.ElementCoverage
	Strict       bool
}

var verificationOrder = []string{model.VerVerified, model.VerPartial, model.VerFailed, model.VerPending, model.VerNone}

func init() {
	templates = append(templates, Template{ID: Verification, Name: "Verification",
		Description: "Whether every requirement is verified, and why: evidence per criterion (tests, steps, inspection, analysis, demonstration and review records), the records, the roll-up of child requirements, inconsistencies and coverage per system element.",
		Params:      []Param{queryParam("requirements")}, build: buildVerification})
	templateNames[Swedish][Verification] = "Verifiering"
	// custom report columns (query fields verification and element)
	fields[EntityRequirement] = append(fields[EntityRequirement], f("verification", "Verification"), f("element", "System elements"))
	fields[EntityTest] = append(fields[EntityTest], f("element", "System elements"))
	enumFields[EntityRequirement]["verification"] = EnumVerification
	texts[Swedish]["System elements"] = "Systemelement"
	enums[English][EnumVerification] = map[string]string{"verified": "Verified", "partial": "Partly verified", "failed": "Failed", "pending": "Not yet passed", "none": "No evidence"}
	enums[Swedish][EnumVerification] = map[string]string{"verified": "Verifierat", "partial": "Delvis verifierat", "failed": "Underkänt", "pending": "Inte godkänt än", "none": "Inga belägg"}
	enums[English][EnumRecordResult] = map[string]string{"pass": "Pass", "fail": "Fail", "conditional": "Conditional pass"}
	enums[Swedish][EnumRecordResult] = map[string]string{"pass": "Godkänd", "fail": "Underkänd", "conditional": "Godkänd med villkor"}
	enums[English][EnumElementKind] = map[string]string{"system": "System", "subsystem": "Subsystem", "component": "Component", "product": "Product", "supplier": "Supplier", "other": "Other"}
	enums[Swedish][EnumElementKind] = map[string]string{"system": "System", "subsystem": "Delsystem", "component": "Komponent", "product": "Produkt", "supplier": "Leverantör", "other": "Annat"}
	for k, v := range map[string]string{
		"Verification":                          "Verifiering",
		"Requirements verified":                 "Verifierade krav",
		"Requirements partly verified":          "Delvis verifierade krav",
		"Requirements failed":                   "Underkända krav",
		"Requirements not yet passed":           "Krav som inte är godkända än",
		"Requirements without evidence":         "Krav utan belägg",
		"Criteria with passing evidence":        "Kriterier med godkända belägg",
		"Verification records":                  "Verifieringsposter",
		"Valid verification records":            "Giltiga verifieringsposter",
		"Strict criterion coverage":             "Strikt kriterietäckning",
		"Inconsistencies":                       "Inkonsekvenser",
		"on":                                    "på",
		"off":                                   "av",
		"Requirements by verification":          "Krav per verifiering",
		"Verification by criterion":             "Verifiering per kriterium",
		"Requirement":                           "Krav",
		"Requirement title":                     "Kravets titel",
		"Criterion":                             "Kriterium",
		"Method":                                "Metod",
		"State":                                 "Läge",
		"Evidence":                              "Belägg",
		"Requirements not verified, and why":    "Krav som inte är verifierade, och varför",
		"Reasons":                               "Skäl",
		"Performed by":                          "Utförd av",
		"Date":                                  "Datum",
		"Valid until":                           "Giltig till",
		"Validity":                              "Giltighet",
		"Sign-off":                              "Kontrasignering",
		"Documents":                             "Dokument",
		"Note":                                  "Anteckning",
		"Hierarchy roll-up":                     "Sammanräkning i kravhierarkin",
		"Own evidence":                          "Egna belägg",
		"With children":                         "Med underkrav",
		"Children verified":                     "Verifierade underkrav",
		"Coverage per system element":           "Täckning per systemelement",
		"Element":                               "Element",
		"Kind":                                  "Typ",
		"With evidence":                         "Med belägg",
		"Verified":                              "Verifierade",
		"Verified share":                        "Andel verifierade",
		"Tests passed":                          "Godkända testfall",
		"verification|Tests":                    "Testfall",
		"verification|Failed":                   "Underkända",
		"verification|Requirements":             "Krav",
		"valid":                                 "giltig",
		"expired":                               "utgången",
		"invalidated":                           "ogiltigförklarad",
		"whole requirement":                     "hela kravet",
		"step":                                  "steg",
		"plain link":                            "länk utan kriterium",
		"suspect":                               "misstänkt",
		"not run":                               "ej körd",
		"step not run":                          "steget ej kört",
		"superseded":                            "ersatt",
		"conditional":                           "med villkor",
		"no test or verification record":        "inget testfall eller verifieringspost",
		"criterion {criterion} has no evidence": "kriterium {criterion} saknar belägg",
		"criterion {criterion} failed":          "kriterium {criterion} underkändes",
		"criterion {criterion} has not passed yet":           "kriterium {criterion} är inte godkänt än",
		"{kind} {key} failed":                                "{kind} {key} underkändes",
		"{kind} {key} has not passed yet":                    "{kind} {key} är inte godkänt än",
		"child {key} is not verified":                        "underkravet {key} är inte verifierat",
		"the status is verified but the evidence is {state}": "statusen är verifierad men beläggen är: {state}",
		"the status is verified but child {key} is {status}": "statusen är verifierad men underkravet {key} är: {status}",
		"record":                           "post",
		"test":                             "testfall",
		"{n} of {total} criteria verified": "{n} av {total} kriterier verifierade",
	} {
		texts[Swedish][k] = v
	}
}

var reasonTexts = map[string]string{
	"no_evidence":             "no test or verification record",
	"criterion_missing":       "criterion {criterion} has no evidence",
	"criterion_failed":        "criterion {criterion} failed",
	"criterion_pending":       "criterion {criterion} has not passed yet",
	"evidence_failed":         "{kind} {key} failed",
	"evidence_pending":        "{kind} {key} has not passed yet",
	"child_not_verified":      "child {key} is not verified",
	"status_inconsistent":     "the status is verified but the evidence is {state}",
	"child_status_unverified": "the status is verified but child {key} is {status}",
}

// reason renders a verification reason in the report's language.
func (l loc) reason(r model.VerificationReason) string {
	tpl, ok := reasonTexts[r.Code]
	if !ok {
		return r.Text
	}
	s := l.t(tpl)
	for k, v := range r.Params {
		switch k {
		case "kind":
			v = l.t(v)
		case "state":
			v = l.enum(EnumVerification, v)
		case "status":
			v = l.enum(EnumStatus, v)
		}
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// evidence renders one piece of evidence: "P-TC-1 step 2: Passed",
// "Inspection 2026-01-10 (A. Person): Pass".
func (l loc) evidence(e model.Evidence) string {
	var b strings.Builder
	if e.Kind == "record" {
		b.WriteString(l.enum(EnumEvalMethod, e.Key))
		if e.Date != "" {
			b.WriteString(" " + e.Date)
		}
		if e.Title != "" {
			b.WriteString(" (" + e.Title + ")")
		}
		b.WriteString(": " + l.enum(EnumRecordResult, e.Result))
	} else {
		b.WriteString(e.Key)
		if e.StepPosition > 0 {
			b.WriteString(" " + l.t("step") + " " + itoa(e.StepPosition))
		}
		b.WriteString(": " + l.enum(EnumResult, e.Result))
	}
	var notes []string
	if e.Plain {
		notes = append(notes, l.t("plain link"))
	}
	if e.Note != "" {
		notes = append(notes, l.t(strings.ReplaceAll(e.Note, "_", " ")))
	}
	if len(notes) > 0 {
		b.WriteString(" [" + strings.Join(notes, ", ") + "]")
	}
	return b.String()
}

func (l loc) evidenceList(ev []model.Evidence) string {
	parts := make([]string, 0, len(ev))
	for _, e := range ev {
		parts = append(parts, l.evidence(e))
	}
	return strings.Join(parts, "\n")
}

func buildVerification(d *Data, _ map[string]string) (*export.Dataset, error) {
	l := d.loc()
	vd := d.Verification
	if vd == nil {
		vd = &VerificationData{}
	}
	counts := map[string]int{}
	critOK, critTotal, inconsistent := 0, 0, 0
	byCrit := export.Section{Name: "criteria", Title: l.t("Verification by criterion"),
		Columns: l.cols("Requirement", "Requirement title", "Criterion", "Method", "State", "Evidence")}
	why := export.Section{Name: "not_verified", Title: l.t("Requirements not verified, and why"),
		Columns: l.cols("Requirement", "Requirement title", "Status", "State", "Reasons")}
	roll := export.Section{Name: "hierarchy", Title: l.t("Hierarchy roll-up"),
		Columns: l.cols("Requirement", "Requirement title", "Own evidence", "With children", "Children verified")}
	inc := export.Section{Name: "inconsistencies", Title: l.t("Inconsistencies"),
		Columns: l.cols("Requirement", "Requirement title", "Status", "Reasons")}
	in := map[string]bool{}
	for _, r := range d.Requirements {
		in[r.ID] = true
		v := vd.Requirements[r.ID]
		if v == nil {
			continue
		}
		if !inactiveStatus(r.Status) {
			counts[v.State]++
			critOK += v.CriteriaVerified
			critTotal += v.CriteriaTotal
		}
		for _, c := range v.Criteria {
			byCrit.Rows = append(byCrit.Rows, []string{r.Key, r.Title, itoa(c.Position) + ". " + c.Description, l.enum(EnumEvalMethod, c.Method),
				l.enum(EnumVerification, c.State), l.evidenceList(c.Evidence)})
		}
		if len(v.Criteria) == 0 || len(v.Evidence) > 0 {
			// evidence about the requirement as a whole (plain links, records without criterion)
			state := ""
			if len(v.Criteria) == 0 {
				state = l.enum(EnumVerification, v.Own)
			}
			byCrit.Rows = append(byCrit.Rows, []string{r.Key, r.Title, l.t("whole requirement"), "", state, l.evidenceList(v.Evidence)})
		}
		if !v.Verified && !inactiveStatus(r.Status) {
			reasons := make([]string, 0, len(v.Reasons))
			for _, x := range v.Reasons {
				reasons = append(reasons, l.reason(x))
			}
			why.Rows = append(why.Rows, []string{r.Key, r.Title, l.enum(EnumStatus, r.Status), l.enum(EnumVerification, v.State), strings.Join(reasons, "\n")})
		}
		if len(v.Children) > 0 {
			active := 0
			for _, c := range v.Children {
				if !inactiveStatus(c.Status) {
					active++
				}
			}
			roll.Rows = append(roll.Rows, []string{r.Key, r.Title, l.enum(EnumVerification, v.Own), l.enum(EnumVerification, v.State),
				itoa(v.ChildrenVerified) + " / " + itoa(active)})
		}
		if len(v.Inconsistencies) > 0 {
			inconsistent++
			reasons := make([]string, 0, len(v.Inconsistencies))
			for _, x := range v.Inconsistencies {
				reasons = append(reasons, l.reason(x))
			}
			inc.Rows = append(inc.Rows, []string{r.Key, r.Title, l.enum(EnumStatus, r.Status), strings.Join(reasons, "\n")})
		}
	}
	recs := export.Section{Name: "records", Title: l.t("Verification records"),
		Columns: l.cols("Requirement", "Criterion", "Method", "Result", "Performed by", "Date", "Valid until", "Validity", "Sign-off", "Documents", "Note")}
	valid := 0
	for _, x := range vd.Records {
		if !in[x.RequirementID] {
			continue
		}
		if x.Validity == "valid" {
			valid++
		}
		crit := l.t("whole requirement")
		if x.CriterionID != nil {
			crit = x.CriterionText
		}
		sign := ""
		if x.Signoff != nil {
			sign = x.Signoff.Name
			if sign == "" {
				sign = x.Signoff.By
			}
			sign += " " + ts(x.Signoff.At)
		}
		var docs []string
		for _, f := range x.Files {
			docs = append(docs, f.Name)
		}
		recs.Rows = append(recs.Rows, []string{x.RequirementKey, crit, l.enum(EnumEvalMethod, x.Method), l.enum(EnumRecordResult, x.Result), x.PerformedBy,
			x.PerformedOn, x.ValidUntil, l.t(x.Validity), sign, strings.Join(docs, ", "), x.Note})
	}
	elems := export.Section{Name: "elements", Title: l.t("Coverage per system element"),
		Columns: []string{l.t("Element"), l.t("Kind"), l.tc("verification", "Requirements"), l.t("With evidence"), l.t("Verified"), l.tc("verification", "Failed"),
			l.t("Verified share"), l.tc("verification", "Tests"), l.t("Tests passed")}}
	for _, e := range vd.Elements {
		elems.Rows = append(elems.Rows, []string{e.Path, l.enum(EnumElementKind, e.Kind), itoa(e.Requirements), itoa(e.Covered), itoa(e.Verified), itoa(e.Failed),
			l.pct(e.Verified, e.Requirements), itoa(e.Tests), itoa(e.TestsPassed)})
	}
	strict := l.t("off")
	if vd.Strict {
		strict = l.t("on")
	}
	n := 0
	for _, c := range counts {
		n += c
	}
	sections := []export.Section{
		l.kpiSection("summary", "Summary",
			kv{"Requirements", itoa(n)},
			kv{"Requirements verified", itoa(counts[model.VerVerified]) + " (" + l.pct(counts[model.VerVerified], n) + ")"},
			kv{"Requirements partly verified", itoa(counts[model.VerPartial])},
			kv{"Requirements failed", itoa(counts[model.VerFailed])},
			kv{"Requirements not yet passed", itoa(counts[model.VerPending])},
			kv{"Requirements without evidence", itoa(counts[model.VerNone])},
			kv{"Criteria with passing evidence", itoa(critOK) + " / " + itoa(critTotal)},
			kv{"Valid verification records", itoa(valid)},
			kv{"Inconsistencies", itoa(inconsistent)},
			kv{"Strict criterion coverage", strict},
		),
		l.countSection("by_state", "Requirements by verification", "Verification", counts, verificationOrder, l.enumLabels(EnumVerification, verificationOrder)),
		byCrit, why, recs,
	}
	if len(roll.Rows) > 0 {
		sections = append(sections, roll)
	}
	if len(inc.Rows) > 0 {
		sections = append(sections, inc)
	}
	if len(elems.Rows) > 0 {
		sections = append(sections, elems)
	}
	return &export.Dataset{Sections: sections}, nil
}

func inactiveStatus(s string) bool { return s == "rejected" || s == "obsolete" }
