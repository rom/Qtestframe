package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/rbac"
)

// sseEvent is one parsed Server-Sent Event.
type sseEvent struct {
	id, event string
	data      map[string]any
	raw       string
}

type stream struct {
	t      *testing.T
	res    *http.Response
	events chan sseEvent
	pings  chan struct{}
}

// openStream opens GET /api/v1/events/stream for a client.
func (c *client) openStream(headers map[string]string, query string) *stream {
	c.e.t.Helper()
	req, err := http.NewRequest("GET", c.e.hs.URL+"/api/v1/events/stream"+query, nil)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	s := &stream{t: c.e.t, res: res, events: make(chan sseEvent, 100), pings: make(chan struct{}, 100)}
	if res.StatusCode != 200 {
		return s
	}
	go func() {
		defer close(s.events)
		sc := bufio.NewScanner(res.Body)
		var cur sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if cur.event != "" {
					s.events <- cur
				}
				cur = sseEvent{}
			case strings.HasPrefix(line, ": ping"):
				select {
				case s.pings <- struct{}{}:
				default:
				}
			case strings.HasPrefix(line, "id: "):
				cur.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				cur.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				cur.raw = strings.TrimPrefix(line, "data: ")
				_ = json.Unmarshal([]byte(cur.raw), &cur.data)
			}
		}
	}()
	c.e.t.Cleanup(func() { res.Body.Close() })
	return s
}

// next returns the next event other than hello.
func (s *stream) next(timeout time.Duration) (sseEvent, bool) {
	s.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				return sseEvent{}, false
			}
			if ev.event == "hello" {
				continue
			}
			return ev, true
		case <-deadline:
			return sseEvent{}, false
		}
	}
}

func (s *stream) want(kind string) sseEvent {
	s.t.Helper()
	ev, ok := s.next(3 * time.Second)
	if !ok || ev.event != kind {
		s.t.Fatalf("want %s, got %+v (ok=%v)", kind, ev, ok)
	}
	return ev
}

// wantEntity skips events until one about entity id arrives.
func (s *stream) wantEntity(id string) sseEvent {
	s.t.Helper()
	for {
		ev, ok := s.next(3 * time.Second)
		if !ok {
			s.t.Fatalf("no event for %s", id)
		}
		if ev.data["entity_id"] == id {
			return ev
		}
	}
}

func (s *stream) quiet(d time.Duration, about string) {
	s.t.Helper()
	deadline := time.After(d)
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				return
			}
			if ev.event != "hello" && strings.Contains(ev.raw, about) {
				s.t.Fatalf("unexpected event %+v", ev)
			}
		case <-deadline:
			return
		}
	}
}

func liveEnv(t *testing.T, mut ...func(*config.Config)) *env {
	return newEnv(t, append([]func(*config.Config){func(c *config.Config) { c.Live.Heartbeat = 50 * time.Millisecond }}, mut...)...)
}

