package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// chatFake is a chat service: it records the bodies it gets and answers
// with status (default 200).
type chatFake struct {
	mu     sync.Mutex
	bodies []map[string]any
	paths  []string
	status int
}

func newChatFake(t *testing.T) (*chatFake, *httptest.Server) {
	f := &chatFake{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		f.mu.Lock()
		f.bodies = append(f.bodies, m)
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		st := f.status
		f.mu.Unlock()
		if st != 0 {
			w.WriteHeader(st)
			_, _ = io.WriteString(w, "no_service")
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *chatFake) take() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.bodies
	f.bodies = nil
	return out
}

func (f *chatFake) set(status int) {
	f.mu.Lock()
	f.status = status
	f.mu.Unlock()
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestChatChannels(t *testing.T) {
	e := newEnv(t)
	fake, srv := newChatFake(t)
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "CHT")
	base := "/api/v1/projects/" + f.pid + "/chat-channels"

	ev := a.m("GET", "/api/v1/chat-events", nil, 200)
	if len(ev["items"].([]any)) != len(model.WebhookEvents) || !strings.Contains(jsonText(ev["groups"]), `"id":"waivers"`) || !strings.Contains(jsonText(ev["items"]), `"sv":"Defekt rapporterad"`) {
		t.Fatal(ev)
	}

	// validation
	a.m("POST", base, map[string]any{"name": "S", "provider": "irc", "url": srv.URL, "events": []string{"*"}}, 400)
	a.m("POST", base, map[string]any{"name": "S", "provider": "slack", "events": []string{"*"}}, 400)
	a.m("POST", base, map[string]any{"name": "S", "provider": "slack", "url": "ftp://x", "events": []string{"*"}}, 400)
	a.m("POST", base, map[string]any{"name": "S", "provider": "slack", "url": srv.URL, "events": []string{"nope"}}, 400)
	a.m("POST", base, map[string]any{"name": "S", "provider": "slack", "url": srv.URL, "events": []string{"*"}, "language": "de"}, 400)
	a.m("POST", base, map[string]any{"name": "S", "provider": "slack", "url": srv.URL, "events": []string{"*"}, "digest_minutes": 5000}, 400)
	a.m("POST", base, map[string]any{"name": "S", "provider": "slack", "url": srv.URL, "events": []string{"*"}, "filters": map[string]any{"min_priority": "urgent"}}, 400)
	a.m("POST", base, map[string]any{"name": "S", "provider": "slack", "url": srv.URL, "events": []string{"*"}, "filters": map[string]any{"release_id": "missing"}}, 400)
	a.m("POST", base, map[string]any{"name": "M", "provider": "matrix", "homeserver": srv.URL, "room": "general", "token": "t", "events": []string{"*"}}, 400)

	secretURL := srv.URL + "/services/T1/B1/secret-part"
	ch := a.m("POST", base, map[string]any{"name": "QA Slack", "provider": "slack", "url": secretURL, "events": []string{"defect.created", "risk.created"},
		"filters": map[string]any{"min_priority": "high"}, "enabled": true}, 201)
	id := ch["id"].(string)
	if ch["url"] != nil || ch["has_secret"] != true || ch["target"] != strings.TrimPrefix(srv.URL, "http://") {
		t.Fatal(ch)
	}
	list := a.m("GET", base, nil, 200)
	if l := items(list); len(l) != 1 || strings.Contains(jsonText(l), "secret-part") || list["restricted"] != false || list["base_url"] != true {
		t.Fatal(list)
	}

	// a defect of high priority is posted, a low one is filtered out
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "Crash on login", "description": "Stack trace attached", "priority": "low"}, 201)
	d := a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "Data loss", "description": "Saved form is empty", "priority": "critical"}, 201)
	// an event the channel did not choose
	a.m("POST", "/api/v1/requirements/"+f.r1+"/comments", map[string]any{"body": "hi"}, 201)
	e.srv.WaitWebhooks()
	got := fake.take()
	if len(got) != 1 {
		t.Fatal(got)
	}
	s := jsonText(got[0])
	for _, want := range []string{"Defect reported", d["key"].(string) + " Data loss", "http://example.test/p/" + f.pid + "/defects?item=" + d["id"].(string), "Priority: critical", "Saved form is empty", "By: admin"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%q not in %s", want, s)
		}
	}
	log := items(a.m("GET", "/api/v1/chat-channels/"+id+"/deliveries", nil, 200))
	if len(log) != 1 || log[0]["status"] != "sent" || log[0]["status_code"].(float64) != 200 || log[0]["event"] != "defect.created" {
		t.Fatal(log)
	}

	// test message: also to a channel that is off; other languages
	a.m("PATCH", "/api/v1/chat-channels/"+id, map[string]any{"enabled": false, "language": "sv"}, 200)
	res := a.m("POST", "/api/v1/chat-channels/"+id+"/test", nil, 200)
	if res["ok"] != true || res["status"].(float64) != 200 {
		t.Fatal(res)
	}
	if got := fake.take(); len(got) != 1 || !strings.Contains(jsonText(got[0]), "Testmeddelande") || !strings.Contains(jsonText(got[0]), "/settings") {
		t.Fatal(got)
	}
	// the URL is write-only: an update without it keeps it; the provider is fixed
	a.m("PATCH", "/api/v1/chat-channels/"+id, map[string]any{"provider": "discord"}, 400)
	up := a.m("PATCH", "/api/v1/chat-channels/"+id, map[string]any{"name": "QA", "enabled": true, "url": ""}, 200)
	if up["name"] != "QA" || up["url"] != nil || up["has_secret"] != true || up["language"] != "sv" {
		t.Fatal(up)
	}
	stored, _ := e.srv.Store.GetChatChannel(context.Background(), tenantOf(t, e), id)
	if stored == nil || stored.URL != secretURL {
		t.Fatal(stored)
	}

	// permissions: testers may not see or change channels
	e.member("tester@example.com", "tester")
	ts := e.login("tester@example.com")
	ts.m("GET", base, nil, 403)
	ts.m("POST", "/api/v1/chat-channels/"+id+"/test", nil, 403)
	// a manager may not let a channel show restricted content
	e.member("lead@example.com", "manager")
	lead := e.login("lead@example.com")
	lead.m("POST", base, map[string]any{"name": "X", "provider": "discord", "url": srv.URL, "events": []string{"*"}, "restricted_content": true}, 403)
	lead.m("PATCH", "/api/v1/chat-channels/"+id, map[string]any{"restricted_content": true}, 403)

	a.m("DELETE", "/api/v1/chat-channels/"+id, nil, 204)
	a.m("DELETE", "/api/v1/chat-channels/"+id, nil, 404)
}

