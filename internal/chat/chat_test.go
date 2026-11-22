package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func sample() Message {
	return Message{Event: "run.completed", Kind: "run", Key: "", Title: "Nightly <regression> & smoke", URL: "https://qtf.example.test/p/1/runs/2",
		Status: "failed", Actor: "Anna Berg", Summary: "12 passed, 2 failed — ping @channel please", Project: "BANK Banking portal",
		Time: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)}
}

func defect(i int) Message {
	return Message{Event: "defect.created", Kind: "defect", Key: fmt.Sprintf("DEF-%d", i), Title: "Login fails", URL: fmt.Sprintf("https://qtf.example.test/p/1/defects?item=%d", i),
		Status: "open", Priority: "high", Actor: "Bo", Summary: strings.Repeat("x", 500), Project: "BANK Banking portal"}
}

// fake is a chat service that records the requests it gets.
type fake struct {
	mu     sync.Mutex
	method string
	path   string
	auth   string
	ctype  string
	body   map[string]any
	status int
	reply  string
}

func (f *fake) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if strings.Contains(r.URL.Path, "/directory/room/") {
			_, _ = io.WriteString(w, `{"room_id":"!resolved:example.test"}`)
			return
		}
		f.method, f.path, f.auth, f.ctype = r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		f.body = nil
		if err := json.Unmarshal(b, &f.body); err != nil {
			t.Errorf("body is not JSON: %s", b)
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
		}
		_, _ = io.WriteString(w, f.reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func get(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, _ := v.(map[string]any)
			v = m[k]
		case int:
			l, _ := v.([]any)
			if k >= len(l) {
				return nil
			}
			v = l[k]
		}
	}
	return v
}

func post(t *testing.T, tg Target, lang string, msgs ...Message) {
	t.Helper()
	res, err := Send(context.Background(), http.DefaultClient, tg, lang, msgs)
	if err != nil || res.Status/100 != 2 {
		t.Fatal(res, err)
	}
}

func TestSlack(t *testing.T) {
	f := &fake{reply: "ok"}
	srv := f.server(t)
	post(t, Target{Provider: Slack, URL: srv.URL + "/services/T0/B0/x"}, "en", sample())
	if f.method != "POST" || f.path != "/services/T0/B0/x" || f.ctype != "application/json" {
		t.Fatal(f.method, f.path, f.ctype)
	}
	if get(f.body, "text") != "Test run completed: Nightly &lt;regression&gt; &amp; smoke" {
		t.Fatal(get(f.body, "text"))
	}
	if get(f.body, "blocks", 0, "type") != "header" || get(f.body, "blocks", 0, "text", "text") != "Test run completed" {
		t.Fatal(f.body)
	}
	att := get(f.body, "attachments", 0)
	if get(att, "color") != "#C62828" {
		t.Fatal(att)
	}
	sec := get(att, "blocks", 0, "text", "text").(string)
	for _, want := range []string{"*<https://qtf.example.test/p/1/runs/2|Nightly &lt;regression&gt; &amp; smoke>*", "Status: failed · By: Anna Berg", "@​channel"} {
		if !strings.Contains(sec, want) {
			t.Fatalf("%q not in %q", want, sec)
		}
	}
	if get(att, "blocks", 1, "elements", 0, "url") != "https://qtf.example.test/p/1/runs/2" {
		t.Fatal(att)
	}
	// a digest: header with the count, at most 20 sections and "and N more"
	var many []Message
	for i := range 25 {
		many = append(many, defect(i))
	}
	post(t, Target{Provider: Slack, URL: srv.URL}, "sv", many...)
	blocks := get(f.body, "blocks").([]any)
	if len(blocks) != 22 || get(blocks[0], "text", "text") != "25 händelser i BANK Banking portal" || get(blocks[21], "elements", 0, "text") != "… och 5 till" {
		t.Fatal(len(blocks), blocks[0], blocks[21])
	}
	if s := get(blocks[1], "text", "text").(string); !strings.Contains(s, "*Defekt rapporterad*: *<https://qtf.example.test/p/1/defects?item=0|DEF-0 Login fails>*") || !strings.Contains(s, "Prioritet: hög") || len([]rune(s)) > 3000 {
		t.Fatal(s)
	}
}

