package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestReportEmail(t *testing.T) {
	e, f := mailEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	base := "/api/v1/projects/" + fx.pid + "/reports"
	tess := e.member("tess@example.com", "tester")
	a.m("POST", "/api/v1/roles", map[string]any{"name": "outsider", "permissions": []string{"project.read", "export"}}, 201)
	out := e.member("out@example.com", "outsider")
	rec := a.m("GET", base+"/recipients", nil, 200)
	if len(items(rec)) != 3 || rec["addresses_allowed"] != true || rec["email_enabled"] != true {
		t.Fatal(rec)
	}
	body := map[string]any{"template": "requirements_coverage", "format": "csv", "recipients": []string{tess.ID, out.ID, "missing"}, "message": "Weekly status"}
	res := a.m("POST", base+"/email", body, 202)
	if res["queued"].(float64) != 1 || len(res["skipped"].([]any)) != 2 {
		t.Fatal(res)
	}
	ms := sent(e, f)
	if len(ms) != 1 || ms[0].To[0].Email != "tess@example.com" || len(ms[0].Attachments) != 1 || !strings.HasSuffix(ms[0].Attachments[0].Name, ".zip") ||
		!strings.Contains(ms[0].Text, "Weekly status") || ms[0].ReplyTo.Email != "admin@example.com" {
		t.Fatal(ms)
	}
	// saved definition and custom config, other addresses (admin has project.update)
	f.Reset()
	d := a.m("POST", base+"/definitions", map[string]any{"name": "Approved", "config": map[string]any{"entity": "requirement", "columns": []string{"key", "title"}}}, 201)
	a.m("POST", base+"/email", map[string]any{"definition_id": d["id"], "format": "pdf", "addresses": []string{" Boss@Partner.example ", "boss@partner.example"}}, 202)
	// nobody chosen may read tests
	a.m("POST", base+"/email", map[string]any{"custom": map[string]any{"entity": "test", "columns": []string{"key"}}, "format": "xlsx", "recipients": []string{out.ID}}, 400)
	a.m("POST", base+"/email", map[string]any{"custom": map[string]any{"entity": "run", "columns": []string{"name"}}, "format": "csv", "recipients": []string{tess.ID}}, 202)
	ms = sent(e, f)
	if len(ms) != 2 {
		t.Fatal(len(ms))
	}
	var pdf bool
	for _, m := range ms {
		if m.To[0].Email == "boss@partner.example" && len(m.To) == 1 && m.Attachments[0].ContentType == "application/pdf" {
			pdf = true
		}
	}
	if !pdf {
		t.Fatal(ms)
	}
	// validation
	for _, b := range []map[string]any{
		{"format": "csv", "recipients": []string{tess.ID}},
		{"template": "project_status", "definition_id": d["id"], "format": "csv", "recipients": []string{tess.ID}},
		{"template": "project_status", "format": "csv"},
		{"template": "project_status", "format": "csv", "addresses": []string{"not an address"}},
		{"template": "project_status", "format": "doc", "recipients": []string{tess.ID}},
		{"template": "project_status", "format": "csv", "recipients": []string{tess.ID}, "message": strings.Repeat("x", 4001)},
		{"custom": "x", "format": "csv", "recipients": []string{tess.ID}},
		{"template": "nope", "format": "csv", "recipients": []string{tess.ID}},
	} {
		a.m("POST", base+"/email", b, 400)
	}
	a.m("POST", base+"/email", "{", 400)
	many := make([]string, maxReportRecipients+1)
	for i := range many {
		many[i] = "x" + itoa(i)
	}
	a.m("POST", base+"/email", map[string]any{"template": "project_status", "format": "csv", "recipients": many}, 400)
	a.m("POST", base+"/email", map[string]any{"definition_id": "missing", "format": "csv", "recipients": []string{tess.ID}}, 404)
	a.m("POST", "/api/v1/projects/"+fx.otherPID+"/reports/email", map[string]any{"definition_id": d["id"], "format": "csv", "recipients": []string{tess.ID}}, 404)
	a.m("POST", "/api/v1/projects/missing/reports/email", map[string]any{"template": "project_status", "format": "csv", "recipients": []string{tess.ID}}, 404)
	a.m("GET", "/api/v1/projects/missing/reports/recipients", nil, 404)
	// a tester may send to members but not to other addresses
	tc := e.login("tess@example.com")
	if tc.m("GET", base+"/recipients", nil, 200)["addresses_allowed"] != false {
		t.Fatal("addresses allowed for a tester")
	}
	tc.m("POST", base+"/email", map[string]any{"template": "project_status", "format": "csv", "addresses": []string{"x@evil.example"}}, 403)
	// a failed delivery becomes an in-app notification of the sender
	f.Reset()
	f.Err = errors.New("550 relay denied")
	tc.m("POST", base+"/email", map[string]any{"template": "project_status", "format": "csv", "recipients": []string{e.admin.ID}}, 202)
	sent(e, f)
	n := tc.m("GET", "/api/v1/notifications", nil, 200)
	if len(items(n)) != 1 || !strings.Contains(items(n)[0]["body"].(string), "relay denied") {
		t.Fatal(n)
	}
	// rate limit per user
	f.Err = nil
	got429 := false
	for i := 0; i < 40 && !got429; i++ {
		res := tc.raw("POST", base+"/email", map[string]any{"template": "project_status", "format": "csv", "recipients": []string{e.admin.ID}})
		res.Body.Close()
		got429 = res.StatusCode == http.StatusTooManyRequests
	}
	if !got429 {
		t.Fatal("no rate limit")
	}
	sent(e, f)
	if len(items(a.m("GET", "/api/v1/audit?q=action:report.email", nil, 200))) < 3 {
		t.Fatal("audit")
	}
}