func tenantOf(t *testing.T, e *env) string {
	t.Helper()
	ten, err := e.srv.Store.GetTenantBySlug(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	return ten.ID
}

func TestChatFiltersAndRestricted(t *testing.T) {
	e := newEnv(t)
	fake, srv := newChatFake(t)
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "CHF")
	base := "/api/v1/projects/" + f.pid + "/chat-channels"
	rel := a.m("POST", "/api/v1/projects/"+f.pid+"/releases", map[string]any{"name": "R1", "version": "1.0"}, 201)
	a.m("POST", base, map[string]any{"name": "Failures", "provider": "mattermost", "url": srv.URL + "/fail", "events": []string{"*"},
		"filters": map[string]any{"failed_only": true, "release_id": rel["id"]}, "enabled": true}, 201)

	// a run of another release and a passed result are left out
	run0 := a.m("POST", "/api/v1/projects/"+f.pid+"/runs", map[string]any{"name": "No release", "test_ids": []string{f.t1}}, 201)
	run := a.m("POST", "/api/v1/projects/"+f.pid+"/runs", map[string]any{"name": "Nightly", "test_ids": []string{f.t1, f.t2}, "release_id": rel["id"]}, 201)
	execs := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))
	a.m("PATCH", "/api/v1/executions/"+execs[0]["id"].(string), map[string]any{"status": "passed"}, 200)
	a.m("PATCH", "/api/v1/executions/"+execs[1]["id"].(string), map[string]any{"status": "failed"}, 200)
	ex0 := items(a.m("GET", "/api/v1/runs/"+run0["id"].(string)+"/executions", nil, 200))
	a.m("PATCH", "/api/v1/executions/"+ex0[0]["id"].(string), map[string]any{"status": "failed"}, 200)
	e.srv.WaitWebhooks()
	// the failed result, and the run that completed with it
	// (posted in parallel: in any order)
	got := fake.take()
	if len(got) == 2 && strings.Contains(jsonText(got[0]), "Test run completed") {
		got[0], got[1] = got[1], got[0]
	}
	if len(got) != 2 || !strings.Contains(jsonText(got[1]), "1 passed, 1 failed") || !strings.Contains(jsonText(got[1]), "#C62828") {
		t.Fatal(len(got), got)
	}
	s := jsonText(got[0])
	if !strings.Contains(s, "Test result recorded") || !strings.Contains(s, execs[1]["test_key"].(string)) || !strings.Contains(s, "/runs/"+run["id"].(string)+"/exec/"+execs[1]["id"].(string)) || !strings.Contains(s, `"value":"failed"`) {
		t.Fatal(s)
	}

	// a restricted project: only key, status and link, unless an
	// organisation administrator allowed the content
	a.m("PUT", "/api/v1/projects/"+f.pid+"/access", map[string]any{"access": "restricted"}, 200)
	a.m("POST", base, map[string]any{"name": "Minimal", "provider": "discord", "url": srv.URL + "/min", "events": []string{"risk.created"}, "enabled": true}, 201)
	a.m("POST", base, map[string]any{"name": "Full", "provider": "discord", "url": srv.URL + "/full", "events": []string{"risk.created"}, "enabled": true, "restricted_content": true}, 201)
	if l := a.m("GET", base, nil, 200); l["restricted"] != true || l["can_allow_restricted"] != true {
		t.Fatal(l)
	}
	risk := a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "Secret merger plan", "likelihood": 4, "impact": 5}, 201)
	e.srv.WaitWebhooks()
	got = fake.take()
	if len(got) != 2 {
		t.Fatal(got)
	}
	full, min := jsonText(got[0]), jsonText(got[1])
	if strings.Contains(min, "Secret merger") {
		min, full = full, min
	}
	if strings.Contains(min, "Secret merger") || !strings.Contains(min, risk["key"].(string)) || !strings.Contains(full, "Secret merger plan") || !strings.Contains(full, "critical") {
		t.Fatal(min, full)
	}
}

