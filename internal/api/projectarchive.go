package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/store"
)

// Project archives: save one project as a ZIP file, load such a file as a
// new project, or duplicate a project in one step (see
// store/projectarchive.go for the format).
func (s *Server) routesProjectArchive(h routeFunc) {
	// saving reads everything in the project: project settings and export
	h("GET /api/v1/projects/{pid}/archive", s.handleSaveProject, "project.update", "export")
	h("POST /api/v1/projects/import", s.handleLoadProject, "project.create")
	h("POST /api/v1/projects/{pid}/duplicate", s.handleDuplicateProject, "project.create", "project.update", "export")
}

// archiveFileName is the download name of a project archive.
func archiveFileName(key string, t time.Time) string {
	return strings.ToLower(key) + "-project-" + t.UTC().Format("20060102-150405") + ".zip"
}

func (s *Server) handleSaveProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	// large projects outlast server.write_timeout
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+archiveFileName(pr.Key, time.Now())+`"`)
	m, err := s.Store.ExportProject(r.Context(), p.TenantID(), pr.ID, w, Version)
	if err != nil {
		// the response is already (partly) written; the archive is unusable
		s.Log.Error("saving project failed", "project", pr.ID, "err", err)
		return
	}
	s.audit(r.Context(), p, "project.save", "project", pr.ID, fmt.Sprintf("%s: %d rows, %d files", pr.Key, m.Rows(), m.Files))
}

// handleLoadProject creates a new project from an uploaded archive
// (multipart: archive, optional key, name and dry_run).
func (s *Server) handleLoadProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreUpload())
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.fail(w, r, badRequest("multipart form with field \"archive\" required: %v", err))
		return
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck // temp files
	fhs := r.MultipartForm.File["archive"]
	if len(fhs) != 1 {
		s.fail(w, r, badRequest("exactly one file in field \"archive\" required"))
		return
	}
	opt := store.ProjectImport{Key: r.FormValue("key"), Name: r.FormValue("name"), CreatedBy: p.User.ID}
	if v := r.FormValue("dry_run"); v != "" {
		dry, err := strconv.ParseBool(v)
		if err != nil {
			s.fail(w, r, badRequest("dry_run must be true or false"))
			return
		}
		opt.DryRun = dry
	}
	f, err := fhs[0].Open()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	res, err := s.Store.ImportProject(r.Context(), p.TenantID(), f, fhs[0].Size, opt)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if opt.DryRun {
		writeJSON(w, http.StatusOK, res)
		return
	}
	if err := s.keepSharing(r.Context(), p, res.Project.ID, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.load", "project", res.Project.ID, fmt.Sprintf("%s from %s (%s): %d rows, %d files",
		res.Project.Key, res.Manifest.Project.Key, store.CleanFileName(fhs[0].Filename), sumCounts(res.Imported), res.Files))
	writeJSON(w, http.StatusCreated, res)
}

// handleDuplicateProject saves a project and loads it again as a new one.
func (s *Server) handleDuplicateProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Key) == "" {
		s.fail(w, r, badRequest("key is required"))
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = pr.Name
	}
	ctx := r.Context()
	tmp, err := os.CreateTemp("", "qtf-duplicate-*.zip")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := s.Store.ExportProject(ctx, p.TenantID(), pr.ID, tmp, Version); err != nil {
		s.fail(w, r, err)
		return
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.Store.ImportProject(ctx, p.TenantID(), tmp, size, store.ProjectImport{Key: req.Key, Name: req.Name, CreatedBy: p.User.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.keepSharing(ctx, p, res.Project.ID, pr.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "project.duplicate", "project", res.Project.ID, fmt.Sprintf("%s from %s: %d rows, %d files",
		res.Project.Key, pr.Key, sumCounts(res.Imported), res.Files))
	writeJSON(w, http.StatusCreated, res)
}

func sumCounts(counts map[string]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}
