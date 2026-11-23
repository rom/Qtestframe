package live

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/testutil"
)

// TestPGBusAcrossInstances: two hubs (two server instances) on one
// PostgreSQL database see each other's events and presence, and a hub
// resyncs its subscribers after its listener reconnects.
func TestPGBusAcrossInstances(t *testing.T) {
	dsn := os.Getenv("QTF_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QTF_TEST_POSTGRES_DSN not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d1 := testutil.OpenDB(t)
	var path string
	if err := d1.QueryRowContext(ctx, `SHOW search_path`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	d2, err := db.Open(ctx, "postgres", dsn+sep+"search_path="+path, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()

	start := func(d *db.DB) (*Hub, *PGBus) {
		h := NewHub(Options{Replay: 16})
		b, err := NewPGBus(ctx, d.DB, nil)
		if err != nil {
			t.Fatal(err)
		}
		b.Backoff = 20 * time.Millisecond
		h.SetBus(b)
		go b.Run(ctx, h)
		return h, b
	}
	h1, b1 := start(d1)
	h2, b2 := start(d2)
	if b1.Channel != b2.Channel || !strings.HasPrefix(b1.Channel, "qtf_live_") {
		t.Fatal(b1.Channel, b2.Channel)
	}
	s2, _ := h2.Subscribe("t", "u")
	s1, _ := h1.Subscribe("t", "u")

	// LISTEN is asynchronous: publish until the other instance hears it
	var got Event
	deadline := time.After(10 * time.Second)
	for got.ID == "" {
		h1.Publish(ctx, Event{Kind: KindChange, TenantID: "t", Type: "test", EntityID: "x", ProjectID: "p", Version: 2})
		select {
		case got = <-s2.C:
		case <-time.After(200 * time.Millisecond):
		case <-deadline:
			t.Fatal("no event across instances")
		}
	}
	if got.Origin != h1.Instance() || got.Version != 2 || got.EntityID != "x" {
		t.Fatalf("%+v", got)
	}
	// the publishing instance delivers locally once, not again from the bus
	drain := func(s *Sub) int {
		n := 0
		for {
			select {
			case <-s.C:
				n++
			case <-time.After(300 * time.Millisecond):
				return n
			}
		}
	}
	drain(s1)
	drain(s2)
	h1.Publish(ctx, Event{Kind: KindChange, TenantID: "t", EntityID: "y"})
	if n := drain(s1); n != 1 {
		t.Fatal("local copies", n)
	}
	// the other instance can resume from an event it received
	if evs, ok := h2.Since("t", got.ID); !ok || len(evs) == 0 {
		t.Fatal(evs, ok)
	}
	// presence travels too
	h2.Publish(ctx, Event{Kind: KindPresence, TenantID: "t", Type: "test", EntityID: "x", ProjectID: "p",
		Presence: &Presence{Type: "test", EntityID: "x", ProjectID: "p", UserID: "u9", Name: "Nina", State: StateEditing}})
	waitFor(t, func() bool { l := h1.Presence("t", "test", "x"); return len(l) == 1 && l[0].State == StateEditing })

	// a notification for many users is split to fit NOTIFY
	users := make([]string, 400)
	for i := range users {
		users[i] = fmt.Sprintf("user-%036d", i)
	}
	drain(s1)
	h2.Publish(ctx, Event{Kind: KindNotification, TenantID: "t", Users: users})
	seen := 0
	waitFor(t, func() bool {
		for {
			select {
			case ev := <-s1.C:
				seen += len(ev.Users)
			default:
				return seen == len(users)
			}
		}
	})

	// the listener's connection is killed: it reconnects and resyncs
	var pid int
	if err := d1.QueryRowContext(ctx, `SELECT pid FROM pg_stat_activity WHERE query = $1 AND pid <> pg_backend_pid() ORDER BY backend_start LIMIT 1`, `LISTEN "`+b1.Channel+`"`).Scan(&pid); err != nil {
		t.Fatal("listener not found", err)
	}
	if _, err := d1.ExecContext(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s1.Lost:
	case <-s2.Lost:
	case <-time.After(10 * time.Second):
		t.Fatal("no resync after reconnect")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out")
}
