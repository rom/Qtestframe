package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

func (c *client) with(headers map[string]string) *client {
	cp := *c
	cp.headers = headers
	return &cp
}

func enableERecords(t *testing.T, a *client, set map[string]any) {
	t.Helper()
	body := map[string]any{"enabled": true, "meanings": []string{"approved", "reviewed", "QA release"}}
	for k, v := range set {
		body[k] = v
	}
	a.m("PUT", "/api/v1/erecords", body, 200)
}

func TestERecordsSettings(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("tim@example.com", rbac.RoleTester)
	tc := e.login("tim@example.com")
	got := a.m("GET", "/api/v1/erecords", nil, 200)
	st := got["settings"].(map[string]any)
	if st["enabled"] != false || st["idle_timeout_minutes"].(float64) != 15 || len(st["meanings"].([]any)) != 5 || len(got["signature_actions"].([]any)) != 4 {
		t.Fatal(got)
	}
	// every member reads them (the dialogs adapt), only administrators change them
	tc.m("GET", "/api/v1/erecords", nil, 200)
	tc.m("PUT", "/api/v1/erecords", map[string]any{"enabled": true}, 403)
	a.m("PUT", "/api/v1/erecords", map[string]any{"enabled": true, "signature_actions": []string{"coffee.break"}}, 400)
	a.m("PUT", "/api/v1/erecords", map[string]any{"enabled": true, "reason_edits": []string{"nope"}}, 400)
	a.m("PUT", "/api/v1/erecords", map[string]any{"enabled": true, "idle_timeout_minutes": 999}, 400)
	out := a.m("PUT", "/api/v1/erecords", map[string]any{"enabled": true, "meanings": []string{" approved ", "approved", ""}, "signature_actions": []string{"release.go"}}, 200)
	if out["updated_by"] != "admin@example.com" || len(out["meanings"].([]any)) != 1 {
		t.Fatal(out)
	}
	list, _ := e.st.ListAudit(context.Background(), e.tenant.ID, 5)
	if list[0].Action != "erecords.enable" {
		t.Fatal(list[0])
	}
	// the ticket endpoint needs the mode
	a.m("PUT", "/api/v1/erecords", map[string]any{"enabled": false}, 200)
	a.m("POST", "/api/v1/esign/tickets", map[string]any{"action": "release.go", "password": testPassword, "meaning": "approved"}, 400)
}

