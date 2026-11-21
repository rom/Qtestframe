package api

import (
	"archive/zip"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk operations on reports: generate several standard templates and saved
// definitions in one go (as one ZIP or each saved in the archive), and
// delete several saved definitions.

const maxBatchReports = 50

type batchFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// handleReportBatch: POST /projects/{pid}/reports/batch
// {"templates": [{"template", "params"}], "definitions": [ids], "format", "archive"}.
// With archive, each report is saved in the document archive →
// {"files": [File], "failed": [{name, error}]}; otherwise all are returned
// in one ZIP (a report that fails makes the whole request fail).
func (s *Server) handleReportBatch(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Templates []struct {
			Template string            `json:"template"`
			Params   map[string]string `json:"params"`
		} `json:"templates"`
		Definitions []string `json:"definitions"`
		Format      string   `json:"format"`
		Archive     bool     `json:"archive"`
		Lang        string   `json:"lang"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	n := len(req.Templates) + len(uniqStrings(req.Definitions))
	if n == 0 {
		s.fail(w, r, badRequest("choose at least one report"))
		return
	}
	if n > maxBatchReports {
		s.fail(w, r, badRequest("at most %d reports at a time", maxBatchReports))
		return
	}
	if req.Archive && !canWriteReports(w, p) {
		return
	}
	lang := reportLang(r, p, req.Lang)
	var jobs []reportJob
	for _, t := range req.Templates {
		params := t.Params
		if params == nil {
			params = map[string]string{}
		}
		name := strings.TrimSpace(t.Template)
		jobs = append(jobs, reportJob{template: name, params: params, format: req.Format, stem: name, lang: lang})
	}
	for _, id := range uniqStrings(req.Definitions) {
		d, err := s.Store.GetReportDefinition(r.Context(), p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if d.ProjectID != pr.ID {
			s.fail(w, r, fmt.Errorf("report definition %s: %w", id, store.ErrNotFound))
			return
		}
		c, err := report.ParseCustomConfig(d.Config)
		if err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", d.Name, reportError(err)))
			return
		}
		if c.Title == "" {
			c.Title = d.Name
		}
		jobs = append(jobs, reportJob{custom: c, format: req.Format, stem: d.Name, lang: lang})
	}

	if req.Archive {
		files, failed := []*model.FileInfo{}, []batchFailure{}
		for _, job := range jobs {
			rep, err := s.render(r, p, pr, job)
			if err == nil {
				var fi *model.FileInfo
				if fi, err = s.archiveReport(r, p, pr, rep); err == nil {
					files = append(files, fi)
					continue
				}
			}
			failed = append(failed, batchFailure{Name: job.stem, Error: err.Error()})
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": files, "failed": failed})
		return
	}
	// render everything first so that an error can still be reported as JSON
	var reps []*renderedReport
	for _, job := range jobs {
		rep, err := s.render(r, p, pr, job)
		if err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", job.stem, err))
			return
		}
		reps = append(reps, rep)
	}
	names := make([]string, len(reps))
	for i, rep := range reps {
		names[i] = rep.name
	}
	names = zipNames(names)
	fname := pr.Key + "-reports-" + time.Now().UTC().Format("20060102-1504") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(fname))
	zw := zip.NewWriter(w)
	now := time.Now()
	for i, rep := range reps {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: names[i], Method: zip.Deflate, Modified: now})
		if err != nil {
			break
		}
		if _, err := fw.Write(rep.data); err != nil {
			break
		}
	}
	_ = zw.Close()
	s.audit(r.Context(), p, "report.generate", "project", pr.ID, fmt.Sprintf("batch of %d", len(reps)))
}

// handleBulkDeleteReportDefinitions: DELETE /projects/{pid}/reports/definitions/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteReportDefinitions(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !canWriteReports(w, p) {
		return
	}
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ids, err := bulkIDs(req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var list []*model.ReportDefinition
	for _, id := range ids {
		d, err := s.Store.GetReportDefinition(r.Context(), p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if d.ProjectID != pr.ID {
			s.fail(w, r, fmt.Errorf("report definition %s: %w", id, store.ErrNotFound))
			return
		}
		list = append(list, d)
	}
	for _, d := range list {
		if err := s.Store.DeleteReportDefinition(r.Context(), p.TenantID(), d.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", d.Name, err))
			return
		}
		s.audit(r.Context(), p, "report_definition.delete", "report_definition", d.ID, d.Name+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(list)})
}
