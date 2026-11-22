package report

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// Change requests in the reports: the change request report (one change
// request with its proposed changes, impact, signatures and timeline, or
// the register of all change requests). Self-contained: it registers its
// template, names and translations here.

// ChangeRequests is the id of the change request report.
const ChangeRequests = "change_requests"

// Enumeration kinds of change requests.
const (
	EnumCRStatus   = "cr_status"
	EnumCRCategory = "cr_category"
	EnumCRDecision = "cr_decision"
)

func init() {
	templates = append(templates, Template{ID: ChangeRequests, Name: "Change requests",
		Description: "One change request (its proposed changes field by field, the assessed change impact, the board's electronic signatures and the timeline), or the register of all change requests.",
		Params:      []Param{{Name: "change_request", Type: "change_request", Description: "the change request (key or id; default: the register of all change requests)"}},
		build:       buildChangeRequests})
	templateNames[Swedish][ChangeRequests] = "Ändringsbegäranden"
	for k, v := range crTexts {
		if _, ok := texts[Swedish][k]; !ok {
			texts[Swedish][k] = v
		}
	}
	enums[English][EnumCRStatus] = map[string]string{"in_review": "In review"}
	enums[Swedish][EnumCRStatus] = map[string]string{"draft": "Utkast", "submitted": "Inskickad", "assessed": "Bedömd", "in_review": "Under beslut", "approved": "Godkänd",
		"rejected": "Avslagen", "deferred": "Uppskjuten", "implemented": "Genomförd", "verified": "Verifierad", "closed": "Stängd", "created": "Skapad", "withdrawn": "Återtagen"}
	enums[English][EnumCRCategory] = map[string]string{"defect_driven": "Defect-driven", "risk_driven": "Risk-driven"}
	enums[Swedish][EnumCRCategory] = map[string]string{"correction": "Rättelse", "enhancement": "Förbättring", "regulatory": "Regelkrav", "defect_driven": "Defektdriven",
		"risk_driven": "Riskdriven", "other": "Övrigt"}
	enums[Swedish][EnumCRDecision] = map[string]string{"approve": "Godkänn", "reject": "Avslå", "defer": "Skjut upp"}
}

var crTexts = map[string]string{
	"Change requests":                       "Ändringsbegäranden",
	"Change request":                        "Ändringsbegäran",
	"Category":                              "Kategori",
	"Reason":                                "Motivering",
	"Requested by":                          "Begärd av",
	"Impact action":                         "Hantering av påverkan",
	"Effort estimate":                       "Uppskattad insats",
	"Assessment notes":                      "Bedömningsanteckningar",
	"Assessed by":                           "Bedömd av",
	"Decided by":                            "Beslutad av",
	"Decision":                              "Beslut",
	"Applied by":                            "Genomförd av",
	"Verified by":                           "Verifierad av",
	"Closed by":                             "Stängd av",
	"Items":                                 "Poster",
	"Proposed changes":                      "Föreslagna ändringar",
	"Item":                                  "Post",
	"Field":                                 "Fält",
	"Current":                               "Nuvarande",
	"Proposed":                              "Föreslaget",
	"Base version":                          "Basversion",
	"Applied version":                       "Genomförd version",
	"Change impact":                         "Ändringspåverkan",
	"Kind":                                  "Typ",
	"Depth":                                 "Djup",
	"Effect":                                "Effekt",
	"Path":                                  "Väg",
	"Signatures":                            "Signaturer",
	"Signed by":                             "Signerad av",
	"Meaning":                               "Betydelse",
	"Comment":                               "Kommentar",
	"Signed":                                "Signerad",
	"Hash":                                  "Kontrollsumma",
	"Valid":                                 "Giltig",
	"Timeline":                              "Tidslinje",
	"When":                                  "När",
	"Step":                                  "Steg",
	"Who":                                   "Vem",
	"Linked defects":                        "Kopplade defekter",
	"Linked risks":                          "Kopplade risker",
	"Tracker issues":                        "Ärenden",
	"Baseline":                              "Baslinje",
	"Created":                               "Skapad",
	"suspect":                               "misstänkt",
	"review recommended":                    "granskning rekommenderas",
	"The impact has not been assessed yet.": "Påverkan har inte bedömts än.",
	"Change request {key} was not found.":   "Ändringsbegäran {key} hittades inte.",
}

