package api

import (
	"archive/zip"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk operations on the document archive of a project: tags, description,
// delete (each needs the uploader or project.update, like one file) and a
// ZIP download.

// maxZipBytes limits the size of the files in one ZIP download.
const maxZipBytes = 512 << 20

type bulkFile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

// bulkFilesOf loads files that belong to the project.
func (s *Server) bulkFilesOf(r *http.Request, p *Principal, projectID string, ids []string) ([]*model.FileInfo, error) {
	ids, err := bulkIDs(ids)
	if err != nil {
		return nil, err
	}
	var out []*model.FileInfo
	for _, id := range ids {
		f, err := s.Store.GetFile(r.Context(), p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if f.ProjectID == nil || *f.ProjectID != projectID {
			return nil, fmt.Errorf("file %s: %w", id, store.ErrNotFound)
		}
		out = append(out, f)
	}
	return out, nil
}

// handleBulkDocuments: POST /projects/{pid}/documents/bulk
// {"ids": [...], "add_tags": [...], "remove_tags": [...], "description"}.
// Files the caller may not change are skipped (reason "not_allowed").
func (s *Server) handleBulkDocuments(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs         []string `json:"ids"`
		AddTags     []string `json:"add_tags"`
		RemoveTags  []string `json:"remove_tags"`
		Description *string  `json:"description"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.AddTags) == 0 && len(req.RemoveTags) == 0 && req.Description == nil {
		s.fail(w, r, badRequest("nothing to change: add_tags, remove_tags or description"))
		return
	}
	list, err := s.bulkFilesOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	remove := map[string]bool{}
	for _, t := range req.RemoveTags {
		remove[strings.ToLower(strings.TrimSpace(t))] = true
	}
	ctx := r.Context()
	updated, skipped := []bulkFile{}, []bulkFile{}
	var changes []*model.FileInfo
	for _, f := range list {
		if !s.canManageFile(p, f) {
			skipped = append(skipped, bulkFile{f.ID, f.Name, "not_allowed"})
			continue
		}
		tags := slices.DeleteFunc(append([]string(nil), f.Tags...), func(t string) bool { return remove[strings.ToLower(t)] })
		for _, t := range req.AddTags {
			if t = strings.TrimSpace(t); t != "" && !slices.ContainsFunc(tags, func(x string) bool { return strings.EqualFold(x, t) }) {
				tags = append(tags, t)
			}
		}
		f.Tags = tags
		if req.Description != nil {
			f.Description = *req.Description
		}
		changes = append(changes, f)
	}
	for _, f := range changes {
		if err := s.Store.UpdateFile(ctx, p.TenantID(), f); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", f.Name, err))
			return
		}
		updated = append(updated, bulkFile{ID: f.ID, Name: f.Name})
		s.audit(ctx, p, "file.update", "file", f.ID, f.Name+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "skipped": skipped})
}

// handleBulkDeleteDocuments: DELETE /projects/{pid}/documents/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteDocuments(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	list, err := s.bulkFilesOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	skipped := []bulkFile{}
	n := 0
	for _, f := range list {
		if !s.canManageFile(p, f) {
			skipped = append(skipped, bulkFile{f.ID, f.Name, "not_allowed"})
			continue
		}
		if err := s.Store.DeleteFile(r.Context(), p.TenantID(), f.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", f.Name, err))
			return
		}
		n++
		s.audit(r.Context(), p, "file.delete", "file", f.ID, f.Name+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n, "skipped": skipped})
}

// zipNames gives every file a unique name inside the archive
// ("report.pdf", "report (2).pdf", …).
func zipNames(names []string) []string {
	used := map[string]bool{}
	out := make([]string, len(names))
	for i, n := range names {
		n = strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || r < 32 {
				return '_'
			}
			return r
		}, strings.TrimSpace(n))
		if n == "" || n == "." || n == ".." {
			n = "file"
		}
		ext := path.Ext(n)
		base := strings.TrimSuffix(n, ext)
		cand := n
		for k := 2; used[strings.ToLower(cand)]; k++ {
			cand = base + " (" + strconv.Itoa(k) + ")" + ext
		}
		used[strings.ToLower(cand)] = true
		out[i] = cand
	}
	return out
}

// handleDocumentsZip: POST /projects/{pid}/documents/zip {"ids": [...]}
// downloads the files as one ZIP archive.
func (s *Server) handleDocumentsZip(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	list, err := s.bulkFilesOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var total int64
	names := make([]string, len(list))
	for i, f := range list {
		total += f.Size
		names[i] = f.Name
	}
	if total > maxZipBytes {
		s.fail(w, r, badRequest("the files are %d MB together; download at most %d MB at a time", total>>20, maxZipBytes>>20))
		return
	}
	names = zipNames(names)
	stamp := time.Now().UTC().Format("20060102-1504")
	fname := pr.Key + "-documents-" + stamp + ".zip"
	hd := w.Header()
	hd.Set("Content-Type", "application/zip")
	hd.Set("Content-Disposition", `attachment; filename="`+fname+`"`)
	hd.Set("X-Content-Type-Options", "nosniff")
	zw := zip.NewWriter(w)
	for i, f := range list {
		data, err := s.Store.FileData(r.Context(), f.ID)
		if err != nil {
			// headers are sent: end the archive early, the client sees a broken file
			s.Log.Warn("documents zip", "file", f.ID, "err", err)
			break
		}
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: names[i], Method: zip.Deflate, Modified: f.CreatedAt})
		if err != nil {
			break
		}
		if _, err := fw.Write(data); err != nil {
			break
		}
	}
	_ = zw.Close()
	s.audit(r.Context(), p, "file.download_zip", "project", pr.ID, fmt.Sprintf("%d files", len(list)))
}
