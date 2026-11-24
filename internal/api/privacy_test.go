package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// exec runs a statement with ? placeholders on the test database.
func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.st.DB.ExecContext(context.Background(), db.Rebind(e.st.DB.Dialect, q), args...); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.st.DB.QueryRowContext(context.Background(), db.Rebind(e.st.DB.Dialect, q), args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestTruncateIP(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.17": "192.0.2.0", "2001:db8:1234:5678::1": "2001:db8:1234::", "not-an-ip": "not-an-ip", "": "",
	} {
		if got := TruncateIP(in); got != want {
			t.Errorf("TruncateIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrivacyNoticeAndTruncatedIP(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.Privacy = config.Privacy{TruncateIP: true, NoticeURL: "https://example.test/privacy"}
	})
	c := e.anon()
	if p := c.m("GET", "/api/v1/auth/providers", nil, 200); p["privacy_notice_url"] != "https://example.test/privacy" {
		t.Fatal(p)
	}
	e.login("admin@example.com")
	ips := e.count(`SELECT COUNT(*) FROM login_events WHERE ip = ?`, "127.0.0.0")
	if ips == 0 {
		t.Fatal("the sign-in was not logged with a truncated address")
	}
}

func TestRetentionPreviewPurgeAndLegalHold(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.Retention = config.Retention{Hour: 2, RunsDays: 30, RunsKeepLatest: 1, LogsDays: 30, NotificationsDays: 30, SessionsDays: 30, ChatDays: 30, ExploratoryDays: 30}
	})
	ctx := context.Background()
	a := e.login("admin@example.com")
	pid := a.project("RET")
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "One"}, 201)["id"].(string)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Two"}, 201)["id"].(string)
	rel := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0"}, 201)["id"].(string)
	run := func(name string, tests []string, release string) string {
		body := map[string]any{"name": name, "test_ids": tests}
		if release != "" {
			body["release_id"] = release
		}
		return a.m("POST", "/api/v1/projects/"+pid+"/runs", body, 201)["id"].(string)
	}
	r1 := run("old", []string{t1}, "")
	r2 := run("old in held release", []string{t1}, rel)
	r4 := run("old signed", []string{t1}, "")
	r5 := run("old latest of two", []string{t2}, "")
	r3 := run("new", []string{t1}, "")
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	for i, id := range []string{r1, r2, r4, r5} {
		e.exec(`UPDATE test_runs SET created_at = ? WHERE id = ?`, old.Add(time.Duration(i)*time.Minute), id)
	}
	e.exec(`INSERT INTO esignatures (id, tenant_id, project_id, entity_type, entity_id, action, signed_at) VALUES (?, ?, ?, 'run', ?, 'run.close', ?)`, "sig1", e.tenant.ID, pid, r4, old)
	// held release
	lead := e.member("lead@example.com", rbac.RoleTester)
	lc := e.login(lead.Email)
	lc.call("PUT", "/api/v1/projects/"+pid+"/releases/"+rel+"/legal-hold", map[string]any{"on": true, "reason": "case 7"}, 403, nil)
	var hv legalHoldView
	lc.call("GET", "/api/v1/projects/"+pid+"/legal-hold", nil, 200, &hv)
	if hv.CanEdit || hv.Project.On || len(hv.Releases) != 1 {
		t.Fatalf("%+v", hv)
	}
	a.call("PUT", "/api/v1/projects/"+pid+"/releases/"+rel+"/legal-hold", map[string]any{"on": true}, 400, nil)
	a.call("PUT", "/api/v1/projects/"+pid+"/releases/"+rel+"/legal-hold", map[string]any{"on": true, "reason": "case 7"}, 200, &hv)
	if !hv.CanEdit || !hv.Releases[0].Hold.On || hv.Releases[0].Hold.By != "admin@example.com" || hv.Releases[0].Hold.At == nil {
		t.Fatalf("%+v", hv)
	}
	a.call("PUT", "/api/v1/projects/"+pid+"/releases/nope/legal-hold", map[string]any{"on": true, "reason": "x"}, 404, nil)
	// a held run cannot be deleted
	a.call("DELETE", "/api/v1/runs/"+r2, nil, 409, nil)

	// a second project under legal hold with an old exploratory note
	pid2 := a.project("HLD")
	a.call("PUT", "/api/v1/projects/"+pid2+"/legal-hold", map[string]any{"on": true, "reason": "audit 2026"}, 200, &hv)
	a.call("DELETE", "/api/v1/projects/"+pid2, nil, 409, nil)
	for i, p := range []string{pid, pid2} {
		sid := "xs" + string(rune('a'+i))
		e.exec(`INSERT INTO exploratory_sessions (id, tenant_id, project_id, seq, key, title, status, created_at, updated_at) VALUES (?, ?, ?, 1, ?, 'S', 'debriefed', ?, ?)`, sid, e.tenant.ID, p, sid, old, old)
		e.exec(`INSERT INTO exploratory_notes (id, tenant_id, session_id, body, created_at, updated_at) VALUES (?, ?, ?, 'note', ?, ?)`, sid+"n", e.tenant.ID, sid, old, old)
	}
	// logs, notifications, tokens, chat, input history, sign-in log, sessions
	e.exec(`INSERT INTO notifications (id, tenant_id, user_id, event, title, created_at) VALUES ('n1', ?, ?, 'mention', 'Old', ?)`, e.tenant.ID, e.admin.ID, old)
	e.exec(`INSERT INTO notifications (id, tenant_id, user_id, event, title, created_at) VALUES ('n2', ?, ?, 'mention', 'New', ?)`, e.tenant.ID, e.admin.ID, time.Now().UTC())
	e.exec(`INSERT INTO api_tokens (id, tenant_id, user_id, name, role, hash, prefix, created_at, expires_at) VALUES ('tk1', ?, ?, 'old', 'viewer', 'h1', 'p', ?, ?)`, e.tenant.ID, e.admin.ID, old, old)
	e.exec(`INSERT INTO ci_deliveries (id, tenant_id, project_id, commit_sha, status, created_at) VALUES ('ci1', ?, ?, 'abc', 'sent', ?)`, e.tenant.ID, pid, old)
	e.exec(`INSERT INTO ci_deliveries (id, tenant_id, project_id, commit_sha, status, created_at) VALUES ('ci2', ?, ?, 'abc', 'sent', ?)`, e.tenant.ID, pid2, old)
	e.exec(`INSERT INTO req_sync_runs (id, tenant_id, project_id, system, trigger_kind, status, started_at, finished_at) VALUES ('rs1', ?, ?, 'jira', 'manual', 'ok', ?, ?)`, e.tenant.ID, pid, old, old)
	e.exec(`INSERT INTO chat_channels (id, tenant_id, project_id, name, provider, created_at, updated_at) VALUES ('ch1', ?, ?, 'c', 'slack', ?, ?)`, e.tenant.ID, pid, old, old)
	e.exec(`INSERT INTO chat_deliveries (id, tenant_id, channel_id, event, status, created_at) VALUES ('cd1', ?, 'ch1', 'x', 'sent', ?)`, e.tenant.ID, old)
	e.exec(`INSERT INTO suggest_history (id, user_id, tenant_id, field, value, value_hash, last_used) VALUES ('sh1', ?, ?, 'f', 'v', 'h', ?)`, e.admin.ID, e.tenant.ID, old)
	e.exec(`INSERT INTO login_events (id, user_id, success, created_at) VALUES ('le1', ?, ?, ?)`, e.admin.ID, false, old)
	e.exec(`INSERT INTO sessions (id, user_id, tenant_id, csrf, created_at, expires_at, last_seen) VALUES ('s-old', ?, ?, 'c', ?, ?, ?)`, e.admin.ID, e.tenant.ID, old, time.Now().UTC().Add(time.Hour), old)
	// web test evidence of an old execution
	execID := ""
	if err := e.st.DB.QueryRowContext(ctx, db.Rebind(e.st.DB.Dialect, `SELECT id FROM test_executions WHERE run_id = ?`), r5).Scan(&execID); err != nil {
		t.Fatal(err)
	}
	f := &model.FileInfo{ProjectID: &pid, Kind: model.FileAttachment, Name: "shot.png", ContentType: "image/png", Tags: []string{"web-test", "screenshot"}}
	if err := e.st.CreateFile(ctx, e.tenant.ID, f, []byte("png")); err != nil {
		t.Fatal(err)
	}
	if err := e.st.Attach(ctx, f.ID, "execution", execID); err != nil {
		t.Fatal(err)
	}
	e.exec(`UPDATE files SET created_at = ? WHERE id = ?`, old, f.ID)

	// only super administrators
	lc.call("GET", "/api/v1/admin/retention", nil, 403, nil)
	lc.call("POST", "/api/v1/admin/retention/run", map[string]any{"confirm": "PURGE"}, 403, nil)

	var v retentionView
	a.call("GET", "/api/v1/admin/retention", nil, 200, &v)
	if v.Enabled || len(v.Reports) != 2 || len(v.Holds) != 2 {
		t.Fatalf("%+v", v)
	}
	want := map[string][2]int{"runs": {1, 2}, "logs": {3, 1}, "notifications": {1, 0}, "sessions": {1, 0}, "chat": {2, 0}, "exploratory": {1, 1}}
	check := func(rep model.RetentionReport) {
		t.Helper()
		for _, c := range rep.Counts {
			if w := want[c.Category]; c.Count != w[0] || c.Held != w[1] {
				t.Errorf("%s: %d deleted, %d held, want %v", c.Category, c.Count, c.Held, w)
			}
		}
	}
	check(v.Reports[1])
	if sys := v.Reports[0]; sys.TenantID != "" || sys.Counts[0].Count != 1 || sys.Counts[1].Count != 1 {
		t.Fatalf("%+v", sys)
	}
	// nothing was deleted by the preview
	if n := e.count(`SELECT COUNT(*) FROM test_runs WHERE tenant_id = ?`, e.tenant.ID); n != 5 {
		t.Fatal(n)
	}

	// an organisation override
	a.call("PUT", "/api/v1/admin/tenants/"+e.tenant.ID+"/policy", map[string]any{"retention": map[string]int{"bogus_days": 1}}, 400, nil)
	a.call("PUT", "/api/v1/admin/tenants/"+e.tenant.ID+"/policy", map[string]any{"retention": map[string]int{"notifications_days": -1}}, 400, nil)
	a.call("PUT", "/api/v1/admin/tenants/"+e.tenant.ID+"/policy", map[string]any{"retention": map[string]int{"notifications_days": 0}}, 200, nil)
	if d := auditDetails(t, e, "tenant.policy"); len(d) != 1 || !strings.Contains(d[0], "retention notifications_days: system setting → 0") {
		t.Fatal(d)
	}
	a.call("GET", "/api/v1/admin/retention", nil, 200, &v)
	want["notifications"] = [2]int{0, 0}
	check(v.Reports[1])
	if strings.Join(v.Reports[1].Policy.Overridden, ",") != "notifications_days" {
		t.Fatal(v.Reports[1].Policy)
	}

	a.call("POST", "/api/v1/admin/retention/run", map[string]any{"confirm": "no"}, 400, nil)
	a.call("POST", "/api/v1/admin/retention/run", map[string]any{"confirm": "PURGE"}, 200, &v)
	check(v.Reports[1])
	if v.LastRun == nil {
		t.Fatal("no last run")
	}
	if n := e.count(`SELECT COUNT(*) FROM test_runs WHERE id IN (?, ?, ?, ?, ?)`, r1, r2, r3, r4, r5); n != 4 {
		t.Fatalf("%d runs left", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM exploratory_notes`); n != 1 {
		t.Fatal(n)
	}
	if n := e.count(`SELECT COUNT(*) FROM files WHERE id = ?`, f.ID); n != 0 {
		t.Fatal("evidence kept")
	}
	if n := e.count(`SELECT COUNT(*) FROM login_events WHERE id = 'le1'`); n != 0 {
		t.Fatal("sign-in log kept")
	}
	if d := auditDetails(t, e, "retention.purge"); len(d) != 1 || !strings.Contains(d[0], "runs: 1 deleted (older than 30 days), 2 kept by legal hold or signature") {
		t.Fatal(d)
	}
	// the audit chain still verifies
	if res, err := e.st.VerifyAudit(ctx, e.tenant.ID); err != nil || !res.OK {
		t.Fatal(res, err)
	}
	// lifting the hold
	a.call("PUT", "/api/v1/projects/"+pid2+"/legal-hold", map[string]any{"on": false}, 200, &hv)
	if hv.Project.On || hv.Project.Reason != "" {
		t.Fatalf("%+v", hv)
	}
	if d := auditDetails(t, e, "legal_hold.lift"); len(d) != 1 {
		t.Fatal(d)
	}
}

func TestRetentionNightlyJob(t *testing.T) {
	now := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	e := newEnv(t, func(c *config.Config) {
		c.Retention = config.Retention{Hour: 2, NotificationsDays: 1}
	})
	e.srv.Clock = func() time.Time { return now }
	ctx := context.Background()
	if ran, err := e.srv.RunRetention(ctx); ran || err != nil {
		t.Fatal("ran while off", err)
	}
	c := e.cfg
	c.Retention.Enabled = true
	c.Retention.Hour = 2
	c.Retention.NotificationsDays = 1
	e.srv.sys.live.Store(&c)
	e.exec(`INSERT INTO notifications (id, tenant_id, user_id, event, title, created_at) VALUES ('n1', ?, ?, 'mention', 'Old', ?)`, e.tenant.ID, e.admin.ID, time.Now().UTC().Add(-72*time.Hour))
	if ran, err := e.srv.RunRetention(ctx); !ran || err != nil {
		t.Fatal("not run", err)
	}
	if ran, _ := e.srv.RunRetention(ctx); ran {
		t.Fatal("ran twice in a day")
	}
	if n := e.count(`SELECT COUNT(*) FROM notifications`); n != 0 {
		t.Fatal(n)
	}
	es, _ := e.st.ListAudit(ctx, e.tenant.ID, 50)
	found := false
	for _, x := range es {
		found = found || (x.Action == "retention.purge" && x.UserEmail == "system")
	}
	if !found {
		t.Fatal("purge not audited")
	}
	now = now.Add(24 * time.Hour)
	if ran, _ := e.srv.RunRetention(ctx); !ran {
		t.Fatal("not run the next day")
	}
	if d := e.srv.retentionDue(time.Date(2026, 3, 10, 1, 0, 0, 0, time.UTC)); d.Day() != 9 {
		t.Fatal(d)
	}
	var v retentionView
	e.login("admin@example.com").call("GET", "/api/v1/admin/retention", nil, 200, &v)
	if !v.Enabled || v.NextRun == nil {
		t.Fatalf("%+v", v)
	}
}

func readZip(t *testing.T, res *http.Response) map[string][]map[string]any {
	t.Helper()
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("status %d: %s", res.StatusCode, data)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]map[string]any{}
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, ".json") || f.Name == "index.json" {
			continue
		}
		r, _ := f.Open()
		var rows []map[string]any
		if err := json.NewDecoder(r).Decode(&rows); err != nil {
			t.Fatal(f.Name, err)
		}
		r.Close()
		out[strings.TrimSuffix(f.Name, ".json")] = rows
	}
	return out
}

func TestPersonalDataExportAndErasure(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	pid := a.project("GDP")
	anna := e.member("anna.berg@example.com", rbac.RoleAdmin)
	ac := e.login(anna.Email)
	ac.call("PATCH", "/api/v1/auth/me", map[string]any{"display_name": "Anna Berg"}, 200, nil)
	rq := ac.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login"}, 201)
	ac.call("POST", "/api/v1/requirements/"+rq["id"].(string)+"/comments", map[string]any{"body": "my comment"}, 201, nil)
	a.call("POST", "/api/v1/requirements/"+rq["id"].(string)+"/comments", map[string]any{"body": "ping @anna.berg please"}, 201, nil)
	e.exec(`INSERT INTO esignatures (id, tenant_id, project_id, entity_type, entity_id, action, user_id, user_email, user_name, signed_at) VALUES ('sg1', ?, ?, 'requirement', ?, 'approve', ?, ?, 'Anna Berg', ?)`,
		e.tenant.ID, pid, rq["id"], anna.ID, anna.Email, time.Now().UTC())
	var tok struct {
		Token string `json:"token"`
	}
	ac.call("POST", "/api/v1/tokens", map[string]any{"name": "ci", "expires_days": 30}, 201, &tok)

	// self-service export
	got := readZip(t, ac.raw("GET", "/api/v1/auth/me/export", nil))
	if got["profile"][0]["email"] != anna.Email || len(got["comments"]) != 1 || len(got["mentions"]) != 1 || len(got["sessions"]) == 0 ||
		len(got["api_tokens"]) != 1 || len(got["electronic_signatures"]) != 1 || len(got["memberships"]) != 1 || len(got["audit_entries"]) == 0 {
		t.Fatalf("%v", got)
	}
	for _, row := range got["api_tokens"] {
		if _, ok := row["hash"]; ok {
			t.Fatal("token hash exported")
		}
	}
	if d := auditDetails(t, e, "user.data_export"); len(d) != 1 || d[0] != "self-service" {
		t.Fatal(d)
	}
	tc := e.anon()
	tc.bearer = tok.Token
	tc.call("GET", "/api/v1/auth/me/export", nil, 403, nil)

	// by a super administrator only
	ac.call("GET", "/api/v1/users/"+e.admin.ID+"/export", nil, 403, nil)
	got = readZip(t, a.raw("GET", "/api/v1/users/"+anna.ID+"/export", nil))
	if got["profile"][0]["display_name"] != "Anna Berg" {
		t.Fatal(got["profile"])
	}
	a.call("GET", "/api/v1/users/nope/export", nil, 404, nil)

	// erasure: typed confirmation, not oneself, super admin only
	ac.call("POST", "/api/v1/users/"+e.admin.ID+"/erase", map[string]any{"confirm": "admin@example.com"}, 403, nil)
	a.call("POST", "/api/v1/users/"+anna.ID+"/erase", map[string]any{"confirm": "someone@example.com"}, 400, nil)
	a.call("POST", "/api/v1/users/"+e.admin.ID+"/erase", map[string]any{"confirm": "admin@example.com"}, 400, nil)
	a.call("POST", "/api/v1/users/nope/erase", map[string]any{"confirm": "x"}, 404, nil)
	var res store.ErasureResult
	a.call("POST", "/api/v1/users/"+anna.ID+"/erase", map[string]any{"confirm": " Anna.Berg@example.com "}, 200, &res)
	if res.Pseudonym != "Former user #1" || res.Email != "former-user-1@erased.invalid" || res.Kept["electronic_signatures"] != 1 || res.Deleted["api_tokens"] != 1 || res.Rewritten == 0 {
		t.Fatalf("%+v", res)
	}
	a.call("POST", "/api/v1/users/"+anna.ID+"/erase", map[string]any{"confirm": res.Email}, 409, nil)
	u, err := e.st.GetUser(ctx, anna.ID)
	if err != nil || !u.Erased || !u.Disabled || u.DisplayName != "Former user #1" || u.PasswordHash != "" {
		t.Fatalf("%+v %v", u, err)
	}
	// authored content names the pseudonym, the mention too
	if n := e.count(`SELECT COUNT(*) FROM requirements WHERE created_by = ?`, anna.Email); n != 0 {
		t.Fatal("authored content still names the address")
	}
	if n := e.count(`SELECT COUNT(*) FROM comments WHERE body = ?`, "ping @former-user-1 please"); n != 1 {
		t.Fatal("mention not rewritten")
	}
	// signatures and the audit trail are kept unchanged, the chain verifies
	if n := e.count(`SELECT COUNT(*) FROM esignatures WHERE user_email = ? AND user_name = 'Anna Berg'`, anna.Email); n != 1 {
		t.Fatal("signature changed")
	}
	if v, err := e.st.VerifyAudit(ctx, e.tenant.ID); err != nil || !v.OK {
		t.Fatal(v, err)
	}
	if d := auditDetails(t, e, "user.erase"); len(d) != 1 || strings.Contains(d[0], anna.Email) || !strings.Contains(d[0], "Former user #1") {
		t.Fatal(d)
	}
	// the account cannot sign in, its token is gone
	e.anon().call("POST", "/api/v1/auth/login", map[string]string{"email": anna.Email, "password": testPassword}, 401, nil)
	tc.call("GET", "/api/v1/projects", nil, 401, nil)
	var list struct {
		Items []model.User `json:"items"`
	}
	a.call("GET", "/api/v1/users", nil, 200, &list)
	erased := 0
	for _, x := range list.Items {
		if x.Erased {
			erased++
		}
	}
	if erased != 1 {
		t.Fatal(erased)
	}
	// a second erasure gets the next number
	bo := e.member("bo@example.com", rbac.RoleViewer)
	a.call("POST", "/api/v1/users/"+bo.ID+"/erase", map[string]any{"confirm": bo.Email}, 200, &res)
	if res.Pseudonym != "Former user #2" {
		t.Fatal(res)
	}
	// the last super administrator cannot be erased
	two := mkUser(t, e.st, "root2@example.com", true)
	if n, _ := e.st.CountSuperAdmins(ctx); n != 2 {
		t.Fatal(n)
	}
	a.call("POST", "/api/v1/users/"+two.ID+"/erase", map[string]any{"confirm": two.Email}, 200, nil)
}