// crValue shows a proposed or current JSON value as text.
func crValue(v json.RawMessage) string {
	if len(v) == 0 || string(v) == "null" {
		return "—"
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var list []string
	if json.Unmarshal(v, &list) == nil {
		return strings.Join(list, ", ")
	}
	return string(v)
}

func buildChangeRequests(d *Data, params map[string]string) (*export.Dataset, error) {
	l := d.loc()
	want := strings.TrimSpace(params["change_request"])
	if want == "" {
		reg := export.Section{Name: "change_requests", Title: l.t("Change requests"), Columns: l.cols("Key", "Title", "Status", "Category", "Priority", "Items", "Requested by", "Decided by", "Reason")}
		for _, cr := range d.ChangeRequests {
			keys := make([]string, len(cr.Items))
			for i, it := range cr.Items {
				keys[i] = it.Key
			}
			reg.Rows = append(reg.Rows, []string{cr.Key, cr.Title, l.enum(EnumCRStatus, cr.Status), l.enum(EnumCRCategory, cr.Category), l.enum(EnumPriority, cr.Priority),
				strings.Join(keys, ", "), cr.CreatedBy, cr.DecidedBy, cr.Reason})
		}
		return &export.Dataset{Sections: []export.Section{reg}}, nil
	}
	var cr *model.ChangeRequest
	for i := range d.ChangeRequests {
		if c := &d.ChangeRequests[i]; c.ID == want || strings.EqualFold(c.Key, want) {
			cr = c
		}
	}
	if cr == nil {
		return nil, invalidf("%s", l.f("Change request {key} was not found.", "key", want))
	}
	keys := func(refs []model.ItemRefLite) string {
		out := make([]string, len(refs))
		for i, r := range refs {
			out[i] = r.Key
		}
		return strings.Join(out, ", ")
	}
	summary := export.Section{Name: "summary", Title: l.t("Change request"), Columns: l.cols("Field", "Value")}
	add := func(k, v string) {
		if v != "" {
			summary.Rows = append(summary.Rows, []string{l.t(k), v})
		}
	}
	add("Key", cr.Key)
	add("Title", cr.Title)
	add("Status", l.enum(EnumCRStatus, cr.Status))
	add("Category", l.enum(EnumCRCategory, cr.Category))
	add("Priority", l.enum(EnumPriority, cr.Priority))
	add("Reason", cr.Reason)
	add("Requested by", cr.CreatedBy+" "+ts(cr.CreatedAt))
	add("Impact action", cr.ImpactAction)
	add("Effort estimate", cr.Effort)
	add("Assessment notes", cr.ImpactNotes)
	if cr.AssessedBy != "" {
		add("Assessed by", cr.AssessedBy+" "+tsPtr(cr.AssessedAt))
	}
	if cr.DecidedBy != "" {
		add("Decided by", cr.DecidedBy+" "+tsPtr(cr.DecidedAt))
		add("Decision", cr.DecisionComment)
	}
	if cr.AppliedBy != "" {
		add("Applied by", cr.AppliedBy+" "+tsPtr(cr.AppliedAt))
	}
	if cr.VerifiedBy != "" {
		add("Verified by", cr.VerifiedBy+" "+tsPtr(cr.VerifiedAt))
	}
	if cr.ClosedBy != "" {
		add("Closed by", cr.ClosedBy+" "+tsPtr(cr.ClosedAt))
	}
	add("Linked defects", keys(cr.Defects))
	add("Linked risks", keys(cr.Risks))
	add("Tracker issues", strings.Join(cr.Links.Issues, ", "))
	add("Hash", cr.ContentHash)
	changes := export.Section{Name: "changes", Title: l.t("Proposed changes"), Columns: l.cols("Item", "Field", "Current", "Proposed", "Base version", "Applied version")}
	for _, it := range cr.Items {
		fields := make([]string, 0, len(it.Proposed))
		for f := range it.Proposed {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			applied := ""
			if it.AppliedVersion > 0 {
				applied = itoa(it.AppliedVersion)
			}
			changes.Rows = append(changes.Rows, []string{it.Key + " " + it.Title, f, crValue(it.Current[f]), crValue(it.Proposed[f]), itoa(it.BaseVersion), applied})
		}
	}
	impact := export.Section{Name: "impact", Title: l.t("Change impact"), Columns: l.cols("Kind", "Key", "Title", "Depth", "Effect", "Path")}
	if cr.Impact == nil {
		impact.Rows = append(impact.Rows, []string{l.t("The impact has not been assessed yet."), "", "", "", "", ""})
	} else {
		lists := cr.Impact.Lists()
		for _, sec := range model.ImpactSections {
			for _, it := range *lists[sec] {
				var path []string
				for _, st := range it.Path {
					path = append(path, st.Key)
				}
				mode := it.Mode
				if mode == model.ModeReview {
					mode = l.t("review recommended")
				} else if mode != "" {
					mode = l.t(mode)
				}
				impact.Rows = append(impact.Rows, []string{humanize(it.Kind), it.Key, it.Title, itoa(it.Depth), mode, strings.Join(path, " → ")})
			}
		}
	}
	sigs := export.Section{Name: "signatures", Title: l.t("Signatures"), Columns: l.cols("Signed by", "Decision", "Meaning", "Comment", "Signed", "Hash", "Valid")}
	for _, a := range cr.Approvals {
		name := a.UserName
		if name == "" {
			name = a.UserEmail
		}
		h := a.ContentHash
		if len(h) > 12 {
			h = h[:12]
		}
		sigs.Rows = append(sigs.Rows, []string{name + " <" + a.UserEmail + ">", l.enum(EnumCRDecision, a.Decision), a.Meaning, a.Comment, ts(a.SignedAt), h, l.yesNo(a.Valid)})
	}
	tl := export.Section{Name: "timeline", Title: l.t("Timeline"), Columns: l.cols("When", "Step", "Who", "Comment")}
	for _, e := range cr.Events {
		step := e.Action
		if st, ok := strings.CutPrefix(e.Action, "signed."); ok {
			step = l.t("Signed") + ": " + l.enum(EnumCRDecision, st)
		} else {
			step = l.enum(EnumCRStatus, step)
		}
		tl.Rows = append(tl.Rows, []string{ts(e.At), step, e.Actor, e.Comment})
	}
	return &export.Dataset{Subtitle: cr.Key + " " + cr.Title, Sections: []export.Section{summary, changes, impact, sigs, tl}}, nil
}
