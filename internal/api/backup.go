package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/backup"
	"github.com/rom/qtestframe/internal/store"
)

// routesBackup registers /admin/backup and /admin/restore.
func (s *Server) routesBackup(h routeFunc) {
	h("GET /api/v1/admin/backup", s.handleBackup)
	h("POST /api/v1/admin/restore", s.handleRestore)
	s.routesBackups(h)
}

// handleBackup streams a logical backup of the whole database (all
// tenants, files included, sessions excluded) as a ZIP archive.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	// large backups outlast server.write_timeout: lift the deadline for this
	// super-admin-only response (errors mean the writer does not support it)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	name := "qtestframe-backup-" + time.Now().UTC().Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	m, err := s.Store.Backup(r.Context(), w, Version)
	if err != nil {
		// the response is already (partly) written; the archive is unusable
		s.Log.Error("backup failed", "err", err)
		return
	}
	s.audit(r.Context(), p, "admin.backup", "system", "", fmt.Sprintf("%d rows in %d tables, schema %d", m.Rows(), len(m.Tables), m.SchemaVersion))
}

// maxRestoreUpload bounds the request body of a restore (the archive plus
// multipart overhead).
func maxRestoreUpload() int64 { return store.MaxRestoreSize + 1<<20 }

// handleRestore replaces all data with an uploaded backup.
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	// large uploads outlast server.read_timeout; the body is size-bounded below
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreUpload())
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.fail(w, r, badRequest("multipart form with field \"backup\" required: %v", err))
		return
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck // temp files
	if v := r.MultipartForm.Value["confirm"]; len(v) != 1 || v[0] != "RESTORE" {
		s.fail(w, r, badRequest("restore replaces all data: send confirm=RESTORE"))
		return
	}
	fhs := r.MultipartForm.File["backup"]
	if len(fhs) != 1 {
		s.fail(w, r, badRequest("exactly one file in field \"backup\" required"))
		return
	}
	f, err := fhs[0].Open()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	var src io.ReaderAt = f
	size := fhs[0].Size
	head := make([]byte, len(backup.Magic))
	if n, _ := f.ReadAt(head, 0); backup.IsEncrypted(head[:n]) {
		// an encrypted archive (scheduled backups, qtestframe backup
		// --encrypt): the passphrase of the form, else the one of the
		// scheduled backups
		pass := ""
		if v := r.MultipartForm.Value["passphrase"]; len(v) == 1 {
			pass = strings.TrimSpace(v[0])
		}
		if pass == "" {
			pass = s.conf().Backup.Passphrase
		}
		a, err := backup.DecryptToTemp(io.NewSectionReader(f, 0, size), pass, "")
		if err != nil {
			s.fail(w, r, badRequest("%v", err))
			return
		}
		defer a.Close()
		src, size = a, a.Size
	}
	counts, err := s.Store.Restore(r.Context(), src, size)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	// the restored system settings apply at once
	if err := s.ReloadSettings(r.Context()); err != nil {
		s.Log.Warn("restored system settings not applied", "err", err)
	}
	// written after the restore, i.e. into the restored data
	s.audit(r.Context(), p, "admin.restore", "system", "", fmt.Sprintf("%d rows in %d tables from %s", total, len(counts), store.CleanFileName(fhs[0].Filename)))
	s.clearCookie(w, sessionCookie)
	writeJSON(w, http.StatusOK, map[string]any{"restored": counts})
}
