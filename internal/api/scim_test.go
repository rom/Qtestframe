package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// scimClient calls /scim/v2 with a SCIM token.
type scimClient struct {
	e   *env
	tok string
}

func (e *env) scimToken(c *client, name string) string {
	e.t.Helper()
	res := c.m("POST", "/api/v1/scim/tokens", map[string]any{"name": name}, 201)
	tok, _ := res["token"].(string)
	if !strings.HasPrefix(tok, SCIMTokenPrefix) {
		e.t.Fatalf("token %q", tok)
	}
	return tok
}

func (e *env) scim(tok string) *scimClient { return &scimClient{e: e, tok: tok} }

func (sc *scimClient) do(method, path string, body any) (*http.Response, map[string]any) {
	sc.e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		buf, _ := json.Marshal(b)
		rd = strings.NewReader(string(buf))
	}
	req, err := http.NewRequest(method, sc.e.hs.URL+"/scim/v2"+path, rd)
	if err != nil {
		sc.e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", scimContentType)
	if sc.tok != "" {
		req.Header.Set("Authorization", "Bearer "+sc.tok)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		sc.e.t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			sc.e.t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
	}
	return res, out
}

func (sc *scimClient) want(method, path string, body any, status int) map[string]any {
	sc.e.t.Helper()
	res, out := sc.do(method, path, body)
	if res.StatusCode != status {
		sc.e.t.Fatalf("%s %s: status %d, want %d: %v", method, path, res.StatusCode, status, out)
	}
	if status != http.StatusNoContent && !strings.HasPrefix(res.Header.Get("Content-Type"), scimContentType) {
		sc.e.t.Fatalf("%s %s: content type %q", method, path, res.Header.Get("Content-Type"))
	}
	return out
}

