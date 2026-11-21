package logging

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

// Manager owns a logger whose level, format and syslog forwarding can be
// changed while the server runs (Administration → System settings).
// Loggers derived with With/WithGroup follow the changes.
type Manager struct {
	out  io.Writer
	root *dynRoot

	mu     sync.Mutex
	closer io.Closer
	cfg    config.Logging
}

type generation struct {
	h   slog.Handler
	gen uint64
}

type dynRoot struct {
	cur atomic.Pointer[generation]
}

// NewManager builds the logger for cfg writing to out (and syslog).
func NewManager(cfg config.Logging, out io.Writer) (*Manager, error) {
	h, closer, err := build(cfg, out)
	if err != nil {
		return nil, err
	}
	m := &Manager{out: out, root: &dynRoot{}, closer: closer, cfg: cfg}
	m.root.cur.Store(&generation{h: h, gen: 1})
	return m, nil
}

// Logger returns the managed logger.
func (m *Manager) Logger() *slog.Logger { return slog.New(&dynamic{root: m.root}) }

// Config returns the configuration in use.
func (m *Manager) Config() config.Logging {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Reconfigure switches to cfg. On error the previous configuration stays
// in use. The previous syslog connection is closed.
func (m *Manager) Reconfigure(cfg config.Logging) error {
	h, closer, err := build(cfg, m.out)
	if err != nil {
		return err
	}
	m.mu.Lock()
	old := m.closer
	m.closer, m.cfg = closer, cfg
	prev := m.root.cur.Load()
	m.root.cur.Store(&generation{h: h, gen: prev.gen + 1})
	m.mu.Unlock()
	return old.Close()
}

// Close releases the syslog connection.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closer.Close()
}

// build returns the handler for cfg (as New does).
func build(cfg config.Logging, out io.Writer) (slog.Handler, io.Closer, error) {
	l, closer, err := New(cfg, out)
	if err != nil {
		return nil, nil, err
	}
	return l.Handler(), closer, nil
}

// dynamic is a handler that delegates to the root's current handler with
// the attributes and groups added by With/WithGroup applied.
type dynamic struct {
	root  *dynRoot
	ops   []func(slog.Handler) slog.Handler
	cache atomic.Pointer[generation]
}

func (d *dynamic) current() slog.Handler {
	g := d.root.cur.Load()
	if c := d.cache.Load(); c != nil && c.gen == g.gen {
		return c.h
	}
	h := g.h
	for _, op := range d.ops {
		h = op(h)
	}
	d.cache.Store(&generation{h: h, gen: g.gen})
	return h
}

func (d *dynamic) Enabled(ctx context.Context, l slog.Level) bool {
	return d.current().Enabled(ctx, l)
}

func (d *dynamic) Handle(ctx context.Context, r slog.Record) error {
	return d.current().Handle(ctx, r)
}

func (d *dynamic) with(op func(slog.Handler) slog.Handler) *dynamic {
	ops := make([]func(slog.Handler) slog.Handler, len(d.ops), len(d.ops)+1)
	copy(ops, d.ops)
	return &dynamic{root: d.root, ops: append(ops, op)}
}

func (d *dynamic) WithAttrs(a []slog.Attr) slog.Handler {
	return d.with(func(h slog.Handler) slog.Handler { return h.WithAttrs(a) })
}

func (d *dynamic) WithGroup(n string) slog.Handler {
	return d.with(func(h slog.Handler) slog.Handler { return h.WithGroup(n) })
}

// SendTestMessage sends one informational message through a new
// connection for cfg (the "Send test message" button) and closes it.
func SendTestMessage(cfg config.Syslog, msg []byte) error {
	w, err := NewWriter(cfg)
	if err != nil {
		return err
	}
	defer w.Close()
	return w.Send(6, time.Now(), msg)
}