func TestLiveStreamChanges(t *testing.T) {
	e := liveEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("LIV")
	e.member("bob@example.com", rbac.RoleTester)
	b := e.login("bob@example.com")
	s := b.openStream(nil, "")
	if s.res.StatusCode != 200 || s.res.Header.Get("Content-Type") != "text/event-stream; charset=utf-8" || s.res.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal(s.res.StatusCode, s.res.Header)
	}
	hello := <-s.events
	if hello.event != "hello" || hello.data["user_id"] == "" {
		t.Fatal(hello)
	}
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Top secret title"}, 201)
	id := rq["id"].(string)
	ev := s.wantEntity(id)
	if ev.event != "change" || ev.id == "" || ev.data["type"] != "requirement" || ev.data["project_id"] != pid || ev.data["action"] != "requirement.create" || ev.data["version"].(float64) != 1 || ev.data["actor_id"] != e.admin.ID {
		t.Fatal(ev)
	}
	if strings.Contains(ev.raw, "secret") || strings.Contains(ev.raw, "tenant") {
		t.Fatal("events carry no content:", ev.raw)
	}
	a.m("PATCH", "/api/v1/requirements/"+id, map[string]any{"title": "v2"}, 200)
	if ev := s.wantEntity(id); ev.data["version"].(float64) != 2 || ev.data["action"] != "requirement.update" {
		t.Fatal(ev)
	}
	// executions name their run
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T"}, 201)["id"].(string)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{tc}}, 201)
	runID := run["id"].(string)
	ex := items(a.m("GET", "/api/v1/runs/"+runID+"/executions", nil, 200))[0]["id"].(string)
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"status": "passed"}, 200)
	if ev := s.wantEntity(ex); ev.data["type"] != "execution" || ev.data["parent_id"] != runID {
		t.Fatal(ev)
	}
	// a deleted item still names its project
	a.call("DELETE", "/api/v1/requirements/"+id, nil, 204, nil)
	if ev := s.wantEntity(id); ev.data["action"] != "requirement.delete" || ev.data["project_id"] != pid {
		t.Fatal(ev)
	}
	// heartbeats keep the stream alive
	select {
	case <-s.pings:
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat")
	}
}

func TestLiveStreamPermissions(t *testing.T) {
	e := liveEnv(t)
	a := e.login("admin@example.com")
	open := a.project("OPN")
	secret := a.project("SEC")
	e.member("olga@example.com", rbac.RoleManager)
	e.member("tess@example.com", rbac.RoleTester)
	a.m("PUT", "/api/v1/projects/"+secret+"/access", map[string]any{"access": "restricted"}, 200)
	a.m("POST", "/api/v1/projects/"+secret+"/members", map[string]any{"email": "tess@example.com", "role": "viewer"}, 201)
	o, ts := e.login("olga@example.com"), e.login("tess@example.com")
	so, st := o.openStream(nil, ""), ts.openStream(nil, "")

	hidden := a.m("POST", "/api/v1/projects/"+secret+"/requirements", map[string]any{"title": "Hidden"}, 201)["id"].(string)
	seen := a.m("POST", "/api/v1/projects/"+open+"/requirements", map[string]any{"title": "Seen"}, 201)["id"].(string)
	// olga is not a member of the restricted project: she only hears of the open one
	if ev := so.wantEntity(seen); ev.data["project_id"] != open {
		t.Fatal(ev)
	}
	so.quiet(150*time.Millisecond, hidden)
	st.wantEntity(hidden)

	// another organisation hears nothing
	other, err := e.st.CreateTenant(context.Background(), "other", "Other")
	if err != nil {
		t.Fatal(err)
	}
	u := mkUser(t, e.st, "zed@example.com", false)
	if err := e.st.SetMembership(context.Background(), other.ID, u.ID, rbac.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	sz := e.login("zed@example.com").openStream(nil, "")
	a.m("PATCH", "/api/v1/requirements/"+seen, map[string]any{"title": "again"}, 200)
	so.wantEntity(seen)
	sz.quiet(150*time.Millisecond, seen)

	// sharing changes take effect at once: olga becomes a member
	a.m("POST", "/api/v1/projects/"+secret+"/members", map[string]any{"email": "olga@example.com", "role": "viewer"}, 201)
	a.m("PATCH", "/api/v1/requirements/"+hidden, map[string]any{"title": "now visible"}, 200)
	so.wantEntity(hidden)

	// read permission of the entity type: a role without test.read hears no tests
	custom := map[string]any{"name": "reqonly", "label": "Requirements only", "permissions": []string{"project.read", "requirement.read"}}
	a.m("POST", "/api/v1/roles", custom, 201)
	e.member("rita@example.com", "reqonly")
	sr := e.login("rita@example.com").openStream(nil, "")
	tc := a.m("POST", "/api/v1/projects/"+open+"/tests", map[string]any{"title": "T"}, 201)["id"].(string)
	rq := a.m("POST", "/api/v1/projects/"+open+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	sr.wantEntity(rq)
	sr.quiet(150*time.Millisecond, tc)

	// an API token works as well (Bearer, GET needs no CSRF token)
	tok := ts.m("POST", "/api/v1/tokens", map[string]any{"name": "live"}, 201)["token"].(string)
	bt := e.anon()
	bt.bearer = tok
	sb := bt.openStream(nil, "")
	a.m("PATCH", "/api/v1/requirements/"+hidden, map[string]any{"title": "for the token"}, 200)
	sb.wantEntity(hidden)
	// anonymous: 401
	if s := e.anon().openStream(nil, ""); s.res.StatusCode != 401 {
		t.Fatal(s.res.StatusCode)
	}
}

func TestLiveStreamResume(t *testing.T) {
	e := liveEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RES")
	// another tab stays open, so the events are kept for replay
	a.openStream(nil, "")
	s := a.openStream(nil, "")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "one"}, 201)["id"].(string)
	last := s.wantEntity(r1).id
	s.res.Body.Close()
	time.Sleep(50 * time.Millisecond)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "two"}, 201)["id"].(string)
	r3 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "three"}, 201)["id"].(string)

	again := a.openStream(map[string]string{"Last-Event-ID": last}, "")
	if ev := again.wantEntity(r2); ev.id == "" {
		t.Fatal(ev)
	}
	again.wantEntity(r3)
	// the same with the query parameter (a reconnect made by the client)
	q := a.openStream(nil, "?last_event_id="+last)
	q.wantEntity(r2)
	// an id no longer kept: reset (the client reloads)
	gone := a.openStream(map[string]string{"Last-Event-ID": "nope-1"}, "")
	gone.want("reset")
}