// wantErr checks a SCIM error response.
func (sc *scimClient) wantErr(method, path string, body any, status int, scimType string) map[string]any {
	sc.e.t.Helper()
	out := sc.want(method, path, body, status)
	if out["status"] != fmt.Sprint(status) || out["scimType"] != nilIfEmpty(scimType) || out["detail"] == "" {
		sc.e.t.Fatalf("%s %s: error body %v (want scimType %q)", method, path, out, scimType)
	}
	if s, _ := out["schemas"].([]any); len(s) != 1 || s[0] != scimErrorSchema {
		sc.e.t.Fatalf("error schemas %v", out["schemas"])
	}
	return out
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func entraUser(userName, externalID string) map[string]any {
	return map[string]any{
		"schemas":     []string{scimUserSchema, "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"},
		"externalId":  externalID,
		"userName":    userName,
		"active":      true,
		"displayName": strings.Split(userName, "@")[0] + " Display",
		"emails":      []any{map[string]any{"primary": true, "type": "work", "value": userName}},
		"meta":        map[string]any{"resourceType": "User"},
		"name":        map[string]any{"formatted": "F L", "familyName": "Lastname", "givenName": "Firstname"},
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": map[string]any{"department": "QA"},
		"title": "Tester",
	}
}

func patch(ops ...map[string]any) map[string]any {
	return map[string]any{"schemas": []string{scimPatchSchema}, "Operations": ops}
}

func listResources(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	var res []map[string]any
	for _, r := range out["Resources"].([]any) {
		res = append(res, r.(map[string]any))
	}
	return res
}

func (e *env) roleOf(userID string) string {
	m, err := e.st.GetMembership(context.Background(), e.tenant.ID, userID)
	if err != nil {
		return ""
	}
	return m.Role
}

func TestSCIMDiscovery(t *testing.T) {
	e := newEnv(t)
	sc := e.scim(e.scimToken(e.login("admin@example.com"), "Entra ID"))
	spc := sc.want("GET", "/ServiceProviderConfig", nil, 200)
	if spc["patch"].(map[string]any)["supported"] != true || spc["bulk"].(map[string]any)["supported"] != false ||
		spc["filter"].(map[string]any)["maxResults"] != float64(scimMaxResults) {
		t.Fatalf("spc %v", spc)
	}
	rts := sc.want("GET", "/ResourceTypes", nil, 200)
	if rts["totalResults"] != float64(2) {
		t.Fatalf("resource types %v", rts)
	}
	u := sc.want("GET", "/ResourceTypes/User", nil, 200)
	if u["endpoint"] != "/Users" || u["meta"].(map[string]any)["location"] != "http://example.test/scim/v2/ResourceTypes/User" {
		t.Fatalf("user type %v", u)
	}
	sc.wantErr("GET", "/ResourceTypes/Device", nil, 404, "")
	schemas := sc.want("GET", "/Schemas", nil, 200)
	if schemas["totalResults"] != float64(2) {
		t.Fatalf("schemas %v", schemas)
	}
	g := sc.want("GET", "/Schemas/"+scimGroupSchema, nil, 200)
	if g["name"] != "Group" || len(g["attributes"].([]any)) != 3 {
		t.Fatalf("group schema %v", g)
	}
	sc.wantErr("GET", "/Schemas/urn:x", nil, 404, "")
	sc.wantErr("GET", "/Me", nil, 501, "")
	sc.wantErr("POST", "/Bulk", map[string]any{}, 501, "")
	sc.wantErr("GET", "/Devices", nil, 404, "")
}

func TestSCIMAuthentication(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin@example.com")
	tok := e.scimToken(admin, "Okta")

	// no token, a wrong token, an API token: 401 with a challenge
	for _, bad := range []string{"", SCIMTokenPrefix + "nope", "garbage"} {
		res, out := e.scim(bad).do("GET", "/Users", nil)
		if res.StatusCode != 401 || res.Header.Get("WWW-Authenticate") == "" || out["status"] != "401" {
			t.Fatalf("token %q: %d %v", bad, res.StatusCode, out)
		}
	}
	api := admin.m("POST", "/api/v1/tokens", map[string]any{"name": "ci", "role": "viewer"}, 201)["token"].(string)
	if res, _ := e.scim(api).do("GET", "/Users", nil); res.StatusCode != 401 {
		t.Fatalf("API token accepted by SCIM: %d", res.StatusCode)
	}
	// the SCIM token does not work for the normal API
	bc := e.anon()
	bc.bearer = tok
	bc.call("GET", "/api/v1/projects", nil, 401, nil)
	bc.call("GET", "/api/v1/scim", nil, 401, nil)

	// listing never returns the secret; last_used is recorded
	e.scim(tok).want("GET", "/Users", nil, 200)
	ov := admin.m("GET", "/api/v1/scim", nil, 200)
	toks := ov["tokens"].([]any)
	if len(toks) != 1 || strings.Contains(fmt.Sprint(ov), tok) {
		t.Fatalf("overview %v", ov)
	}
	t0 := toks[0].(map[string]any)
	if t0["last_used"] == nil || t0["name"] != "Okta" || t0["created_by"] != "admin@example.com" || !strings.HasPrefix(tok, t0["prefix"].(string)) {
		t.Fatalf("token %v", t0)
	}
	if ov["base_url"] != "http://example.test/scim/v2" || ov["default_role"] != "viewer" {
		t.Fatalf("overview %v", ov)
	}

	// expired tokens are refused
	exp := admin.m("POST", "/api/v1/scim/tokens", map[string]any{"name": "short", "expires_days": 1}, 201)
	e.st.SetClock(func() time.Time { return time.Now().UTC().Add(48 * time.Hour) })
	if res, _ := e.scim(exp["token"].(string)).do("GET", "/Users", nil); res.StatusCode != 401 {
		t.Fatalf("expired token: %d", res.StatusCode)
	}
	e.st.SetClock(func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) })
	admin.call("POST", "/api/v1/scim/tokens", map[string]any{"name": " "}, 400, nil)
	admin.call("POST", "/api/v1/scim/tokens", map[string]any{"name": "x", "expires_days": -1}, 400, nil)

	// revoked tokens are refused at once
	admin.call("DELETE", "/api/v1/scim/tokens/"+t0["id"].(string), nil, 204, nil)
	admin.call("DELETE", "/api/v1/scim/tokens/"+t0["id"].(string), nil, 404, nil)
	if res, _ := e.scim(tok).do("GET", "/Users", nil); res.StatusCode != 401 {
		t.Fatalf("revoked token: %d", res.StatusCode)
	}

	// only tenant admins manage SCIM; API tokens never create SCIM tokens
	e.member("lead@example.com", rbac.RoleManager)
	e.login("lead@example.com").call("GET", "/api/v1/scim", nil, 403, nil)
	ac := e.anon()
	ac.bearer = admin.m("POST", "/api/v1/tokens", map[string]any{"name": "adm", "role": "admin"}, 201)["token"].(string)
	ac.call("POST", "/api/v1/scim/tokens", map[string]any{"name": "x"}, 403, nil)
}

func TestSCIMRateLimits(t *testing.T) {
	e := newEnv(t)
	tok := e.scimToken(e.login("admin@example.com"), "Entra")
	e.srv.scimLimiter = newRateLimiter(2, time.Minute)
	sc := e.scim(tok)
	sc.want("GET", "/Users", nil, 200)
	sc.want("GET", "/Groups", nil, 200)
	res, _ := sc.do("GET", "/Users", nil)
	if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d", res.StatusCode)
	}
	// failed authentications lock out the client address
	e.srv.scimLimiter = newRateLimiter(100, time.Minute)
	e.srv.scimAuthLimiter = newRateLimiter(2, time.Minute)
	e.scim("bad").want("GET", "/Users", nil, 401)
	e.scim("bad").want("GET", "/Users", nil, 401)
	sc.wantErr("GET", "/Users", nil, 429, "")
}

