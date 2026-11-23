package live

import (
	"sort"
	"sync"
	"time"
)

// Presence states.
const (
	StateViewing = "viewing"
	StateEditing = "editing"
	StateLeft    = "left"
)

// Presence says that a user views or edits an item.
type Presence struct {
	TenantID  string    `json:"tenant_id,omitempty"`
	Type      string    `json:"type"`
	EntityID  string    `json:"entity_id"`
	ProjectID string    `json:"project_id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar,omitempty"`
	State     string    `json:"state"`
	Expires   time.Time `json:"expires"`
}

type presenceKey struct{ tenant, typ, id string }

// presenceSet holds who is on which item, in memory.
type presenceSet struct {
	ttl time.Duration
	now func() time.Time
	mu  sync.Mutex
	m   map[presenceKey]map[string]Presence // by user id
}

func newPresenceSet(ttl time.Duration, now func() time.Time) *presenceSet {
	return &presenceSet{ttl: ttl, now: now, m: map[presenceKey]map[string]Presence{}}
}

// apply records a heartbeat or a departure; it reports whether the set of
// people on the item (or their state) changed.
func (ps *presenceSet) apply(p Presence) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	k := presenceKey{p.TenantID, p.Type, p.EntityID}
	users := ps.m[k]
	old, had := users[p.UserID]
	if had && old.Expires.Before(ps.now()) {
		had = false
	}
	if p.State == StateLeft {
		if users != nil {
			delete(users, p.UserID)
			if len(users) == 0 {
				delete(ps.m, k)
			}
		}
		return had
	}
	if users == nil {
		users = map[string]Presence{}
		ps.m[k] = users
	}
	if p.Expires.IsZero() {
		p.Expires = ps.now().Add(ps.ttl)
	}
	users[p.UserID] = p
	return !had || old.State != p.State
}

// list returns who is on an item now, by name.
func (ps *presenceSet) list(tenant, typ, id string) []Presence {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	now := ps.now()
	out := []Presence{}
	for _, p := range ps.m[presenceKey{tenant, typ, id}] {
		if p.Expires.After(now) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

// sweep removes the expired entries and returns them (state left).
func (ps *presenceSet) sweep() []Presence {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	now := ps.now()
	var gone []Presence
	for k, users := range ps.m {
		for u, p := range users {
			if !p.Expires.After(now) {
				delete(users, u)
				p.State = StateLeft
				gone = append(gone, p)
			}
		}
		if len(users) == 0 {
			delete(ps.m, k)
		}
	}
	return gone
}

// Presence returns who is viewing or editing an item now.
func (h *Hub) Presence(tenant, typ, id string) []Presence {
	return h.presence.list(tenant, typ, id)
}

// SweepPresence removes expired presences and tells the local subscribers
// (every instance sweeps its own copy).
func (h *Hub) SweepPresence() {
	for _, p := range h.presence.sweep() {
		p := p
		h.mu.Lock()
		h.fanout(Event{ID: "", Kind: KindPresence, TenantID: p.TenantID, Type: p.Type, EntityID: p.EntityID, ProjectID: p.ProjectID,
			ActorID: p.UserID, ActorName: p.Name, Presence: &p, Time: h.opts.Now().UTC(), Origin: h.instance})
		h.mu.Unlock()
	}
}