func TestLiveGapWithoutListeners(t *testing.T) {
	e := liveEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("GAP")
	s := a.openStream(nil, "")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "one"}, 201)["id"].(string)
	last := s.wantEntity(r1).id
	s.res.Body.Close()
	// wait until the stream is gone: changes are then not looked up at all
	for i := 0; i < 100 && e.srv.live.Count() > 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "two"}, 201)
	a.openStream(map[string]string{"Last-Event-ID": last}, "").want("reset")
}

func TestLiveNotifications(t *testing.T) {
	e := liveEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("NOT")
	e.member("vera@example.com", rbac.RoleTester)
	e.member("walt@example.com", rbac.RoleTester)
	sv := e.login("vera@example.com").openStream(nil, "")
	sw := e.login("walt@example.com").openStream(nil, "")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	a.m("POST", "/api/v1/requirements/"+rq+"/comments", map[string]any{"body": "see @vera"}, 201)
	for {
		ev, ok := sv.next(3 * time.Second)
		if !ok {
			t.Fatal("vera got no notification event")
		}
		if ev.event == "notification" {
			if len(ev.data) != 2 || ev.data["kind"] != "notification" {
				t.Fatal("notification events carry nothing but a ping:", ev.raw)
			}
			break
		}
	}
	for {
		ev, ok := sw.next(200 * time.Millisecond)
		if !ok {
			break
		}
		if ev.event == "notification" {
			t.Fatal("walt was not notified")
		}
	}
}

