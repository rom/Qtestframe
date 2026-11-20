package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
)

// mailEnv is an env whose server sends e-mail to a fake sender.
func mailEnv(t *testing.T) (*env, *mail.Fake) {
	t.Helper()
	e := newEnv(t, func(c *config.Config) {
		c.Email.From, c.Email.FromName = "qtf@example.test", "Qtestframe"
	})
	f := &mail.Fake{}
	e.srv.Mail = f
	return e, f
}

// sent waits for e-mails in flight and returns them.
func sent(e *env, f *mail.Fake) []*mail.Message {
	e.srv.WaitMail()
	return f.Messages()
}

func setPrefs(t *testing.T, c *client, prefs map[string]any) {
	t.Helper()
	c.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": prefs}, 200)
}

func TestEmailOff(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	if a.m("GET", "/api/v1/meta", nil, 200)["email_enabled"] != false {
		t.Fatal("email_enabled")
	}
	pid := a.project("OFF")
	a.m("POST", "/api/v1/admin/email/test", map[string]any{}, 400)
	a.m("POST", "/api/v1/members", map[string]any{"email": "new@example.com", "role": "tester", "invite": true}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/reports/email", map[string]any{"template": "project_status", "format": "csv", "recipients": []string{e.admin.ID}}, 400)
	// notifications are stored, nothing else happens
	setPrefs(t, a, map[string]any{"email_notifications": "immediate"})
	if _, err := e.srv.RunDigest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.srv.RunReportSubscriptions(context.Background()); n != 0 {
		t.Fatal(n)
	}
	e.srv.RunJobs(context.Background())
	e.srv.StartJobs(context.Background())
	a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"email_notifications": "weekly"}}, 400)
}

func TestTestEmail(t *testing.T) {
	e, f := mailEnv(t)
	a := e.login("admin@example.com")
	if a.m("GET", "/api/v1/meta", nil, 200)["email_enabled"] != true {
		t.Fatal("email_enabled")
	}
	res := a.m("POST", "/api/v1/admin/email/test", nil, 200)
	if res["ok"] != true || res["to"] != "admin@example.com" {
		t.Fatal(res)
	}
	ms := f.Messages()
	if len(ms) != 1 || ms[0].To[0].Email != "admin@example.com" || ms[0].From.Email != "qtf@example.test" || !strings.Contains(ms[0].HTML, "<html") {
		t.Fatal(ms)
	}
	f.Err = errors.New("535 authentication failed")
	res = a.m("POST", "/api/v1/admin/email/test", nil, 200)
	if res["ok"] != false || !strings.Contains(res["error"].(string), "535") {
		t.Fatal(res)
	}
	e.member("tess@example.com", "tester")
	e.login("tess@example.com").m("POST", "/api/v1/admin/email/test", nil, 403)
	audit := a.m("GET", "/api/v1/audit?q=action:email.test", nil, 200)
	if len(items(audit)) != 2 {
		t.Fatal(audit)
	}
}