func TestSCIMUserLifecycleEntra(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin@example.com")
	sc := e.scim(e.scimToken(admin, "Entra ID"))
	ctx := context.Background()

	// Entra ID first looks the user up, then creates it
	if out := sc.want("GET", "/Users?filter="+url.QueryEscape(`userName eq "Ada@Example.com"`), nil, 200); out["totalResults"] != float64(0) {
		t.Fatalf("empty list %v", out)
	}
	res, created := sc.do("POST", "/Users", entraUser("Ada@Example.com", "ext-ada"))
	if res.StatusCode != 201 {
		t.Fatalf("create: %d %v", res.StatusCode, created)
	}
	id := created["id"].(string)
	if res.Header.Get("Location") != "http://example.test/scim/v2/Users/"+id || res.Header.Get("ETag") == "" {
		t.Fatalf("headers %v", res.Header)
	}
	meta := created["meta"].(map[string]any)
	if created["userName"] != "Ada@Example.com" || created["externalId"] != "ext-ada" || created["active"] != true ||
		meta["resourceType"] != "User" || meta["version"] == "" || meta["created"] == "" ||
		created["name"].(map[string]any)["givenName"] != "Firstname" || created["displayName"] != "Ada Display" {
		t.Fatalf("created %v", created)
	}
	u, err := e.st.GetUser(ctx, id)
	if err != nil || u.Email != "ada@example.com" || u.AuthProvider != scimAuthProvider || u.PasswordHash != "" || u.SuperAdmin {
		t.Fatalf("user %+v %v", u, err)
	}
	if e.roleOf(id) != rbac.RoleViewer {
		t.Fatalf("role %q", e.roleOf(id))
	}
	// no local password: signing in locally fails
	e.anon().call("POST", "/api/v1/auth/login", map[string]string{"email": "ada@example.com", "password": ""}, 401, nil)

	// lookups Entra ID and Okta send
	for _, f := range []string{
		`userName eq "ada@example.com"`,
		`externalId eq "ext-ada"`,
		`emails[type eq "work"].value eq "ADA@example.com"`,
		`active eq true and name.givenName sw "first"`,
		`urn:ietf:params:scim:schemas:core:2.0:User:userName eq "ada@example.com"`,
	} {
		out := sc.want("GET", "/Users?filter="+url.QueryEscape(f), nil, 200)
		if out["totalResults"] != float64(1) || listResources(t, out)[0]["id"] != id {
			t.Fatalf("filter %s: %v", f, out)
		}
	}
	if out := sc.want("GET", "/Users?filter="+url.QueryEscape(`externalId eq "EXT-ADA"`), nil, 200); out["totalResults"] != float64(0) {
		t.Fatalf("externalId is case exact: %v", out)
	}
	sc.wantErr("GET", "/Users?filter="+url.QueryEscape(`userName equals "x"`), nil, 400, "invalidFilter")
	sc.wantErr("POST", "/Users", entraUser("ada@example.com", "other"), 409, "uniqueness")

	// sessions and API tokens of the user in this tenant
	sess := &model.Session{ID: auth.HashToken("s1"), UserID: id, TenantID: e.tenant.ID, CSRF: "c", ExpiresAt: time.Now().Add(time.Hour)}
	if err := e.st.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := e.st.CreateToken(ctx, &model.APIToken{TenantID: e.tenant.ID, UserID: id, Name: "t", Role: "viewer", Hash: auth.HashToken("qtf_x"), Prefix: "qtf_x"}); err != nil {
		t.Fatal(err)
	}

	// Entra ID deactivates with a string boolean
	out := sc.want("PATCH", "/Users/"+id, patch(map[string]any{"op": "Replace", "path": "active", "value": "False"}), 200)
	if out["active"] != false || e.roleOf(id) != "" {
		t.Fatalf("deactivated %v role %q", out, e.roleOf(id))
	}
	if _, err := e.st.GetSession(ctx, sess.ID); err == nil {
		t.Fatal("session survived deactivation")
	}
	if ts, _ := e.st.ListTokens(ctx, e.tenant.ID, id); len(ts) != 0 {
		t.Fatalf("tokens survived deactivation: %v", ts)
	}
	if out["meta"].(map[string]any)["version"] == meta["version"] {
		t.Fatal("version unchanged")
	}
	// reactivation restores the membership
	sc.want("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "value": map[string]any{"active": true}}), 200)
	if e.roleOf(id) != rbac.RoleViewer {
		t.Fatalf("reactivated role %q", e.roleOf(id))
	}

	// path-less replace with dotted keys, as Entra ID sends it
	out = sc.want("PATCH", "/Users/"+id, patch(map[string]any{"op": "Replace", "value": map[string]any{
		"displayName":                  "Ada Lovelace",
		"name.givenName":               "Ada",
		"name.familyName":              "Lovelace",
		`emails[type eq "work"].value`: "ada.lovelace@example.com",
		"externalId":                   "ext-ada-2",
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department": "R&D",
	}}), 200)
	if out["displayName"] != "Ada Lovelace" || out["externalId"] != "ext-ada-2" ||
		out["emails"].([]any)[0].(map[string]any)["value"] != "ada.lovelace@example.com" ||
		out["name"].(map[string]any)["familyName"] != "Lovelace" {
		t.Fatalf("patched %v", out)
	}
	if u, _ := e.st.GetUser(ctx, id); u.Email != "ada.lovelace@example.com" || u.DisplayName != "Ada Lovelace" {
		t.Fatalf("user %+v", u)
	}
	// Okta-style ops with paths, add, remove
	out = sc.want("PATCH", "/Users/"+id, patch(
		map[string]any{"op": "replace", "path": "userName", "value": "ada.lovelace@example.com"},
		map[string]any{"op": "add", "path": "name", "value": map[string]any{"givenName": "A.", "familyName": "L."}},
		map[string]any{"op": "remove", "path": "externalId"},
		map[string]any{"op": "add", "path": "title", "value": "Countess"},
		map[string]any{"op": "replace", "path": `emails[type eq "work"]`, "value": map[string]any{"value": "ada.lovelace@example.com", "type": "work"}},
	), 200)
	if out["userName"] != "ada.lovelace@example.com" || out["externalId"] != nil || out["name"].(map[string]any)["givenName"] != "A." {
		t.Fatalf("patched %v", out)
	}
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "remove", "path": "userName"}), 400, "mutability")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "move", "path": "userName"}), 400, "invalidSyntax")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": "active", "value": "maybe"}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": "emails[type eq", "value": "x"}), 400, "invalidPath")
	sc.wantErr("PATCH", "/Users/"+id, patch(), 400, "invalidValue")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "remove"}), 400, "noTarget")
	sc.wantErr("PATCH", "/Users/missing", patch(map[string]any{"op": "replace", "path": "active", "value": false}), 404, "")
	sc.wantErr("PATCH", "/Users/"+id, "{", 400, "invalidSyntax")

	// PUT replaces the resource (Okta deactivates this way)
	put := entraUser("ada.lovelace@example.com", "ext-3")
	put["active"] = false
	out = sc.want("PUT", "/Users/"+id, put, 200)
	if out["active"] != false || out["externalId"] != "ext-3" || e.roleOf(id) != "" {
		t.Fatalf("put %v", out)
	}
	put["active"] = true
	sc.want("PUT", "/Users/"+id, put, 200)

	got := sc.want("GET", "/Users/"+id+"?attributes=userName", nil, 200)
	if got["userName"] == nil || got["displayName"] != nil || got["id"] != id {
		t.Fatalf("attributes %v", got)
	}

	// the audit log names the token
	evs := admin.m("GET", "/api/v1/scim/events", nil, 200)
	actions := map[string]bool{}
	for _, ev := range items(evs) {
		if ev["user_email"] != "SCIM: Entra ID" && !strings.HasPrefix(ev["action"].(string), "scim.token") {
			t.Fatalf("actor %v", ev)
		}
		actions[ev["action"].(string)] = true
	}
	for _, a := range []string{"scim.user.create", "scim.user.update", "scim.user.deactivate", "scim.user.reactivate", "scim.token.create"} {
		if !actions[a] {
			t.Fatalf("no %s in %v", a, actions)
		}
	}

	// delete removes the membership and the provisioning record
	sc.want("DELETE", "/Users/"+id, nil, 204)
	sc.wantErr("GET", "/Users/"+id, nil, 404, "")
	sc.wantErr("DELETE", "/Users/"+id, nil, 404, "")
	if e.roleOf(id) != "" {
		t.Fatal("membership survived delete")
	}
	if _, err := e.st.GetUser(ctx, id); err != nil {
		t.Fatal("the account itself stays", err)
	}
}