func TestChatDigestRetryAndDisable(t *testing.T) {
	e := newEnv(t)
	fake, srv := newChatFake(t)
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "CHD")
	base := "/api/v1/projects/" + f.pid + "/chat-channels"
	now := time.Now().UTC()
	clock := func() time.Time { return now }
	e.srv.Store.SetClock(func() time.Time { return clock() })
	advance := func(d time.Duration) { now = now.Add(d) }
	ctx := context.Background()

	// digest: two events, one post after 10 minutes
	dg := a.m("POST", base, map[string]any{"name": "Digest", "provider": "teams", "url": srv.URL + "/digest", "events": []string{"risk.created"}, "digest_minutes": 10, "enabled": true}, 201)
	a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "Vendor lock-in"}, 201)
	a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "Key person"}, 201)
	e.srv.WaitWebhooks()
	if l := items(a.m("GET", base, nil, 200)); l[0]["pending"].(float64) != 2 || l[0]["next_digest_at"] == nil {
		t.Fatal(l)
	}
	if _, err := e.srv.RunChatJobs(ctx); err != nil || len(fake.take()) != 0 {
		t.Fatal("digest posted too early", err)
	}
	advance(11 * time.Minute)
	if n, err := e.srv.RunChatJobs(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	got := fake.take()
	if len(got) != 1 || !strings.Contains(jsonText(got[0]), "2 events in CHD") || !strings.Contains(jsonText(got[0]), "Vendor lock-in") || !strings.Contains(jsonText(got[0]), "Key person") {
		t.Fatal(got)
	}
	if l := items(a.m("GET", "/api/v1/chat-channels/"+dg["id"].(string)+"/deliveries", nil, 200)); len(l) != 1 || l[0]["event"] != "digest" || l[0]["count"].(float64) != 2 {
		t.Fatal(l)
	}

	// retries with backoff: a server error is tried again 1, 2, 4, 8, 16 minutes later
	ch := a.m("POST", base, map[string]any{"name": "Flaky", "provider": "slack", "url": srv.URL + "/flaky", "events": []string{"defect.created"}, "enabled": true}, 201)
	id := ch["id"].(string)
	fake.set(500)
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "D1"}, 201)
	e.srv.WaitWebhooks()
	deliveries := func() []map[string]any {
		return items(a.m("GET", "/api/v1/chat-channels/"+id+"/deliveries", nil, 200))
	}
	if l := deliveries(); len(l) != 1 || l[0]["status"] != "retry" || l[0]["attempts"].(float64) != 1 || l[0]["status_code"].(float64) != 500 {
		t.Fatal(l)
	}
	for i, wait := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute} {
		// not before the backoff has passed
		advance(wait - 10*time.Second)
		if n, _ := e.srv.RunChatJobs(ctx); n != 0 || deliveries()[0]["attempts"].(float64) != float64(i+1) {
			t.Fatal("retried too early", i)
		}
		advance(10 * time.Second)
		_, _ = e.srv.RunChatJobs(ctx)
		if l := deliveries(); l[0]["attempts"].(float64) != float64(i+2) {
			t.Fatal(i, l)
		}
	}
	if l := deliveries(); l[0]["status"] != "failed" || !strings.Contains(l[0]["error"].(string), "HTTP 500") || strings.Contains(l[0]["error"].(string), "flaky") {
		t.Fatal(l)
	}
	// a refused URL is not retried; after five failed deliveries in a row
	// the channel turns itself off and the administrators hear about it
	fake.set(404)
	for range model.ChatMaxFailures - 1 {
		a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "D"}, 201)
		e.srv.WaitWebhooks()
	}
	c := items(a.m("GET", base, nil, 200))
	var flaky map[string]any
	for _, x := range c {
		if x["id"] == id {
			flaky = x
		}
	}
	if flaky["enabled"] != false || !strings.Contains(flaky["disabled_reason"].(string), "HTTP 404") || flaky["failures"].(float64) != model.ChatMaxFailures {
		t.Fatal(flaky)
	}
	n := a.m("GET", "/api/v1/notifications", nil, 200)
	if !strings.Contains(jsonText(n), `"event":"chat.disabled"`) || !strings.Contains(jsonText(n), "Flaky") {
		t.Fatal(n)
	}
	// while off, nothing is posted; turning it on clears the failures
	fake.set(0)
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "D6"}, 201)
	e.srv.WaitWebhooks()
	if got := fake.take(); len(got) != 0 && strings.Contains(jsonText(got), "D6") {
		t.Fatal(got)
	}
	if on := a.m("PATCH", "/api/v1/chat-channels/"+id, map[string]any{"enabled": true}, 200); on["failures"].(float64) != 0 || on["disabled_reason"] != "" {
		t.Fatal(on)
	}
	fake.take()
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "D7"}, 201)
	e.srv.WaitWebhooks()
	if got := fake.take(); len(got) != 1 {
		t.Fatal(got)
	}
}

