package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

// waiverFixture: a project with a MUST requirement whose test failed in a
// release run, a requirement without tests, both in the release scope.
type waiverFixture struct {
	pid, r1, r2, t1, rel string
}

func newWaiverFixture(t *testing.T, a *client) waiverFixture {
	t.Helper()
	pid := a.project("WVR")
	base := "/api/v1/projects/" + pid
	r1 := a.m("POST", base+"/requirements", map[string]any{"title": "Encrypt backups", "description": "The system MUST encrypt backups.", "status": "approved",
		"source": "ISO/IEC 27001:2022 A.8.13"}, 201)["id"].(string)
	r2 := a.m("POST", base+"/requirements", map[string]any{"title": "Rotate keys", "description": "The system MUST rotate keys.", "status": "approved"}, 201)["id"].(string)
	t1 := a.m("POST", base+"/tests", map[string]any{"title": "Backup is encrypted"}, 201)["id"].(string)
	a.m("POST", base+"/links", map[string]any{"requirement_id": r1, "test_case_id": t1}, 201)
	rel := a.m("POST", base+"/releases", map[string]any{"name": "1.0"}, 201)["id"].(string)
	a.m("POST", "/api/v1/releases/"+rel+"/requirements", map[string]any{"add": []string{r1, r2}}, 200)
	run := a.m("POST", base+"/runs", map[string]any{"name": "R1", "release_id": rel, "test_ids": []string{t1}}, 201)["id"].(string)
	for _, ex := range items(a.m("GET", "/api/v1/runs/"+run+"/executions", nil, 200)) {
		a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "failed"}, 200)
	}
	return waiverFixture{pid: pid, r1: r1, r2: r2, t1: t1, rel: rel}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func exitCrit(t *testing.T, st map[string]any, typ string) map[string]any {
	t.Helper()
	for _, c := range st["criteria"].([]any) {
		if m := c.(map[string]any); m["type"] == typ {
			return m
		}
	}
	t.Fatalf("no criterion %s in %v", typ, st["criteria"])
	return nil
}