func TestLivePresence(t *testing.T) {
	e := liveEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("PRS")
	secret := a.project("PRX")
	a.m("PUT", "/api/v1/projects/"+secret+"/access", map[string]any{"access": "restricted"}, 200)
	e.member("bob@example.com", rbac.RoleTester)
	b := e.login("bob@example.com")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	hid := a.m("POST", "/api/v1/projects/"+secret+"/requirements", map[string]any{"title": "H"}, 201)["id"].(string)
	sb := b.openStream(nil, "")

	res := a.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": rq, "state": "editing"}, 200)
	if len(res["others"].([]any)) != 0 || res["ttl_ms"].(float64) <= 0 {
		t.Fatal(res)
	}
	ev := sb.want("presence")
	if ev.data["entity_id"] != rq || ev.data["presence"].(map[string]any)["state"] != "editing" || ev.data["presence"].(map[string]any)["user_id"] != e.admin.ID || ev.id != "" {
		t.Fatal(ev)
	}
	res = b.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": rq}, 200)
	others := res["others"].([]any)
	if len(others) != 1 || others[0].(map[string]any)["state"] != "editing" {
		t.Fatal(res)
	}
	if got := b.m("GET", "/api/v1/presence?entity_type=requirement&entity_id="+rq, nil, 200); len(got["others"].([]any)) != 1 {
		t.Fatal(got)
	}
	// the same heartbeat again is not news
	a.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": rq, "state": "editing"}, 200)
	sb.quiet(100*time.Millisecond, "presence")
	a.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": rq, "state": "left"}, 200)
	if ev := sb.want("presence"); ev.data["presence"].(map[string]any)["state"] != "left" {
		t.Fatal(ev)
	}
	// presence on items one may not see: 404, and nobody hears of it
	b.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": hid}, 404)
	b.m("GET", "/api/v1/presence?entity_type=requirement&entity_id="+hid, nil, 404)
	a.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": hid}, 200)
	sb.quiet(100*time.Millisecond, hid)
	b.m("POST", "/api/v1/presence", map[string]any{"entity_type": "bogus", "entity_id": rq}, 400)
	b.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": rq, "state": "dancing"}, 400)
	b.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": "missing"}, 404)
}

func TestLiveDisabledAndLimits(t *testing.T) {
	off := liveEnv(t, func(c *config.Config) { c.Live.Enabled = false })
	a := off.login("admin@example.com")
	if s := a.openStream(nil, ""); s.res.StatusCode != 404 {
		t.Fatal(s.res.StatusCode)
	}
	a.m("POST", "/api/v1/presence", map[string]any{"entity_type": "requirement", "entity_id": "x"}, 404)
	a.m("GET", "/api/v1/presence?entity_type=requirement&entity_id=x", nil, 404)
	if m := a.m("GET", "/api/v1/meta", nil, 200); m["live_updates"] != false {
		t.Fatal(m["live_updates"])
	}
	pid := a.project("OFF")
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)

	e := liveEnv(t, func(c *config.Config) { c.Live.MaxConnectionsPerUser = 1 })
	b := e.login("admin@example.com")
	if m := b.m("GET", "/api/v1/meta", nil, 200); m["live_updates"] != true {
		t.Fatal(m["live_updates"])
	}
	first := b.openStream(nil, "")
	if first.res.StatusCode != 200 {
		t.Fatal(first.res.StatusCode)
	}
	if s := b.openStream(nil, ""); s.res.StatusCode != 503 || s.res.Header.Get("Retry-After") == "" {
		t.Fatal(s.res.StatusCode)
	}
}

func TestLiveStreamEnds(t *testing.T) {
	e := liveEnv(t)
	e.srv.liveReauth = 100 * time.Millisecond
	a := e.login("admin@example.com")
	other := e.login("admin@example.com")
	s := a.openStream(nil, "")
	<-s.events // hello
	// signed out elsewhere: the stream notices at its next check
	a.call("POST", "/api/v1/auth/logout", nil, 204, nil)
	if ev := s.want("bye"); ev.data["reason"] != "unauthorized" {
		t.Fatal(ev)
	}
	// graceful shutdown ends the open streams
	s2 := other.openStream(nil, "")
	<-s2.events
	e.srv.StopLive()
	if ev := s2.want("bye"); ev.data["reason"] != "shutdown" {
		t.Fatal(ev)
	}
	if s := other.openStream(nil, ""); s.res.StatusCode != 503 {
		t.Fatal(s.res.StatusCode)
	}
}
