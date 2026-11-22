package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func TestSimilarAndReview(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SIM")
	req := func(title, desc string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "description": desc}, 201)["id"].(string)
	}
	login := req("User login", "The user MUST be able to sign in with their e-mail address and password.")
	login2 := req("Log in with email", "Users SHALL log in using their email address and a password.")
	pw1 := req("Password length", "Passwords MUST be at least 12 characters long.")
	pw2 := req("Maximum password length", "Passwords MUST be at most 8 characters.")
	req("Nightly backup", "The database SHALL be backed up every night.")
	req("Dark mode", "The user interface SHOULD offer a dark colour scheme.")
	req("Keyboard navigation", "Every function of the UI MUST be reachable with the keyboard.")

	// while writing: the most similar first, the item itself left out
	res := a.m("POST", "/api/v1/projects/"+pid+"/similar", map[string]any{"title": "Sign in", "description": "A user signs in with e-mail and password."}, 200)
	hits := anyList(res["items"])
	if res["engine"] != EngineTFIDF || len(hits) == 0 || hits[0].(map[string]any)["id"] != login && hits[0].(map[string]any)["id"] != login2 {
		t.Fatal(res)
	}
	res = a.m("POST", "/api/v1/projects/"+pid+"/similar", map[string]any{"title": "User login", "description": "The user MUST be able to sign in with their e-mail address and password.", "exclude_id": login}, 200)
	for _, h := range anyList(res["items"]) {
		if h.(map[string]any)["id"] == login {
			t.Fatal("the item itself", res)
		}
	}
	if empty := a.m("POST", "/api/v1/projects/"+pid+"/similar", map[string]any{"title": " "}, 200); len(anyList(empty["items"])) != 0 {
		t.Fatal(empty)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/similar", map[string]any{"kind": "risk", "title": "x"}, 400)

	// the review: duplicates and conflicts with evidence
	rev := a.m("GET", "/api/v1/projects/"+pid+"/consistency/review", nil, 200)
	dups, cons := anyList(rev["duplicates"]), anyList(rev["conflicts"])
	if len(dups) != 1 || len(cons) != 1 || rev["threshold"].(float64) != 0.68 {
		t.Fatal(rev)
	}
	d := dups[0].(map[string]any)
	if d["a"].(map[string]any)["id"] != login || d["b"].(map[string]any)["id"] != login2 {
		t.Fatal(d)
	}
	c := cons[0].(map[string]any)
	if c["kind"] != "threshold" || c["reason"] != "threshold" || c["confidence"].(float64) < 0.6 || !strings.Contains(c["note"].(string), "12") {
		t.Fatal(c)
	}
	marked := false
	for _, part := range anyList(c["evidence_a"].(map[string]any)["parts"]) {
		if p := part.(map[string]any); p["mark"] == true {
			marked = true
		}
	}
	if !marked {
		t.Fatal("no evidence", c)
	}
	// the built-in checks report them too
	con := a.m("GET", "/api/v1/projects/"+pid+"/analytics/consistency", nil, 200)
	if ids := violationIDs(resultOf(t, con, model.CheckPotentialConflict)); len(ids) != 1 || ids[0] != pw2 {
		t.Fatal(ids)
	}
	if ids := violationIDs(resultOf(t, con, model.CheckDuplicateSuspect)); len(ids) != 1 || ids[0] != login2 {
		t.Fatal(ids)
	}

	// dismiss the conflict: gone, listed with dismissed=true
	body := map[string]any{"check": model.CheckPotentialConflict, "item_id": pw2, "other_id": pw1}
	a.m("POST", "/api/v1/projects/"+pid+"/consistency/dismissals", body, 201)
	rev = a.m("GET", "/api/v1/projects/"+pid+"/consistency/review", nil, 200)
	if len(anyList(rev["conflicts"])) != 0 || rev["dismissed"].(float64) != 1 {
		t.Fatal(rev)
	}
	rev = a.m("GET", "/api/v1/projects/"+pid+"/consistency/review?dismissed=true", nil, 200)
	if cs := anyList(rev["conflicts"]); len(cs) != 1 || cs[0].(map[string]any)["dismissed"] != true {
		t.Fatal(rev)
	}
	a.m("DELETE", "/api/v1/projects/"+pid+"/consistency/dismissals", body, 204)
	// marked as conflicting: gone from the review and the check
	a.m("POST", "/api/v1/requirements/"+pw2+"/relations", map[string]any{"target_id": pw1, "type": "conflicts_with"}, 201)
	rev = a.m("GET", "/api/v1/projects/"+pid+"/consistency/review", nil, 200)
	if len(anyList(rev["conflicts"])) != 0 || rev["dismissed"].(float64) != 0 {
		t.Fatal(rev)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/consistency/review?threshold=2", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/consistency/review?kind=x", nil, 400)

	// a viewer without requirement.read may not
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/projects/"+pid+"/consistency/review", nil, 200)
	v.m("POST", "/api/v1/projects/"+pid+"/consistency/merge", map[string]any{"survivor_id": login, "duplicate_id": login2}, 403)
}