func TestTeams(t *testing.T) {
	f := &fake{status: 202}
	srv := f.server(t)
	post(t, Target{Provider: Teams, URL: srv.URL + "/workflows/x"}, "en", sample())
	if get(f.body, "type") != "message" || get(f.body, "attachments", 0, "contentType") != "application/vnd.microsoft.card.adaptive" {
		t.Fatal(f.body)
	}
	card := get(f.body, "attachments", 0, "content")
	if get(card, "type") != "AdaptiveCard" || get(card, "version") != "1.4" || get(card, "body", 0, "text") != "Test run completed" {
		t.Fatal(card)
	}
	items := get(card, "body", 1, "items").([]any)
	if get(items[0], "text") != `[Nightly &lt;regression&gt; & smoke](https://qtf.example.test/p/1/runs/2)` {
		t.Fatal(items[0])
	}
	if get(items[1], "text") != "Status: failed" || get(items[1], "color") != "Attention" {
		t.Fatal(items[1])
	}
	if get(card, "actions", 0, "type") != "Action.OpenUrl" || get(card, "actions", 0, "url") != "https://qtf.example.test/p/1/runs/2" {
		t.Fatal(card)
	}
	// legacy Office 365 connector card
	post(t, Target{Provider: TeamsLegacy, URL: srv.URL}, "en", sample())
	if get(f.body, "@type") != "MessageCard" || get(f.body, "themeColor") != "C62828" || get(f.body, "potentialAction", 0, "targets", 0, "uri") != "https://qtf.example.test/p/1/runs/2" {
		t.Fatal(f.body)
	}
	if get(f.body, "sections", 0, "facts", 0, "name") != "Status" {
		t.Fatal(f.body)
	}
	var many []Message
	for i := range 12 {
		many = append(many, defect(i))
	}
	post(t, Target{Provider: TeamsLegacy, URL: srv.URL}, "en", many...)
	if s := get(f.body, "sections").([]any); len(s) != 11 || get(s[10], "text") != "… and 2 more" {
		t.Fatal(s)
	}
}

func TestMattermostAndRocketChat(t *testing.T) {
	f := &fake{reply: "ok"}
	srv := f.server(t)
	post(t, Target{Provider: Mattermost, URL: srv.URL + "/hooks/x"}, "en", defect(1))
	a := get(f.body, "attachments", 0)
	if get(f.body, "username") != "Qtestframe" || get(a, "title") != "DEF-1 Login fails" || get(a, "title_link") != "https://qtf.example.test/p/1/defects?item=1" || get(a, "color") != "#1565C0" || get(a, "author_name") != "Bo" {
		t.Fatal(f.body)
	}
	if get(a, "fields", 1, "title") != "Priority" || get(a, "fields", 1, "value") != "high" {
		t.Fatal(a)
	}
	post(t, Target{Provider: RocketChat, URL: srv.URL + "/hooks/y"}, "en", sample())
	if get(f.body, "alias") != "Qtestframe" || get(f.body, "attachments", 0, "title") != "Nightly <regression> & smoke" {
		t.Fatal(f.body)
	}
	if txt := get(f.body, "attachments", 0, "text").(string); !strings.Contains(txt, "@​channel") {
		t.Fatal(txt)
	}
}

