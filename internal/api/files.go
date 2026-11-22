package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// routeFunc registers an authenticated route with required permissions.
type routeFunc func(pattern string, fn handlerFunc, perm ...string)

// MaxImageBytes limits avatar and project icon uploads.
const MaxImageBytes = 1 << 20

func (s *Server) routesFiles(h routeFunc) {
	h("GET /api/v1/files/{id}", s.handleGetFile)
	h("GET /api/v1/files/{id}/content", s.handleFileContent)
	h("PATCH /api/v1/files/{id}", s.handleUpdateFile)
	h("DELETE /api/v1/files/{id}", s.handleDeleteFile)
	for _, t := range []string{"requirements", "tests", "runs", "executions", "step-results"} {
		h("GET /api/v1/"+t+"/{id}/attachments", s.handleListAttachments(t))
		h("POST /api/v1/"+t+"/{id}/attachments", s.handleUploadAttachment(t))
		h("POST /api/v1/"+t+"/{id}/attachments/link", s.handleLinkAttachment(t))
		h("DELETE /api/v1/"+t+"/{id}/attachments/{file}", s.handleUnlinkAttachment(t))
	}
	h("GET /api/v1/projects/{pid}/documents", s.handleListDocuments, "project.read")
	h("POST /api/v1/projects/{pid}/documents", s.handleUploadDocument, "project.read")
	h("POST /api/v1/projects/{pid}/documents/bulk", s.atomic(s.handleBulkDocuments), "project.read")
	h("DELETE /api/v1/projects/{pid}/documents/bulk", s.atomic(s.handleBulkDeleteDocuments), "project.read")
	h("POST /api/v1/projects/{pid}/documents/zip", s.handleDocumentsZip, "project.read")
	h("POST /api/v1/projects/{pid}/documents/generate", s.handleGenerateDocument, "export", "requirement.read")
	h("PUT /api/v1/auth/me/avatar", s.handleUploadAvatar)
	h("GET /api/v1/avatars/{id}", s.handleAvatar)
	h("PUT /api/v1/projects/{pid}/icon", s.handleUploadIcon, "project.update")
	h("GET /api/v1/auth/logins", s.handleListLogins)
}

// ---------------------------------------------------------------- uploads

type upload struct {
	name, contentType, description string
	tags                           []string
	data                           []byte
}

// readUpload parses a multipart upload (field "file") within max bytes.
func readUpload(w http.ResponseWriter, r *http.Request, field string, max int64) (*upload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, max+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, badRequest("file is too large (max %d MB)", max>>20)
		}
		return nil, badRequest("expected a multipart/form-data upload: %v", err)
	}
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return nil, badRequest("missing form field %q", field)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, badRequest("reading upload: %v", err)
	}
	if int64(len(data)) > max {
		return nil, badRequest("file is too large (max %d MB)", max>>20)
	}
	if len(data) == 0 {
		return nil, badRequest("file is empty")
	}
	u := &upload{name: store.CleanFileName(hdr.Filename), data: data,
		description: strings.TrimSpace(r.FormValue("description"))}
	if t := r.FormValue("tags"); t != "" {
		u.tags = strings.Split(t, ",")
	}
	u.contentType = contentTypeOf(hdr.Header.Get("Content-Type"), data)
	return u, nil
}

// contentTypeOf prefers a well-formed declared type and falls back to
// sniffing. The type only matters for inline display, which is restricted
// to a safe allow-list.
func contentTypeOf(declared string, data []byte) string {
	if mt, _, err := mime.ParseMediaType(declared); err == nil && mt != "application/octet-stream" && !strings.Contains(mt, "*") {
		return mt
	}
	mt, _, _ := mime.ParseMediaType(http.DetectContentType(data))
	return mt
}

var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// inlineTypes may be shown in the browser; everything else is downloaded.
var inlineTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"application/pdf": true, "text/plain": true}

func (s *Server) maxUpload() int64 { return int64(s.conf().Files.MaxUploadMB) << 20 }