func TestImmediateNotificationMail(t *testing.T) {
	e, f := mailEnv(t)
	a := e.login("admin@example.com")
	anna := e.member("anna@example.com", "tester")
	ac := e.login("anna@example.com")
	ac.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"email_notifications": "weekly"}}, 400)
	setPrefs(t, ac, map[string]any{"email_notifications": "immediate", "language": "sv"})
	setPrefs(t, a, map[string]any{"email_notifications": "immediate"})
	pid := a.project("NTF")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "owner": "anna@example.com", "description": "Ask @admin@example.com"}, 201)
	ms := sent(e, f)
	if len(ms) != 1 {
		t.Fatalf("got %d mails", len(ms))
	}
	m := ms[0]
	if m.To[0].Email != "anna@example.com" || !strings.Contains(m.Subject, "Login") || !strings.Contains(m.Text, "Tilldelad dig") ||
		!strings.Contains(m.Text, "http://example.test/p/"+pid+"/requirements?item="+rq["id"].(string)) || !strings.Contains(m.Text, "/profile#email") ||
		m.Headers["List-Unsubscribe"] == "" {
		t.Fatalf("%+v\n%s", m, m.Text)
	}
	// the notification is marked as e-mailed: it is not in a digest
	list, err := e.st.DigestNotifications(context.Background(), anna.ID, 10)
	if err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	// anna's own action does not mail her
	f.Reset()
	ac.m("POST", "/api/v1/requirements/"+rq["id"].(string)+"/comments", map[string]any{"body": "Note to self @anna"}, 201)
	if ms := sent(e, f); len(ms) != 0 {
		t.Fatal(ms)
	}

	// a rolled back request transaction sends nothing
	f.Reset()
	p := &Principal{User: e.admin, Tenant: e.tenant, Role: "admin"}
	n := model.Notification{ProjectID: pid, Event: "mention", Title: "rolled back"}
	err = e.st.InTx(context.Background(), func(ctx context.Context) error {
		e.srv.notifyUsers(ctx, p, []string{anna.ID}, n)
		return errors.New("fail")
	})
	if err == nil || len(sent(e, f)) != 0 {
		t.Fatal("mail sent for a rolled back transaction")
	}
	_ = e.st.InTx(context.Background(), func(ctx context.Context) error {
		e.srv.notifyUsers(ctx, p, []string{anna.ID, e.admin.ID}, n)
		if len(f.Messages()) != 0 {
			t.Error("sent inside the transaction")
		}
		return nil
	})
	if ms := sent(e, f); len(ms) != 1 || ms[0].To[0].Email != "anna@example.com" {
		t.Fatal(ms)
	}
	// a failed delivery leaves the notification for the digest
	f.Reset()
	f.Err = errors.New("down")
	e.srv.notifyUsers(context.Background(), p, []string{anna.ID}, model.Notification{Event: "custom.event", Title: "Hello"})
	sent(e, f)
	if list, _ := e.st.DigestNotifications(context.Background(), anna.ID, 10); len(list) != 1 {
		t.Fatal(list)
	}
	if eventLabel("en", "custom.event") != "custom.event" || eventLabel("sv", "mention") != "Du har nämnts" {
		t.Fatal("labels")
	}
	if u := e.srv.notificationURL(&model.Notification{}); u != "http://example.test/notifications" {
		t.Fatal(u)
	}
	for typ, want := range map[string]string{"test": "/tests?item=x", "defect": "/defects?item=x", "risk": "/risks?risk=x", "execution": "/runs", "": "/dashboard"} {
		if u := e.srv.notificationURL(&model.Notification{ProjectID: "p", EntityType: typ, EntityID: "x"}); !strings.HasSuffix(u, want) {
			t.Error(typ, u)
		}
	}
}

func TestDigest(t *testing.T) {
	e, f := mailEnv(t)
	ctx := context.Background()
	bob := e.member("bob@example.com", "tester")
	bc := e.login("bob@example.com")
	setPrefs(t, bc, map[string]any{"email_notifications": "digest"})
	p := &Principal{User: e.admin, Tenant: e.tenant, Role: "admin"}
	now := time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC) // before the digest hour (7)
	e.srv.Clock = func() time.Time { return now }
	// first start: the digest of yesterday 07:00 is caught up once
	ran, err := e.srv.RunDigest(ctx)
	if err != nil || !ran || len(f.Messages()) != 0 {
		t.Fatal(ran, err)
	}
	if ran, _ := e.srv.RunDigest(ctx); ran {
		t.Fatal("ran twice")
	}
	for i := 0; i < 3; i++ {
		e.srv.notifyUsers(ctx, p, []string{bob.ID}, model.Notification{Event: "mention", Title: "Mention " + itoa(i), Body: "body"})
	}
	if len(sent(e, f)) != 0 {
		t.Fatal("digest users get no immediate mail")
	}
	now = now.Add(2 * time.Hour) // 08:00: due
	e.srv.RunJobs(ctx)
	ms := f.Messages()
	if len(ms) != 1 || ms[0].To[0].Email != "bob@example.com" || !strings.Contains(ms[0].Subject, "3 new notifications") || !strings.Contains(ms[0].Text, "Mention 2") {
		t.Fatal(ms)
	}
	// the next day: only new notifications, and none → no mail
	now = now.Add(24 * time.Hour)
	f.Reset()
	if ran, _ := e.srv.RunDigest(ctx); !ran || len(f.Messages()) != 0 {
		t.Fatal("empty digest sent")
	}
	e.srv.notifyUsers(ctx, p, []string{bob.ID}, model.Notification{Event: "assigned", Title: "One"})
	now = now.Add(24 * time.Hour)
	if ran, _ := e.srv.RunDigest(ctx); !ran {
		t.Fatal("not run")
	}
	if ms := f.Messages(); len(ms) != 1 || ms[0].Subject != "Qtestframe: 1 new notification" {
		t.Fatal(ms)
	}
	// read notifications are not mailed; a failed delivery is retried next time
	f.Reset()
	e.srv.notifyUsers(ctx, p, []string{bob.ID}, model.Notification{Event: "assigned", Title: "Read"})
	bc.m("POST", "/api/v1/notifications/read", map[string]any{}, 200)
	for i := 0; i < digestLimit+2; i++ {
		e.srv.notifyUsers(ctx, p, []string{bob.ID}, model.Notification{Event: "mention", Title: "Many"})
	}
	f.Err = errors.New("down")
	now = now.Add(24 * time.Hour)
	e.srv.RunDigest(ctx)
	f.Err = nil
	now = now.Add(24 * time.Hour)
	e.srv.RunDigest(ctx)
	ms = f.Messages()
	if len(ms) != 1 || len(strings.Split(ms[0].Text, "* Many")) != digestLimit+1 || !strings.Contains(ms[0].Text, "more unread") {
		t.Fatal(len(ms))
	}
	if d := e.srv.digestDue(time.Date(2026, 1, 1, 7, 0, 0, 0, time.UTC)); d.Day() != 1 {
		t.Fatal(d)
	}
}