func TestReportSubscriptions(t *testing.T) {
	e, f := mailEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	base := "/api/v1/projects/" + fx.pid + "/reports"
	tess := e.member("tess@example.com", "tester")
	mia := e.member("mia@example.com", "manager")
	d := a.m("POST", base+"/definitions", map[string]any{"name": "Approved", "config": map[string]any{"entity": "requirement", "columns": []string{"key", "title"}}}, 201)
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) // Friday
	e.srv.Clock = func() time.Time { return now }
	mc := e.login("mia@example.com")
	sub := mc.m("POST", base+"/subscriptions", map[string]any{"definition_id": d["id"], "frequency": "weekly", "weekday": 1, "time": "08:00",
		"timezone": "Europe/Stockholm", "format": "csv", "recipients": []string{tess.ID, mia.ID}, "message": "Hej"}, 201)
	id := sub["id"].(string)
	if sub["next_run_at"] != "2026-11-23T07:00:00Z" || sub["owner_id"] != mia.ID || sub["definition_name"] != "Approved" {
		t.Fatal(sub)
	}
	// validation
	for _, b := range []map[string]any{
		{"frequency": "weekly", "recipients": []string{tess.ID}},
		{"definition_id": d["id"], "frequency": "hourly", "recipients": []string{tess.ID}},
		{"definition_id": d["id"], "time": "8am", "recipients": []string{tess.ID}},
		{"definition_id": d["id"], "timezone": "Mars", "recipients": []string{tess.ID}},
		{"definition_id": d["id"], "format": "doc", "recipients": []string{tess.ID}},
		{"definition_id": d["id"], "recipients": []string{}},
		{"definition_id": d["id"], "recipients": []string{"stranger"}},
		{"definition_id": d["id"], "addresses": []string{"bad"}},
		{"definition_id": d["id"], "recipients": []string{tess.ID}, "message": strings.Repeat("x", 4001)},
	} {
		mc.m("POST", base+"/subscriptions", b, 400)
	}
	mc.m("POST", base+"/subscriptions", "{", 400)
	mc.m("POST", base+"/subscriptions", map[string]any{"definition_id": "missing", "recipients": []string{tess.ID}}, 404)
	mc.m("POST", "/api/v1/projects/"+fx.otherPID+"/reports/subscriptions", map[string]any{"definition_id": d["id"], "recipients": []string{tess.ID}}, 404)
	mc.m("POST", "/api/v1/projects/missing/reports/subscriptions", map[string]any{"definition_id": d["id"], "recipients": []string{tess.ID}}, 404)
	// a tester may not change someone else's subscription; a viewer may not create one
	tc := e.login("tess@example.com")
	tc.m("PATCH", "/api/v1/reports/subscriptions/"+id, map[string]any{"enabled": false}, 403)
	tc.m("POST", base+"/subscriptions", map[string]any{"definition_id": d["id"], "recipients": []string{tess.ID}, "addresses": []string{"x@evil.example"}}, 403)
	e.member("vic@example.com", "viewer")
	e.login("vic@example.com").m("POST", base+"/subscriptions", map[string]any{"definition_id": d["id"], "recipients": []string{tess.ID}}, 403)
	list := a.m("GET", base+"/subscriptions", nil, 200)
	if len(items(list)) != 1 || list["email_enabled"] != true {
		t.Fatal(list)
	}
	a.m("GET", "/api/v1/projects/missing/reports/subscriptions", nil, 404)
	// not due yet
	if n, _ := e.srv.RunReportSubscriptions(ctx); n != 0 {
		t.Fatal(n)
	}
	// the server was down for three weeks: sent once, next run from now
	now = time.Date(2026, 12, 14, 12, 0, 0, 0, time.UTC)
	if n, err := e.srv.RunReportSubscriptions(ctx); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	if n, _ := e.srv.RunReportSubscriptions(ctx); n != 0 {
		t.Fatal("sent again", n)
	}
	ms := f.Messages()
	if len(ms) != 1 || len(ms[0].To) != 2 || !strings.Contains(ms[0].Text, "scheduled report") || ms[0].Headers["Auto-Submitted"] == "" {
		t.Fatal(ms)
	}
	x := items(a.m("GET", base+"/subscriptions", nil, 200))[0]
	if x["last_sent_at"] != "2026-12-14T12:00:00Z" || x["next_run_at"] != "2026-12-21T07:00:00Z" || x["last_error"] != "" {
		t.Fatal(x)
	}
	// update: monthly, an outside address (mia is a manager: project.update)
	up := mc.m("PATCH", "/api/v1/reports/subscriptions/"+id, map[string]any{"frequency": "monthly", "day_of_month": 1, "time": "06:30", "timezone": "UTC",
		"addresses": []string{"Ext@Example.org"}}, 200)
	if up["next_run_at"] != "2027-01-01T06:30:00Z" || up["addresses"].([]any)[0] != "ext@example.org" {
		t.Fatal(up)
	}
	mc.m("PATCH", "/api/v1/reports/subscriptions/"+id, map[string]any{"definition_id": "other"}, 400)
	mc.m("PATCH", "/api/v1/reports/subscriptions/"+id, map[string]any{"day_of_month": 30}, 400)
	mc.m("PATCH", "/api/v1/reports/subscriptions/"+id, "{", 400)
	mc.m("PATCH", "/api/v1/reports/subscriptions/missing", map[string]any{"enabled": true}, 404)
	// send now
	f.Reset()
	res := mc.m("POST", "/api/v1/reports/subscriptions/"+id+"/send", nil, 200)
	if res["last_error"] != "" || len(f.Messages()) != 1 || len(f.Messages()[0].To) != 3 {
		t.Fatal(res, f.Messages())
	}
	f.Err = errors.New("421 try later")
	res = mc.m("POST", "/api/v1/reports/subscriptions/"+id+"/send", nil, 200)
	if !strings.Contains(res["last_error"].(string), "421") || res["next_run_at"] != "2027-01-01T06:30:00Z" {
		t.Fatal(res)
	}
	f.Err = nil
	// the owner loses the export permission: the run records the error
	a.m("POST", "/api/v1/roles", map[string]any{"name": "noexport", "permissions": []string{"project.read", "requirement.read", "requirement.write"}}, 201)
	a.m("PATCH", "/api/v1/members/"+mia.ID, map[string]any{"role": "noexport"}, 200)
	now = time.Date(2027, 1, 1, 7, 0, 0, 0, time.UTC)
	f.Reset()
	if n, _ := e.srv.RunReportSubscriptions(ctx); n != 1 || len(f.Messages()) != 0 {
		t.Fatal("sent without permission")
	}
	x = items(a.m("GET", base+"/subscriptions", nil, 200))[0]
	if !strings.Contains(x["last_error"].(string), "export") || x["next_run_at"] != "2027-02-01T06:30:00Z" {
		t.Fatal(x)
	}
	// with export back but without project.update, outside addresses are dropped
	a.m("PATCH", "/api/v1/members/"+mia.ID, map[string]any{"role": "tester"}, 200)
	now = time.Date(2027, 2, 1, 7, 0, 0, 0, time.UTC)
	e.srv.RunJobs(ctx)
	if ms := f.Messages(); len(ms) != 1 || len(ms[0].To) != 2 {
		t.Fatal(ms)
	}
	// owner removed from the tenant, or disabled
	if _, err := e.srv.ownerPrincipal(ctx, e.tenant.ID, "missing"); err == nil {
		t.Fatal("missing owner")
	}
	u, _ := e.st.GetUser(ctx, tess.ID)
	u.Disabled = true
	_ = e.st.UpdateUser(ctx, u)
	if _, err := e.srv.ownerPrincipal(ctx, e.tenant.ID, tess.ID); err == nil {
		t.Fatal("disabled owner")
	}
	stranger := mkUser(t, e.st, "stranger@example.com", false)
	if _, err := e.srv.ownerPrincipal(ctx, e.tenant.ID, stranger.ID); err == nil {
		t.Fatal("non-member owner")
	}
	// disable, then delete
	a.m("PATCH", "/api/v1/reports/subscriptions/"+id, map[string]any{"enabled": false}, 200)
	if x := items(a.m("GET", base+"/subscriptions", nil, 200))[0]; x["next_run_at"] != nil {
		t.Fatal(x)
	}
	a.m("DELETE", "/api/v1/reports/subscriptions/"+id, nil, 204)
	a.m("DELETE", "/api/v1/reports/subscriptions/"+id, nil, 404)
	a.m("POST", "/api/v1/reports/subscriptions/"+id+"/send", nil, 404)
	// deleting the saved report deletes its subscriptions
	a.m("POST", base+"/subscriptions", map[string]any{"definition_id": d["id"], "frequency": "daily", "recipients": []string{e.admin.ID}}, 201)
	a.m("DELETE", "/api/v1/reports/definitions/"+d["id"].(string), nil, 204)
	if len(items(a.m("GET", base+"/subscriptions", nil, 200))) != 0 {
		t.Fatal("subscription of a deleted report")
	}
	// e-mail off
	e.srv.Mail = nil
	a.m("POST", "/api/v1/reports/subscriptions/x/send", nil, 400)
}