// serveFile writes a file's content with defensive headers.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, f *model.FileInfo, inline bool) {
	data, err := s.Store.FileData(r.Context(), f.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	disp := "attachment"
	ct := "application/octet-stream"
	pdf := inline && f.ContentType == "application/pdf"
	if inline && inlineTypes[f.ContentType] {
		disp, ct = "inline", f.ContentType
		if ct == "text/plain" {
			ct = "text/plain; charset=utf-8"
		}
	}
	hd := w.Header()
	hd.Set("Content-Type", ct)
	hd.Set("Content-Disposition", disp+`; filename="`+asciiName(f.Name)+`"; filename*=UTF-8''`+url.PathEscape(f.Name))
	hd.Set("Content-Length", strconv.Itoa(len(data)))
	hd.Set("X-Content-Type-Options", "nosniff")
	// files are untrusted: sandbox them; same-origin framing allows previews.
	// Chromium refuses to render PDFs in a sandboxed document, so inline PDFs
	// get a CSP without "sandbox" (the viewer runs in its own isolated origin).
	if pdf {
		hd.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
	} else {
		hd.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; frame-ancestors 'self'")
	}
	hd.Set("X-Frame-Options", "SAMEORIGIN")
	hd.Set("Cache-Control", "private, max-age=300")
	hd.Set("ETag", `"`+f.SHA256+`"`)
	if r.Header.Get("If-None-Match") == `"`+f.SHA256+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(data)
}

func asciiName(n string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, n)
}

// ---------------------------------------------------------------- files

