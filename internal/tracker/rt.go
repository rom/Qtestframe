package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- Request Tracker
//
// Request Tracker (RT, Best Practical) 5.x through its REST2 API
// (/REST/2.0). The token (an authentication token of an RT user, made
// under Settings → Auth Tokens) is sent as "Authorization: token <t>".
// Tickets go to the configured queue; a ticket is referred to as #123 (or
// 123, RT#123). RT has no labels: they are written to a custom field when
// LabelsField names one, else left out. A ticket's content is its first
// message and cannot be replaced, so an update changes the subject and
// adds the new description as a comment.

type rt struct {
	cfg  *model.DefectTracker
	hc   *http.Client
	base string // …/REST/2.0
	web  string // the instance URL
}

// RTBase strips /REST/2.0 and trailing slashes from an RT URL.
func RTBase(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	return strings.TrimRight(strings.TrimSuffix(u, "/REST/2.0"), "/")
}

func newRT(t *model.DefectTracker, hc *http.Client) *rt {
	web := RTBase(t.URL)
	return &rt{cfg: t, hc: hc, base: web + "/REST/2.0", web: web}
}

func (c *rt) Markup() string { return MarkupMarkdown }

func (c *rt) auth(r *http.Request) {
	if c.cfg.Token != "" {
		r.Header.Set("Authorization", "token "+c.cfg.Token)
	}
}

type rtTicket struct {
	ID      int    `json:"id"`
	Subject string `json:"Subject"`
	Status  string `json:"Status"`
}

// RTMapped maps an RT ticket status (of the default lifecycle) to a
// defect status: new → open, open and stalled → in_progress, resolved →
// closed, rejected and deleted → rejected; "" for the statuses of custom
// lifecycles.
func RTMapped(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "new":
		return "open"
	case "open", "stalled":
		return "in_progress"
	case "resolved":
		return "closed"
	case "rejected", "deleted":
		return "rejected"
	}
	return ""
}

func rtClosed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "resolved", "rejected", "deleted":
		return true
	}
	return false
}

func (c *rt) issue(t *rtTicket) *Issue {
	id := strconv.Itoa(t.ID)
	return &Issue{Key: "#" + id, URL: c.web + "/Ticket/Display.html?id=" + id, Title: t.Subject, Status: t.Status,
		Mapped: RTMapped(t.Status), Closed: rtClosed(t.Status)}
}

const rtKeyHint = "an RT ticket is referred to by its number, e.g. #123"

// rtID extracts the ticket number from "#123", "RT#123", "rt 123" or "123".
func rtID(key string) (string, error) {
	k := strings.TrimSpace(key)
	if len(k) >= 2 && strings.EqualFold(k[:2], "rt") {
		k = strings.TrimSpace(k[2:])
	}
	k = strings.TrimPrefix(k, "#")
	if n, err := strconv.Atoi(k); err != nil || n <= 0 {
		return "", fmt.Errorf("%w: %s", ErrBadKey, rtKeyHint)
	}
	return k, nil
}

// plainText drops the markers of the managed part (RT shows text/plain
// content as it is, so the HTML comments would be visible).
func plainText(s string) string {
	s = strings.ReplaceAll(s, BeginMarker+"\n", "")
	s = strings.ReplaceAll(s, "\n"+EndMarker, "")
	s = strings.ReplaceAll(strings.ReplaceAll(s, BeginMarker, ""), EndMarker, "")
	return strings.TrimSpace(s)
}

// labels are the custom field values of a ticket's labels (nil when no
// labels field is configured).
func (c *rt) labels(in []string) map[string]any {
	field := strings.TrimSpace(c.cfg.LabelsField)
	l := mergeLabels(splitLabels(c.cfg.Labels), in)
	if field == "" || len(l) == 0 {
		return nil
	}
	return map[string]any{field: l}
}

func (c *rt) Create(ctx context.Context, in NewIssue) (*Issue, error) {
	desc := in.Description
	if in.Severity != "" {
		desc = strings.TrimSpace(desc + "\n\nSeverity: " + in.Severity)
	}
	body := map[string]any{"Queue": c.cfg.Queue, "Subject": in.Title, "Content": plainText(desc), "ContentType": "text/plain"}
	if cf := c.labels(in.Labels); cf != nil {
		body["CustomFields"] = cf
	}
	var created struct {
		ID json.Number `json:"id"`
	}
	if err := do(ctx, c.hc, http.MethodPost, c.base+"/ticket", body, c.auth, &created); err != nil {
		return nil, err
	}
	if created.ID == "" {
		return nil, fmt.Errorf("unexpected answer from the tracker: no ticket id")
	}
	return c.Get(ctx, created.ID.String())
}

func (c *rt) Get(ctx context.Context, key string) (*Issue, error) {
	id, err := rtID(key)
	if err != nil {
		return nil, err
	}
	var t rtTicket
	if err := do(ctx, c.hc, http.MethodGet, c.base+"/ticket/"+id, nil, c.auth, &t); err != nil {
		return nil, err
	}
	return c.issue(&t), nil
}

func (c *rt) Update(ctx context.Context, key string, in NewIssue) (*Issue, error) {
	id, err := rtID(key)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"Subject": in.Title}
	if cf := c.labels(in.Labels); cf != nil {
		body["CustomFields"] = cf
	}
	if err := do(ctx, c.hc, http.MethodPut, c.base+"/ticket/"+id, body, c.auth, nil); err != nil {
		return nil, err
	}
	if d := plainText(in.Description); d != "" {
		if err := c.Comment(ctx, id, d); err != nil {
			return nil, err
		}
	}
	return c.Get(ctx, id)
}

func (c *rt) Comment(ctx context.Context, key, text string) error {
	id, err := rtID(key)
	if err != nil {
		return err
	}
	return do(ctx, c.hc, http.MethodPost, c.base+"/ticket/"+id+"/comment", map[string]string{"Content": text, "ContentType": "text/plain"}, c.auth, nil)
}

// Check reads the queue (by name or ID), which needs a valid token and
// the right to see the queue.
func (c *rt) Check(ctx context.Context) error {
	return do(ctx, c.hc, http.MethodGet, c.base+"/queue/"+url.PathEscape(strings.TrimSpace(c.cfg.Queue)), nil, c.auth, nil)
}
