package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// Logotypes on the first page of reports and generated documents (see
// docs/api.md "Logo"): the organisation's (settings.manage) is the
// default, a project may upload its own (project.update) or turn the logo
// off. Uploads are PNG, JPEG or SVG (export.PrepareLogo: decoded and
// encoded again; an SVG is rasterised by Qtestframe's own renderer, which
// neither runs scripts nor fetches anything) and stored as files of kind
// logo.

func (s *Server) routesLogo(h routeFunc) {
	h("GET /api/v1/settings/logo", s.handleGetOrgLogo)
	h("PUT /api/v1/settings/logo", s.handlePutOrgLogo, "settings.manage")
	h("PUT /api/v1/settings/logo/image", s.handleUploadOrgLogo, "settings.manage")
	h("DELETE /api/v1/settings/logo/image", s.handleDeleteOrgLogo, "settings.manage")
	h("GET /api/v1/settings/logo/image", s.handleOrgLogoImage)
	h("GET /api/v1/projects/{pid}/logo", s.handleGetProjectLogo, "project.read")
	h("PUT /api/v1/projects/{pid}/logo", s.handlePutProjectLogo, "project.update")
	h("PUT /api/v1/projects/{pid}/logo/image", s.handleUploadProjectLogo, "project.update")
	h("DELETE /api/v1/projects/{pid}/logo/image", s.handleDeleteProjectLogo, "project.update")
	h("GET /api/v1/projects/{pid}/logo/image", s.handleProjectLogoImage, "project.read")
}

// logoView is a logo's settings with the URL of its image ("" without one;
// ?v= changes with the image).
type logoView struct {
	model.LogoSettings
	URL string `json:"url"`
}

func orgLogoView(l model.LogoSettings) logoView {
	v := logoView{LogoSettings: l}
	if l.FileID != "" {
		v.URL = "/api/v1/settings/logo/image?v=" + l.FileID
	}
	return v
}

// projectLogoView: the project's own logo settings, the organisation's
// logo, and which of them its documents show (Effective: project,
// organisation or none).
type projectLogoView struct {
	Own          logoView `json:"own"`
	Organisation logoView `json:"organisation"`
	Effective    string   `json:"effective"`
}

