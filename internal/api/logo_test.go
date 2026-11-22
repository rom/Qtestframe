package api

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

const apiLogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 40"><script>alert(1)</script><rect width="120" height="40" fill="#1f3864"/><circle cx="20" cy="20" r="12" fill="#fff" onclick="x()"/></svg>`

func TestLogoSettings(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("LOGO")

	// no logo yet: defaults
	l := a.m("GET", "/api/v1/settings/logo", nil, 200)
	if l["file_id"] != "" || l["placement"] != "left" || l["max_width_mm"] != float64(50) || l["max_height_mm"] != float64(20) || l["url"] != "" {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/settings/logo/image", nil, 404)

	// an SVG becomes a PNG; scripts are never kept
	l = a.upload("PUT", "/api/v1/settings/logo/image", "file", "acme.svg", "image/svg+xml", []byte(apiLogoSVG), nil, 200)
	if l["content_type"] != "image/png" || l["source"] != "svg" || l["name"] != "acme.svg" || l["width"] != float64(1000) || l["height"] != float64(333) {
		t.Fatal(l)
	}
	first := l["file_id"].(string)
	res, img := a.getBytes(l["url"].(string), 200)
	if res.Header.Get("Content-Type") != "image/png" || !bytes.HasPrefix(img, []byte("\x89PNG")) || bytes.Contains(img, []byte("script")) {
		t.Fatal(res.Header)
	}
	// placement and size
	l = a.m("PUT", "/api/v1/settings/logo", map[string]any{"placement": "right", "max_width_mm": 40}, 200)
	if l["placement"] != "right" || l["max_width_mm"] != float64(40) || l["max_height_mm"] != float64(20) || l["file_id"] != first {
		t.Fatal(l)
	}
	a.m("PUT", "/api/v1/settings/logo", map[string]any{"placement": "top"}, 400)
	a.m("PUT", "/api/v1/settings/logo", map[string]any{"max_width_mm": 500}, 400)
	a.m("PUT", "/api/v1/settings/logo", map[string]any{"max_height_mm": 1}, 400)
	a.m("PUT", "/api/v1/settings/logo", map[string]any{"off": true}, 400)
	// the general settings cannot change it
	st := a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "logo": map[string]any{"file_id": ""}}, 200)
	if st["logo"].(map[string]any)["file_id"] != first {
		t.Fatal(st["logo"])
	}
	// bad uploads
	a.upload("PUT", "/api/v1/settings/logo/image", "file", "x.gif", "image/gif", []byte("GIF89a....."), nil, 400)
	a.upload("PUT", "/api/v1/settings/logo/image", "file", "x.svg", "image/svg+xml", []byte(`<!DOCTYPE svg [<!ENTITY e SYSTEM "file:///etc/passwd">]><svg viewBox="0 0 1 1">&e;</svg>`), nil, 400)
	a.upload("PUT", "/api/v1/settings/logo/image", "file", "big.png", "image/png", make([]byte, 3<<20), nil, 400)
	// replacing deletes the old image
	l = a.upload("PUT", "/api/v1/settings/logo/image", "file", "acme.png", "", logoPNG(t), nil, 200)
	if l["file_id"] == first || l["source"] != "png" || l["placement"] != "right" {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/files/"+first, nil, 404)

	// a viewer sees the logo but cannot change it
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/settings/logo", nil, 200)
	v.getBytes("/api/v1/settings/logo/image", 200)
	v.m("PUT", "/api/v1/settings/logo", map[string]any{"placement": "left"}, 403)
	v.upload("PUT", "/api/v1/settings/logo/image", "file", "a.png", "", logoPNG(t), nil, 403)
	v.m("DELETE", "/api/v1/settings/logo/image", nil, 403)

	// the project uses the organisation's logo
	pl := a.m("GET", "/api/v1/projects/"+pid+"/logo", nil, 200)
	if pl["effective"] != "organisation" || pl["own"].(map[string]any)["file_id"] != "" {
		t.Fatal(pl)
	}
	gen := "/api/v1/projects/" + pid + "/reports/generate"
	_, docx := a.download(gen, map[string]any{"template": "project_status", "format": "docx"}, 200)
	parts := zipFiles(t, docx)
	if _, ok := parts["word/media/logo.png"]; !ok || !strings.Contains(parts["word/document.xml"], `<w:jc w:val="right"/>`) || !strings.Contains(parts["word/document.xml"], `descr="`) {
		t.Fatal("no organisation logo in the report")
	}
	_, pdf := a.download(gen, map[string]any{"template": "project_status", "format": "pdf"}, 200)
	if !bytes.Contains(pdf, []byte("/Subtype /Image")) {
		t.Fatal("no logo in the PDF report")
	}

	// its own logo
	pl = a.upload("PUT", "/api/v1/projects/"+pid+"/logo/image", "file", "proj.svg", "", []byte(apiLogoSVG), nil, 200)
	own := pl["own"].(map[string]any)
	if pl["effective"] != "project" || own["source"] != "svg" || !strings.HasPrefix(own["url"].(string), "/api/v1/projects/"+pid+"/logo/image") {
		t.Fatal(pl)
	}
	a.getBytes(own["url"].(string), 200)
	pl = a.m("PUT", "/api/v1/projects/"+pid+"/logo", map[string]any{"placement": "center", "max_height_mm": 30}, 200)
	if pl["own"].(map[string]any)["placement"] != "center" {
		t.Fatal(pl)
	}
	f := a.m("POST", "/api/v1/projects/"+pid+"/documents/generate", map[string]any{"kind": "requirements_spec", "format": "md"}, 201)
	md := a.fileContent(f["id"].(string))
	if !strings.HasPrefix(md, `<p align="center"><img src="data:image/png;base64,`) || !strings.Contains(md, `alt="LOGO project"`) {
		t.Fatal(md[:min(len(md), 120)])
	}
	// the logo files are not in the document archive
	for _, d := range a.m("GET", "/api/v1/projects/"+pid+"/documents", nil, 200)["items"].([]any) {
		if d.(map[string]any)["kind"] == "logo" {
			t.Fatal("logo listed as a document")
		}
	}

	// the project archive carries its own logo
	_, archive := a.getBytes("/api/v1/projects/"+pid+"/archive", 200)
	out := a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", archive, map[string]string{"key": "LOGO2", "name": "Copy"}, 201)
	np := out["project"].(map[string]any)["id"].(string)
	cl := a.m("GET", "/api/v1/projects/"+np+"/logo", nil, 200)
	cown := cl["own"].(map[string]any)
	if cl["effective"] != "project" || cown["file_id"] == own["file_id"] || cown["placement"] != "center" {
		t.Fatal(cl)
	}
	a.getBytes(cown["url"].(string), 200)

	// off: no logo at all
	pl = a.m("PUT", "/api/v1/projects/"+pid+"/logo", map[string]any{"off": true}, 200)
	if pl["effective"] != "none" {
		t.Fatal(pl)
	}
	_, docx = a.download(gen, map[string]any{"template": "project_status", "format": "docx"}, 200)
	if _, ok := zipFiles(t, docx)["word/media/logo.png"]; ok {
		t.Fatal("logo although off")
	}
	// removing the project's image: the organisation's again (after turning it on)
	a.m("PUT", "/api/v1/projects/"+pid+"/logo", map[string]any{"off": false}, 200)
	pl = a.m("DELETE", "/api/v1/projects/"+pid+"/logo/image", nil, 200)
	if pl["effective"] != "organisation" {
		t.Fatal(pl)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/logo/image", nil, 404)
	v.m("PUT", "/api/v1/projects/"+pid+"/logo", map[string]any{"off": true}, 403)
	v.m("GET", "/api/v1/projects/"+pid+"/logo", nil, 200)
	a.m("PUT", "/api/v1/projects/"+pid+"/logo", map[string]any{"placement": "nowhere"}, 400)

	// removing the organisation's logo
	l = a.m("DELETE", "/api/v1/settings/logo/image", nil, 200)
	if l["file_id"] != "" || l["placement"] != "right" {
		t.Fatal(l)
	}
	if pl := a.m("GET", "/api/v1/projects/"+pid+"/logo", nil, 200); pl["effective"] != "none" {
		t.Fatal(pl)
	}
	// audit
	if audit := jsonText(a.m("GET", "/api/v1/audit?q=action:settings.logo.upload", nil, 200)); !strings.Contains(audit, "acme.svg (svg, 1000×333 px)") {
		t.Fatal(audit)
	}
}

func logoPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