func TestSCIMCreateValidation(t *testing.T) {
	e := newEnv(t)
	sc := e.scim(e.scimToken(e.login("admin@example.com"), "Okta"))
	sc.wantErr("POST", "/Users", map[string]any{"emails": []any{map[string]any{"value": "a@b.c"}}}, 400, "invalidValue")
	sc.wantErr("POST", "/Users", map[string]any{"userName": "jdoe"}, 400, "invalidValue")
	sc.wantErr("POST", "/Users", "not json", 400, "invalidSyntax")
	sc.wantErr("POST", "/Users", "", 400, "invalidSyntax")
	// a userName that is no e-mail address, with the e-mail in emails
	out := sc.want("POST", "/Users", map[string]any{"userName": "jdoe", "active": "true",
		"emails": []any{map[string]any{"value": "home@example.com", "type": "home"}, map[string]any{"value": "jdoe@example.com", "type": "work"}}}, 201)
	if out["userName"] != "jdoe" || out["emails"].([]any)[0].(map[string]any)["value"] != "jdoe@example.com" {
		t.Fatalf("created %v", out)
	}
	sc.wantErr("POST", "/Users", map[string]any{"userName": "JDOE", "emails": []any{map[string]any{"value": "other@example.com"}}}, 409, "uniqueness")
	// e-mail taken by another user
	e.member("taken@example.com", rbac.RoleTester)
	sc.wantErr("PATCH", "/Users/"+out["id"].(string), patch(map[string]any{"op": "replace", "path": "emails", "value": []any{map[string]any{"value": "taken@example.com", "primary": true}}}), 409, "uniqueness")
}