func TestGoogleChat(t *testing.T) {
	f := &fake{reply: "{}"}
	srv := f.server(t)
	post(t, Target{Provider: GoogleChat, URL: srv.URL + "/v1/spaces/x/messages?key=k&token=t"}, "sv", sample())
	card := get(f.body, "cardsV2", 0, "card")
	if get(card, "header", "title") != "Testkörning avslutad" || get(card, "header", "subtitle") != "BANK Banking portal" {
		t.Fatal(card)
	}
	w := get(card, "sections", 0, "widgets").([]any)
	if get(w[0], "decoratedText", "text") != "<b>Nightly &lt;regression&gt; &amp; smoke</b>" || get(w[1], "textParagraph", "text") != "Status: underkänt · Av: Anna Berg · Projekt: BANK Banking portal" {
		t.Fatal(w)
	}
	if get(w[3], "buttonList", "buttons", 0, "onClick", "openLink", "url") != "https://qtf.example.test/p/1/runs/2" {
		t.Fatal(w[3])
	}
	if !strings.HasPrefix(get(f.body, "text").(string), "Testkörning avslutad: ") {
		t.Fatal(f.body)
	}
}

func TestDiscord(t *testing.T) {
	f := &fake{status: 204}
	srv := f.server(t)
	post(t, Target{Provider: Discord, URL: srv.URL + "/api/webhooks/1/x"}, "en", sample())
	e := get(f.body, "embeds", 0)
	if get(e, "title") != "Nightly <regression> & smoke" || get(e, "url") != "https://qtf.example.test/p/1/runs/2" || get(e, "color") != float64(0xC62828) || get(e, "timestamp") != "2026-11-02T10:00:00Z" {
		t.Fatal(e)
	}
	if l, ok := get(f.body, "allowed_mentions", "parse").([]any); !ok || len(l) != 0 {
		t.Fatal(f.body)
	}
	var many []Message
	for i := range 14 {
		many = append(many, defect(i))
	}
	post(t, Target{Provider: Discord, URL: srv.URL}, "en", many...)
	embeds := get(f.body, "embeds").([]any)
	total := 0
	for _, e := range embeds {
		b, _ := json.Marshal(e)
		total += len(get(e, "title").(string)) + len(get(e, "description").(string))
		_ = b
	}
	if len(embeds) != 10 || total > 6000 || !strings.Contains(get(f.body, "content").(string), "… and 4 more") {
		t.Fatal(len(embeds), total, get(f.body, "content"))
	}
}

func TestMatrix(t *testing.T) {
	f := &fake{reply: `{"event_id":"$1"}`}
	srv := f.server(t)
	tg := Target{Provider: Matrix, Homeserver: srv.URL + "/", Room: "!abc:example.test", Token: "mx-token-value"}
	post(t, tg, "en", sample())
	if f.method != "PUT" || !strings.HasPrefix(f.path, "/_matrix/client/v3/rooms/%21abc:example.test/send/m.room.message/qtf-") || f.auth != "Bearer mx-token-value" {
		t.Fatal(f.method, f.path, f.auth)
	}
	if get(f.body, "msgtype") != "m.notice" || get(f.body, "format") != "org.matrix.custom.html" {
		t.Fatal(f.body)
	}
	fb := get(f.body, "formatted_body").(string)
	if !strings.Contains(fb, `<a href="https://qtf.example.test/p/1/runs/2">Nightly &lt;regression&gt; &amp; smoke</a>`) || !strings.Contains(get(f.body, "body").(string), "Status: failed") {
		t.Fatal(f.body)
	}
	// a room alias is resolved first
	tg.Room = "#qa:example.test"
	post(t, tg, "en", defect(1), defect(2))
	if !strings.Contains(f.path, "/rooms/%21resolved:example.test/") || !strings.Contains(get(f.body, "formatted_body").(string), "<ul><li>Defect reported: ") {
		t.Fatal(f.path, f.body)
	}
}