func TestWaiverFlow(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("mgr@example.com", rbac.RoleManager)
	e.member("tester@example.com", rbac.RoleTester)
	mgr, tester := e.login("mgr@example.com"), e.login("tester@example.com")
	fx := newWaiverFixture(t, a)
	base := "/api/v1/projects/" + fx.pid

	st := a.m("GET", "/api/v1/releases/"+fx.rel+"/status", nil, 200)
	if st["go"] != false || exitCrit(t, st, "mandatory_passed")["met"] != false {
		t.Fatal(st)
	}

	// validation
	a.m("POST", base+"/waivers", map[string]any{"title": ""}, 400)
	a.m("POST", base+"/waivers", map[string]any{"title": "x", "type": "nope"}, 400)
	a.m("POST", base+"/waivers", map[string]any{"title": "x", "expires_on": "tomorrow"}, 400)
	a.m("POST", base+"/waivers", map[string]any{"title": "x", "approvers": []string{"no-at"}}, 400)
	a.m("POST", base+"/waivers", map[string]any{"title": "x", "submit": true}, 400) // no requirements
	a.m("POST", base+"/waivers", map[string]any{"title": "x", "type": "deviation", "requirement_ids": []string{fx.r1}, "justification": "why", "submit": true}, 400)
	tester.m("POST", base+"/waivers", map[string]any{"title": "x"}, 403)

	w := a.m("POST", base+"/waivers", map[string]any{"title": "Ship 1.0 without key rotation", "release_id": fx.rel, "requirement_ids": []string{fx.r1, fx.r2},
		"justification": "The HSM arrives in 1.1", "compensating": "Manual rotation every month", "conditions": "Only for 1.0",
		"approvers": []string{"MGR@example.com"}, "expires_on": "2999-12-31", "new_risk": map[string]any{"likelihood": 2, "impact": 4}}, 201)
	id := w["id"].(string)
	if w["key"] != "WVR-WVR-1" || w["status"] != "draft" || w["requested_by"] != "admin@example.com" || w["risk_key"] != "WVR-RISK-1" ||
		len(w["requirements"].([]any)) != 2 || w["approvers"].([]any)[0] != "mgr@example.com" || w["release_name"] != "1.0" {
		t.Fatal(w)
	}
	a.m("PATCH", "/api/v1/waivers/"+id, map[string]any{"conditions": "Only for release 1.0"}, 200)
	mgr.m("POST", "/api/v1/waivers/"+id+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 409) // draft
	a.m("POST", "/api/v1/waivers/"+id+"/submit", nil, 200)
	a.m("POST", "/api/v1/waivers/"+id+"/submit", nil, 409)
	if n := mgr.m("GET", "/api/v1/notifications", nil, 200); !strings.Contains(toJSON(n), "waiver.requested") {
		t.Fatal(n)
	}

	// who may sign
	a.m("POST", "/api/v1/waivers/"+id+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 403)      // the requester
	tester.m("POST", "/api/v1/waivers/"+id+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 403) // no waiver.approve
	mgr.m("POST", "/api/v1/waivers/"+id+"/decision", map[string]any{"decision": "approve", "password": "wrong"}, 403)
	mgr.m("POST", "/api/v1/waivers/"+id+"/decision", map[string]any{"decision": "reject", "password": testPassword}, 400) // no comment
	w = mgr.m("POST", "/api/v1/waivers/"+id+"/decision", map[string]any{"decision": "approve", "comment": "OK for 1.0", "password": testPassword}, 200)
	if w["status"] != "approved" || w["active"] != true || w["decided_by"] != "mgr@example.com" || len(w["approvals"].([]any)) != 1 ||
		w["approvals"].([]any)[0].(map[string]any)["valid"] != true {
		t.Fatal(w)
	}
	if n := a.m("GET", "/api/v1/notifications", nil, 200); !strings.Contains(toJSON(n), "waiver.approved") {
		t.Fatal(n)
	}
	a.m("PATCH", "/api/v1/waivers/"+id, map[string]any{"title": "changed"}, 409)
	a.m("DELETE", "/api/v1/waivers/"+id, nil, 409)

	// the release: the requirements are accepted, marked as waived
	st = a.m("GET", "/api/v1/releases/"+fx.rel+"/status", nil, 200)
	if st["go"] != true || st["summary"].(map[string]any)["waived"] != 2.0 || len(st["waivers"].([]any)) != 1 {
		t.Fatal(toJSON(st))
	}
	for _, typ := range []string{"coverage", "mandatory_passed", "passed", "no_failed"} {
		c := exitCrit(t, st, typ)
		if c["met"] != true || len(c["waived"].([]any)) == 0 || !strings.Contains(toJSON(c["waived"]), "WVR-WVR-1") {
			t.Fatal(typ, c)
		}
	}
	if l := items(a.m("GET", "/api/v1/requirements/"+fx.r2+"/waivers", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/releases/"+fx.rel+"/waivers", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	for q, n := range map[string]int{"": 1, "?status=approved": 1, "?status=draft": 0, "?active=true": 1, "?release_id=project": 0, "?release_id=" + fx.rel: 1, "?type=deviation": 0, "?requirement_id=" + fx.r1: 1} {
		if l := items(a.m("GET", base+"/waivers"+q, nil, 200)); len(l) != n {
			t.Fatal(q, len(l))
		}
	}
	a.m("GET", base+"/waivers?status=bogus", nil, 400)

	// release readiness report
	_, data := a.download(base+"/reports/generate", map[string]any{"template": "release", "format": "md", "params": map[string]string{"release_id": fx.rel}}, 200)
	if md := string(data); !strings.Contains(md, "# Release readiness") || !strings.Contains(md, "WVR-WVR-1") || !strings.Contains(md, "| Verdict | Go |") {
		t.Fatal(md)
	}
	a.download(base+"/reports/generate", map[string]any{"template": "release", "format": "md", "params": map[string]string{"release_id": "nope"}}, 400)
	// specification: marked and in an appendix
	f := a.m("POST", base+"/documents/generate", map[string]any{"kind": "requirements_spec", "format": "md"}, 201)
	if md := a.fileContent(f["id"].(string)); !strings.Contains(md, "Waivers and deviations") || !strings.Contains(md, "**Waived by:** WVR-WVR-1 (Waiver, until 2999-12-31)") {
		t.Fatal(md)
	}

	// expiry: the requirements are no longer accepted, people are told
	ctx := context.Background()
	if _, err := e.st.DB.ExecContext(ctx, e.st.DB.Rebind(`UPDATE waivers SET expires_on = '2000-01-01' WHERE id = ?`), id); err != nil {
		t.Fatal(err)
	}
	if n, err := e.srv.RunWaiverExpiry(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, _ := e.srv.RunWaiverExpiry(ctx); n != 0 {
		t.Fatal("expired twice")
	}
	if w = a.m("GET", "/api/v1/waivers/"+id, nil, 200); w["status"] != "expired" || w["active"] != false {
		t.Fatal(w)
	}
	if n := mgr.m("GET", "/api/v1/notifications", nil, 200); !strings.Contains(toJSON(n), "waiver.expired") {
		t.Fatal(n)
	}
	if st = a.m("GET", "/api/v1/releases/"+fx.rel+"/status", nil, 200); st["go"] != false {
		t.Fatal(st)
	}

	// a project-wide waiver without designated approvers: one approval; analytics show it separately
	w2 := a.m("POST", base+"/waivers", map[string]any{"type": "deviation", "title": "Keys rotated by the platform", "requirement_ids": []string{fx.r2},
		"justification": "The cloud KMS rotates them", "compensating": "KMS automatic rotation", "submit": true}, 201)
	id2 := w2["id"].(string)
	if w2["status"] != "requested" {
		t.Fatal(w2)
	}
	a.m("POST", "/api/v1/waivers/"+id2+"/withdraw", nil, 200)
	a.m("PATCH", "/api/v1/waivers/"+id2, map[string]any{"conditions": "while on the cloud KMS"}, 200)
	a.m("POST", "/api/v1/waivers/"+id2+"/submit", nil, 200)
	mgr.m("POST", "/api/v1/waivers/"+id2+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 200)
	an := a.m("GET", base+"/analytics", nil, 200)
	if an["summary"].(map[string]any)["waived"] != 1.0 || !strings.Contains(toJSON(an["findings"]), `"code":"waived"`) {
		t.Fatal(toJSON(an["summary"]))
	}
	cmp := a.m("GET", base+"/analytics/compliance?source=iso-27001", nil, 200)
	if !strings.Contains(toJSON(cmp), `"waived":`) {
		t.Fatal(toJSON(cmp))
	}
	_, data = a.download(base+"/reports/generate", map[string]any{"template": "compliance", "format": "md"}, 200)
	if md := string(data); !strings.Contains(md, "Waivers and deviations") || !strings.Contains(md, "Keys rotated by the platform") {
		t.Fatal(md)
	}
	mgr.m("POST", "/api/v1/waivers/"+id2+"/revoke", map[string]any{}, 400)
	tester.m("POST", "/api/v1/waivers/"+id2+"/revoke", map[string]any{"comment": "no"}, 403)
	if w2 = mgr.m("POST", "/api/v1/waivers/"+id2+"/revoke", map[string]any{"comment": "Moved off the KMS"}, 200); w2["status"] != "revoked" {
		t.Fatal(w2)
	}
	a.m("DELETE", "/api/v1/waivers/"+id2, nil, 204)
	a.m("GET", "/api/v1/waivers/"+id2, nil, 404)

	// rejection by a designated approver
	w3 := a.m("POST", base+"/waivers", map[string]any{"title": "No tests", "requirement_ids": []string{fx.r2}, "justification": "later", "approvers": []string{"mgr@example.com", "admin2@example.com"}, "submit": true}, 201)
	e.member("other@example.com", rbac.RoleManager)
	e.login("other@example.com").m("POST", "/api/v1/waivers/"+w3["id"].(string)+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 403)
	if w3 = mgr.m("POST", "/api/v1/waivers/"+w3["id"].(string)+"/decision", map[string]any{"decision": "reject", "comment": "Write the tests", "password": testPassword}, 200); w3["status"] != "rejected" {
		t.Fatal(w3)
	}
	// API tokens cannot sign
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "ci", "role": "admin"}, 201)["token"].(string)
	bot := e.anon()
	bot.bearer = tok
	w4 := a.m("POST", base+"/waivers", map[string]any{"title": "Token", "requirement_ids": []string{fx.r2}, "justification": "x y z", "submit": true}, 201)
	bot.m("POST", "/api/v1/waivers/"+w4["id"].(string)+"/decision", map[string]any{"decision": "approve"}, 403)
}

