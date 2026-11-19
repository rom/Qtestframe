package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/rbac"
)

// multipartBody builds a multipart form with one file and extra fields.
func multipartBody(t *testing.T, field, name, ctype string, data []byte, extra map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range extra {
		_ = mw.WriteField(k, v)
	}
	if field != "" {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{`form-data; name="` + field + `"; filename="` + name + `"`}
		if ctype != "" {
			h["Content-Type"] = []string{ctype}
		}
		part, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

// upload performs a multipart request and decodes the JSON response.
func (c *client) upload(method, path, field, name, ctype string, data []byte, extra map[string]string, want int) map[string]any {
	c.e.t.Helper()
	body, ct := multipartBody(c.e.t, field, name, ctype, data, extra)
	req, _ := http.NewRequest(method, c.e.hs.URL+path, body)
	req.Header.Set("Content-Type", ct)
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
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, raw)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func pngBytes(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAttachmentsAndArchive(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Files.MaxUploadMB = 1 })
	a := e.login("admin@example.com")
	pid := a.project("FIL")
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T"}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "Run", "test_ids": []string{tc["id"].(string)}}, 201)
	exid := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]["id"].(string)

	f := a.upload("POST", "/api/v1/requirements/"+rid+"/attachments", "file", "../../etc/spec.txt", "text/plain", []byte("hello spec"),
		map[string]string{"description": "Spec", "tags": "a, b,,a"}, 201)
	if f["name"] != "spec.txt" || f["size"].(float64) != 10 || f["kind"] != "attachment" || len(f["links"].([]any)) != 1 ||
		len(f["tags"].([]any)) != 2 || f["content_type"] != "text/plain" {
		t.Fatal(f)
	}
	fid := f["id"].(string)
	// content: inline only for safe types, always sandboxed
	res := a.raw("GET", "/api/v1/files/"+fid+"/content?inline=1", nil)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "hello spec" || !strings.HasPrefix(res.Header.Get("Content-Disposition"), "inline") ||
		!strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox") || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal(res.Header)
	}
	etag := res.Header.Get("ETag")
	req, _ := http.NewRequest("GET", e.hs.URL+"/api/v1/files/"+fid+"/content", nil)
	req.Header.Set("If-None-Match", etag)
	res, _ = a.hc.Do(req)
	res.Body.Close()
	if res.StatusCode != 304 {
		t.Fatal(res.StatusCode)
	}
	// HTML is never served inline
	h := a.upload("POST", "/api/v1/tests/"+tc["id"].(string)+"/attachments", "file", "x.html", "text/html", []byte("<script>alert(1)</script>"), nil, 201)
	res = a.raw("GET", "/api/v1/files/"+h["id"].(string)+"/content?inline=1", nil)
	res.Body.Close()
	if res.Header.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment") {
		t.Fatal(res.Header)
	}
	a.upload("POST", "/api/v1/runs/"+run["id"].(string)+"/attachments", "file", "log.bin", "", []byte{0, 1, 2}, nil, 201)
	a.upload("POST", "/api/v1/executions/"+exid+"/attachments", "file", "shot.png", "image/png", pngBytes(t), nil, 201)
	// errors
	a.upload("POST", "/api/v1/requirements/"+rid+"/attachments", "file", "big.bin", "", make([]byte, 2<<20), nil, 400)
	a.upload("POST", "/api/v1/requirements/"+rid+"/attachments", "file", "empty.txt", "", nil, nil, 400)
	a.upload("POST", "/api/v1/requirements/"+rid+"/attachments", "", "", "", nil, nil, 400)
	a.m("POST", "/api/v1/requirements/"+rid+"/attachments", map[string]any{}, 400)
	a.upload("POST", "/api/v1/requirements/missing/attachments", "file", "a.txt", "", []byte("x"), nil, 404)

	// link an existing file to another item; listing shows both links
	a.m("POST", "/api/v1/tests/"+tc["id"].(string)+"/attachments/link", map[string]any{"file_id": fid}, 200)
	a.m("POST", "/api/v1/tests/"+tc["id"].(string)+"/attachments/link", map[string]any{"file_id": fid}, 409)
	a.m("POST", "/api/v1/tests/"+tc["id"].(string)+"/attachments/link", map[string]any{"file_id": "missing"}, 404)
	a.m("POST", "/api/v1/tests/"+tc["id"].(string)+"/attachments/link", "{", 400)
	if l := items(a.m("GET", "/api/v1/tests/"+tc["id"].(string)+"/attachments", nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	got := a.m("GET", "/api/v1/files/"+fid, nil, 200)
	if len(got["links"].([]any)) != 2 {
		t.Fatal(got)
	}
	other := a.project("FIL2")
	otherReq := a.m("POST", "/api/v1/projects/"+other+"/requirements", map[string]any{"title": "O"}, 201)["id"].(string)
	a.m("POST", "/api/v1/requirements/"+otherReq+"/attachments/link", map[string]any{"file_id": fid}, 400)

	// archive
	doc := a.upload("POST", "/api/v1/projects/"+pid+"/documents", "file", "plan.pdf", "application/pdf", []byte("%PDF-1.4 test"), map[string]string{"tags": "plan"}, 201)
	all := items(a.m("GET", "/api/v1/projects/"+pid+"/documents", nil, 200))
	if len(all) != 5 {
		t.Fatal(len(all))
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/documents?kind=document", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/documents?q="+url.QueryEscape("linked:FIL-REQ-1 type:text/plain"), nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/documents?kind=avatar", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/documents?q=bad:1", nil, 400)
	up := a.m("PATCH", "/api/v1/files/"+doc["id"].(string), map[string]any{"name": "Plan v2.pdf", "description": "d", "tags": []string{"x"}}, 200)
	if up["name"] != "Plan v2.pdf" {
		t.Fatal(up)
	}
	a.m("PATCH", "/api/v1/files/"+doc["id"].(string), "{", 400)

	// permissions: a viewer can read but not upload or manage others' files
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/files/"+fid, nil, 200)
	v.upload("POST", "/api/v1/projects/"+pid+"/documents", "file", "v.txt", "", []byte("v"), nil, 403)
	v.upload("POST", "/api/v1/requirements/"+rid+"/attachments", "file", "v.txt", "", []byte("v"), nil, 403)
	v.m("PATCH", "/api/v1/files/"+fid, map[string]any{"name": "x"}, 403)
	v.m("DELETE", "/api/v1/files/"+fid, nil, 403)
	e.member("tester@example.com", rbac.RoleTester)
	ts := e.login("tester@example.com")
	mine := ts.upload("POST", "/api/v1/projects/"+pid+"/documents", "file", "t.txt", "", []byte("t"), nil, 201)
	ts.m("PATCH", "/api/v1/files/"+mine["id"].(string), map[string]any{"description": "own"}, 200)
	ts.call("DELETE", "/api/v1/files/"+mine["id"].(string), nil, 204, nil)
	// tenant isolation
	ctx := context.Background()
	evil, _ := e.st.CreateTenant(ctx, "evil", "Evil")
	eu := mkUser(t, e.st, "evil@example.com", false)
	_ = e.st.SetMembership(ctx, evil.ID, eu.ID, rbac.RoleAdmin)
	ev := e.login("evil@example.com")
	ev.m("GET", "/api/v1/files/"+fid, nil, 404)
	ev.m("GET", "/api/v1/files/"+fid+"/content", nil, 404)

	// unlinking the last link deletes an attachment file; deleting items cleans up
	a.call("DELETE", "/api/v1/tests/"+tc["id"].(string)+"/attachments/"+fid, nil, 204, nil)
	a.call("DELETE", "/api/v1/requirements/"+rid+"/attachments/"+fid, nil, 204, nil)
	a.m("GET", "/api/v1/files/"+fid, nil, 404)
	a.m("DELETE", "/api/v1/requirements/"+rid+"/attachments/"+fid, nil, 404)
	a.call("DELETE", "/api/v1/runs/"+run["id"].(string), nil, 204, nil)
	a.call("DELETE", "/api/v1/tests/"+tc["id"].(string), nil, 204, nil)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/documents", nil, 200)); len(l) != 1 || l[0]["kind"] != "document" {
		t.Fatal(l) // only the archive document remains
	}
	a.call("DELETE", "/api/v1/files/"+doc["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/files/"+doc["id"].(string), nil, 404)
}

func TestAvatarsAndIcons(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("AVA")
	me := a.upload("PUT", "/api/v1/auth/me/avatar", "file", "me.png", "application/octet-stream", pngBytes(t), nil, 200)
	av := me["user"].(map[string]any)["avatar"].(string)
	if !strings.HasPrefix(av, "file:") {
		t.Fatal(av)
	}
	// any signed-in user of any tenant can see avatars
	ctx := context.Background()
	other, _ := e.st.CreateTenant(ctx, "other", "Other")
	ou := mkUser(t, e.st, "o@example.com", false)
	_ = e.st.SetMembership(ctx, other.ID, ou.ID, rbac.RoleViewer)
	o := e.login("o@example.com")
	res := o.raw("GET", "/api/v1/avatars/"+strings.TrimPrefix(av, "file:"), nil)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" {
		t.Fatal(res.StatusCode, res.Header)
	}
	o.m("GET", "/api/v1/avatars/missing", nil, 404)
	a.upload("PUT", "/api/v1/auth/me/avatar", "file", "fake.png", "image/png", []byte("<svg onload=alert(1)>"), nil, 400)
	a.upload("PUT", "/api/v1/auth/me/avatar", "file", "big.png", "image/png", make([]byte, MaxImageBytes+1), nil, 400)
	// switching to a preset removes the uploaded file
	me = a.m("PATCH", "/api/v1/auth/me", map[string]any{"avatar": "preset:fox"}, 200)
	if me["user"].(map[string]any)["avatar"] != "preset:fox" {
		t.Fatal(me)
	}
	o.m("GET", "/api/v1/avatars/"+strings.TrimPrefix(av, "file:"), nil, 404)
	a.m("PATCH", "/api/v1/auth/me", map[string]any{"avatar": "file:abc"}, 400)
	a.m("PATCH", "/api/v1/auth/me", map[string]any{"avatar": "preset:Bad Name"}, 400)
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "t"}, 201)["token"].(string)
	b := e.anon()
	b.bearer = tok
	b.upload("PUT", "/api/v1/auth/me/avatar", "file", "me.png", "", pngBytes(t), nil, 403)

	pr := a.upload("PUT", "/api/v1/projects/"+pid+"/icon", "file", "icon.png", "", pngBytes(t), nil, 200)
	icon := pr["icon"].(string)
	if !strings.HasPrefix(icon, "file:") {
		t.Fatal(pr)
	}
	res = a.raw("GET", "/api/v1/files/"+strings.TrimPrefix(icon, "file:")+"/content?inline=1", nil)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" {
		t.Fatal(res.Header)
	}
	if p := a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"icon": "preset:rocket"}, 200); p["icon"] != "preset:rocket" {
		t.Fatal(p)
	}
	a.m("GET", "/api/v1/files/"+strings.TrimPrefix(icon, "file:"), nil, 404)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"icon": "rocket"}, 400)
	e.member("viewer@example.com", rbac.RoleViewer)
	e.login("viewer@example.com").upload("PUT", "/api/v1/projects/"+pid+"/icon", "file", "i.png", "", pngBytes(t), nil, 403)
}

