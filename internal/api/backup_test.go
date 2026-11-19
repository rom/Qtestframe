package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// getBytes performs a GET and returns the response and body.
func (c *client) getBytes(path string, want int) (*http.Response, []byte) {
	c.e.t.Helper()
	res := c.raw("GET", path, nil)
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("GET %s: status %d, want %d: %s", path, res.StatusCode, want, data)
	}
	return res, data
}

// restoreUpload posts a multipart restore request.
func (c *client) restoreUpload(archive []byte, confirm string, want int) (*http.Response, map[string]any) {
	c.e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if confirm != "" {
		mw.WriteField("confirm", confirm)
	}
	if archive != nil {
		fw, _ := mw.CreateFormFile("backup", "backup.zip")
		fw.Write(archive)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", c.e.hs.URL+"/api/v1/admin/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("restore: status %d, want %d: %s", res.StatusCode, want, data)
	}
	out := map[string]any{}
	json.Unmarshal(data, &out)
	return res, out
}

func editManifest(t *testing.T, b []byte, fn func(m map[string]any)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "manifest.json" {
			var m map[string]any
			json.Unmarshal(data, &m)
			fn(m)
			data, _ = json.Marshal(m)
		}
		w, _ := zw.Create(f.Name)
		w.Write(data)
	}
	zw.Close()
	return out.Bytes()
}

func TestBackupAndRestoreAPI(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	pid := a.project("BAK")
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Keep me"}, 201)
	f := &model.FileInfo{ProjectID: &pid, Kind: model.FileDocument, Name: "doc.bin", UploadedBy: "admin@example.com"}
	blob := []byte{0, 1, 2, 255, 254, '\n', '"'}
	if err := e.st.CreateFile(ctx, e.tenant.ID, f, blob); err != nil {
		t.Fatal(err)
	}

	res, archive := a.getBytes("/api/v1/admin/backup", 200)
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatal(ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="qtestframe-backup-`) {
		t.Fatal(cd)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	var m store.BackupManifest
	for _, zf := range zr.File {
		if zf.Name == "manifest.json" {
			rc, _ := zf.Open()
			json.NewDecoder(rc).Decode(&m)
			rc.Close()
		}
	}
	if m.FormatVersion != 1 || m.AppVersion != Version || m.Tables["projects"] != 1 || m.Tables["file_blobs"] != 1 {
		t.Fatalf("%+v", m)
	}

	if acts := auditActions(t, e); !acts["admin.backup"] {
		t.Fatal(acts)
	}
	// changes after the backup are undone by the restore
	a.project("GONE")
	// permission and validation errors
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.getBytes("/api/v1/admin/backup", 403)
	v.restoreUpload(archive, "RESTORE", 403)
	a.restoreUpload(archive, "", 400)
	a.restoreUpload(archive, "yes", 400)
	a.restoreUpload(nil, "RESTORE", 400)
	a.restoreUpload([]byte("not a zip"), "RESTORE", 400)
	_, msg := a.restoreUpload(editManifest(t, archive, func(m map[string]any) { m["schema_version"] = 1 }), "RESTORE", 409)
	if msg["code"] != "conflict" || !strings.Contains(msg["error"].(string), "schema version 1 does not match") {
		t.Fatal(msg)
	}
	a.m("POST", "/api/v1/admin/restore", map[string]any{"confirm": "RESTORE"}, 400) // not multipart
	// a super admin's API token is not enough
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "ci", "role": "admin"}, 201)
	bt := &client{e: e, hc: http.DefaultClient, bearer: tok["token"].(string)}
	bt.getBytes("/api/v1/admin/backup", 403)
	bt.restoreUpload(archive, "RESTORE", 403)
	if ps, _ := e.st.ListProjects(ctx, e.tenant.ID); len(ps) != 2 {
		t.Fatal("failed restores must not change data", len(ps))
	}

	res, out := a.restoreUpload(archive, "RESTORE", 200)
	restored := out["restored"].(map[string]any)
	if restored["projects"].(float64) != 1 || restored["users"].(float64) != 1 || restored["file_blobs"].(float64) != 1 {
		t.Fatal(restored)
	}
	cleared := false
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("session cookie not cleared")
	}
	ps, _ := e.st.ListProjects(ctx, e.tenant.ID)
	if len(ps) != 1 || ps[0].Key != "BAK" {
		t.Fatal(ps)
	}
	if data, err := e.st.FileData(ctx, f.ID); err != nil || !bytes.Equal(data, blob) {
		t.Fatal(data, err)
	}
	// everyone is signed out
	a.m("GET", "/api/v1/auth/me", nil, 401)
	// the viewer did not exist at backup time
	e.anon().m("POST", "/api/v1/auth/login", map[string]string{"email": "viewer@example.com", "password": testPassword}, 401)
	// the restore is audited into the restored data (the backup's own audit
	// entry was written after the snapshot)
	if acts := auditActions(t, e); !acts["admin.restore"] || acts["admin.backup"] {
		t.Fatal(acts)
	}
	// signing in again works
	a2 := e.login("admin@example.com")
	a2.getBytes("/api/v1/admin/backup", 200)
}

func TestBackupFailureIsLogged(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	// a table the backup does not know makes it fail
	if _, err := e.st.DB.Exec(`CREATE TABLE zz_unknown (id TEXT)`); err != nil {
		t.Fatal(err)
	}
	res := a.raw("GET", "/api/v1/admin/backup", nil)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if _, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("a failed backup must not be a valid archive")
	}
	if maxRestoreUpload() <= store.MaxRestoreSize {
		t.Fatal("upload limit")
	}
}

func auditActions(t *testing.T, e *env) map[string]bool {
	t.Helper()
	entries, err := e.st.ListAudit(context.Background(), e.tenant.ID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	acts := map[string]bool{}
	for _, en := range entries {
		acts[en.Action] = true
	}
	return acts
}
