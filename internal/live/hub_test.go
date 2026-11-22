package live

import (
	"context"
	"errors"
	"testing"
	"time"
)

func recv(t *testing.T, s *Sub) Event {
	t.Helper()
	select {
	case ev := <-s.C:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	return Event{}
}

func none(t *testing.T, s *Sub) {
	t.Helper()
	select {
	case ev := <-s.C:
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestHubFanOutPerTenant(t *testing.T) {
	h := NewHub(Options{Replay: 8})
	a, _ := h.Subscribe("t1", "u1")
	b, _ := h.Subscribe("t2", "u2")
	h.Publish(context.Background(), Event{Kind: KindChange, TenantID: "t1", Type: "requirement", EntityID: "r1", ProjectID: "p1", Version: 3})
	ev := recv(t, a)
	if ev.ID == "" || ev.Origin != h.Instance() || ev.Time.IsZero() || ev.Version != 3 {
		t.Fatalf("%+v", ev)
	}
	none(t, b)
	if h.Count() != 2 {
		t.Fatal(h.Count())
	}
	h.Unsubscribe(a)
	h.Unsubscribe(a) // idempotent
	if h.Count() != 1 {
		t.Fatal(h.Count())
	}
}

func TestHubLimits(t *testing.T) {
	h := NewHub(Options{MaxConnections: 3, MaxPerUser: 2})
	s1, err := h.Subscribe("t", "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe("t", "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe("t", "u"); !errors.Is(err, ErrTooMany) {
		t.Fatal("per user limit", err)
	}
	if _, err := h.Subscribe("t", "v"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe("t", "w"); !errors.Is(err, ErrTooMany) {
		t.Fatal("total limit", err)
	}
	h.Unsubscribe(s1)
	if _, err := h.Subscribe("t", "w"); err != nil {
		t.Fatal(err)
	}
	h.Close()
	h.Close()
	if _, err := h.Subscribe("t", "x"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestHubCloseEndsSubscriptions(t *testing.T) {
	h := NewHub(Options{})
	s, _ := h.Subscribe("t", "u")
	h.Close()
	select {
	case <-s.Done:
	case <-time.After(time.Second):
		t.Fatal("not closed")
	}
	// events after the close go nowhere
	h.Publish(context.Background(), Event{Kind: KindChange, TenantID: "t"})
	none(t, s)
}

func TestHubReplay(t *testing.T) {
	h := NewHub(Options{Replay: 4})
	ctx := context.Background()
	var ids []string
	for i := 0; i < 3; i++ {
		s, _ := h.Subscribe("t", "u")
		h.Publish(ctx, Event{Kind: KindChange, TenantID: "t", EntityID: "x"})
		ids = append(ids, recv(t, s).ID)
		h.Unsubscribe(s)
	}
	h.Publish(ctx, Event{Kind: KindChange, TenantID: "other"})
	got, ok := h.Since("t", ids[0])
	if !ok || len(got) != 2 || got[0].ID != ids[1] || got[1].ID != ids[2] {
		t.Fatalf("%v %+v", ok, got)
	}
	if got, ok := h.Since("t", ids[2]); !ok || len(got) != 0 {
		t.Fatal(got, ok)
	}
	if _, ok := h.Since("t", "unknown"); ok {
		t.Fatal("unknown id must ask for a reload")
	}
	// the buffer wraps: the first event is gone
	h.Publish(ctx, Event{Kind: KindChange, TenantID: "t"})
	h.Publish(ctx, Event{Kind: KindChange, TenantID: "t"})
	if _, ok := h.Since("t", ids[0]); ok {
		t.Fatal("overwritten event must ask for a reload")
	}
	if got, ok := h.Since("t", ids[2]); !ok || len(got) != 2 {
		t.Fatal(got, ok)
	}
	// presence and access events are not replayed
	h.Publish(ctx, Event{Kind: KindAccess, TenantID: "t"})
	if got, _ := h.Since("t", ids[2]); len(got) != 2 {
		t.Fatal(got)
	}
	// a gap (events skipped while nobody listened) asks for a reload
	h.Gap("t")
	h.Gap("t") // deduplicated
	if _, ok := h.Since("t", ids[2]); ok {
		t.Fatal("gap must ask for a reload")
	}
	// a gap of another tenant does not matter
	h2 := NewHub(Options{Replay: 4})
	s2, _ := h2.Subscribe("o", "u")
	h2.Publish(ctx, Event{Kind: KindChange, TenantID: "o"})
	first := recv(t, s2).ID
	h2.Gap("t")
	if got, ok := h2.Since("o", first); !ok || len(got) != 0 {
		t.Fatal(got, ok)
	}
	// no buffer: never resumable
	nb := NewHub(Options{})
	nb.Gap("t")
	if _, ok := nb.Since("t", "x"); ok {
		t.Fatal("no buffer")
	}
}

func TestHubWanted(t *testing.T) {
	h := NewHub(Options{})
	if h.Wanted("t") {
		t.Fatal("nobody listens")
	}
	s, _ := h.Subscribe("t", "u")
	if !h.Wanted("t") || h.Wanted("o") {
		t.Fatal("local subscriber")
	}
	h.Unsubscribe(s)
	h.SetBus(&fakeBus{})
	if !h.Wanted("o") {
		t.Fatal("other instances may listen")
	}
}

type fakeBus struct{ sent []Event }

func (b *fakeBus) Send(_ context.Context, ev Event) error { b.sent = append(b.sent, ev); return nil }
func (b *fakeBus) Run(context.Context, *Hub)              {}

func TestHubSlowSubscriberGetsLost(t *testing.T) {
	h := NewHub(Options{Queue: 2})
	s, _ := h.Subscribe("t", "u")
	for i := 0; i < 5; i++ {
		h.Publish(context.Background(), Event{Kind: KindChange, TenantID: "t"})
	}
	select {
	case <-s.Lost:
	default:
		t.Fatal("lost not signalled")
	}
	h.Resync()
	select {
	case <-s.Lost:
	default:
		t.Fatal("resync not signalled")
	}
}

func TestHubBusSendsAndSkipsNothing(t *testing.T) {
	h := NewHub(Options{})
	b := &fakeBus{}
	h.SetBus(b)
	h.Publish(context.Background(), Event{Kind: KindChange, TenantID: "t"})
	if len(b.sent) != 1 || b.sent[0].Origin != h.Instance() {
		t.Fatal(b.sent)
	}
	// an event of another instance keeps its id and origin
	s, _ := h.Subscribe("t", "u")
	h.Deliver(Event{ID: "abc-1", Origin: "abc", Kind: KindChange, TenantID: "t"})
	if ev := recv(t, s); ev.ID != "abc-1" {
		t.Fatal(ev)
	}
}

func TestPresence(t *testing.T) {
	now := time.Unix(1000, 0)
	h := NewHub(Options{PresenceTTL: 10 * time.Second, Now: func() time.Time { return now }})
	s, _ := h.Subscribe("t", "watcher")
	ctx := context.Background()
	beat := func(user, name, state string) {
		h.Publish(ctx, Event{Kind: KindPresence, TenantID: "t", Type: "requirement", EntityID: "r1", ProjectID: "p",
			Presence: &Presence{Type: "requirement", EntityID: "r1", ProjectID: "p", UserID: user, Name: name, State: state}})
	}
	beat("u2", "Bea", StateViewing)
	if ev := recv(t, s); ev.Presence.State != StateViewing || ev.Presence.TenantID != "" && ev.Presence.TenantID != "t" {
		t.Fatal(ev)
	}
	beat("u1", "Al", StateViewing)
	recv(t, s)
	// a heartbeat without change is not fanned out
	beat("u1", "Al", StateViewing)
	none(t, s)
	beat("u1", "Al", StateEditing)
	if ev := recv(t, s); ev.Presence.State != StateEditing {
		t.Fatal(ev)
	}
	list := h.Presence("t", "requirement", "r1")
	if len(list) != 2 || list[0].Name != "Al" || list[0].State != StateEditing || list[1].Name != "Bea" {
		t.Fatalf("%+v", list)
	}
	if len(h.Presence("other", "requirement", "r1")) != 0 {
		t.Fatal("tenants are separate")
	}
	beat("u1", "Al", StateLeft)
	if ev := recv(t, s); ev.Presence.State != StateLeft {
		t.Fatal(ev)
	}
	beat("u1", "Al", StateLeft) // already gone
	none(t, s)
	// Bea's heartbeats stop: she expires
	now = now.Add(11 * time.Second)
	if len(h.Presence("t", "requirement", "r1")) != 0 {
		t.Fatal("expired entries are not listed")
	}
	h.SweepPresence()
	if ev := recv(t, s); ev.Presence.State != StateLeft || ev.Presence.UserID != "u2" {
		t.Fatal(ev)
	}
	h.SweepPresence()
	none(t, s)
	// coming back after expiry counts as a change
	beat("u2", "Bea", StateViewing)
	recv(t, s)
	now = now.Add(11 * time.Second)
	beat("u2", "Bea", StateViewing)
	recv(t, s)
}