func TestMergeRequirements(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("MRG")
	req := func(title, desc string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "description": desc}, 201)["id"].(string)
	}
	keep := req("Account lockout", "The system SHALL lock the account after 5 failed login attempts.")
	dup := req("Lock account", "After five failed sign-in attempts the account MUST be locked.")
	other := req("Unlock", "An administrator MUST be able to unlock a locked account.")
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Lockout"}, 201)["id"].(string)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Lockout twice"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": dup, "test_case_id": t1}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": dup, "test_case_id": t2}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": keep, "test_case_id": t2}, 201)
	a.m("POST", "/api/v1/requirements/"+other+"/relations", map[string]any{"target_id": dup, "type": "depends_on"}, 201)

	a.m("POST", "/api/v1/projects/"+pid+"/consistency/merge", map[string]any{"survivor_id": keep, "duplicate_id": keep}, 400)
	res := a.m("POST", "/api/v1/projects/"+pid+"/consistency/merge", map[string]any{"survivor_id": keep, "duplicate_id": dup}, 200)
	moved := res["moved"].(map[string]any)
	if moved["links"].(float64) != 1 || moved["relations"].(float64) != 1 || moved["dropped"].(float64) != 1 || res["duplicate"].(map[string]any)["status"] != "obsolete" {
		t.Fatal(res)
	}
	if st := a.m("GET", "/api/v1/requirements/"+dup, nil, 200)["status"]; st != "obsolete" {
		t.Fatal(st)
	}
	links := anyList(a.m("GET", "/api/v1/requirements/"+keep+"/links", nil, 200)["items"])
	if len(links) != 2 {
		t.Fatal(links)
	}
	rels := anyList(a.m("GET", "/api/v1/requirements/"+keep+"/relations", nil, 200)["items"])
	types := map[string]bool{}
	for _, r := range rels {
		types[r.(map[string]any)["type"].(string)] = true
	}
	if !types["duplicates"] || !types["depends_on"] {
		t.Fatal(rels)
	}
	if cs := anyList(a.m("GET", "/api/v1/requirements/"+keep+"/comments", nil, 200)["items"]); len(cs) != 1 || !strings.Contains(cs[0].(map[string]any)["body"].(string), "Merged") {
		t.Fatal(cs)
	}
	if hist := anyList(a.m("GET", "/api/v1/requirements/"+dup+"/history", nil, 200)["items"]); len(hist) < 2 {
		t.Fatal(hist)
	}
}