func TestERecordsReasonAndSignatures(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ER")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in."}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Logout"}, 201)["id"].(string)
	enableERecords(t, a, map[string]any{"reason_edits": []string{"requirement.update"}, "signature_actions": []string{"requirement.approve", "baseline.create", "test.approve"}})

	// a reason for change
	res := a.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"title": "Log in"}, 428)
	if res["code"] != "reason_required" || res["edit_type"] != "requirement.update" {
		t.Fatal(res)
	}
	why := a.with(map[string]string{"X-Change-Reason": url.PathEscape("Typo (CAPA-7)")})
	why.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"title": "Log in"}, 200)
	list, _ := e.st.ListAudit(context.Background(), e.tenant.ID, 3)
	found := false
	for _, x := range list {
		if x.Action == "requirement.update" && x.Reason == "Typo (CAPA-7)" {
			found = true
		}
	}
	if !found {
		t.Fatal(list)
	}
	// reads and edits without a configured reason are not affected
	a.m("GET", "/api/v1/requirements/"+r1, nil, 200)
	tid := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T", "steps": []map[string]any{{"action": "Open", "expected": "Shown"}}}, 201)["id"].(string)
	a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"title": "T2"}, 200)

	// approving needs a signature
	res = why.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"status": "approved"}, 428)
	if res["code"] != "signature_required" || res["action"] != "requirement.approve" || len(res["meanings"].([]any)) != 3 {
		t.Fatal(res)
	}
	ticket := func(c *client, action string, want int, extra map[string]any) string {
		t.Helper()
		body := map[string]any{"action": action, "password": testPassword, "meaning": "approved", "time_zone": "Europe/Stockholm", "comment": "Checked"}
		for k, v := range extra {
			body[k] = v
		}
		out := c.m("POST", "/api/v1/esign/tickets", body, want)
		tk, _ := out["ticket"].(string)
		return tk
	}
	ticket(a, "requirement.approve", 403, map[string]any{"password": "wrong"})
	ticket(a, "requirement.approve", 400, map[string]any{"meaning": ""})
	ticket(a, "requirement.approve", 400, map[string]any{"meaning": "liked"})
	ticket(a, "coffee", 400, nil)
	tk := ticket(a, "baseline.create", 201, nil)
	// a ticket for another action does not count
	res = a.with(map[string]string{"X-Change-Reason": "ok%20now", "X-Signature-Ticket": tk}).m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"status": "approved"}, 428)
	if !strings.Contains(res["error"].(string), "not valid") {
		t.Fatal(res)
	}
	tk = ticket(a, "requirement.approve", 201, nil)
	signed := a.with(map[string]string{"X-Change-Reason": "Approved%20in%20review", "X-Signature-Ticket": tk})
	signed.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"status": "approved"}, 200)
	// single use
	signed.m("PATCH", "/api/v1/requirements/"+r2, map[string]any{"status": "approved"}, 428)
	sigs := items(a.m("GET", "/api/v1/esignatures?entity_type=requirement&entity_id="+r1, nil, 200))
	if len(sigs) != 1 || sigs[0]["meaning"] != "approved" || sigs[0]["time_zone"] != "Europe/Stockholm" || sigs[0]["valid"] != true ||
		sigs[0]["method"] != "password" || sigs[0]["action"] != "requirement.approve" || sigs[0]["comment"] != "Checked" {
		t.Fatal(sigs)
	}
	// an already approved requirement needs no new signature for other edits; a change flags the signature
	why.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"status": "approved", "description": "The user MUST log in with a password."}, 200)
	sigs = items(a.m("GET", "/api/v1/esignatures?entity_type=requirement&entity_id="+r1, nil, 200))
	if sigs[0]["valid"] != false {
		t.Fatal(sigs)
	}
	// bulk changes are enforced as well
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{r2}, "set": map[string]any{"status": "approved"}}, 428)
	tk = ticket(a, "requirement.approve", 201, nil)
	a.with(map[string]string{"X-Change-Reason": "bulk", "X-Signature-Ticket": tk}).m("POST", "/api/v1/projects/"+pid+"/requirements/bulk",
		map[string]any{"ids": []string{r2}, "set": map[string]any{"status": "approved"}}, 200)
	if l := items(a.m("GET", "/api/v1/esignatures?entity_type=requirement&entity_id="+r2, nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	// review decisions on tests
	a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"status": "review"}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/tests/review", map[string]any{"ids": []string{tid}, "decision": "ready"}, 428)
	a.m("POST", "/api/v1/projects/"+pid+"/tests/review", map[string]any{"ids": []string{tid}, "decision": "ready", "dry_run": true}, 200)
	tk = ticket(a, "test.approve", 201, nil)
	if rv := a.with(map[string]string{"X-Signature-Ticket": tk}).m("POST", "/api/v1/projects/"+pid+"/tests/review", map[string]any{"ids": []string{tid}, "decision": "ready"}, 200); len(rv["updated"].([]any)) != 1 {
		t.Fatal(rv)
	}
	if l := items(a.m("GET", "/api/v1/esignatures?entity_type=test&entity_id="+tid, nil, 200)); len(l) != 1 || l[0]["valid"] != true {
		t.Fatal(l)
	}
	// baselines
	a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "B1"}, 428)
	tk = ticket(a, "baseline.create", 201, nil)
	bid := a.with(map[string]string{"X-Signature-Ticket": tk}).m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "B1"}, 201)["id"].(string)
	if l := items(a.m("GET", "/api/v1/esignatures?entity_type=baseline&entity_id="+bid, nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	// a baseline sign-off uses the organisation's meanings and records the manifestation
	a.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "released", "password": testPassword}, 400)
	a.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"password": testPassword}, 400)
	a.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "QA release", "password": testPassword, "time_zone": "Mars/Base"}, 201)
	l := items(a.m("GET", "/api/v1/esignatures?entity_type=baseline&entity_id="+bid, nil, 200))
	if len(l) != 2 || l[0]["action"] != "baseline.signoff" || l[0]["meaning"] != "QA release" || l[0]["time_zone"] != "UTC" {
		t.Fatal(l)
	}
	// organisation-wide list for auditors
	if all := items(a.m("GET", "/api/v1/esignatures?project_id="+pid, nil, 200)); len(all) != 5 {
		t.Fatal(len(all))
	}
	e.member("vera@example.com", rbac.RoleViewer)
	v := e.login("vera@example.com")
	v.m("GET", "/api/v1/esignatures", nil, 403)
	v.m("GET", "/api/v1/esignatures?entity_type=requirement&entity_id="+r1, nil, 200)
	// API tokens cannot sign
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "ci"}, 201)
	tc := e.anon()
	tc.bearer = tok["token"].(string)
	tc.m("POST", "/api/v1/esign/tickets", map[string]any{"action": "requirement.approve", "password": testPassword, "meaning": "approved"}, 403)
	// a verification record sign-off is a signature in this mode
	e.member("rita@example.com", rbac.RoleManager)
	rc := e.login("rita@example.com")
	rec := a.m("POST", "/api/v1/requirements/"+r1+"/verification-records", map[string]any{"method": "inspection", "result": "pass"}, 201)["id"].(string)
	if res := rc.m("POST", "/api/v1/verification-records/"+rec+"/signoff", map[string]any{"comment": "ok"}, 428); res["action"] != "verification.signoff" {
		t.Fatal(res)
	}
	rc.m("POST", "/api/v1/verification-records/"+rec+"/signoff", map[string]any{"comment": "ok", "meaning": "reviewed"}, 403)
	rc.m("POST", "/api/v1/verification-records/"+rec+"/signoff", map[string]any{"comment": "ok", "password": testPassword, "meaning": "reviewed"}, 200)
	if l := items(rc.m("GET", "/api/v1/esignatures?entity_type=verification_record&entity_id="+rec, nil, 200)); len(l) != 1 || l[0]["user_email"] != "rita@example.com" {
		t.Fatal(l)
	}
	// or with a signing ticket, as the web client does after the 428
	rc.m("DELETE", "/api/v1/verification-records/"+rec+"/signoff", nil, 200)
	tk = ticket(rc, "verification.signoff", 201, map[string]any{"meaning": "reviewed"})
	rc.with(map[string]string{"X-Signature-Ticket": tk}).m("POST", "/api/v1/verification-records/"+rec+"/signoff", map[string]any{}, 200)
	rc.with(map[string]string{"X-Signature-Ticket": tk}).m("POST", "/api/v1/verification-records/"+rec+"/signoff", map[string]any{}, 428)
	if l := items(rc.m("GET", "/api/v1/esignatures?entity_type=verification_record&entity_id="+rec, nil, 200)); len(l) != 2 || l[0]["meaning"] != "reviewed" {
		t.Fatal(l)
	}
	// a baseline sign-off with a ticket takes its meaning
	tk = ticket(a, "baseline.signoff", 201, map[string]any{"meaning": "reviewed"})
	if so := a.with(map[string]string{"X-Signature-Ticket": tk}).m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{}, 201); so["meaning"] != "reviewed" {
		t.Fatal(so)
	}
	a.m("GET", "/api/v1/esignatures?entity_type=nonsense&entity_id=x", nil, 400)
}