func inviteToken(t *testing.T, m *mail.Message) string {
	t.Helper()
	i := strings.Index(m.Text, "/invite/")
	if i < 0 {
		t.Fatalf("no link in %s", m.Text)
	}
	return strings.Fields(m.Text[i+len("/invite/"):])[0]
}

func TestInvitations(t *testing.T) {
	e, f := mailEnv(t)
	a := e.login("admin@example.com")
	a.m("POST", "/api/v1/members", map[string]any{"email": "nina@example.com", "role": "tester", "invite": true, "password": "x"}, 400)
	a.m("POST", "/api/v1/members", map[string]any{"email": "bad\r\naddress", "role": "tester", "invite": true}, 400)
	res := a.m("POST", "/api/v1/members", map[string]any{"email": "Nina@Example.com", "role": "tester", "invite": true}, 201)
	inv := res["invitation"].(map[string]any)
	uid := res["user_id"].(string)
	if inv["expired"] != false || inv["invited_by"] != "admin@example.com" {
		t.Fatal(res)
	}
	ms := sent(e, f)
	if len(ms) != 1 || ms[0].To[0].Email != "nina@example.example" && ms[0].To[0].Email != "nina@example.com" || ms[0].ReplyTo == nil || !strings.Contains(ms[0].Subject, "Acme Corp") {
		t.Fatal(ms)
	}
	tok := inviteToken(t, ms[0])
	// the members list shows the invitation; the token is not stored
	var found bool
	for _, m := range items(a.m("GET", "/api/v1/members", nil, 200)) {
		if m["user_id"] == uid {
			found = m["invitation"] != nil
		}
	}
	if !found {
		t.Fatal("invitation not listed")
	}
	if i, _ := e.st.OpenInvitation(context.Background(), e.tenant.ID, uid); i == nil || i.TokenHash == tok || i.TokenHash == "" {
		t.Fatal("token stored in plain text")
	}
	// the invited person cannot sign in yet
	e.anon().call("POST", "/api/v1/auth/login", map[string]string{"email": "nina@example.com", "password": ""}, 401, nil)
	// public page
	anon := e.anon()
	info := anon.m("GET", "/api/v1/invitations/"+tok, nil, 200)
	if info["email"] != "nina@example.com" || info["tenant_name"] != "Acme Corp" || info["role"] != "tester" || info["sso"] != false || info["display_name"] != "" {
		t.Fatal(info)
	}
	anon.m("GET", "/api/v1/invitations/nope", nil, 404)
	anon.m("POST", "/api/v1/invitations/"+tok+"/accept", map[string]any{"display_name": "Nina", "password": "short"}, 400)
	anon.m("POST", "/api/v1/invitations/"+tok+"/accept", map[string]any{"display_name": "Nina"}, 400)
	anon.m("POST", "/api/v1/invitations/"+tok+"/accept", map[string]any{"display_name": " ", "password": testPassword}, 400)
	anon.m("POST", "/api/v1/invitations/"+tok+"/accept", nil, 400)
	me := anon.m("POST", "/api/v1/invitations/"+tok+"/accept", map[string]any{"display_name": " Nina N ", "password": testPassword}, 200)
	if me["user"].(map[string]any)["display_name"] != "Nina N" || me["tenant"].(map[string]any)["id"] != e.tenant.ID || me["csrf_token"] == "" {
		t.Fatal(me)
	}
	anon.csrf = me["csrf_token"].(string)
	anon.m("GET", "/api/v1/projects", nil, 200)
	// single use
	e.anon().m("GET", "/api/v1/invitations/"+tok, nil, 404)
	e.anon().m("POST", "/api/v1/invitations/"+tok+"/accept", map[string]any{"display_name": "X", "password": testPassword}, 404)
	e.login("nina@example.com")
	a.m("POST", "/api/v1/members/"+uid+"/invitation", nil, 409) // already uses the account
	a.m("DELETE", "/api/v1/members/"+uid+"/invitation", nil, 404)
	// an existing user who is added gets a notice, no invitation
	f.Reset()
	e.member("old@example.com", "viewer")
	otherTenant, _ := e.st.CreateTenant(context.Background(), "other", "Other")
	_ = otherTenant
	res = a.m("POST", "/api/v1/members", map[string]any{"email": "old@example.com", "role": "reviewer", "invite": true}, 201)
	if res["invitation"] != nil {
		t.Fatal(res)
	}
	if ms := sent(e, f); len(ms) != 1 || !strings.Contains(ms[0].Subject, "added") {
		t.Fatal(ms)
	}
	// audit trail
	acts := map[string]bool{}
	for _, x := range items(a.m("GET", "/api/v1/audit", nil, 200)) {
		acts[x["action"].(string)] = true
	}
	for _, want := range []string{"invitation.send", "invitation.accept", "member.add"} {
		if !acts[want] {
			t.Error("missing audit", want)
		}
	}
}

