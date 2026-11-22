package app

import (
	"context"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// demoLogoSVG is the demo organisation's logotype: shapes only (texts are
// not drawn by the logo renderer).
const demoLogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 80">
  <rect x="4" y="4" width="72" height="72" rx="14" fill="#1f3864"/>
  <circle cx="40" cy="38" r="20" fill="none" stroke="#ffffff" stroke-width="8"/>
  <path d="M48 48 L64 66" stroke="#f2b134" stroke-width="9"/>
  <rect x="92" y="18" width="136" height="12" rx="6" fill="#1f3864"/>
  <rect x="92" y="38" width="104" height="10" rx="5" fill="#2f5496"/>
  <rect x="92" y="56" width="72" height="8" rx="4" fill="#f2b134"/>
</svg>`

// seedLogo gives the demo organisation a logo on the first page of its
// reports and generated documents (top right, at most 45 × 15 mm).
func seedLogo(ctx context.Context, st *store.Store, tenantID string) error {
	img, err := export.PrepareLogo("demo-logo.svg", []byte(demoLogoSVG))
	if err != nil {
		return err
	}
	f := &model.FileInfo{Kind: model.FileLogo, Name: "demo-logo.png", ContentType: img.ContentType, Description: "Logo (demo-logo.svg)", UploadedBy: "seed"}
	if err := st.CreateFile(ctx, tenantID, f, img.Data); err != nil {
		return err
	}
	return st.SaveTenantLogo(ctx, tenantID, &model.LogoSettings{FileID: f.ID, ContentType: img.ContentType, Width: img.Width, Height: img.Height,
		Source: img.Source, Name: "demo-logo.svg", Placement: model.LogoRight, MaxWidthMM: 45, MaxHeightMM: 15})
}
