package api

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/reqimport"
)

// Risk register import: a CSV or XLSX file with the columns of the risk
// register (the risks report's Risk register sheet, in English or
// Swedish, or a sheet of one's own) creates risks, and updates the risks
// of the project whose keys it names. Columns: Key, Title, Description,
// Category, Owner, Likelihood, Impact, Status, Mitigation, Review date,
// Review owner, ATT&CK techniques and EMB3D threats. Unknown ATT&CK,
// ATLAS and EMB3D ids, statuses and numbers out of range are left out
// with a warning on the row. ?dry_run=true previews without saving.

// riskColumns maps normalised headers to risk fields.
var riskColumns = map[string]string{
	"key": "key", "nyckel": "key", "id": "key",
	"title": "title", "titel": "title", "risk": "title", "name": "title", "namn": "title",
	"description": "description", "beskrivning": "description",
	"category": "category", "kategori": "category",
	"owner": "owner", "ägare": "owner", "riskägare": "owner",
	"likelihood": "likelihood", "sannolikhet": "likelihood", "probability": "likelihood",
	"impact": "impact", "konsekvens": "impact", "severity": "impact",
	"status": "status",
	"mitigation": "mitigation", "åtgärd": "mitigation", "åtgärder": "mitigation",
	"review date": "review_date", "granskningsdatum": "review_date", "nästa granskning": "review_date",
	"review owner": "review_owner", "granskningsansvarig": "review_owner",
	"att&ck techniques": "attack", "att&ck": "attack", "attack": "attack", "att&ck-tekniker": "attack", "mitre att&ck": "attack",
	"techniques": "attack", "tekniker": "attack", "att&ck/atlas": "attack",
	"emb3d threats": "emb3d", "emb3d": "emb3d", "emb3d-hot": "emb3d", "emb3d hot": "emb3d",
}

// riskImportRow is the outcome of one row.
type riskImportRow struct {
	Row      int      `json:"row"`
	Key      string   `json:"key,omitempty"`
	Title    string   `json:"title"`
	Action   string   `json:"action"` // create | update | unchanged | error
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings"`
}

type riskImportResult struct {
	DryRun    bool            `json:"dry_run"`
	Sheet     string          `json:"sheet"`
	Columns   []string        `json:"columns"`
	Created   int             `json:"created"`
	Updated   int             `json:"updated"`
	Unchanged int             `json:"unchanged"`
	Errors    int             `json:"errors"`
	Rows      []riskImportRow `json:"rows"`
}

// riskStatus maps a status code or its English or Swedish label.
func riskStatus(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, st := range model.RiskStatuses {
		if v == st || v == strings.ToLower(report.EnumLabel("en", report.EnumRiskStatus, st)) || v == strings.ToLower(report.EnumLabel("sv", report.EnumRiskStatus, st)) {
			return st, true
		}
	}
	return "", false
}

// riskTable finds the sheet and header row of a risk register: the header
// with a Title and the most other risk columns (the report's Risk register
// rather than its other sheets).
func riskTable(sheets []reqimport.Sheet) (sheet reqimport.Sheet, header int, cols map[int]string, ok bool) {
	for _, s := range sheets {
		for i := 0; i < len(s.Rows) && i < 20; i++ {
			m := map[int]string{}
			seen := map[string]bool{}
			for j, h := range s.Rows[i] {
				if f, ok := riskColumns[reqimport.NormHeader(h)]; ok && !seen[f] {
					m[j], seen[f] = f, true
				}
			}
			if seen["title"] && len(m) >= 2 && len(m) > len(cols) {
				sheet, header, cols, ok = s, i, m, true
			}
		}
	}
	return sheet, header, cols, ok
}

