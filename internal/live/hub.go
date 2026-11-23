// Package live fans out change events and presence to the open event
// streams (Server-Sent Events) of the web clients.
//
// A Hub lives in each server instance. Publish hands an event to the local
// subscribers of its tenant and, when a Bus is set, to the other instances
// sharing the database (PostgreSQL LISTEN/NOTIFY, see PGBus); events from
// the bus are delivered to the local subscribers the same way. Every event
// is kept in a small ring buffer so that a client reconnecting with
// Last-Event-ID receives what it missed (Since). Presence (who is viewing or
// editing an item) is kept in memory with a time to live; it travels over
// the same bus but is not replayed.
//
// The hub knows tenants, not permissions: the stream handler filters every
// event by what its user may see before writing it (internal/api/live.go).
package live

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Event kinds.
const (
	// KindChange: an item was created, changed or deleted.
	KindChange = "change"
	// KindNotification: new notifications for Users (refresh the bell).
	KindNotification = "notification"
	// KindPresence: someone started or stopped viewing or editing an item.
	KindPresence = "presence"
	// KindAccess: project access or roles changed in the tenant; never sent
	// to clients, it makes the streams re-read what their users may see.
	KindAccess = "access"
	// KindReset: events were lost (slow client, bus reconnected); the
	// client reloads what it shows.
	KindReset = "reset"
)