func TestDuplicateTests(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("DT")
	tst := func(title, desc string, steps ...string) string {
		var st []map[string]any
		for _, s := range steps {
			st = append(st, map[string]any{"action": s, "expected": "ok"})
		}
		return a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": title, "description": desc, "steps": st, "status": "ready"}, 201)["id"].(string)
	}
	t1 := tst("Sign in with valid password", "Log in as a user with e-mail and password.", "Open the login page", "Enter e-mail and password")
	t2 := tst("Login with a valid password", "Sign in as a user with email and password.", "Open the sign-in page", "Enter email and password")
	tst("Export a report", "Export the monthly report to PDF.", "Open reports", "Click export")
	tst("Dark mode", "Switch the theme to dark.", "Open the profile", "Choose dark")
	tst("Delete account", "Remove the own account.", "Open the profile", "Delete")
	con := a.m("GET", "/api/v1/projects/"+pid+"/analytics/consistency", nil, 200)
	if ids := violationIDs(resultOf(t, con, model.CheckTestDuplicateSuspect)); len(ids) != 1 || ids[0] != t2 {
		t.Fatal(ids)
	}
	rev := a.m("GET", "/api/v1/projects/"+pid+"/consistency/review?kind=test", nil, 200)
	if len(anyList(rev["duplicates"])) != 1 || len(anyList(rev["conflicts"])) != 0 {
		t.Fatal(rev)
	}
	// dismiss for tests needs test.write; then restore and merge
	body := map[string]any{"check": model.CheckTestDuplicateSuspect, "item_id": t2, "other_id": t1}
	a.m("POST", "/api/v1/projects/"+pid+"/consistency/dismissals", body, 201)
	if rev := a.m("GET", "/api/v1/projects/"+pid+"/consistency/review?kind=test", nil, 200); len(anyList(rev["duplicates"])) != 0 {
		t.Fatal(rev)
	}
	a.m("DELETE", "/api/v1/projects/"+pid+"/consistency/dismissals", body, 204)
	req := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": req, "test_case_id": t2}, 201)
	res := a.m("POST", "/api/v1/projects/"+pid+"/consistency/merge", map[string]any{"kind": "test", "survivor_id": t1, "duplicate_id": t2}, 200)
	if res["moved"].(map[string]any)["links"].(float64) != 1 || res["duplicate"].(map[string]any)["status"] != "deprecated" {
		t.Fatal(res)
	}
	if st := a.m("GET", "/api/v1/tests/"+t2, nil, 200)["status"]; st != "deprecated" {
		t.Fatal(st)
	}
	links := anyList(a.m("GET", "/api/v1/tests/"+t2+"/item-links", nil, 200)["items"])
	if len(links) != 1 || links[0].(map[string]any)["label"] != model.DuplicateLinkLabel {
		t.Fatal(links)
	}
	con = a.m("GET", "/api/v1/projects/"+pid+"/analytics/consistency", nil, 200)
	if ids := violationIDs(resultOf(t, con, model.CheckTestDuplicateSuspect)); len(ids) != 0 {
		t.Fatal(ids)
	}
}

