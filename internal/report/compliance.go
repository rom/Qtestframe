package report

import (
	"strings"

	"github.com/rom/qtestframe/internal/analytics"
	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/export"
)

// The compliance report: the project's compliance with laws, regulations,
// standards and best practices of the catalogue's source registry
// (analytics.Compliance). Self-contained: it registers its template, name
// and translations here.

// Compliance is the id of the compliance report.
const Compliance = "compliance"

// ComplianceCatalog returns the catalogue the compliance report measures
// against (the built-in one; tests may replace it).
var ComplianceCatalog = catalog.Default

func init() {
	templates = append(templates, Template{ID: Compliance, Name: "Compliance",
		Description: "Compliance with selected laws, standards and best practices: adopted and missing catalogue requirements, fulfilment by test results, per source and clause, with the gaps.",
		Params:      []Param{{Name: "sources", Type: "sources", Description: "comma-separated reference source ids (default: every source the project uses)"}},
		build:       buildCompliance})
	templateNames[Swedish][Compliance] = "Efterlevnad"
	for k, v := range complianceTexts {
		texts[Swedish][k] = v
	}
}

// complianceTexts are the Swedish texts of the compliance report.
var complianceTexts = map[string]string{
	"Compliance summary":             "Sammanfattning av efterlevnad",
	"Sources analysed":               "Analyserade källor",
	"Catalogue requirements":         "Katalogkrav",
	"Adopted":                        "Införda",
	"Missing":                        "Saknas",
	"Own requirements":               "Egna krav",
	"Fulfilled":                      "Uppfyllda",
	"Failing":                        "Underkända",
	"Untested":                       "Ej testade",
	"Compliance":                     "Efterlevnad",
	"Compliance by source":           "Efterlevnad per källa",
	"Source":                         "Källa",
	"Kind":                           "Slag",
	"Jurisdiction":                   "Jurisdiktion",
	"Adoption":                       "Införandegrad",
	"MUST compliance":                "Efterlevnad av MUST-krav",
	"Compliance by clause – ":        "Efterlevnad per avsnitt – ",
	"Clause":                         "Avsnitt",
	"(general)":                      "(allmänt)",
	"Missing catalogue requirements": "Katalogkrav som saknas",
	"Catalogue id":                   "Katalog-id",
	"Clauses":                        "Avsnitt",
	"Requirements not fulfilled":     "Krav som inte är uppfyllda",
	"State":                          "Läge",
	"state|Fulfilled":                "Uppfyllt",
	"state|Failing":                  "Underkänt",
	"state|Blocked":                  "Blockerat",
	"state|Untested":                 "Ej testat",
	"Own":                            "Eget",
	"Unknown sources: ":              "Okända källor: ",
	"kind|Law or regulation":         "Lag eller förordning",
	"kind|Directive":                 "Direktiv",
	"kind|Standard":                  "Standard",
	"kind|Best practice":             "Bästa praxis",
	"kind|Guideline":                 "Vägledning",
	"international":                  "internationell",
}

var complianceStateNames = map[string]string{analytics.StatePassed: "passed", analytics.StateFailing: "failing", analytics.StateBlocked: "blocked", analytics.StateUntested: "untested"}

func (l loc) state(s string) string {
	n, ok := complianceStateNames[s]
	if !ok {
		n = "untested"
	}
	return l.tc("state", map[string]string{"passed": "Fulfilled", "failing": "Failing", "blocked": "Blocked", "untested": "Untested"}[n])
}

func (l loc) sourceKind(k string) string {
	names := map[string]string{"law": "Law or regulation", "directive": "Directive", "standard": "Standard", "practice": "Best practice", "guideline": "Guideline"}
	if n, ok := names[k]; ok {
		return l.tc("kind", n)
	}
	return humanize(k)
}

func (l loc) clause(c string) string {
	if c == "" {
		return l.t("(general)")
	}
	return c
}

func (l loc) clauses(cs []string) string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = l.clause(c)
	}
	return strings.Join(out, ", ")
}

// ComplianceInput builds the analysis input from report data.
func (d *Data) ComplianceInput() analytics.ComplianceInput {
	tests := d.testIndex()
	byReq := d.linksByRequirement()
	in := analytics.ComplianceInput{Lang: d.loc().lang}
	for _, r := range d.Requirements {
		x := analytics.ComplianceRequirement{ID: r.ID, Key: r.Key, Title: r.Title, Status: r.Status, Obligation: r.Obligation, CatalogRef: r.CatalogRef, Source: r.Source}
		for _, tid := range byReq[r.ID] {
			res := ""
			if t := tests[tid]; t != nil {
				res = t.LastResult
			}
			x.Results = append(x.Results, res)
		}
		in.Requirements = append(in.Requirements, x)
	}
	return in
}