// Event is one change in a tenant. It never carries titles or content:
// clients reload what they show through the regular API.
type Event struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	TenantID string `json:"tenant_id,omitempty"`
	// Type is the entity type (requirement, test, run, execution, ...).
	Type     string `json:"type,omitempty"`
	EntityID string `json:"entity_id,omitempty"`
	// ParentID is the owning item (the run of an execution).
	ParentID  string `json:"parent_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	// Version of a requirement or test after the change (0: not versioned).
	Version   int       `json:"version,omitempty"`
	Action    string    `json:"action,omitempty"`
	ActorID   string    `json:"actor_id,omitempty"`
	ActorName string    `json:"actor_name,omitempty"`
	Users     []string  `json:"users,omitempty"`
	Presence  *Presence `json:"presence,omitempty"`
	Time      time.Time `json:"time"`
	// Origin is the instance that published the event.
	Origin string `json:"origin,omitempty"`
}

// Bus carries events between the server instances sharing a database.
type Bus interface {
	// Send publishes an event to the other instances.
	Send(ctx context.Context, ev Event) error
	// Run receives the events of the other instances and delivers them
	// with h.Deliver until ctx ends; after a reconnection it calls
	// h.Resync (events may have been missed).
	Run(ctx context.Context, h *Hub)
}

// Errors of Subscribe.
var (
	ErrClosed  = errors.New("live: the server is shutting down")
	ErrTooMany = errors.New("live: too many open event streams")
)

// Options configure a hub.
type Options struct {
	// MaxConnections and MaxPerUser bound the subscriptions (0 = no limit).
	MaxConnections int
	MaxPerUser     int
	// Replay is the size of the replay buffer.
	Replay int
	// PresenceTTL is how long a presence lasts without a heartbeat.
	PresenceTTL time.Duration
	// Queue is the buffer of each subscription (default 256); a
	// subscription that falls behind gets a reset instead of the events.
	Queue int
	// Now is the clock (tests); nil = time.Now.
	Now func() time.Time
}

// Hub is the in-process fan-out of one server instance.
type Hub struct {
	opts     Options
	instance string
	seq      atomic.Uint64

	mu      sync.Mutex
	closed  bool
	subs    map[string]map[*Sub]struct{} // by tenant
	perUser map[string]int               // tenant/user → open subscriptions
	total   int
	ring    []Event
	head    int // next write position
	full    bool

	bus      Bus
	presence *presenceSet
}

// NewHub returns a hub with a random instance id.
func NewHub(o Options) *Hub {
	if o.Queue <= 0 {
		o.Queue = 256
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.PresenceTTL <= 0 {
		o.PresenceTTL = 45 * time.Second
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	h := &Hub{opts: o, instance: hex.EncodeToString(b), subs: map[string]map[*Sub]struct{}{}, perUser: map[string]int{}}
	if o.Replay > 0 {
		h.ring = make([]Event, o.Replay)
	}
	h.presence = newPresenceSet(o.PresenceTTL, o.Now)
	return h
}

// Instance returns the id of this hub (the Origin of its events).
func (h *Hub) Instance() string { return h.instance }

// SetBus sets the bus to the other instances (before the first Publish).
func (h *Hub) SetBus(b Bus) {
	h.mu.Lock()
	h.bus = b
	h.mu.Unlock()
}

// Sub is one subscription (one open event stream).
type Sub struct {
	Tenant, User string
	// C receives the events of the tenant.
	C chan Event
	// Lost is signalled when events were dropped (the queue was full or
	// the bus reconnected): the stream sends a reset.
	Lost chan struct{}
	// Done is closed when the hub shuts down.
	Done chan struct{}
	once sync.Once
}

func (s *Sub) lose() {
	select {
	case s.Lost <- struct{}{}:
	default:
	}
}

// Subscribe opens a subscription to the events of a tenant.
func (h *Hub) Subscribe(tenant, user string) (*Sub, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	uk := tenant + "/" + user
	if (h.opts.MaxConnections > 0 && h.total >= h.opts.MaxConnections) || (h.opts.MaxPerUser > 0 && h.perUser[uk] >= h.opts.MaxPerUser) {
		return nil, ErrTooMany
	}
	s := &Sub{Tenant: tenant, User: user, C: make(chan Event, h.opts.Queue), Lost: make(chan struct{}, 1), Done: make(chan struct{})}
	if h.subs[tenant] == nil {
		h.subs[tenant] = map[*Sub]struct{}{}
	}
	h.subs[tenant][s] = struct{}{}
	h.perUser[uk]++
	h.total++
	return s, nil
}

// Unsubscribe closes a subscription (idempotent).
func (h *Hub) Unsubscribe(s *Sub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[s.Tenant][s]; !ok {
		return
	}
	delete(h.subs[s.Tenant], s)
	if len(h.subs[s.Tenant]) == 0 {
		delete(h.subs, s.Tenant)
	}
	uk := s.Tenant + "/" + s.User
	if h.perUser[uk]--; h.perUser[uk] <= 0 {
		delete(h.perUser, uk)
	}
	h.total--
}

// Count returns the number of open subscriptions.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.total
}

// Close ends every subscription (graceful shutdown); later Subscribe
// calls fail with ErrClosed.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for _, set := range h.subs {
		for s := range set {
			s.once.Do(func() { close(s.Done) })
		}
	}
}

// Publish stamps an event (id, origin, time), delivers it to the local
// subscribers and sends it to the other instances.
func (h *Hub) Publish(ctx context.Context, ev Event) {
	if ev.ID == "" {
		ev.ID = h.instance + "-" + strconv.FormatUint(h.seq.Add(1), 36)
	}
	if ev.Origin == "" {
		ev.Origin = h.instance
	}
	if ev.Time.IsZero() {
		ev.Time = h.opts.Now().UTC()
	}
	h.Deliver(ev)
	h.mu.Lock()
	bus := h.bus
	h.mu.Unlock()
	if bus != nil {
		_ = bus.Send(ctx, ev)
	}
}

// Deliver hands an event (local or from the bus) to the subscribers of its
// tenant; change and notification events are kept for replay, presence
// updates the presence set and is only fanned out when it changed it.
func (h *Hub) Deliver(ev Event) {
	if ev.Kind == KindPresence && ev.Presence != nil {
		p := *ev.Presence
		p.TenantID = ev.TenantID
		if !h.presence.apply(p) {
			return
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if ev.Kind != KindPresence && ev.Kind != KindAccess && len(h.ring) > 0 {
		h.ring[h.head] = ev
		h.head = (h.head + 1) % len(h.ring)
		if h.head == 0 {
			h.full = true
		}
	}
	h.fanout(ev)
}

// fanout queues an event for the subscribers of its tenant (h.mu held).
func (h *Hub) fanout(ev Event) {
	if h.closed {
		return
	}
	for s := range h.subs[ev.TenantID] {
		select {
		case s.C <- ev:
		default:
			s.lose()
		}
	}
}

// Wanted reports whether events of a tenant can reach anyone: a local
// subscriber, or other instances over the bus. When not, the caller may
// skip building the event; it calls Gap instead so that a client resuming
// later reloads rather than missing it.
func (h *Hub) Wanted(tenant string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bus != nil || len(h.subs[tenant]) > 0
}

// Gap records in the replay buffer that events of a tenant were skipped.
func (h *Hub) Gap(tenant string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.ring) == 0 {
		return
	}
	prev := (h.head - 1 + len(h.ring)) % len(h.ring)
	if (h.head > 0 || h.full) && h.ring[prev].Kind == KindReset && h.ring[prev].TenantID == tenant {
		return
	}
	h.ring[h.head] = Event{Kind: KindReset, TenantID: tenant}
	h.head = (h.head + 1) % len(h.ring)
	if h.head == 0 {
		h.full = true
	}
}

// Resync tells every subscriber that events may have been missed.
func (h *Hub) Resync() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for s := range set {
			s.lose()
		}
	}
}

// Since returns the events of a tenant after the one with id last, oldest
// first. ok is false when last is no longer (or was never) in the buffer:
// the client must reload instead.
func (h *Hub) Since(tenant, last string) (out []Event, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := h.head
	start := 0
	if h.full {
		n = len(h.ring)
		start = h.head
	}
	found := false
	for i := 0; i < n; i++ {
		ev := h.ring[(start+i)%max(len(h.ring), 1)]
		if found {
			if ev.TenantID == tenant {
				if ev.Kind == KindReset {
					return nil, false
				}
				out = append(out, ev)
			}
			continue
		}
		if ev.ID == last {
			found = true
		}
	}
	return out, found
}
