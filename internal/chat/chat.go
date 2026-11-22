// Package chat renders Qtestframe events as chat messages for Slack,
// Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix and
// Discord, and posts them.
//
// A message is built from the plain fields of Message, never from the
// stored item, so what reaches a channel is exactly what the API put
// there (the API leaves out the content of restricted projects). Texts are
// escaped for each format, broadcast mentions (@channel, @here, @all,
// @everyone) are defused and every provider's length limits are kept:
// long texts are shortened and a digest lists at most a provider-specific
// number of events followed by "… and N more". The colour of a message
// always repeats its status in text.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Providers.
const (
	Slack       = "slack"
	Teams       = "teams"        // Workflows (Power Automate) webhook, Adaptive Card
	TeamsLegacy = "teams_legacy" // Office 365 connector, MessageCard
	Mattermost  = "mattermost"
	RocketChat  = "rocketchat"
	GoogleChat  = "googlechat"
	Matrix      = "matrix"
	Discord     = "discord"
)

// Providers lists the supported chat tools.
var Providers = []string{Slack, Teams, TeamsLegacy, Mattermost, RocketChat, GoogleChat, Matrix, Discord}

// Message is one event as shown in a channel.
type Message struct {
	Event    string    `json:"event"`
	Kind     string    `json:"kind,omitempty"`
	Key      string    `json:"key,omitempty"`
	Title    string    `json:"title,omitempty"`
	URL      string    `json:"url,omitempty"`
	Status   string    `json:"status,omitempty"`
	Priority string    `json:"priority,omitempty"`
	Actor    string    `json:"actor,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	Project  string    `json:"project,omitempty"`
	Time     time.Time `json:"time"`
	// Tone overrides the colour class worked out from event and status
	// (a completed run with failed results: danger).
	Tone string `json:"tone,omitempty"`
}

// Tones of a message.
const (
	ToneGood    = "good"
	ToneWarning = "warning"
	ToneDanger  = "danger"
	ToneInfo    = "info"
)

// ToneOf is the colour class of a message: Tone, else from its event and
// status.
func (m Message) ToneOf() string {
	if toneHex[m.Tone] != "" {
		return m.Tone
	}
	ev := strings.ToLower(m.Event)
	st := strings.ToLower(m.Status)
	switch {
	case strings.HasSuffix(ev, ".regression"), strings.HasSuffix(ev, ".rejected"),
		st == "failed", st == "rejected", st == "critical", st == "error":
		return ToneDanger
	case strings.HasSuffix(ev, ".revoked"), strings.HasSuffix(ev, ".expired"),
		st == "blocked", st == "changes_requested", st == "high", st == "major", st == "warning", st == "expired", st == "revoked":
		return ToneWarning
	case strings.HasSuffix(ev, ".released"), strings.HasSuffix(ev, ".approved"),
		st == "passed", st == "approved", st == "resolved", st == "closed", st == "released", st == "mitigated":
		return ToneGood
	}
	return ToneInfo
}

var toneHex = map[string]string{ToneGood: "#2E7D32", ToneWarning: "#B35C00", ToneDanger: "#C62828", ToneInfo: "#1565C0"}

// Color is the hex colour (#RRGGBB) of a message.
func (m Message) Color() string { return toneHex[m.ToneOf()] }

// Heading is the event label, with the item key and title in one line.
func (m Message) itemText() string {
	switch {
	case m.Key != "" && m.Title != "":
		return m.Key + " " + m.Title
	case m.Key != "":
		return m.Key
	}
	return m.Title
}

// Target is where a channel posts.
type Target struct {
	Provider string
	// URL: the incoming webhook URL (secret) of every provider but Matrix.
	URL string
	// Matrix: homeserver base URL, room ID (!id:server) or alias
	// (#alias:server) and the access token (secret).
	Homeserver string
	Room       string
	Token      string
}

// secrets are the values that must never appear in an error.
func (t Target) secrets() []string {
	var out []string
	for _, v := range []string{t.URL, t.Token} {
		if v != "" {
			out = append(out, v)
			if u, err := url.Parse(v); err == nil && u.Path != "" && u.Path != "/" {
				out = append(out, u.Path)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- limits

// maxItems: events shown in one digest message per provider.
var maxItems = map[string]int{Slack: 20, Teams: 15, TeamsLegacy: 10, Mattermost: 25, RocketChat: 25, GoogleChat: 20, Matrix: 30, Discord: 10}

const (
	maxTitle         = 200  // item title
	maxSummary       = 1000 // summary of a single event
	maxDigestSummary = 200  // summary per event in a digest
)

// shorten cuts s to at most n runes, ending in "…".
func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// oneLine joins the lines of s.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// broadcast matches the mentions that ping a whole channel.
var broadcast = regexp.MustCompile(`(?i)@(channel|here|all|everyone)\b`)

// defuse breaks broadcast mentions so a message never pings a whole channel.
func defuse(s string) string { return broadcast.ReplaceAllString(s, "@\u200b$1") }

// prepared returns the messages to show (at most limit) with their texts
// cleaned and shortened, and how many were left out.
func prepared(msgs []Message, limit int) ([]Message, int) {
	digest := len(msgs) > 1
	n := min(len(msgs), limit)
	out := make([]Message, n)
	for i := range n {
		m := msgs[i]
		m.Title = defuse(shorten(oneLine(m.Title), maxTitle))
		m.Key = shorten(oneLine(m.Key), 60)
		m.Actor = defuse(shorten(oneLine(m.Actor), 120))
		m.Project = defuse(shorten(oneLine(m.Project), 160))
		m.Status = shorten(oneLine(m.Status), 60)
		m.Priority = shorten(oneLine(m.Priority), 60)
		if digest {
			m.Summary = shorten(oneLine(m.Summary), maxDigestSummary)
		} else {
			m.Summary = shorten(m.Summary, maxSummary)
		}
		m.Summary = defuse(m.Summary)
		if u, err := url.Parse(m.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			m.URL = ""
		}
		out[i] = m
	}
	return out, len(msgs) - n
}

// header is the first line: the event (one message) or the digest count.
func header(lang string, msgs []Message) string {
	if len(msgs) == 1 {
		return EventLabel(lang, msgs[0].Event)
	}
	if p := msgs[0].Project; p != "" {
		return fmt.Sprintf(text(lang, "digest"), len(msgs), defuse(shorten(oneLine(p), 160)))
	}
	return fmt.Sprintf(text(lang, "digest1"), len(msgs))
}

// facts are the label/value pairs of a message.
func facts(lang string, m Message, digest bool) [][2]string {
	var out [][2]string
	if m.Status != "" {
		out = append(out, [2]string{text(lang, "status"), ValueLabel(lang, m.Status)})
	}
	if m.Priority != "" {
		out = append(out, [2]string{text(lang, "priority"), ValueLabel(lang, m.Priority)})
	}
	if m.Actor != "" {
		out = append(out, [2]string{text(lang, "by"), m.Actor})
	}
	if !digest && m.Project != "" {
		out = append(out, [2]string{text(lang, "project"), m.Project})
	}
	return out
}

// plainLine is "Status: failed · By: Anna" of a message.
func plainLine(lang string, m Message, digest bool) string {
	var parts []string
	for _, f := range facts(lang, m, digest) {
		parts = append(parts, f[0]+": "+f[1])
	}
	return strings.Join(parts, " · ")
}

func moreText(lang string, n int) string { return fmt.Sprintf(text(lang, "more"), n) }

// openText is the accessible name of the link to an item.
func openText(lang string, m Message) string {
	if m.Key != "" {
		return fmt.Sprintf(text(lang, "openKey"), m.Key)
	}
	return text(lang, "open")
}

// Payload renders the body of one post of msgs (one event, or a digest
// when there are several).
func Payload(provider, lang string, msgs []Message) ([]byte, error) {
	if len(msgs) == 0 {
		return nil, errors.New("no messages")
	}
	if lang != "sv" {
		lang = "en"
	}
	var v any
	switch provider {
	case Slack:
		v = slackPayload(lang, msgs)
	case Teams:
		v = teamsPayload(lang, msgs)
	case TeamsLegacy:
		v = teamsLegacyPayload(lang, msgs)
	case Mattermost, RocketChat:
		v = attachmentPayload(provider, lang, msgs)
	case GoogleChat:
		v = googleChatPayload(lang, msgs)
	case Discord:
		v = discordPayload(lang, msgs)
	case Matrix:
		v = matrixPayload(lang, msgs)
	default:
		return nil, fmt.Errorf("unknown chat provider %q", provider)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(b.Bytes()), nil
}

// ---------------------------------------------------------------- sending

// Result is the outcome of a post.
type Result struct {
	// Status is the HTTP status (0 when no response came).
	Status int
	// Permanent: retrying cannot help (the URL or token was refused).
	Permanent bool
	// RetryAfter is the wait the service asked for (HTTP 429).
	RetryAfter time.Duration
}

// UserAgent of the posts.
const UserAgent = "Qtestframe-Chat/1"

// Send posts msgs to the target. Errors never contain the URL or token.
func Send(ctx context.Context, client *http.Client, t Target, lang string, msgs []Message) (Result, error) {
	res, err := send(ctx, client, t, lang, msgs)
	if err != nil {
		err = errors.New(redact(err.Error(), t.secrets()))
	}
	return res, err
}

func send(ctx context.Context, client *http.Client, t Target, lang string, msgs []Message) (Result, error) {
	body, err := Payload(t.Provider, lang, msgs)
	if err != nil {
		return Result{Permanent: true}, err
	}
	method, target := http.MethodPost, t.URL
	if t.Provider == Matrix {
		room := t.Room
		if strings.HasPrefix(room, "#") {
			if room, err = resolveAlias(ctx, client, t); err != nil {
				return Result{}, err
			}
		}
		method = http.MethodPut
		target = strings.TrimSuffix(t.Homeserver, "/") + "/_matrix/client/v3/rooms/" + url.PathEscape(room) + "/send/m.room.message/" + url.PathEscape("qtf-"+uuid.NewString())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return Result{Permanent: true}, errors.New("the channel URL is not valid")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	if t.Provider == Matrix {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}
	return do(client, req)
}

// resolveAlias looks up the room ID of a Matrix room alias.
func resolveAlias(ctx context.Context, client *http.Client, t Target) (string, error) {
	u := strings.TrimSuffix(t.Homeserver, "/") + "/_matrix/client/v3/directory/room/" + url.PathEscape(t.Room)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", errors.New("the homeserver URL is not valid")
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+t.Token)
	res, err := client.Do(req)
	if err != nil {
		return "", transportError(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("room alias %s: HTTP %d%s", t.Room, res.StatusCode, detail(b))
	}
	var out struct {
		RoomID string `json:"room_id"`
	}
	if json.Unmarshal(b, &out) != nil || out.RoomID == "" {
		return "", fmt.Errorf("room alias %s: no room ID in the answer", t.Room)
	}
	return out.RoomID, nil
}

func do(client *http.Client, req *http.Request) (Result, error) {
	res, err := client.Do(req)
	if err != nil {
		return Result{}, transportError(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	out := Result{Status: res.StatusCode}
	if res.StatusCode < 300 {
		return out, nil
	}
	if res.StatusCode == http.StatusTooManyRequests {
		if s, err := strconv.Atoi(strings.TrimSpace(res.Header.Get("Retry-After"))); err == nil && s > 0 {
			out.RetryAfter = time.Duration(s) * time.Second
		}
	}
	// a refused URL, token or payload stays refused (not 408 / 429)
	out.Permanent = res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != http.StatusRequestTimeout && res.StatusCode != http.StatusTooManyRequests
	return out, fmt.Errorf("HTTP %d%s", res.StatusCode, detail(b))
}

// transportError drops the URL that *url.Error puts in its text.
func transportError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return errors.New("no answer in time")
		}
		return ue.Err
	}
	return err
}

// detail is a short text of an error answer: ": invalid_payload".
func detail(b []byte) string {
	var m struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
		ErrCode string `json:"errcode"`
	}
	s := ""
	if json.Unmarshal(b, &m) == nil {
		switch {
		case m.ErrCode != "":
			s = m.ErrCode
		case m.Message != "":
			s = m.Message
		case m.Error != nil:
			s = fmt.Sprint(m.Error)
		}
	} else if utf8.Valid(b) {
		s = string(b)
	}
	s = shorten(oneLine(s), 200)
	if s == "" {
		return ""
	}
	return ": " + s
}

// redact removes the secrets from a text.
func redact(s string, secrets []string) string {
	for _, v := range secrets {
		if len(v) >= 4 {
			s = strings.ReplaceAll(s, v, "[secret]")
		}
	}
	return s
}