func buildCompliance(d *Data, params map[string]string) (*export.Dataset, error) {
	l := d.loc()
	c, err := ComplianceCatalog()
	if err != nil {
		return nil, err
	}
	in := d.ComplianceInput()
	in.Catalog = c
	if v := strings.TrimSpace(params["sources"]); v != "" {
		in.Sources = strings.Split(v, ",")
	}
	rep := analytics.Compliance(in)
	if len(rep.Unknown) > 0 && len(rep.Sources) == 0 {
		return nil, invalidf("unknown reference source %s", strings.Join(rep.Unknown, ", "))
	}
	sum := rep.Summary
	ds := &export.Dataset{Sections: []export.Section{
		l.kpiSection("summary", "Compliance summary",
			kv{"Sources analysed", itoa(sum.Sources)},
			kv{"Catalogue requirements", itoa(sum.Catalogue)},
			kv{"Adopted", itoa(sum.Adopted)},
			kv{"Missing", itoa(sum.Missing)},
			kv{"Own requirements", itoa(sum.Own)},
			kv{"Fulfilled", itoa(sum.Fulfilment.Passed)},
			kv{"Compliance", l.pct(sum.Fulfilment.Passed, sum.Catalogue+sum.Own)},
		),
	}}
	if len(rep.Unknown) > 0 {
		ds.Subtitle = d.subtitle() + " · " + l.t("Unknown sources: ") + strings.Join(rep.Unknown, ", ")
	}
	bySource := export.Section{Name: "sources", Title: l.t("Compliance by source"),
		Columns: l.cols("Source", "Kind", "Jurisdiction", "Catalogue requirements", "Adopted", "Missing", "Own requirements", "Fulfilled", "Failing", "Blocked", "Untested", "Adoption", "Compliance", "MUST compliance")}
	gaps := export.Section{Name: "gaps", Title: l.t("Missing catalogue requirements"), Columns: l.cols("Source", "Catalogue id", "Title", "Obligation", "Priority", "Clauses")}
	open := export.Section{Name: "not_fulfilled", Title: l.t("Requirements not fulfilled"), Columns: l.cols("Source", "Key", "Title", "State", "Catalogue id", "Clauses")}
	var clauseSections []export.Section
	for _, s := range rep.Sources {
		name := s.Name
		if s.Short != "" {
			name = s.Short
		}
		jur := s.Jurisdiction
		if jur == "international" {
			jur = l.t("international")
		}
		total := s.Catalogue + s.Own
		must := "–"
		if s.MandatoryPercent >= 0 {
			must = l.pctValue(s.MandatoryPercent)
		}
		bySource.Rows = append(bySource.Rows, []string{s.Name, l.sourceKind(s.Kind), jur, itoa(s.Catalogue), itoa(s.Adopted), itoa(s.Missing), itoa(s.Own),
			itoa(s.Fulfilment.Passed), itoa(s.Fulfilment.Failing), itoa(s.Fulfilment.Blocked), itoa(s.Fulfilment.Untested),
			l.pct(s.Adopted, s.Catalogue), l.pct(s.Fulfilment.Passed, total), must})
		cs := export.Section{Name: "clauses_" + s.ID, Title: l.t("Compliance by clause – ") + name,
			Columns: l.cols("Clause", "Catalogue requirements", "Adopted", "Missing", "Own requirements", "Fulfilled", "Compliance")}
		for _, cl := range s.Clauses {
			cs.Rows = append(cs.Rows, []string{l.clause(cl.Clause), itoa(cl.Catalogue), itoa(cl.Adopted), itoa(cl.Missing), itoa(cl.Own), itoa(cl.Fulfilment.Passed),
				l.pct(cl.Fulfilment.Passed, cl.Catalogue+cl.Own)})
		}
		clauseSections = append(clauseSections, cs)
		for _, g := range s.Gaps {
			gaps.Rows = append(gaps.Rows, []string{name, g.CatalogID, g.Title, l.enum(EnumObligation, g.Obligation), l.enum(EnumPriority, g.Priority), l.clauses(g.Clauses)})
		}
		for _, it := range s.Items {
			if it.State == analytics.StatePassed {
				continue
			}
			ref := it.CatalogRef
			if it.Own {
				ref = l.t("Own")
			}
			open.Rows = append(open.Rows, []string{name, it.Key, it.Title, l.state(it.State), ref, l.clauses(it.Clauses)})
		}
	}
	ds.Sections = append(ds.Sections, bySource)
	ds.Sections = append(ds.Sections, clauseSections...)
	ds.Sections = append(ds.Sections, gaps, open)
	return ds, nil
}

// pctValue formats a whole percentage in the report language.
func (l loc) pctValue(p int) string { return l.pct(p, 100) }