func TestSCIMPagination(t *testing.T) {
	e := newEnv(t)
	sc := e.scim(e.scimToken(e.login("admin@example.com"), "Okta"))
	for i := range 5 {
		sc.want("POST", "/Users", map[string]any{"userName": fmt.Sprintf("u%d@example.com", i)}, 201)
	}
	out := sc.want("GET", "/Users?startIndex=2&count=2", nil, 200)
	rs := listResources(t, out)
	if out["totalResults"] != float64(5) || out["startIndex"] != float64(2) || out["itemsPerPage"] != float64(2) ||
		rs[0]["userName"] != "u1@example.com" || rs[1]["userName"] != "u2@example.com" {
		t.Fatalf("page %v", out)
	}
	if out := sc.want("GET", "/Users?startIndex=5&count=10", nil, 200); out["itemsPerPage"] != float64(1) {
		t.Fatalf("last page %v", out)
	}
	if out := sc.want("GET", "/Users?count=0", nil, 200); out["totalResults"] != float64(5) || out["itemsPerPage"] != float64(0) {
		t.Fatalf("count=0 %v", out)
	}
	if out := sc.want("GET", "/Users?startIndex=0&count=99999", nil, 200); out["startIndex"] != float64(1) || out["itemsPerPage"] != float64(5) {
		t.Fatalf("clamped %v", out)
	}
	if out := sc.want("GET", "/Users?startIndex=9", nil, 200); out["itemsPerPage"] != float64(0) {
		t.Fatalf("beyond %v", out)
	}
	sc.wantErr("GET", "/Users?count=x", nil, 400, "invalidValue")
	sc.wantErr("GET", "/Users?startIndex=x", nil, 400, "invalidValue")
	ex := sc.want("GET", "/Users?excludedAttributes=groups,emails", nil, 200)
	if r := listResources(t, ex)[0]; r["groups"] != nil || r["emails"] != nil || r["userName"] == nil {
		t.Fatalf("excluded %v", r)
	}
}