func (s *Server) handleImportRisks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	u, err := readUpload(w, r, "file", s.maxUpload())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	dry := r.FormValue("dry_run") == "true" || r.FormValue("dry_run") == "1"
	sheets, err := reqimport.ReadSheets(u.data, u.name)
	if err != nil {
		if errors.Is(err, reqimport.ErrInvalid) {
			s.fail(w, r, badRequest("%s", strings.TrimPrefix(err.Error(), "invalid import: ")))
			return
		}
		s.fail(w, r, err)
		return
	}
	sheet, header, cols, ok := riskTable(sheets)
	if !ok {
		s.fail(w, r, badRequest("no header row with a Title column and risk columns (Likelihood, Impact, Mitigation, ATT&CK techniques, EMB3D threats …) was found"))
		return
	}
	ctx := r.Context()
	existing, err := s.Store.ListRisks(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byKey := map[string]*model.Risk{}
	for i := range existing {
		byKey[strings.ToLower(existing[i].Key)] = &existing[i]
	}
	out := riskImportResult{DryRun: dry, Sheet: sheet.Name, Columns: []string{}, Rows: []riskImportRow{}}
	for j := 0; j < len(sheet.Rows[header]); j++ {
		if f, ok := cols[j]; ok {
			out.Columns = append(out.Columns, f)
		}
	}
	for i := header + 1; i < len(sheet.Rows); i++ {
		row := sheet.Rows[i]
		cell := map[string]string{}
		has := map[string]bool{}
		blank := true
		for j, f := range cols {
			v := ""
			if j < len(row) {
				v = reqimport.CleanCell(row[j])
			}
			cell[f], has[f] = v, true
			if v != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		res := riskImportRow{Row: sheet.Lines[i], Key: cell["key"], Title: cell["title"], Warnings: []string{}}
		warn := func(msg string) { res.Warnings = append(res.Warnings, msg) }
		cur := byKey[strings.ToLower(cell["key"])]
		if cell["key"] != "" && cur == nil {
			warn("no risk " + cell["key"] + " in the project: a new risk is created")
		}
		var k model.Risk
		if cur != nil {
			k = *cur
			k.Attack, k.Emb3d = append([]string{}, cur.Attack...), append([]string{}, cur.Emb3d...)
		} else {
			k = model.Risk{ProjectID: pr.ID, CreatedBy: p.User.Email}
		}
		for _, f := range []struct {
			name string
			dst  *string
		}{{"title", &k.Title}, {"description", &k.Description}, {"category", &k.Category}, {"mitigation", &k.Mitigation}, {"review_date", &k.ReviewDate}} {
			if has[f.name] && (cell[f.name] != "" || f.name != "title") {
				*f.dst = cell[f.name]
			}
		}
		for _, f := range []struct {
			name string
			dst  *int
		}{{"likelihood", &k.Likelihood}, {"impact", &k.Impact}} {
			if v := cell[f.name]; v != "" {
				n, err := strconv.Atoi(strings.TrimSuffix(v, ".0"))
				if err != nil || n < 1 || n > 5 {
					warn(f.name + " \"" + v + "\" is not a number from 1 to 5; left out")
					continue
				}
				*f.dst = n
			}
		}
		if v := cell["status"]; v != "" {
			if st, ok := riskStatus(v); ok {
				k.Status = st
			} else {
				warn("unknown status \"" + v + "\"; left out")
			}
		}
		if has["owner"] {
			k.Owner = cell["owner"]
		}
		if has["review_owner"] {
			k.ReviewOwner = cell["review_owner"]
		}
		if has["attack"] {
			ids, w := reqimport.FrameworkIDs(reqimport.FrameworkAttack, cell["attack"])
			k.Attack = ids
			if w != "" {
				warn(w)
			}
		}
		if has["emb3d"] {
			ids, w := reqimport.FrameworkIDs(reqimport.FrameworkEmb3dThreats, cell["emb3d"])
			k.Emb3d = ids
			if w != "" {
				warn(w)
			}
		}
		switch {
		case strings.TrimSpace(k.Title) == "":
			res.Action, res.Error = "error", "a risk needs a title"
		case cur == nil:
			res.Action = "create"
		case riskUnchanged(cur, &k):
			res.Action = "unchanged"
		default:
			res.Action = "update"
		}
		if res.Action == "create" || res.Action == "update" {
			if err := s.saveImportedRisk(r, p, cur, &k, dry); err != nil {
				res.Action, res.Error = "error", errorText(err)
			} else {
				res.Key, res.Title = k.Key, k.Title
			}
		}
		switch res.Action {
		case "create":
			out.Created++
		case "update":
			out.Updated++
		case "unchanged":
			out.Unchanged++
		default:
			out.Errors++
		}
		out.Rows = append(out.Rows, res)
	}
	if !dry {
		s.audit(ctx, p, "risk.import", "project", pr.ID, u.name+": "+strconv.Itoa(out.Created)+" created, "+strconv.Itoa(out.Updated)+" updated")
	}
	writeJSON(w, http.StatusOK, out)
}

func riskUnchanged(a, b *model.Risk) bool {
	return a.Title == b.Title && a.Description == b.Description && a.Category == b.Category && a.Mitigation == b.Mitigation &&
		a.ReviewDate == b.ReviewDate && a.Likelihood == b.Likelihood && a.Impact == b.Impact && a.Status == b.Status &&
		a.Owner == b.Owner && a.ReviewOwner == b.ReviewOwner && slices.Equal(a.Attack, b.Attack) && slices.Equal(a.Emb3d, b.Emb3d)
}

// saveImportedRisk creates or updates a risk as the risk endpoints do
// (owners resolved, ids checked); a dry run only checks.
func (s *Server) saveImportedRisk(r *http.Request, p *Principal, cur, k *model.Risk, dry bool) error {
	if err := s.checkTechniques(k); err != nil {
		return err
	}
	if err := s.checkFrameworkIDs(nil, &k.Emb3d, true); err != nil {
		return err
	}
	oldOwner, oldReview := "", ""
	if cur != nil {
		oldOwner, oldReview = cur.Owner, cur.ReviewOwner
	}
	if err := s.riskOwners(r, p, k, k.Owner != oldOwner, k.ReviewOwner != oldReview); err != nil {
		return err
	}
	if dry {
		if strings.TrimSpace(k.Title) == "" {
			return badRequest("a risk needs a title")
		}
		return nil
	}
	ctx := r.Context()
	if cur == nil {
		if err := s.Store.CreateRisk(ctx, p.TenantID(), k); err != nil {
			return err
		}
		s.audit(ctx, p, "risk.create", "risk", k.ID, k.Key+" "+k.Title)
		s.riskAssigned(r, p, k, "", "")
		return nil
	}
	if err := s.Store.UpdateRisk(ctx, p.TenantID(), k); err != nil {
		return err
	}
	s.audit(ctx, p, "risk.update", "risk", k.ID, k.Key+" "+k.Status)
	s.riskAssigned(r, p, k, oldOwner, oldReview)
	return nil
}

// errorText is the message of an error for a row.
func errorText(err error) string {
	msg := err.Error()
	for _, prefix := range []string{"bad request: ", "invalid: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	return msg
}