func (s *Server) projectLogoView(ctx context.Context, p *Principal, pr *model.Project) (*projectLogoView, error) {
	own, err := s.Store.ProjectLogo(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	st, err := s.Store.TenantSettings(ctx, p.TenantID())
	if err != nil {
		return nil, err
	}
	v := &projectLogoView{Own: logoView{LogoSettings: *own}, Organisation: orgLogoView(st.Logo)}
	if own.FileID != "" {
		v.Own.URL = "/api/v1/projects/" + pr.ID + "/logo/image?v=" + own.FileID
	}
	switch {
	case own.Off:
		v.Effective = "none"
	case own.FileID != "":
		v.Effective = "project"
	case st.Logo.FileID != "":
		v.Effective = "organisation"
	default:
		v.Effective = "none"
	}
	return v, nil
}

// documentLogo returns the logo of a project's reports and documents (nil:
// none).
func (s *Server) documentLogo(ctx context.Context, p *Principal, pr *model.Project) *export.Logo {
	own, err := s.Store.ProjectLogo(ctx, p.TenantID(), pr.ID)
	if err != nil || own.Off {
		return nil
	}
	l, alt := own, pr.Name
	if own.FileID == "" {
		st, err := s.Store.TenantSettings(ctx, p.TenantID())
		if err != nil || st.Logo.FileID == "" {
			return nil
		}
		l, alt = &st.Logo, ""
		if p.Tenant != nil {
			alt = p.Tenant.Name
		}
	}
	data, err := s.Store.LogoFile(ctx, p.TenantID(), l.FileID)
	if err != nil || data == nil {
		return nil
	}
	return &export.Logo{Data: data, ContentType: l.ContentType, Width: l.Width, Height: l.Height, Placement: l.Placement,
		MaxWidthMM: float64(l.MaxWidthMM), MaxHeightMM: float64(l.MaxHeightMM), Alt: alt}
}

// logoInput is the body of PUT …/logo: fields left out keep their value.
type logoInput struct {
	Placement   *string `json:"placement"`
	MaxWidthMM  *int    `json:"max_width_mm"`
	MaxHeightMM *int    `json:"max_height_mm"`
	Off         *bool   `json:"off"`
}

func (in *logoInput) apply(l *model.LogoSettings, project bool) error {
	if in.Placement != nil {
		if !model.Valid(model.LogoPlacements, *in.Placement) {
			return badRequest("placement must be one of %s", strings.Join(model.LogoPlacements, ", "))
		}
		l.Placement = *in.Placement
	}
	if in.MaxWidthMM != nil {
		if *in.MaxWidthMM < model.MinLogoMM || *in.MaxWidthMM > model.MaxLogoWidthMM {
			return badRequest("max_width_mm must be %d to %d", model.MinLogoMM, model.MaxLogoWidthMM)
		}
		l.MaxWidthMM = *in.MaxWidthMM
	}
	if in.MaxHeightMM != nil {
		if *in.MaxHeightMM < model.MinLogoMM || *in.MaxHeightMM > model.MaxLogoHeightMM {
			return badRequest("max_height_mm must be %d to %d", model.MinLogoMM, model.MaxLogoHeightMM)
		}
		l.MaxHeightMM = *in.MaxHeightMM
	}
	if in.Off != nil {
		if !project {
			return badRequest("off applies to projects only (delete the image to remove the organisation's logo)")
		}
		l.Off = *in.Off
	}
	return nil
}

func logoDetails(l *model.LogoSettings) string {
	d := fmt.Sprintf("placement=%s max=%d×%d mm", l.Placement, l.MaxWidthMM, l.MaxHeightMM)
	if l.Off {
		d += " off"
	}
	return d
}

// readLogo reads and prepares an uploaded logo image.
func readLogo(w http.ResponseWriter, r *http.Request) (*upload, *export.PreparedLogo, error) {
	u, err := readUpload(w, r, "file", export.MaxLogoBytes)
	if err != nil {
		return nil, nil, err
	}
	img, err := export.PrepareLogo(u.name, u.data)
	if err != nil {
		if errors.Is(err, export.ErrLogo) {
			return nil, nil, badRequest("%s", strings.TrimPrefix(err.Error(), "logo: "))
		}
		return nil, nil, err
	}
	return u, img, nil
}

// storeLogo saves a prepared logo as a file and points l at it; it returns
// the id of the file it replaces ("" none).
func (s *Server) storeLogo(ctx context.Context, p *Principal, projectID *string, u *upload, img *export.PreparedLogo, l *model.LogoSettings) (string, error) {
	name := u.name
	ext := ".png"
	if img.ContentType == "image/jpeg" {
		ext = ".jpg"
	}
	stored := strings.TrimSuffix(name, path.Ext(name)) + ext
	f := &model.FileInfo{ProjectID: projectID, Kind: model.FileLogo, Name: stored, ContentType: img.ContentType,
		Description: "Logo (" + name + ")", UploadedBy: p.User.Email}
	if err := s.Store.CreateFile(ctx, p.TenantID(), f, img.Data); err != nil {
		return "", err
	}
	old := l.FileID
	l.FileID, l.ContentType, l.Width, l.Height, l.Source, l.Name = f.ID, img.ContentType, img.Width, img.Height, img.Source, name
	return old, nil
}

func uploadDetails(l *model.LogoSettings) string {
	return fmt.Sprintf("%s (%s, %d×%d px)", l.Name, l.Source, l.Width, l.Height)
}

func (s *Server) handleGetOrgLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	st, err := s.Store.TenantSettings(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, orgLogoView(st.Logo))
}

func (s *Server) handlePutOrgLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	var in logoInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.Store.TenantSettings(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := st.Logo
	if err := in.apply(&l, false); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveTenantLogo(r.Context(), p.TenantID(), &l); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "settings.logo", "tenant", p.TenantID(), logoDetails(&l))
	writeJSON(w, http.StatusOK, orgLogoView(l))
}