func TestResidualRisk(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("mgr@example.com", rbac.RoleManager)
	e.member("tester@example.com", rbac.RoleTester)
	mgr, tester := e.login("mgr@example.com"), e.login("tester@example.com")
	fx := newWaiverFixture(t, a)
	base := "/api/v1/projects/" + fx.pid

	a.m("POST", base+"/risks", map[string]any{"title": "x", "residual_likelihood": 2}, 400)
	a.m("POST", base+"/risks", map[string]any{"title": "x", "residual_likelihood": 6, "residual_impact": 1}, 400)
	a.m("POST", base+"/risks", map[string]any{"title": "x", "residual_likelihood": 1, "residual_impact": 1, "residual_manual": true}, 400)
	a.m("POST", base+"/risks", map[string]any{"title": "x", "review_date": "soon"}, 400)
	k := a.m("POST", base+"/risks", map[string]any{"title": "Backups readable", "likelihood": 5, "impact": 5, "requirement_ids": []string{fx.r1},
		"residual_likelihood": 1, "residual_impact": 2, "review_date": "2000-01-01", "review_owner": "ciso"}, 201)
	id := k["id"].(string)
	// the mitigation (r1) failed: the inherent score still applies
	if k["residual_source"] != "inherent" || k["residual_level"] != "critical" || k["above_threshold"] != true || k["threshold"] != "medium" || k["review_due"] != true {
		t.Fatal(k)
	}
	st := a.m("GET", "/api/v1/releases/"+fx.rel+"/status", nil, 200)
	if st["risk_threshold"] != "medium" || len(st["residual_risks"].([]any)) != 1 || st["summary"].(map[string]any)["residual_above_threshold"] != 1.0 {
		t.Fatal(toJSON(st))
	}
	// entered by hand
	a.m("PATCH", "/api/v1/risks/"+id, map[string]any{"residual_manual": true}, 400)
	k = a.m("PATCH", "/api/v1/risks/"+id, map[string]any{"residual_manual": true, "residual_justification": "Backups are offline"}, 200)
	if k["residual_source"] != "manual" || k["residual_score"] != 2.0 || k["residual_level"] != "low" || k["above_threshold"] != false {
		t.Fatal(k)
	}
	// the threshold
	if s := a.m("GET", base+"/risk-settings", nil, 200); s["effective"] != "medium" || s["threshold"] != "" {
		t.Fatal(s)
	}
	a.m("PUT", base+"/risk-settings", map[string]any{"threshold": "bogus"}, 400)
	tester.m("PUT", base+"/risk-settings", map[string]any{"threshold": "low"}, 403)
	a.m("PUT", base+"/risk-settings", map[string]any{"threshold": "low"}, 200)
	k = a.m("PATCH", "/api/v1/risks/"+id, map[string]any{"residual_likelihood": 2, "residual_impact": 3}, 200)
	if k["residual_level"] != "medium" || k["above_threshold"] != true || k["threshold"] != "low" {
		t.Fatal(k)
	}
	// a release criterion with its own threshold
	a.m("PATCH", "/api/v1/releases/"+fx.rel, map[string]any{"exit_criteria": []map[string]any{{"type": "residual_risk", "level": "nope"}}}, 400)
	a.m("PATCH", "/api/v1/releases/"+fx.rel, map[string]any{"exit_criteria": []map[string]any{{"type": "residual_risk"}}}, 200)
	st = a.m("GET", "/api/v1/releases/"+fx.rel+"/status", nil, 200)
	if c := exitCrit(t, st, "residual_risk"); c["met"] != false || len(c["items"].([]any)) != 1 {
		t.Fatal(c)
	}
	// accept risk
	tester.m("POST", "/api/v1/risks/"+id+"/accept", map[string]any{"password": testPassword}, 403)
	mgr.m("POST", "/api/v1/risks/"+id+"/accept", map[string]any{"password": "wrong"}, 403)
	k = mgr.m("POST", "/api/v1/risks/"+id+"/accept", map[string]any{"password": testPassword, "comment": "Accepted for 1.0"}, 200)
	if k["status"] != "accepted" || k["accepted_by"] != "mgr@example.com" || k["acceptance_valid"] != true {
		t.Fatal(k)
	}
	st = a.m("GET", "/api/v1/releases/"+fx.rel+"/status", nil, 200)
	if c := exitCrit(t, st, "residual_risk"); c["met"] != true || len(c["accepted"].([]any)) != 1 || st["go"] != true {
		t.Fatal(toJSON(st))
	}
	// risk report: before and after
	_, data := a.download(base+"/reports/generate", map[string]any{"template": "risks", "format": "csv"}, 200)
	files := zipFiles(t, data)
	if _, ok := files["heat_map_residual.csv"]; !ok || !strings.Contains(files["above_threshold.csv"], "Backups readable") || !strings.Contains(files["summary.csv"], "Risk acceptance threshold,Low") {
		t.Fatal(files["summary.csv"], files["above_threshold.csv"])
	}
	// a changed assessment invalidates the acceptance
	k = a.m("PATCH", "/api/v1/risks/"+id, map[string]any{"residual_impact": 4}, 200)
	if k["acceptance_valid"] != false || k["accepted_by"] != "mgr@example.com" {
		t.Fatal(k)
	}
	k = mgr.m("DELETE", "/api/v1/risks/"+id+"/accept", nil, 200)
	if k["accepted_by"] != "" || k["status"] != "mitigating" {
		t.Fatal(k)
	}
	a.m("PATCH", "/api/v1/risks/"+id, map[string]any{"status": "closed"}, 200)
	mgr.m("POST", "/api/v1/risks/"+id+"/accept", map[string]any{"password": testPassword}, 400)
}