func TestChatLogKeepsFifty(t *testing.T) {
	e := newEnv(t)
	_, srv := newChatFake(t)
	a := e.login("admin@example.com")
	pid := a.project("CHL")
	ch := a.m("POST", "/api/v1/projects/"+pid+"/chat-channels", map[string]any{"name": "Busy", "provider": "googlechat", "url": srv.URL, "events": []string{"risk.created"}, "enabled": true}, 201)
	for range 53 {
		a.m("POST", "/api/v1/chat-channels/"+ch["id"].(string)+"/test", nil, 200)
	}
	if l := items(a.m("GET", "/api/v1/chat-channels/"+ch["id"].(string)+"/deliveries", nil, 200)); len(l) != 50 {
		t.Fatal(len(l))
	}
}

func TestChatMatrix(t *testing.T) {
	e := newEnv(t)
	fake, srv := newChatFake(t)
	a := e.login("admin@example.com")
	pid := a.project("CHM")
	ch := a.m("POST", "/api/v1/projects/"+pid+"/chat-channels", map[string]any{"name": "Matrix", "provider": "matrix", "homeserver": srv.URL, "room": "!qa:example.test",
		"token": "mx-secret-token", "events": []string{"*"}, "enabled": true}, 201)
	if ch["token"] != nil || ch["target"] != "!qa:example.test @ "+strings.TrimPrefix(srv.URL, "http://") {
		t.Fatal(ch)
	}
	if res := a.m("POST", "/api/v1/chat-channels/"+ch["id"].(string)+"/test", nil, 200); res["ok"] != true {
		t.Fatal(res)
	}
	got := fake.take()
	fake.mu.Lock()
	path := fake.paths[len(fake.paths)-1]
	fake.mu.Unlock()
	if len(got) != 1 || got[0]["msgtype"] != "m.notice" || !strings.HasPrefix(path, "PUT /_matrix/client/v3/rooms/!qa:example.test/send/m.room.message/") {
		t.Fatal(path, got)
	}
	// an update of the room keeps the token
	a.m("PATCH", "/api/v1/chat-channels/"+ch["id"].(string), map[string]any{"room": "#qa:example.test"}, 200)
	c, err := e.srv.Store.GetChatChannel(context.Background(), tenantOf(t, e), ch["id"].(string))
	if err != nil || c.Token != "mx-secret-token" || c.Room != "#qa:example.test" {
		t.Fatal(c, err)
	}
}
