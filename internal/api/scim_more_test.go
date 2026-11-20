package api

import (
	"testing"

	"github.com/rom/qtestframe/internal/config"
)

func TestSCIMMoreAttributeForms(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Server.BaseURL = "" })
	admin := e.login("admin@example.com")
	ov := admin.m("GET", "/api/v1/scim", nil, 200)
	if ov["base_url"] != e.hs.URL+"/scim/v2" {
		t.Fatalf("base url from the request: %v", ov["base_url"])
	}
	sc := e.scim(e.scimToken(admin, "OneLogin"))
	id := sc.want("POST", "/Users", map[string]any{"userName": "m@example.com", "name": map[string]any{"formatted": "Formatted Name"}}, 201)["id"].(string)
	out := sc.want("PATCH", "/Users/"+id, patch(
		map[string]any{"op": "replace", "path": "emails", "value": map[string]any{"value": "m2@example.com", "type": "work"}},
		map[string]any{"op": "replace", "path": `emails[type eq "work"].type`, "value": "work"},
		map[string]any{"op": "remove", "path": "name"},
		map[string]any{"op": "remove", "path": "active"},
		map[string]any{"op": "replace", "path": "name.formatted", "value": "ignored while a display name exists"},
		map[string]any{"op": "replace", "path": "displayName", "value": []any{"Listed"}},
	), 200)
	if out["displayName"] != "Listed" || out["emails"].([]any)[0].(map[string]any)["value"] != "m2@example.com" || len(out["name"].(map[string]any)) != 0 {
		t.Fatalf("patched %v", out)
	}
	// removing the e-mail falls back to the userName (an e-mail address)
	out = sc.want("PATCH", "/Users/"+id, patch(map[string]any{"op": "remove", "path": "emails"}), 200)
	if out["emails"].([]any)[0].(map[string]any)["value"] != "m@example.com" {
		t.Fatalf("emails removed %v", out)
	}
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": "name", "value": "x"}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": "emails", "value": 5}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "path": `emails[type eq "work"]`, "value": 5}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "value": "x"}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Users/"+id, patch(map[string]any{"op": "replace", "value": map[string]any{"emails[": 1}}), 400, "invalidPath")
	// a userName that is no e-mail address keeps the e-mail
	if out := sc.want("PUT", "/Users/"+id, map[string]any{"userName": "nomail"}, 200); out["emails"].([]any)[0].(map[string]any)["value"] != "m@example.com" {
		t.Fatalf("put %v", out)
	}
	sc.wantErr("PUT", "/Users/"+id, map[string]any{"displayName": "no user name"}, 400, "invalidValue")
	sc.wantErr("PUT", "/Users/missing", map[string]any{"userName": "a@b.c"}, 404, "")
	sc.wantErr("PUT", "/Users/"+id, "{", 400, "invalidSyntax")
	// an inactive user is created without a membership
	in := sc.want("POST", "/Users", map[string]any{"userName": "off@example.com", "active": false}, 201)["id"].(string)
	if e.roleOf(in) != "" {
		t.Fatal("inactive user got a membership")
	}
	// groups: PUT and PATCH errors
	g := sc.want("POST", "/Groups", map[string]any{"displayName": "G1"}, 201)["id"].(string)
	sc.want("POST", "/Groups", map[string]any{"displayName": "G2"}, 201)
	sc.wantErr("PUT", "/Groups/"+g, map[string]any{"displayName": "G2"}, 409, "uniqueness")
	sc.wantErr("PUT", "/Groups/"+g, map[string]any{"displayName": "G1", "members": []any{map[string]any{"value": "x"}}}, 400, "invalidValue")
	sc.wantErr("PUT", "/Groups/"+g, "[", 400, "invalidSyntax")
	sc.wantErr("PUT", "/Groups/nope", map[string]any{"displayName": "G1"}, 404, "")
	sc.wantErr("POST", "/Groups", "[", 400, "invalidSyntax")
	sc.wantErr("PATCH", "/Groups/"+g, patch(map[string]any{"op": "replace", "value": 1}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Groups/"+g, patch(map[string]any{"op": "replace", "path": "displayName", "value": 1}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Groups/"+g, patch(map[string]any{"op": "replace", "path": "externalId", "value": 1}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Groups/"+g, patch(map[string]any{"op": "replace", "path": "members[", "value": 1}), 400, "invalidPath")
	sc.wantErr("PATCH", "/Groups/"+g, patch(map[string]any{"op": "replace", "value": map[string]any{"members": []any{map[string]any{"value": "ghost"}}}}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Groups/"+g, patch(map[string]any{"op": "replace", "path": "members", "value": []any{map[string]any{"value": "ghost"}}}), 400, "invalidValue")
	sc.wantErr("PATCH", "/Groups/nope", patch(map[string]any{"op": "remove", "path": "members"}), 404, "")
	sc.wantErr("PATCH", "/Groups/"+g, "x", 400, "invalidSyntax")
	sc.want("PATCH", "/Groups/"+g, patch(map[string]any{"op": "add", "path": "members", "value": map[string]any{"value": id}},
		map[string]any{"op": "replace", "path": "schemas", "value": []string{scimGroupSchema}}), 200)
	sc.want("PATCH", "/Groups/"+g, patch(map[string]any{"op": "remove", "path": "members", "value": []any{map[string]any{"value": id}}}), 200)
	if (&scimError{Detail: "d"}).Error() != "d" || describeUser("x", true, "") != "x (no role)" {
		t.Fatal("helpers")
	}
	evs := admin.m("GET", "/api/v1/scim/events?limit=3", nil, 200)
	if len(items(evs)) != 3 {
		t.Fatalf("events %v", evs)
	}
}