func TestERecordsSecondFactorAndIdle(t *testing.T) {
	e := newEnv(t)
	clk := withClock(e)
	a := e.login("admin@example.com")
	secret, _ := enrol(t, a, clk)
	enableERecords(t, a, map[string]any{"signature_actions": []string{"release.go"}, "idle_timeout_minutes": 1})
	body := map[string]any{"action": "release.go", "password": testPassword, "meaning": "approved"}
	if res := a.m("POST", "/api/v1/esign/tickets", body, 403); res["code"] != "mfa_required" {
		t.Fatal(res)
	}
	body["mfa_code"] = "000000"
	if res := a.m("POST", "/api/v1/esign/tickets", body, 403); res["code"] != "mfa_invalid" {
		t.Fatal(res)
	}
	clk.add(time.Minute)
	body["mfa_code"] = totpAt(t, secret, clk.Now())
	a.m("POST", "/api/v1/esign/tickets", body, 201)
	pid := a.project("RL")
	rid := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0"}, 201)["id"].(string)
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"description": "first"}, 200)
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"status": "released"}, 428)

	// inactivity ends the session; polling is no activity
	off := time.Duration(0)
	e.st.SetClock(func() time.Time { return time.Now().UTC().Add(off).Truncate(time.Microsecond) })
	e.member("tim@example.com", rbac.RoleTester)
	tc := e.login("tim@example.com")
	off = 50 * time.Second
	tc.with(map[string]string{"X-Qtf-Background": "1"}).m("GET", "/api/v1/auth/me", nil, 200)
	off = 80 * time.Second
	tc.m("GET", "/api/v1/auth/me", nil, 401)
	t2 := e.login("tim@example.com")
	off = 130 * time.Second
	t2.m("GET", "/api/v1/auth/me", nil, 200) // activity
	off = 180 * time.Second
	t2.m("GET", "/api/v1/auth/me", nil, 200)
	off = 300 * time.Second
	t2.m("GET", "/api/v1/auth/me", nil, 401)
}