func TestSCIMGroupsAndRoles(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin@example.com")
	sc := e.scim(e.scimToken(admin, "Okta"))
	a := sc.want("POST", "/Users", map[string]any{"userName": "a@example.com"}, 201)["id"].(string)
	b := sc.want("POST", "/Users", map[string]any{"userName": "b@example.com"}, 201)["id"].(string)
	manual := e.member("manual@example.com", rbac.RoleTester)

	res, g := sc.do("POST", "/Groups", map[string]any{"schemas": []string{scimGroupSchema}, "displayName": "QA Leads", "externalId": "g1",
		"members": []any{map[string]any{"value": a}}})
	if res.StatusCode != 201 || res.Header.Get("Location") == "" {
		t.Fatalf("group %d %v", res.StatusCode, g)
	}
	leads := g["id"].(string)
	if ms := g["members"].([]any); len(ms) != 1 || ms[0].(map[string]any)["display"] != "a@example.com" {
		t.Fatalf("members %v", g)
	}
	sc.wantErr("POST", "/Groups", map[string]any{"displayName": "QA Leads"}, 409, "uniqueness")
	sc.wantErr("POST", "/Groups", map[string]any{"displayName": ""}, 400, "invalidValue")
	sc.wantErr("POST", "/Groups", map[string]any{"displayName": "X", "members": []any{map[string]any{"value": manual.ID}}}, 400, "invalidValue")
	testers := sc.want("POST", "/Groups", map[string]any{"displayName": "Testers"}, 201)["id"].(string)

	// unmapped groups do not grant anything
	if e.roleOf(a) != rbac.RoleViewer {
		t.Fatalf("role %q", e.roleOf(a))
	}
	// the admin maps the groups; members follow at once
	admin.m("PATCH", "/api/v1/scim/groups/"+leads, map[string]any{"role": rbac.RoleManager}, 200)
	admin.m("PATCH", "/api/v1/scim/groups/"+testers, map[string]any{"role": rbac.RoleTester}, 200)
	admin.call("PATCH", "/api/v1/scim/groups/"+testers, map[string]any{"role": "wizard"}, 400, nil)
	admin.call("PATCH", "/api/v1/scim/groups/nope", map[string]any{"role": "tester"}, 404, nil)
	if e.roleOf(a) != rbac.RoleManager {
		t.Fatalf("mapped role %q", e.roleOf(a))
	}

	// Okta: add a member; the most privileged group wins
	sc.want("PATCH", "/Groups/"+testers, patch(map[string]any{"op": "add", "path": "members", "value": []any{map[string]any{"value": a}, map[string]any{"value": b}}}), 200)
	if e.roleOf(a) != rbac.RoleManager || e.roleOf(b) != rbac.RoleTester {
		t.Fatalf("roles %q %q", e.roleOf(a), e.roleOf(b))
	}
	ug := sc.want("GET", "/Users/"+a, nil, 200)["groups"].([]any)
	if len(ug) != 2 {
		t.Fatalf("user groups %v", ug)
	}
	// Okta: remove with a filtered path
	sc.want("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "remove", "path": fmt.Sprintf(`members[value eq "%s"]`, a)}), 200)
	if e.roleOf(a) != rbac.RoleTester {
		t.Fatalf("after remove %q", e.roleOf(a))
	}
	// Entra ID: remove with a value list, rename path-less
	sc.want("PATCH", "/Groups/"+testers, patch(
		map[string]any{"op": "Remove", "path": "members", "value": []any{map[string]any{"value": b}}},
		map[string]any{"op": "Replace", "value": map[string]any{"id": testers, "displayName": "All testers"}},
	), 200)
	if e.roleOf(b) != rbac.RoleViewer {
		t.Fatalf("b %q", e.roleOf(b))
	}
	got := sc.want("GET", "/Groups/"+testers+"?excludedAttributes=members", nil, 200)
	if got["displayName"] != "All testers" || got["members"] != nil {
		t.Fatalf("group %v", got)
	}
	// filters on groups
	out := sc.want("GET", "/Groups?filter="+url.QueryEscape(`displayName eq "all testers"`)+"&excludedAttributes=members", nil, 200)
	if out["totalResults"] != float64(1) {
		t.Fatalf("filter %v", out)
	}
	out = sc.want("GET", "/Groups?filter="+url.QueryEscape(fmt.Sprintf(`members[value eq "%s"]`, a)), nil, 200)
	if out["totalResults"] != float64(1) {
		t.Fatalf("member filter %v", out)
	}
	// replace members, add one via path-less value, externalId ops
	sc.want("PATCH", "/Groups/"+leads, patch(
		map[string]any{"op": "replace", "path": "members", "value": []any{map[string]any{"value": b}}},
		map[string]any{"op": "add", "value": map[string]any{"members": []any{map[string]any{"value": a}}}},
		map[string]any{"op": "remove", "path": "externalId"},
		map[string]any{"op": "add", "path": "externalId", "value": "g1b"},
	), 200)
	if e.roleOf(a) != rbac.RoleManager || e.roleOf(b) != rbac.RoleManager {
		t.Fatalf("replace %q %q", e.roleOf(a), e.roleOf(b))
	}
	sc.wantErr("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "remove", "path": "displayName"}), 400, "mutability")
	sc.wantErr("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "add", "path": "members", "value": []any{map[string]any{"value": "ghost"}}}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "replace", "path": "displayName", "value": "All testers"}), 409, "uniqueness")
	sc.wantErr("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "replace", "path": `members[value eq "x"]`, "value": []any{}}), 400, "invalidPath")
	sc.wantErr("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "add", "path": "members", "value": "x"}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "remove"}), 400, "noTarget")
	sc.wantErr("GET", "/Groups/nope", nil, 404, "")

	// PUT replaces the members; remove of all members
	sc.want("PUT", "/Groups/"+leads, map[string]any{"displayName": "Leads", "members": []any{map[string]any{"value": a}}}, 200)
	if e.roleOf(b) != rbac.RoleViewer {
		t.Fatalf("put %q", e.roleOf(b))
	}
	sc.want("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "remove", "path": "members"}), 200)
	if e.roleOf(a) != rbac.RoleTester {
		t.Fatalf("all removed %q", e.roleOf(a))
	}

	// default role: none means no access without a mapped group
	sc.want("PATCH", "/Groups/"+testers, patch(map[string]any{"op": "add", "path": "members", "value": []any{map[string]any{"value": a}}}), 200)
	admin.m("PATCH", "/api/v1/scim/settings", map[string]any{"default_role": ""}, 200)
	if e.roleOf(a) != rbac.RoleTester || e.roleOf(b) != "" {
		t.Fatalf("no default %q %q", e.roleOf(a), e.roleOf(b))
	}
	admin.call("PATCH", "/api/v1/scim/settings", map[string]any{"default_role": "wizard"}, 400, nil)
	admin.m("PATCH", "/api/v1/scim/settings", map[string]any{"default_role": rbac.RoleReviewer}, 200)
	if e.roleOf(b) != rbac.RoleReviewer {
		t.Fatalf("default %q", e.roleOf(b))
	}
	// deleting a group drops its role
	sc.want("DELETE", "/Groups/"+testers, nil, 204)
	sc.wantErr("DELETE", "/Groups/"+testers, nil, 404, "")
	if e.roleOf(a) != rbac.RoleReviewer {
		t.Fatalf("after group delete %q", e.roleOf(a))
	}
	ov := admin.m("GET", "/api/v1/scim", nil, 200)
	if ov["users"] != float64(2) || ov["active_users"] != float64(2) || len(ov["groups"].([]any)) != 1 || ov["default_role"] != "reviewer" {
		t.Fatalf("overview %v", ov)
	}
	// deleting a user removes it from its groups
	sc.want("PATCH", "/Groups/"+leads, patch(map[string]any{"op": "add", "path": "members", "value": []any{map[string]any{"value": a}}}), 200)
	sc.want("DELETE", "/Users/"+a, nil, 204)
	if ms := sc.want("GET", "/Groups/"+leads, nil, 200)["members"].([]any); len(ms) != 0 {
		t.Fatalf("members after user delete %v", ms)
	}
}

func TestSCIMLastAdminAndSuperAdmin(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin@example.com")
	sc := e.scim(e.scimToken(admin, "Entra"))

	// taking over the only admin with the default role (viewer) would leave
	// the tenant without an admin: refused, nothing saved
	sc.wantErr("POST", "/Users", map[string]any{"userName": "admin@example.com"}, 409, "")
	if ok, _ := e.st.IsSCIMUser(context.Background(), e.tenant.ID, e.admin.ID); ok || e.roleOf(e.admin.ID) != rbac.RoleAdmin {
		t.Fatal("rolled back")
	}

	x := sc.want("POST", "/Users", map[string]any{"userName": "x@example.com"}, 201)["id"].(string)
	g := sc.want("POST", "/Groups", map[string]any{"displayName": "Admins", "members": []any{map[string]any{"value": x}}}, 201)["id"].(string)
	admin.m("PATCH", "/api/v1/scim/groups/"+g, map[string]any{"role": rbac.RoleAdmin}, 200)
	if e.roleOf(x) != rbac.RoleAdmin {
		t.Fatalf("x %q", e.roleOf(x))
	}
	// the manual admin leaves; x is the last admin now
	admin.call("DELETE", "/api/v1/members/"+e.admin.ID, nil, 204, nil)
	sc.wantErr("PATCH", "/Users/"+x, patch(map[string]any{"op": "replace", "path": "active", "value": false}), 409, "")
	sc.wantErr("DELETE", "/Users/"+x, nil, 409, "")
	sc.wantErr("DELETE", "/Groups/"+g, nil, 409, "")
	sc.wantErr("PATCH", "/Groups/"+g, patch(map[string]any{"op": "remove", "path": "members"}), 409, "")
	if e.roleOf(x) != rbac.RoleAdmin {
		t.Fatalf("x lost admin: %q", e.roleOf(x))
	}
	if got := sc.want("GET", "/Users/"+x, nil, 200); got["active"] != true {
		t.Fatalf("x %v", got)
	}
	// the super admin (no member any more: the removal ended the session) may not unmap it either
	admin = e.login("admin@example.com")
	admin.call("POST", "/api/v1/auth/switch-tenant", map[string]string{"tenant_id": e.tenant.ID}, 200, nil)
	admin.call("PATCH", "/api/v1/scim/groups/"+g, map[string]any{"role": ""}, 409, nil)
	admin.call("PATCH", "/api/v1/scim/settings", map[string]any{"default_role": "viewer"}, 200, nil)

	// a super admin provisioned into the tenant keeps the flag and profile
	sup := mkUser(t, e.st, "root@example.com", true)
	if err := e.st.SetMembership(context.Background(), e.tenant.ID, sup.ID, rbac.RoleTester); err != nil {
		t.Fatal(err)
	}
	sc.want("POST", "/Users", map[string]any{"userName": "root@example.com", "displayName": "Changed"}, 201)
	sc.wantErr("PATCH", "/Users/"+sup.ID, patch(map[string]any{"op": "replace", "path": "userName", "value": "evil@example.com"}), 400, "mutability")
	sc.want("PATCH", "/Users/"+sup.ID, patch(map[string]any{"op": "replace", "path": "active", "value": false}), 200)
	u, _ := e.st.GetUser(context.Background(), sup.ID)
	if !u.SuperAdmin || u.Disabled || u.DisplayName == "Changed" || e.roleOf(sup.ID) != "" {
		t.Fatalf("super admin %+v role %q", u, e.roleOf(sup.ID))
	}
}

func TestSCIMMembersAreProtected(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin@example.com")
	sc := e.scim(e.scimToken(admin, "Entra"))
	id := sc.want("POST", "/Users", map[string]any{"userName": "p@example.com"}, 201)["id"].(string)
	other := e.member("o@example.com", rbac.RoleTester)

	var list struct{ Items []model.Membership }
	admin.call("GET", "/api/v1/members", nil, 200, &list)
	flags := map[string]bool{}
	for _, m := range list.Items {
		flags[m.UserID] = m.SCIM
	}
	if !flags[id] || flags[other.ID] || flags[e.admin.ID] {
		t.Fatalf("flags %v", flags)
	}
	admin.call("PATCH", "/api/v1/members/"+id, map[string]any{"role": "tester"}, 409, nil)
	admin.call("DELETE", "/api/v1/members/"+id, nil, 409, nil)
	admin.call("POST", "/api/v1/members", map[string]any{"email": "p@example.com", "role": "admin"}, 409, nil)
	admin.call("POST", "/api/v1/members/"+id+"/reset-password", nil, 400, nil)
	res := admin.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{id, other.ID}, "action": "role", "role": "reviewer"}, 200)
	sk := res["skipped"].([]any)
	if len(sk) != 1 || sk[0].(map[string]any)["reason"] != "scim" || e.roleOf(other.ID) != rbac.RoleReviewer || e.roleOf(id) != rbac.RoleViewer {
		t.Fatalf("bulk %v", res)
	}
}

func TestSCIMTenantIsolationAndAdoption(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.login("admin@example.com")
	scA := e.scim(e.scimToken(admin, "A"))
	other, err := e.st.CreateTenant(ctx, "other", "Other Org")
	if err != nil {
		t.Fatal(err)
	}
	admin.call("POST", "/api/v1/auth/switch-tenant", map[string]string{"tenant_id": other.ID}, 200, nil)
	scB := e.scim(e.scimToken(admin, "B"))

	// one person provisioned into both tenants
	id := scA.want("POST", "/Users", map[string]any{"userName": "both@example.com", "displayName": "Both"}, 201)["id"].(string)
	if got := scB.want("POST", "/Users", map[string]any{"userName": "both@example.com", "displayName": "Other name"}, 201)["id"]; got != id {
		t.Fatalf("same account expected: %v", got)
	}
	if out := scB.want("GET", "/Users", nil, 200); out["totalResults"] != float64(1) {
		t.Fatalf("B sees %v", out)
	}
	gA := scA.want("POST", "/Groups", map[string]any{"displayName": "G"}, 201)["id"].(string)
	scB.wantErr("GET", "/Groups/"+gA, nil, 404, "")
	// B deactivates: A's membership stays
	scB.want("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": "active", "value": false}), 200)
	if e.roleOf(id) != rbac.RoleViewer {
		t.Fatalf("A role %q", e.roleOf(id))
	}
	// a shared user's profile is not changed by one tenant
	if u, _ := e.st.GetUser(ctx, id); u.DisplayName != "Both" {
		t.Fatalf("name %q", u.DisplayName)
	}
	scB.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": "userName", "value": "new@example.com"}), 400, "mutability")
	scB.want("DELETE", "/Users/"+id, nil, 204)
	scA.want("GET", "/Users/"+id, nil, 200)
}

func TestSCIMSingleSignOnLink(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {})
	admin := e.login("admin@example.com")
	sc := e.scim(e.scimToken(admin, "Entra"))
	id := sc.want("POST", "/Users", map[string]any{"userName": "sso@example.com"}, 201)["id"].(string)
	ctx := context.Background()
	prov := config.Provisioning{AutoProvision: true, DefaultTenant: "acme", DefaultRole: "tester", RoleMapping: map[string]string{"admins": "admin"}}
	u, tenantID, err := e.srv.provision(ctx, &auth.Identity{Provider: "oidc:entra", Subject: "sub-1", Email: "SSO@example.com", Groups: []string{"admins"}}, prov)
	if err != nil || u.ID != id || tenantID != e.tenant.ID || u.AuthProvider != "oidc:entra" || u.ExternalID != "sub-1" {
		t.Fatalf("linked %+v %v", u, err)
	}
	// the SSO group mapping does not override SCIM
	if e.roleOf(id) != rbac.RoleViewer {
		t.Fatalf("role %q", e.roleOf(id))
	}
	// a deactivated user does not get the membership back by signing in
	sc.want("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": "active", "value": false}), 200)
	if _, _, err := e.srv.provision(ctx, &auth.Identity{Provider: "oidc:entra", Subject: "sub-1", Email: "sso@example.com"}, prov); err != nil {
		t.Fatal(err)
	}
	if e.roleOf(id) != "" {
		t.Fatalf("membership came back: %q", e.roleOf(id))
	}
}