func (s *Server) handleUploadOrgLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	u, img, err := readLogo(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	st, err := s.Store.TenantSettings(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := st.Logo
	old, err := s.storeLogo(ctx, p, nil, u, img, &l)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveTenantLogo(ctx, p.TenantID(), &l); err != nil {
		s.fail(w, r, err)
		return
	}
	s.dropLogoFile(ctx, p, old)
	s.audit(ctx, p, "settings.logo.upload", "tenant", p.TenantID(), uploadDetails(&l))
	writeJSON(w, http.StatusOK, orgLogoView(l))
}

// dropLogoFile deletes a replaced or removed logo image.
func (s *Server) dropLogoFile(ctx context.Context, p *Principal, id string) {
	if id == "" {
		return
	}
	if f, err := s.Store.GetFile(ctx, p.TenantID(), id); err == nil && f.Kind == model.FileLogo {
		_ = s.Store.DeleteFile(ctx, p.TenantID(), id)
	}
}

func (s *Server) handleDeleteOrgLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	ctx := r.Context()
	st, err := s.Store.TenantSettings(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := st.Logo
	old := l.FileID
	l.FileID, l.ContentType, l.Width, l.Height, l.Source, l.Name = "", "", 0, 0, "", ""
	if err := s.Store.SaveTenantLogo(ctx, p.TenantID(), &l); err != nil {
		s.fail(w, r, err)
		return
	}
	s.dropLogoFile(ctx, p, old)
	s.audit(ctx, p, "settings.logo.delete", "tenant", p.TenantID(), "")
	writeJSON(w, http.StatusOK, orgLogoView(l))
}

func (s *Server) serveLogo(w http.ResponseWriter, r *http.Request, p *Principal, fileID string) {
	if fileID == "" {
		writeError(w, http.StatusNotFound, "not_found", "no logo")
		return
	}
	f, err := s.Store.GetFile(r.Context(), p.TenantID(), fileID)
	if err != nil || f.Kind != model.FileLogo {
		writeError(w, http.StatusNotFound, "not_found", "no logo")
		return
	}
	s.serveFile(w, r, f, true)
}

func (s *Server) handleOrgLogoImage(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	st, err := s.Store.TenantSettings(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.serveLogo(w, r, p, st.Logo.FileID)
}

func (s *Server) handleGetProjectLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	v, err := s.projectLogoView(r.Context(), p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handlePutProjectLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in logoInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	l, err := s.Store.ProjectLogo(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := in.apply(l, true); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveProjectLogo(ctx, p.TenantID(), pr.ID, l); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "project.logo", "project", pr.ID, logoDetails(l))
	s.respondProjectLogo(w, r, p, pr)
}

func (s *Server) respondProjectLogo(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project) {
	v, err := s.projectLogoView(r.Context(), p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleUploadProjectLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	u, img, err := readLogo(w, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	l, err := s.Store.ProjectLogo(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	old, err := s.storeLogo(ctx, p, &pr.ID, u, img, l)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l.Off = false
	if err := s.Store.SaveProjectLogo(ctx, p.TenantID(), pr.ID, l); err != nil {
		s.fail(w, r, err)
		return
	}
	s.dropLogoFile(ctx, p, old)
	s.audit(ctx, p, "project.logo.upload", "project", pr.ID, uploadDetails(l))
	s.respondProjectLogo(w, r, p, pr)
}

func (s *Server) handleDeleteProjectLogo(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	l, err := s.Store.ProjectLogo(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	old := l.FileID
	l.FileID, l.ContentType, l.Width, l.Height, l.Source, l.Name = "", "", 0, 0, "", ""
	if err := s.Store.SaveProjectLogo(ctx, p.TenantID(), pr.ID, l); err != nil {
		s.fail(w, r, err)
		return
	}
	s.dropLogoFile(ctx, p, old)
	s.audit(ctx, p, "project.logo.delete", "project", pr.ID, "")
	s.respondProjectLogo(w, r, p, pr)
}

func (s *Server) handleProjectLogoImage(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	l, err := s.Store.ProjectLogo(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.serveLogo(w, r, p, l.FileID)
}