// file loads {id} of the caller's tenant and checks read access.
func (s *Server) file(w http.ResponseWriter, r *http.Request, p *Principal) (*model.FileInfo, bool) {
	f, err := s.Store.GetFile(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	if !p.Can(rbac.ProjectRead) {
		writeError(w, http.StatusForbidden, "forbidden", "permission project.read required")
		return nil, false
	}
	return f, true
}

func (s *Server) canManageFile(p *Principal, f *model.FileInfo) bool {
	return f.UploadedBy == p.User.Email || p.Can(rbac.ProjectUpdate)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request, p *Principal) {
	if f, ok := s.file(w, r, p); ok {
		writeJSON(w, http.StatusOK, f)
	}
}

func (s *Server) handleFileContent(w http.ResponseWriter, r *http.Request, p *Principal) {
	if f, ok := s.file(w, r, p); ok {
		s.serveFile(w, r, f, r.URL.Query().Get("inline") == "1")
	}
}

func (s *Server) handleUpdateFile(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, ok := s.file(w, r, p)
	if !ok {
		return
	}
	if !s.canManageFile(p, f) {
		writeError(w, http.StatusForbidden, "forbidden", "only the uploader or a project manager can change this file")
		return
	}
	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Tags        *[]string `json:"tags"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		f.Name = *req.Name
	}
	if req.Description != nil {
		f.Description = *req.Description
	}
	if req.Tags != nil {
		f.Tags = *req.Tags
	}
	if err := s.Store.UpdateFile(r.Context(), p.TenantID(), f); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "file.update", "file", f.ID, f.Name)
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, ok := s.file(w, r, p)
	if !ok {
		return
	}
	if !s.canManageFile(p, f) {
		writeError(w, http.StatusForbidden, "forbidden", "only the uploader or a project manager can delete this file")
		return
	}
	if err := s.Store.DeleteFile(r.Context(), p.TenantID(), f.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "file.delete", "file", f.ID, f.Name)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- attachments

// attachTarget resolves an attachable item: entity type, project and the
// read/write permissions that apply.
type attachTarget struct {
	entity, projectID string
	read, write       rbac.Permission
}

var pathEntity = map[string]string{"requirements": model.KindRequirement, "tests": model.KindTest, "runs": "run", "executions": "execution", "step-results": "step"}

func (s *Server) target(ctx context.Context, p *Principal, kind, id string) (*attachTarget, error) {
	t := &attachTarget{entity: pathEntity[kind]}
	switch kind {
	case "requirements":
		r, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		t.projectID, t.read, t.write = r.ProjectID, rbac.RequirementRd, rbac.RequirementWr
	case "tests":
		tc, err := s.Store.GetTest(ctx, p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		t.projectID, t.read, t.write = tc.ProjectID, rbac.TestRead, rbac.TestWrite
	case "runs":
		run, err := s.Store.GetRun(ctx, p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		t.projectID, t.read, t.write = run.ProjectID, rbac.RunRead, rbac.RunExecute
	case "step-results":
		eid, err := s.Store.StepResultExecution(ctx, p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		e, err := s.Store.GetExecution(ctx, p.TenantID(), eid)
		if err != nil {
			return nil, err
		}
		run, err := s.Store.GetRun(ctx, p.TenantID(), e.RunID)
		if err != nil {
			return nil, err
		}
		t.projectID, t.read, t.write = run.ProjectID, rbac.RunRead, rbac.RunExecute
	default:
		e, err := s.Store.GetExecution(ctx, p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		run, err := s.Store.GetRun(ctx, p.TenantID(), e.RunID)
		if err != nil {
			return nil, err
		}
		t.projectID, t.read, t.write = run.ProjectID, rbac.RunRead, rbac.RunExecute
	}
	return t, nil
}

// targetFor loads the target and checks perm ("read" or "write").
func (s *Server) targetFor(w http.ResponseWriter, r *http.Request, p *Principal, kind string, write bool) (*attachTarget, bool) {
	t, err := s.target(r.Context(), p, kind, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	perm := t.read
	if write {
		perm = t.write
	}
	if !p.Can(perm) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(perm)+" required")
		return nil, false
	}
	return t, true
}

func (s *Server) handleListAttachments(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		t, ok := s.targetFor(w, r, p, kind, false)
		if !ok {
			return
		}
		fs, err := s.Store.ListAttachments(r.Context(), p.TenantID(), t.entity, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(fs))
	}
}

func (s *Server) handleUploadAttachment(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		t, ok := s.targetFor(w, r, p, kind, true)
		if !ok {
			return
		}
		u, err := readUpload(w, r, "file", s.maxUpload())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		ctx := r.Context()
		f := &model.FileInfo{ProjectID: &t.projectID, Kind: model.FileAttachment, Name: u.name, ContentType: u.contentType,
			Description: u.description, Tags: u.tags, UploadedBy: p.User.Email}
		if err := s.Store.CreateFile(ctx, p.TenantID(), f, u.data); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.Attach(ctx, f.ID, t.entity, r.PathValue("id")); err != nil {
			_ = s.Store.DeleteFile(ctx, p.TenantID(), f.ID)
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "attachment.add", t.entity, r.PathValue("id"), f.Name)
		out, err := s.Store.GetFile(ctx, p.TenantID(), f.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

func (s *Server) handleLinkAttachment(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		t, ok := s.targetFor(w, r, p, kind, true)
		if !ok {
			return
		}
		var req struct {
			FileID string `json:"file_id"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		ctx := r.Context()
		f, err := s.Store.GetFile(ctx, p.TenantID(), req.FileID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if f.ProjectID == nil || *f.ProjectID != t.projectID {
			s.fail(w, r, badRequest("the file belongs to another project"))
			return
		}
		if err := s.Store.Attach(ctx, f.ID, t.entity, r.PathValue("id")); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "attachment.link", t.entity, r.PathValue("id"), f.Name)
		out, err := s.Store.GetFile(ctx, p.TenantID(), f.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) handleUnlinkAttachment(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		t, ok := s.targetFor(w, r, p, kind, true)
		if !ok {
			return
		}
		if err := s.Store.Detach(r.Context(), p.TenantID(), r.PathValue("file"), t.entity, r.PathValue("id")); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, "attachment.remove", t.entity, r.PathValue("id"), r.PathValue("file"))
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---------------------------------------------------------------- archive

var documentSchema = &query.Schema{
	Fields:  []string{"name", "description", "tag", "kind", "type", "uploaded_by", "created_at", "size", "linked", "id"},
	Aliases: map[string]string{"tags": "tag", "content_type": "type", "by": "uploaded_by"},
}

func fileRecord(f *model.FileInfo) query.MapRecord {
	var linked []string
	for _, l := range f.Links {
		linked = append(linked, l.Key)
	}
	return query.MapRecord{"id": {f.ID}, "name": {f.Name}, "description": {f.Description}, "tag": f.Tags, "kind": {f.Kind},
		"type": {f.ContentType}, "uploaded_by": {f.UploadedBy}, "created_at": {ts(f.CreatedAt)},
		"size": {strconv.FormatInt(f.Size, 10)}, "linked": linked}
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind != "" && !model.Valid([]string{model.FileAttachment, model.FileDocument, model.FileReport}, kind) {
		s.fail(w, r, badRequest("kind must be attachment, document or report"))
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), documentSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	fs, err := s.Store.ListFiles(r.Context(), p.TenantID(), pr.ID, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.FileInfo{}
	for i := range fs {
		if q.Match(fileRecord(&fs[i])) {
			out = append(out, fs[i])
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	if !p.Can(rbac.RequirementWr) && !p.Can(rbac.TestWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission requirement.write or test.write required")
		return
	}
	u, err := readUpload(w, r, "file", s.maxUpload())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := &model.FileInfo{ProjectID: &pr.ID, Kind: model.FileDocument, Name: u.name, ContentType: u.contentType,
		Description: u.description, Tags: u.tags, UploadedBy: p.User.Email}
	if err := s.Store.CreateFile(r.Context(), p.TenantID(), f, u.data); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "document.upload", "file", f.ID, f.Name)
	writeJSON(w, http.StatusCreated, f)
}

// ---------------------------------------------------------------- avatars and icons

var presetRe = regexp.MustCompile(`^preset:[a-z0-9-]{1,32}$`)

// validPicture reports whether v is "", "preset:<name>" or "file:<id>".
func validPreset(v string) bool { return v == "" || presetRe.MatchString(v) }

func readImage(w http.ResponseWriter, r *http.Request) (*upload, error) {
	u, err := readUpload(w, r, "file", MaxImageBytes)
	if err != nil {
		return nil, err
	}
	// trust the content, not the declared type
	sniffed, _, _ := mime.ParseMediaType(http.DetectContentType(u.data))
	if !imageTypes[sniffed] {
		return nil, badRequest("image must be PNG, JPEG, GIF or WebP")
	}
	u.contentType = sniffed
	return u, nil
}

func (s *Server) handleUploadAvatar(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Token != nil {
		writeError(w, http.StatusForbidden, "forbidden", "the profile cannot be changed with an API token")
		return
	}
	u, err := readImage(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	f := &model.FileInfo{Kind: model.FileAvatar, Name: u.name, ContentType: u.contentType, UploadedBy: p.User.Email}
	if err := s.Store.CreateFile(ctx, "", f, u.data); err != nil {
		s.fail(w, r, err)
		return
	}
	old := p.User.Avatar
	p.User.Avatar = "file:" + f.ID
	if err := s.Store.UpdateUser(ctx, p.User); err != nil {
		s.fail(w, r, err)
		return
	}
	s.dropOldPicture(ctx, "", old)
	s.audit(ctx, p, "profile.avatar", "user", p.User.ID, "")
	s.handleMe(w, r, p)
}

// dropOldPicture deletes a replaced uploaded avatar or icon.
func (s *Server) dropOldPicture(ctx context.Context, tenantID, old string) {
	if id, ok := strings.CutPrefix(old, "file:"); ok {
		_ = s.Store.DeleteFile(ctx, tenantID, id)
	}
}

func (s *Server) handleAvatar(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, err := s.Store.GetFile(r.Context(), "", r.PathValue("id"))
	if err != nil || f.Kind != model.FileAvatar {
		writeError(w, http.StatusNotFound, "not_found", "avatar not found")
		return
	}
	s.serveFile(w, r, f, true)
}

func (s *Server) handleUploadIcon(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	u, err := readImage(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	f := &model.FileInfo{ProjectID: &pr.ID, Kind: model.FileIcon, Name: u.name, ContentType: u.contentType, UploadedBy: p.User.Email}
	if err := s.Store.CreateFile(ctx, p.TenantID(), f, u.data); err != nil {
		s.fail(w, r, err)
		return
	}
	old := pr.Icon
	pr.Icon = "file:" + f.ID
	if err := s.Store.UpdateProject(ctx, p.TenantID(), pr); err != nil {
		s.fail(w, r, err)
		return
	}
	s.dropOldPicture(ctx, p.TenantID(), old)
	s.audit(ctx, p, "project.icon", "project", pr.ID, "")
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) handleListLogins(w http.ResponseWriter, r *http.Request, p *Principal) {
	es, err := s.Store.ListLogins(r.Context(), p.User.ID, 50)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(es))
}