func TestInvitationResendRevokeExpire(t *testing.T) {
	e, f := mailEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	uid := a.m("POST", "/api/v1/members", map[string]any{"email": "omar@example.com", "role": "viewer", "invite": true}, 201)["user_id"].(string)
	old := inviteToken(t, sent(e, f)[0])
	f.Reset()
	a.m("POST", "/api/v1/members/"+uid+"/invitation", nil, 200)
	tok := inviteToken(t, sent(e, f)[0])
	if tok == old {
		t.Fatal("same token")
	}
	e.anon().m("GET", "/api/v1/invitations/"+old, nil, 404)
	e.anon().m("GET", "/api/v1/invitations/"+tok, nil, 200)
	// expired
	real := e.st.Now()
	e.st.SetClock(func() time.Time { return real.Add(InvitationTTL + time.Minute) })
	e.anon().m("GET", "/api/v1/invitations/"+tok, nil, 410)
	if i, _ := e.st.OpenInvitation(ctx, e.tenant.ID, uid); i == nil || !e.srv.viewInvitation(i).Expired {
		t.Fatal("not expired")
	}
	e.st.SetClock(func() time.Time { return real })
	// revoke: the link stops working and the person leaves the tenant
	res := a.m("DELETE", "/api/v1/members/"+uid+"/invitation", nil, 200)
	if res["removed"] != true {
		t.Fatal(res)
	}
	e.anon().m("GET", "/api/v1/invitations/"+tok, nil, 404)
	a.m("POST", "/api/v1/members/"+uid+"/invitation", nil, 404)
	// invited again later (the account was never used)
	f.Reset()
	res = a.m("POST", "/api/v1/members", map[string]any{"email": "omar@example.com", "role": "viewer", "invite": true}, 201)
	if res["invitation"] == nil || len(sent(e, f)) != 1 {
		t.Fatal(res)
	}
	// a member removed by other means: the link no longer works
	tok = inviteToken(t, f.Messages()[0])
	a.m("DELETE", "/api/v1/members/"+uid, nil, 204)
	e.anon().m("GET", "/api/v1/invitations/"+tok, nil, 404)
	// resend needs e-mail
	e.srv.Mail = nil
	a.m("POST", "/api/v1/members/"+uid+"/invitation", nil, 400)
	e.srv.Mail = f
	_ = ctx
	// rate limit of the public endpoints
	anon := e.anon()
	for i := 0; i < 12; i++ {
		res := anon.raw("GET", "/api/v1/invitations/bad-token", nil)
		res.Body.Close()
		if res.StatusCode == http.StatusTooManyRequests {
			return
		}
	}
	t.Fatal("no rate limit")
}

func TestInvitationSSO(t *testing.T) {
	e, f := mailEnv(t)
	a := e.login("admin@example.com")
	uid := a.m("POST", "/api/v1/members", map[string]any{"email": "sso@example.com", "role": "tester", "invite": true, "sso": true}, 201)["user_id"].(string)
	m := sent(e, f)[0]
	if !strings.Contains(m.Text, "organisation's account") {
		t.Fatal(m.Text)
	}
	tok := inviteToken(t, m)
	if info := e.anon().m("GET", "/api/v1/invitations/"+tok, nil, 200); info["sso"] != true {
		t.Fatal(info)
	}
	e.anon().m("POST", "/api/v1/invitations/"+tok+"/accept", map[string]any{"display_name": "S", "password": testPassword}, 400)
	// signing in (here: as the user) accepts the invitation
	u, _ := e.st.GetUser(context.Background(), uid)
	p := &Principal{User: u, Tenant: e.tenant}
	e.srv.acceptOnSignIn(context.Background(), p)
	e.anon().m("GET", "/api/v1/invitations/"+tok, nil, 404)
	for _, x := range items(a.m("GET", "/api/v1/members", nil, 200)) {
		if x["user_id"] == uid && x["invitation"] != nil {
			t.Fatal(x)
		}
	}
	if roleLabel("sv", "tester") != "testare" || roleLabel("en", "custom") != "custom" {
		t.Fatal("role labels")
	}
}
