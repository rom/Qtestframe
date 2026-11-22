package report

import (
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/soa"
)

// The Statement of Applicability (package soa) as a standard report: every
// control of a reference source with its applicability, justification,
// implementation status, owner, requirements and evidence. Self-contained:
// it registers its template, names and translations here.

// SoA is the id of the Statement of Applicability report.
const SoA = "soa"

// DefaultSoASource is the source of the SoA report when none is given.
const DefaultSoASource = "iso-27001"

// Enumeration kind of the SoA implementation states.
const EnumSoAStatus = "soa_status"

func init() {
	templates = append(templates, Template{ID: SoA, Name: "Statement of Applicability",
		Description: "Every control of a reference source (for ISO/IEC 27001:2022 the 93 controls of Annex A): applicable or not with the justification, implementation status from the adopted requirements and their verification, owner, requirements and evidence.",
		Params:      []Param{{Name: "source", Type: "source", Description: "the reference source id (default iso-27001)"}},
		build:       buildSoA})
	templateNames[Swedish][SoA] = "Uttalande om tillämplighet"
	for k, v := range soaTexts {
		if _, ok := texts[Swedish][k]; !ok {
			texts[Swedish][k] = v
		}
	}
	enums[English][EnumSoAStatus] = map[string]string{soa.Implemented: "Implemented", soa.Partial: "Partially implemented", soa.Planned: "Planned", soa.NotImplemented: "Not implemented"}
	enums[Swedish][EnumSoAStatus] = map[string]string{soa.Implemented: "Införd", soa.Partial: "Delvis införd", soa.Planned: "Planerad", soa.NotImplemented: "Inte införd"}
}

var soaTexts = map[string]string{
	"Statement of Applicability": "Uttalande om tillämplighet",
	"Controls":                   "Åtgärder",
	"Applicable":                 "Tillämplig",
	"Excluded":                   "Undantagen",
	"Not decided":                "Ej beslutad",
	"Implemented":                "Införda",
	"Partially implemented":      "Delvis införda",
	"Planned":                    "Planerade",
	"Not implemented":            "Inte införda",
	"Control":                    "Åtgärd",
	"Group":                      "Grupp",
	"Implementation":             "Införande",
	"Evidence":                   "Belägg",
	"Applicability by group":     "Tillämplighet per grupp",
}

func buildSoA(d *Data, params map[string]string) (*export.Dataset, error) {
	l := d.loc()
	c, err := ComplianceCatalog()
	if err != nil {
		return nil, err
	}
	source := strings.TrimSpace(params["source"])
	if source == "" {
		source = DefaultSoASource
	}
	if c.Source(source) == nil {
		return nil, invalidf("unknown reference source %q", source)
	}
	tests := d.testIndex()
	byReq := d.linksByRequirement()
	waived := d.waivedRequirements()
	in := soa.Input{Catalog: c, SourceID: source, Lang: l.lang, Decisions: d.SoADecisions}
	for _, r := range d.Requirements {
		x := soa.Requirement{ID: r.ID, Key: r.Key, Title: r.Title, Status: r.Status, CatalogRef: r.CatalogRef, Source: r.Source, Waiver: strings.Join(waived[r.ID], ", ")}
		for _, tid := range byReq[r.ID] {
			if t := tests[tid]; t != nil {
				x.Tests = append(x.Tests, soa.Evidence{ID: t.ID, Key: t.Key, Title: t.Title, Result: t.LastResult})
			}
		}
		in.Requirements = append(in.Requirements, x)
	}
	st, err := soa.Build(in)
	if err != nil {
		return nil, invalidf("%v", err)
	}
	sum := st.Summary
	summary := l.kpiSection("summary", "Key figures",
		kv{"Source", st.Source.Title},
		kv{"Controls", itoa(sum.Controls)},
		kv{"Applicable", itoa(sum.Applicable)},
		kv{"Excluded", itoa(sum.Excluded)},
		kv{"Not decided", itoa(sum.Undecided)},
		kv{"Implemented", itoa(sum.Implemented)},
		kv{"Partially implemented", itoa(sum.Partial)},
		kv{"Planned", itoa(sum.Planned)},
		kv{"Not implemented", itoa(sum.NotImplemented)},
	)
	type groupCount struct {
		title                                                  string
		controls, applicable, excluded, undecided, implemented int
	}
	var order []string
	groups := map[string]*groupCount{}
	table := export.Section{Name: "statement", Title: l.t("Statement of Applicability"),
		Columns: l.cols("Control", "Title", "Applicable", "Justification", "Implementation", "Owner", "Requirements", "Evidence")}
	for _, r := range st.Rows {
		app := l.t("Not decided")
		if r.Applicable != nil {
			app = l.yesNo(*r.Applicable)
		}
		var reqs, ev []string
		for _, q := range r.Requirements {
			k := q.Key
			if q.Waiver != "" {
				k += " (" + l.tc("state", "Waived") + ": " + q.Waiver + ")"
			}
			reqs = append(reqs, k)
		}
		for _, e := range r.Evidence {
			res := e.Result
			if res == "" {
				res = "not_run"
			}
			ev = append(ev, e.Key+" ("+l.enum(EnumResult, res)+")")
		}
		title := r.Title
		if r.GroupTitle != "" && title == "" {
			title = r.GroupTitle
		}
		table.Rows = append(table.Rows, []string{r.ID, title, app, r.Justification, l.enum(EnumSoAStatus, r.Status), r.Owner, strings.Join(reqs, ", "), strings.Join(ev, ", ")})
		g := groups[r.Group]
		if g == nil {
			g = &groupCount{title: r.GroupTitle}
			groups[r.Group] = g
			order = append(order, r.Group)
		}
		g.controls++
		switch {
		case r.Applicable == nil:
			g.undecided++
		case *r.Applicable:
			g.applicable++
			if r.Status == soa.Implemented {
				g.implemented++
			}
		default:
			g.excluded++
		}
	}
	ds := &export.Dataset{Sections: []export.Section{summary}}
	if len(order) > 1 || (len(order) == 1 && order[0] != "") {
		by := export.Section{Name: "groups", Title: l.t("Applicability by group"), Columns: l.cols("Group", "Controls", "Applicable", "Excluded", "Not decided", "Implemented")}
		for _, k := range order {
			g := groups[k]
			name := k
			if g.title != "" {
				name = k + " " + g.title
			}
			by.Rows = append(by.Rows, []string{name, itoa(g.controls), itoa(g.applicable), itoa(g.excluded), itoa(g.undecided), itoa(g.implemented)})
		}
		ds.Sections = append(ds.Sections, by)
	}
	ds.Sections = append(ds.Sections, table)
	ds.Subtitle = d.subtitle() + " · " + st.Source.Title
	return ds, nil
}