func TestSoA(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("tester@example.com", rbac.RoleTester)
	tester := e.login("tester@example.com")
	fx := newWaiverFixture(t, a)
	base := "/api/v1/projects/" + fx.pid
	q := a.m("POST", base+"/requirements", map[string]any{"title": "MFA", "status": "approved", "source": "ISO/IEC 27001:2022 A.8.5"}, 201)["id"].(string)
	tc := a.m("POST", base+"/tests", map[string]any{"title": "MFA works"}, 201)["id"].(string)
	a.m("POST", base+"/links", map[string]any{"requirement_id": q, "test_case_id": tc}, 201)
	run := a.m("POST", base+"/runs", map[string]any{"name": "R2", "test_ids": []string{tc}}, 201)["id"].(string)
	for _, ex := range items(a.m("GET", "/api/v1/runs/"+run+"/executions", nil, 200)) {
		a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "passed"}, 200)
	}

	srcs := items(a.m("GET", base+"/soa/sources", nil, 200))
	if srcs[0]["id"] != "iso-27001" || srcs[0]["shipped"] != true || srcs[0]["controls"] != 93.0 {
		t.Fatal(srcs[0])
	}
	st := a.m("GET", base+"/soa", nil, 200)
	rows := st["rows"].([]any)
	row := func(st map[string]any, id string) map[string]any {
		for _, r := range st["rows"].([]any) {
			if m := r.(map[string]any); m["id"] == id {
				return m
			}
		}
		t.Fatalf("no row %s", id)
		return nil
	}
	if len(rows) != 93 || row(st, "A.8.5")["status"] != "implemented" || row(st, "A.8.13")["status"] != "planned" || row(st, "A.8.5")["suggested"] != true {
		t.Fatal(len(rows), row(st, "A.8.5"), row(st, "A.8.13"))
	}
	a.m("GET", base+"/soa?source=nope", nil, 400)
	sv := a.m("GET", base+"/soa?lang=sv", nil, 200)
	if row(sv, "A.5.1")["title"] != "Informationssäkerhetspolicyer" {
		t.Fatal(row(sv, "A.5.1"))
	}

	put := base + "/soa/decisions"
	a.m("PUT", put, map[string]any{"decisions": []map[string]any{{"control_id": "A.9.9", "applicable": true}}}, 400)
	a.m("PUT", put, map[string]any{"decisions": []map[string]any{{"control_id": "A.7.1", "applicable": false}}}, 400)
	a.m("PUT", put, map[string]any{"source": "nope", "decisions": []map[string]any{}}, 400)
	tester.m("PUT", put, map[string]any{"decisions": []map[string]any{{"control_id": "A.8.5", "applicable": true}}}, 403)
	res := a.m("PUT", put, map[string]any{"source": "iso-27001", "decisions": []map[string]any{
		{"control_id": "A.8.5", "applicable": true, "justification": "Users sign in", "owner": "ciso"},
		{"control_id": "A.7.1", "applicable": false, "justification": "No own premises"},
		{"control_id": "A.5.1", "applicable": nil},
	}}, 200)
	if res["changed"] != 2.0 {
		t.Fatal(res["changed"])
	}
	st = res["statement"].(map[string]any)
	if r := row(st, "A.8.5"); r["applicable"] != true || r["owner"] != "ciso" || r["updated_by"] != "admin@example.com" {
		t.Fatal(r)
	}
	if r := row(st, "A.7.1"); r["applicable"] != false || r["status"] != "" {
		t.Fatal(r)
	}
	sum := st["summary"].(map[string]any)
	if sum["applicable"] != 1.0 || sum["excluded"] != 1.0 || sum["implemented"] != 1.0 {
		t.Fatal(sum)
	}
	// unchanged again: nothing changes; clearing removes
	if res = a.m("PUT", put, map[string]any{"decisions": []map[string]any{{"control_id": "A.8.5", "applicable": true, "justification": "Users sign in", "owner": "ciso"}}}, 200); res["changed"] != 0.0 {
		t.Fatal(res["changed"])
	}
	if res = a.m("PUT", put, map[string]any{"decisions": []map[string]any{{"control_id": "A.7.1", "applicable": nil}}}, 200); res["changed"] != 1.0 {
		t.Fatal(res["changed"])
	}
	// derived controls for a source without a shipped list
	gd := a.m("GET", base+"/soa?source=gdpr", nil, 200)
	if gd["source"].(map[string]any)["shipped"] != false || len(gd["rows"].([]any)) == 0 {
		t.Fatal(gd["source"])
	}

	// the document in every format, and saved to the archive
	for _, f := range []string{"pdf", "docx", "xlsx", "md"} {
		_, data := a.download(base+"/reports/generate", map[string]any{"template": "soa", "format": f, "params": map[string]string{"source": "iso-27001"}}, 200)
		if len(data) == 0 {
			t.Fatal(f)
		}
	}
	_, data := a.download(base+"/reports/generate", map[string]any{"template": "soa", "format": "md", "lang": "sv"}, 200)
	if md := string(data); !strings.Contains(md, "# Uttalande om tillämplighet") || !strings.Contains(md, "A.8.34") || !strings.Contains(md, "Användares slutpunktsenheter") {
		t.Fatal(md)
	}
	fi := a.m("POST", base+"/reports/generate", map[string]any{"template": "soa", "format": "xlsx", "archive": true}, 201)
	if !strings.HasPrefix(fi["name"].(string), "soa-") {
		t.Fatal(fi)
	}
	a.download(base+"/reports/generate", map[string]any{"template": "soa", "format": "md", "params": map[string]string{"source": "nope"}}, 400)
}