func TestAuditVerifyAndArchive(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	a.project("AU")
	e.member("tim@example.com", rbac.RoleTester)
	e.login("tim@example.com").m("POST", "/api/v1/audit/verify", nil, 403)
	v := a.m("POST", "/api/v1/audit/verify", nil, 200)
	if v["ok"] != true || v["entries"].(float64) < 2 || v["checkpoints"].(float64) < 1 || v["key_protected"] != true {
		t.Fatal(v)
	}
	// the JSON export verifies offline
	res := a.raw("GET", "/api/v1/audit/archive", nil)
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Disposition"), "audit-trail-acme") {
		t.Fatal(res.Status, string(b))
	}
	var arc store.AuditArchive
	if err := json.Unmarshal(b, &arc); err != nil {
		t.Fatal(err, string(b))
	}
	if ok := store.VerifyAuditArchive(&arc); !ok.OK || ok.Entries == 0 || arc.Format != store.AuditArchiveFormat {
		t.Fatalf("%+v", ok)
	}
	// a changed export is detected
	arc.Entries[0].Details = "changed"
	if ok := store.VerifyAuditArchive(&arc); ok.OK || ok.Broken.Problem != model.AuditEntryChanged {
		t.Fatalf("%+v", ok)
	}
	res = a.raw("GET", "/api/v1/audit/archive?format=csv", nil)
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.HasPrefix(b, []byte("seq,id,created_at,user_id,user_email,action,entity_type,entity_id,details,reason,prev_hash,hash\n1,")) {
		t.Fatal(string(b))
	}
	a.m("GET", "/api/v1/audit/archive?format=xml", nil, 400)
	// the audit list shows the chain
	l := items(a.m("GET", "/api/v1/audit", nil, 200))
	if l[0]["seq"].(float64) == 0 || len(l[0]["hash"].(string)) != 64 {
		t.Fatal(l[0])
	}
	// tampering shows the first broken link
	if _, err := e.st.DB.ExecContext(context.Background(), rebindTest(e.st, `UPDATE audit_log SET details = 'x' WHERE tenant_id = ? AND seq = 2`), e.tenant.ID); err != nil {
		t.Fatal(err)
	}
	v = a.m("POST", "/api/v1/audit/verify", nil, 200)
	if br, _ := v["broken"].(map[string]any); v["ok"] != false || br["seq"].(float64) != 2 || br["problem"] != model.AuditEntryChanged {
		t.Fatal(v)
	}
	// the compliance report
	rep := a.m("GET", "/api/v1/erecords/report", nil, 200)
	ctl := map[string]map[string]any{}
	for _, c := range rep["controls"].([]any) {
		m := c.(map[string]any)
		ctl[m["id"].(string)] = m
	}
	if ctl["audit_trail"]["status"] != "attention" || ctl["validation"]["provider"] != "customer" || ctl["components"]["status"] != "not_configured" || rep["disclaimer"] == "" {
		t.Fatal(ctl)
	}
	res = a.raw("GET", "/api/v1/erecords/report?format=md", nil)
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(b), "| Validation of the system |") {
		t.Fatal(string(b))
	}
}

func rebindTest(st *store.Store, q string) string {
	if st.DB.Dialect != "postgres" {
		return q
	}
	n := 0
	var b strings.Builder
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString("$" + itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