// fakeEmbeddings is an OpenAI-compatible embeddings endpoint: a text's
// vector counts a few words, so texts about logging in point the same way.
func fakeEmbeddings(t *testing.T, calls *atomic.Int32, inputs *atomic.Int32) *httptest.Server {
	t.Helper()
	words := []string{"login", "sign", "log", "password", "export", "report", "dark", "theme"}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" || r.URL.Path != "/v1/embeddings" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls.Add(1)
		inputs.Add(int32(len(req.Input)))
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		out := struct {
			Data []item `json:"data"`
		}{}
		for i, in := range req.Input {
			v := make([]float32, len(words)+1)
			low := strings.ToLower(in)
			for k, w := range words {
				v[k] = float32(strings.Count(low, w))
			}
			v[len(words)] = 0.1
			out.Data = append(out.Data, item{Index: i, Embedding: v})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
}

func TestEmbeddings(t *testing.T) {
	var calls, inputs atomic.Int32
	srv := fakeEmbeddings(t, &calls, &inputs)
	defer srv.Close()
	e := newEnv(t, func(c *config.Config) {
		c.Similarity.Embeddings = config.Embeddings{Enabled: true, URL: srv.URL + "/v1/embeddings", Model: "fake", APIKey: "test-key"}
	})
	a := e.login("admin@example.com")
	pid := a.project("EMB")
	req := func(title, desc string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "description": desc}, 201)["id"].(string)
	}
	l1 := req("Login", "Sign in with a password.")
	req("Export", "Export a report.")
	req("Theme", "A dark theme.")
	res := a.m("POST", "/api/v1/projects/"+pid+"/similar", map[string]any{"title": "Log in", "description": "login with password"}, 200)
	if res["engine"] != EngineEmbeddings || anyList(res["items"])[0].(map[string]any)["id"] != l1 {
		t.Fatal(res)
	}
	// the vectors are cached per item version: the second time only the query
	before := inputs.Load()
	a.m("POST", "/api/v1/projects/"+pid+"/similar", map[string]any{"title": "Export", "description": "report"}, 200)
	if got := inputs.Load() - before; got != 1 {
		t.Fatalf("%d texts sent again", got)
	}
	rev := a.m("GET", "/api/v1/projects/"+pid+"/consistency/review", nil, 200)
	if rev["engine"] != EngineEmbeddings {
		t.Fatal(rev)
	}
	// the settings test reports the vector size; a wrong key fails
	if r := a.m("POST", "/api/v1/admin/settings/test/embeddings", map[string]any{}, 200); r["ok"] != true || !strings.Contains(r["message"].(string), "9") {
		t.Fatal(r)
	}
	if r := a.m("POST", "/api/v1/admin/settings/test/embeddings", map[string]any{"values": map[string]any{"similarity.embeddings.api_key": "wrong"}}, 200); r["ok"] != false {
		t.Fatal(r)
	}
	// the key is write-only
	var view struct {
		Settings []map[string]any `json:"settings"`
	}
	a.call("GET", "/api/v1/admin/settings", nil, 200, &view)
	for _, s := range view.Settings {
		if s["key"] == "similarity.embeddings.api_key" && s["value"] != nil {
			t.Fatal(s)
		}
	}
	// a failing service falls back to the local similarity
	srv.Close()
	res = a.m("POST", "/api/v1/projects/"+pid+"/similar", map[string]any{"title": "Sign in", "description": "Sign in with a password."}, 200)
	if res["engine"] != EngineTFIDF || len(anyList(res["items"])) == 0 {
		t.Fatal(res)
	}
}

func TestCatalogSimilar(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("CSIM")
	cat := items(a.m("GET", "/api/v1/catalog/requirements?lang=en", nil, 200))
	if len(cat) == 0 {
		t.Skip("no catalogue")
	}
	first := cat[0]
	// the project already has the same requirement under another name
	full := a.m("GET", "/api/v1/catalog/requirements/"+first["id"].(string)+"?lang=en", nil, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": full["title"], "description": full["description"]}, 201)
	res := items(a.m("POST", "/api/v1/projects/"+pid+"/catalog/similar", map[string]any{"requirement_ids": []string{first["id"].(string)}, "lang": "en"}, 200))
	if len(res) != 1 || len(anyList(res[0]["similar"])) != 1 {
		t.Fatal(res)
	}
	imp := a.m("POST", "/api/v1/projects/"+pid+"/catalog/import", map[string]any{"requirement_ids": []string{first["id"].(string)}, "lang": "en", "skip_similar": true, "include_tests": false}, 201)
	if sk := anyList(imp["skipped"]); len(sk) != 1 || sk[0].(map[string]any)["reason"] != "similar" || len(anyList(imp["requirements"])) != 0 {
		t.Fatal(imp)
	}
	imp = a.m("POST", "/api/v1/projects/"+pid+"/catalog/import", map[string]any{"requirement_ids": []string{first["id"].(string)}, "lang": "en", "include_tests": false}, 201)
	if rs := anyList(imp["requirements"]); len(rs) != 1 || len(anyList(rs[0].(map[string]any)["similar"])) != 1 {
		t.Fatal(imp)
	}
}