func TestErrorsAreClassifiedAndRedacted(t *testing.T) {
	f := &fake{status: 404, reply: "no_service"}
	srv := f.server(t)
	secretURL := srv.URL + "/services/T000/B000/very-secret-part"
	res, err := Send(context.Background(), http.DefaultClient, Target{Provider: Slack, URL: secretURL}, "en", []Message{sample()})
	if err == nil || !res.Permanent || res.Status != 404 || err.Error() != "HTTP 404: no_service" {
		t.Fatal(res, err)
	}
	f.status, f.reply = 429, `{"message":"rate limited"}`
	srvRetry := srv
	res, err = Send(context.Background(), http.DefaultClient, Target{Provider: Discord, URL: srvRetry.URL}, "en", []Message{sample()})
	if err == nil || res.Permanent || !strings.Contains(err.Error(), "rate limited") {
		t.Fatal(res, err)
	}
	retry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
	}))
	defer retry.Close()
	if res, _ := Send(context.Background(), http.DefaultClient, Target{Provider: Slack, URL: retry.URL}, "en", []Message{sample()}); res.RetryAfter != 30*time.Second {
		t.Fatal(res)
	}
	// the transport error names no URL
	closed := httptest.NewServer(http.NotFoundHandler())
	u := closed.URL + "/services/T/B/hidden-token"
	closed.Close()
	_, err = Send(context.Background(), http.DefaultClient, Target{Provider: Slack, URL: u}, "en", []Message{sample()})
	if err == nil || strings.Contains(err.Error(), "hidden-token") || strings.Contains(err.Error(), "services") {
		t.Fatal(err)
	}
	// a Matrix error with the token in it
	mx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"errcode":"M_UNKNOWN_TOKEN","error":"bad token tok-1234"}`)
	}))
	defer mx.Close()
	res, err = Send(context.Background(), http.DefaultClient, Target{Provider: Matrix, Homeserver: mx.URL, Room: "!r:x", Token: "tok-1234"}, "en", []Message{sample()})
	if err == nil || !res.Permanent || err.Error() != "HTTP 401: M_UNKNOWN_TOKEN" {
		t.Fatal(res, err)
	}
	if redact("a tok-1234 b", []string{"tok-1234"}) != "a [secret] b" {
		t.Fatal("redact")
	}
	if _, err := Payload("irc", "en", []Message{sample()}); err == nil {
		t.Fatal("unknown provider")
	}
	if _, err := Payload(Slack, "en", nil); err == nil {
		t.Fatal("no messages")
	}
}

func TestLabelsGroupsAndTones(t *testing.T) {
	if Group("execution.updated") != "runs" || Group("waiver.approved") != "waivers" || Group("discussion.created") != "requirements" || Group("widget.made") != "other" {
		t.Fatal("groups")
	}
	if EventLabel("sv", "defect.created") != "Defekt rapporterad" || EventLabel("en", "cycle.started") != "Cycle started" || EventLabel("en", "exploratory_session.ended") != "Exploratory session ended" {
		t.Fatal(EventLabel("en", "cycle.started"))
	}
	if GroupLabel("sv", "risks") != "Risker" || GroupLabel("en", "foo") != "Foo" {
		t.Fatal("group labels")
	}
	if ValueLabel("sv", "in_progress") != "pågår" || ValueLabel("en", "in_progress") != "in progress" {
		t.Fatal("values")
	}
	for m, want := range map[Message]string{
		{Event: "defect.regression"}:                             ToneDanger,
		{Event: "release.released"}:                              ToneGood,
		{Event: "review.completed", Status: "changes_requested"}: ToneWarning,
		{Event: "requirement.updated"}:                           ToneInfo,
	} {
		if m.ToneOf() != want {
			t.Fatal(m, m.ToneOf())
		}
	}
	if defuse("hi @Here and @everyone, @hereby") != "hi @​Here and @​everyone, @hereby" {
		t.Fatal(defuse("hi @Here and @everyone, @hereby"))
	}
	// unsafe links are dropped; long titles shortened
	ms, _ := prepared([]Message{{Event: "x", URL: "javascript:alert(1)", Title: strings.Repeat("é", 300)}}, 5)
	if ms[0].URL != "" || len([]rune(ms[0].Title)) != maxTitle {
		t.Fatal(ms[0])
	}
}