func TestLoginInfoAndPasswordReset(t *testing.T) {
	e := newEnv(t)
	anon := e.anon()
	me := anon.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": testPassword}, 200)
	if me["first_login"] != true || me["previous_login"] != nil {
		t.Fatal(me)
	}
	anon.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "wrong-pass-1"}, 401)
	anon.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "wrong-pass-2"}, 401)
	time.Sleep(2 * time.Millisecond)
	a := e.login("admin@example.com")
	me = a.m("GET", "/api/v1/auth/me", nil, 200)
	prev, _ := me["previous_login"].(map[string]any)
	if me["first_login"] != false || prev == nil || prev["method"] != "local" || me["failed_logins_since_previous"].(float64) != 2 {
		t.Fatal(me)
	}
	if l := items(a.m("GET", "/api/v1/auth/logins", nil, 200)); len(l) != 4 || l[0]["success"] != true {
		t.Fatal(l)
	}

	// reset by a tenant admin: user only in this tenant
	ctx := context.Background()
	member := e.member("m@example.com", rbac.RoleTester)
	m := e.login("m@example.com")
	tenantAdmin := e.member("ta@example.com", rbac.RoleAdmin)
	_ = tenantAdmin
	ta := e.login("ta@example.com")
	r := ta.m("POST", "/api/v1/members/"+member.ID+"/reset-password", nil, 200)
	temp := r["temporary_password"].(string)
	m.m("GET", "/api/v1/auth/me", nil, 401) // sessions ended
	m2 := e.anon()
	me = m2.m("POST", "/api/v1/auth/login", map[string]string{"email": "m@example.com", "password": temp}, 200)
	m2.csrf = me["csrf_token"].(string)
	if me["user"].(map[string]any)["must_change_password"] != true {
		t.Fatal(me)
	}
	// everything but /auth is blocked until the password is changed
	res := m2.raw("GET", "/api/v1/projects", nil)
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != 403 || body["code"] != "password_change_required" {
		t.Fatal(res.StatusCode, body)
	}
	m2.call("POST", "/api/v1/auth/password", map[string]string{"current_password": temp, "new_password": "Fresh-pass-99"}, 204, nil)
	m2.m("GET", "/api/v1/projects", nil, 200)

	// a tenant admin cannot reset users who belong to other tenants or super admins
	other, _ := e.st.CreateTenant(ctx, "o2", "O2")
	_ = e.st.SetMembership(ctx, other.ID, member.ID, rbac.RoleViewer)
	ta.m("POST", "/api/v1/members/"+member.ID+"/reset-password", nil, 403)
	ta.m("POST", "/api/v1/members/"+e.admin.ID+"/reset-password", nil, 403)
	ta.m("POST", "/api/v1/members/missing/reset-password", nil, 404)
	// a super admin can; not for themselves; not for SSO users
	a.m("POST", "/api/v1/members/"+member.ID+"/reset-password", nil, 200)
	a.m("POST", "/api/v1/members/"+e.admin.ID+"/reset-password", nil, 400)
	sso := e.member("sso@example.com", rbac.RoleViewer)
	sso.AuthProvider, sso.ExternalID = "oidc:corp", "x"
	_ = e.st.UpdateUser(ctx, sso)
	a.m("POST", "/api/v1/members/"+sso.ID+"/reset-password", nil, 400)
	e.member("tst@example.com", rbac.RoleTester)
	e.login("tst@example.com").m("POST", "/api/v1/members/"+member.ID+"/reset-password", nil, 403)
	// super admin users endpoint can force a change too
	u := a.m("PATCH", "/api/v1/users/"+member.ID, map[string]any{"must_change_password": true}, 200)
	if u["must_change_password"] != true {
		t.Fatal(u)
	}
}

func TestAuditRangeAndCSV(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	a.project("AUD")
	today := time.Now().UTC().Format("2006-01-02")
	if l := items(a.m("GET", "/api/v1/audit?from="+today+"&to="+today, nil, 200)); len(l) == 0 {
		t.Fatal("no entries today")
	}
	if l := items(a.m("GET", "/api/v1/audit?to=2000-01-01", nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/audit?from="+url.QueryEscape(time.Now().Add(time.Hour).UTC().Format(time.RFC3339)), nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/audit?from=yesterday", nil, 400)
	a.m("GET", "/api/v1/audit?to=31/12", nil, 400)
	res := a.raw("GET", "/api/v1/audit?format=csv&q=action:project.*", nil)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/csv") || !strings.Contains(string(body), "project.create") {
		t.Fatal(res.Header, string(body))
	}
}
