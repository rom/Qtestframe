// Package tracing is a minimal OpenTelemetry-compatible tracer: spans
// with attributes and status, W3C Trace Context propagation (the
// traceparent header) in and out, head sampling by trace id with the
// parent's decision respected, and an exporter that sends batches of
// spans to an OTLP/HTTP collector as JSON (POST …/v1/traces).
//
// A nil *Span is valid and does nothing, so callers need not check
// whether tracing is on: Start returns a nil span when it is off.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TraceID and SpanID identify a trace and a span.
type (
	TraceID [16]byte
	SpanID  [8]byte
)

func (t TraceID) String() string { return hex.EncodeToString(t[:]) }
func (s SpanID) String() string  { return hex.EncodeToString(s[:]) }

// IsValid reports whether the id is not all zeros.
func (t TraceID) IsValid() bool { return t != TraceID{} }

// IsValid reports whether the id is not all zeros.
func (s SpanID) IsValid() bool { return s != SpanID{} }

// SpanContext is the part of a span that crosses process boundaries.
type SpanContext struct {
	TraceID TraceID
	SpanID  SpanID
	Sampled bool
	// Remote is true for a context received in a traceparent header.
	Remote bool
}

// IsValid reports whether both ids are set.
func (c SpanContext) IsValid() bool { return c.TraceID.IsValid() && c.SpanID.IsValid() }

// Kind is the OTLP span kind.
type Kind int

// Span kinds (values of the OTLP protocol).
const (
	KindInternal Kind = 1
	KindServer   Kind = 2
	KindClient   Kind = 3
)

// Status codes (values of the OTLP protocol).
const (
	StatusUnset = 0
	StatusOK    = 1
	StatusError = 2
)

// Attr is one attribute of a span; Value is a string, bool, int, int64
// or float64.
type Attr struct {
	Key   string
	Value any
}

// Span is an operation of a trace. Its methods are safe on a nil span
// and for concurrent use.
type Span struct {
	tracer *Tracer
	sc     SpanContext
	parent SpanID
	kind   Kind
	start  time.Time

	mu        sync.Mutex
	name      string
	attrs     []Attr
	status    int
	statusMsg string
	ended     bool
}

// Context returns the span context (zero for a nil span).
func (s *Span) Context() SpanContext {
	if s == nil {
		return SpanContext{}
	}
	return s.sc
}

// SetName changes the name of the span.
func (s *Span) SetName(name string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.name = name
	s.mu.Unlock()
}

// SetAttr adds attributes.
func (s *Span) SetAttr(attrs ...Attr) {
	if s == nil || !s.sc.Sampled {
		return
	}
	s.mu.Lock()
	s.attrs = append(s.attrs, attrs...)
	s.mu.Unlock()
}

// SetError marks the span as failed with err (nil: no change).
func (s *Span) SetError(err error) {
	if s == nil || err == nil {
		return
	}
	s.SetStatus(StatusError, err.Error())
}

// SetStatus sets the status code and its message.
func (s *Span) SetStatus(code int, msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.status, s.statusMsg = code, msg
	s.mu.Unlock()
}

// End finishes the span and queues it for export (sampled spans only).
// Later calls do nothing.
func (s *Span) End() {
	if s == nil {
		return
	}
	end := time.Now()
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	d := spanData{sc: s.sc, parent: s.parent, kind: s.kind, name: s.name, start: s.start, end: end,
		attrs: s.attrs, status: s.status, statusMsg: s.statusMsg}
	s.mu.Unlock()
	if s.sc.Sampled && s.tracer != nil {
		s.tracer.enqueue(d)
	}
}

// spanData is an ended span.
type spanData struct {
	sc        SpanContext
	parent    SpanID
	kind      Kind
	name      string
	start     time.Time
	end       time.Time
	attrs     []Attr
	status    int
	statusMsg string
}

// ---------------------------------------------------------------- context

type spanKey struct{}
type remoteKey struct{}

// ContextWithSpan returns ctx carrying span.
func ContextWithSpan(ctx context.Context, s *Span) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, spanKey{}, s)
}

// SpanFromContext returns the span of ctx (nil if none).
func SpanFromContext(ctx context.Context) *Span {
	s, _ := ctx.Value(spanKey{}).(*Span)
	return s
}

// ContextWithRemote returns ctx carrying a parent received from another
// process (Extract).
func ContextWithRemote(ctx context.Context, sc SpanContext) context.Context {
	if !sc.IsValid() {
		return ctx
	}
	sc.Remote = true
	return context.WithValue(ctx, remoteKey{}, sc)
}

// SpanContextFromContext returns the context of the current span of ctx,
// else a remote parent; ok is false when there is neither.
func SpanContextFromContext(ctx context.Context) (SpanContext, bool) {
	if s := SpanFromContext(ctx); s != nil {
		return s.sc, true
	}
	sc, ok := ctx.Value(remoteKey{}).(SpanContext)
	return sc, ok
}

// ---------------------------------------------------------------- W3C trace context

// Header is the W3C Trace Context header.
const Header = "traceparent"

// ParseTraceparent parses a traceparent header value
// ("00-<trace id>-<span id>-<flags>"). Future versions are read by their
// first four fields, as the specification asks.
func ParseTraceparent(v string) (SpanContext, error) {
	bad := errors.New("tracing: invalid traceparent")
	v = strings.TrimSpace(v)
	parts := strings.Split(v, "-")
	if len(parts) < 4 || len(parts[0]) != 2 || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return SpanContext{}, bad
	}
	if parts[0] == "ff" || (parts[0] == "00" && len(parts) != 4) || !isLowerHex(v[:55]) {
		return SpanContext{}, bad
	}
	var sc SpanContext
	var flags [1]byte
	if _, err := hex.Decode(sc.TraceID[:], []byte(parts[1])); err != nil {
		return SpanContext{}, bad
	}
	if _, err := hex.Decode(sc.SpanID[:], []byte(parts[2])); err != nil {
		return SpanContext{}, bad
	}
	if _, err := hex.Decode(flags[:], []byte(parts[3])); err != nil {
		return SpanContext{}, bad
	}
	if !sc.IsValid() {
		return SpanContext{}, bad
	}
	sc.Sampled = flags[0]&1 == 1
	return sc, nil
}

func isLowerHex(s string) bool {
	for _, c := range s {
		if !(c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// Traceparent formats sc as a traceparent header value.
func Traceparent(sc SpanContext) string {
	flags := "00"
	if sc.Sampled {
		flags = "01"
	}
	return "00-" + sc.TraceID.String() + "-" + sc.SpanID.String() + "-" + flags
}

// Extract returns ctx with the parent of the traceparent header of h (ctx
// unchanged without a valid header).
func Extract(ctx context.Context, h http.Header) context.Context {
	sc, err := ParseTraceparent(h.Get(Header))
	if err != nil {
		return ctx
	}
	return ContextWithRemote(ctx, sc)
}

// Inject sets the traceparent header of h from the current span of ctx.
func Inject(ctx context.Context, h http.Header) {
	if sc, ok := SpanContextFromContext(ctx); ok && sc.IsValid() {
		h.Set(Header, Traceparent(sc))
	}
}

// ---------------------------------------------------------------- tracer

// Config configures a tracer.
type Config struct {
	Enabled bool
	// Endpoint is the OTLP/HTTP traces URL; a URL without a path gets
	// /v1/traces.
	Endpoint string
	// Headers are sent with every export (e.g. an authorization header).
	Headers map[string]string
	// SampleRatio is the share of new traces that are recorded (0–1);
	// a trace started elsewhere follows the decision of its parent.
	SampleRatio    float64
	ServiceName    string
	ServiceVersion string
}

// Stats counts the work of the exporter.
type Stats struct {
	Exported     uint64     `json:"exported"`
	Dropped      uint64     `json:"dropped"`
	Failed       uint64     `json:"failed"`
	LastExport   *time.Time `json:"last_export,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	LastErrorAt  *time.Time `json:"last_error_at,omitempty"`
	QueueLength  int        `json:"queue_length"`
	QueueMaximum int        `json:"queue_maximum"`
}

// Tracer starts spans and exports the sampled ones.
type Tracer struct {
	cfg atomic.Pointer[Config]
	// Client sends the batches (nil: a client with a 10 s timeout).
	Client *http.Client
	// BatchSize and Interval: a batch is sent when it is full or after
	// Interval (0: 512 spans, 5 s).
	BatchSize int
	Interval  time.Duration
	// MaxQueue is the most spans waiting for export; more are dropped
	// (0: 4096).
	MaxQueue int

	mu      sync.Mutex
	queue   []spanData
	running bool
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	sending sync.Mutex

	exported, dropped, failed atomic.Uint64
	statMu                    sync.Mutex
	lastExport, lastErrorAt   time.Time
	lastError                 string

	// random is the source of ids (tests); nil = crypto/rand.
	random func([]byte)
}

// New returns a tracer that is off until Configure turns it on.
func New() *Tracer { return &Tracer{} }

// Configure applies cfg. Turning tracing on starts the exporter;
// spans started before stay with the configuration they were started with.
func (t *Tracer) Configure(cfg Config) {
	c := cfg
	if c.SampleRatio < 0 {
		c.SampleRatio = 0
	}
	if c.SampleRatio > 1 {
		c.SampleRatio = 1
	}
	if c.ServiceName == "" {
		c.ServiceName = "qtestframe"
	}
	c.Endpoint = TracesURL(c.Endpoint)
	if c.Enabled && c.Endpoint == "" {
		c.Enabled = false
	}
	t.cfg.Store(&c)
	if c.Enabled {
		t.startExporter()
	}
}

// TracesURL returns the traces URL of an OTLP/HTTP endpoint: a URL with
// only a host (or "/") gets the path /v1/traces.
func TracesURL(endpoint string) string {
	e := strings.TrimSpace(endpoint)
	if e == "" {
		return ""
	}
	if i := strings.Index(e, "://"); i >= 0 {
		rest := e[i+3:]
		if j := strings.IndexByte(rest, '/'); j < 0 || rest[j:] == "/" {
			return strings.TrimSuffix(e, "/") + "/v1/traces"
		}
	}
	return e
}

// Config returns the configuration in use.
func (t *Tracer) Config() Config {
	if c := t.cfg.Load(); c != nil {
		return *c
	}
	return Config{}
}

// Enabled reports whether new spans are recorded.
func (t *Tracer) Enabled() bool {
	c := t.cfg.Load()
	return c != nil && c.Enabled
}

func (t *Tracer) fill(b []byte) {
	if t.random != nil {
		t.random(b)
		return
	}
	_, _ = rand.Read(b)
}

func (t *Tracer) newSpanID() SpanID {
	var id SpanID
	for !id.IsValid() {
		t.fill(id[:])
	}
	return id
}

// sampled decides on a new trace from its id (the same decision on every
// server that uses the same ratio).
func sampled(id TraceID, ratio float64) bool {
	switch {
	case ratio >= 1:
		return true
	case ratio <= 0:
		return false
	}
	// the lower 63 bits of the last 8 bytes, as the OpenTelemetry
	// TraceIdRatioBased sampler does
	x := binary.BigEndian.Uint64(id[8:]) >> 1
	return x < uint64(ratio*(1<<63))
}

// Start starts a span named name as a child of the span (or remote
// parent) of ctx, or as the root of a new trace. It returns nil (and ctx)
// when tracing is off.
func (t *Tracer) Start(ctx context.Context, name string, kind Kind, attrs ...Attr) (context.Context, *Span) {
	if t == nil || !t.Enabled() {
		return ctx, nil
	}
	s := &Span{tracer: t, kind: kind, name: name, start: time.Now()}
	if p, ok := SpanContextFromContext(ctx); ok && p.IsValid() {
		s.sc = SpanContext{TraceID: p.TraceID, Sampled: p.Sampled}
		s.parent = p.SpanID
	} else {
		for !s.sc.TraceID.IsValid() {
			t.fill(s.sc.TraceID[:])
		}
		s.sc.Sampled = sampled(s.sc.TraceID, t.Config().SampleRatio)
	}
	s.sc.SpanID = t.newSpanID()
	if s.sc.Sampled {
		s.attrs = append(s.attrs, attrs...)
	}
	return ContextWithSpan(ctx, s), s
}

// StartChild starts a span only inside a sampled span of ctx (e.g. a
// database query of a traced request); otherwise it returns ctx and nil.
func StartChild(ctx context.Context, name string, kind Kind, attrs ...Attr) (context.Context, *Span) {
	p := SpanFromContext(ctx)
	if p == nil || !p.sc.Sampled || p.tracer == nil {
		return ctx, nil
	}
	return p.tracer.Start(ctx, name, kind, attrs...)
}

// ---------------------------------------------------------------- exporter

func (t *Tracer) limits() (batch int, interval time.Duration, maxQueue int) {
	batch, interval, maxQueue = t.BatchSize, t.Interval, t.MaxQueue
	if batch <= 0 {
		batch = 512
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if maxQueue <= 0 {
		maxQueue = 4096
	}
	return
}

func (t *Tracer) enqueue(d spanData) {
	batch, _, maxQueue := t.limits()
	t.mu.Lock()
	if len(t.queue) >= maxQueue {
		t.mu.Unlock()
		t.dropped.Add(1)
		return
	}
	t.queue = append(t.queue, d)
	full := len(t.queue) >= batch
	wake := t.wake
	t.mu.Unlock()
	if full && wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (t *Tracer) startExporter() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return
	}
	t.running = true
	t.wake = make(chan struct{}, 1)
	t.stop = make(chan struct{})
	t.done = make(chan struct{})
	go t.loop(t.wake, t.stop, t.done)
}

func (t *Tracer) loop(wake, stop, done chan struct{}) {
	defer close(done)
	_, interval, _ := t.limits()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		case <-wake:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = t.Flush(ctx)
		cancel()
	}
}

// Flush exports the queued spans now.
func (t *Tracer) Flush(ctx context.Context) error {
	t.sending.Lock()
	defer t.sending.Unlock()
	batch, _, _ := t.limits()
	var errs []error
	for {
		t.mu.Lock()
		n := min(len(t.queue), batch)
		spans := t.queue[:n:n]
		t.queue = t.queue[n:]
		if len(t.queue) == 0 {
			t.queue = nil
		}
		t.mu.Unlock()
		if n == 0 {
			return errors.Join(errs...)
		}
		cfg := t.Config()
		if cfg.Endpoint == "" {
			t.dropped.Add(uint64(n))
			continue
		}
		if err := t.send(ctx, cfg, spans); err != nil {
			t.failed.Add(uint64(n))
			t.statMu.Lock()
			t.lastError, t.lastErrorAt = err.Error(), time.Now()
			t.statMu.Unlock()
			errs = append(errs, err)
			if ctx.Err() != nil {
				return errors.Join(errs...)
			}
			continue
		}
		t.exported.Add(uint64(n))
		t.statMu.Lock()
		t.lastExport = time.Now()
		t.statMu.Unlock()
	}
}

// Shutdown stops the exporter and sends what is queued.
func (t *Tracer) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	running, stop, done := t.running, t.stop, t.done
	t.running = false
	t.mu.Unlock()
	if running {
		close(stop)
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return t.Flush(ctx)
}

// Stats returns the counters of the exporter.
func (t *Tracer) Stats() Stats {
	_, _, maxQueue := t.limits()
	t.mu.Lock()
	q := len(t.queue)
	t.mu.Unlock()
	s := Stats{Exported: t.exported.Load(), Dropped: t.dropped.Load(), Failed: t.failed.Load(), QueueLength: q, QueueMaximum: maxQueue}
	t.statMu.Lock()
	defer t.statMu.Unlock()
	if !t.lastExport.IsZero() {
		x := t.lastExport.UTC()
		s.LastExport = &x
	}
	if t.lastError != "" {
		x := t.lastErrorAt.UTC()
		s.LastError, s.LastErrorAt = t.lastError, &x
	}
	return s
}

func (t *Tracer) client() *http.Client {
	if t.Client != nil {
		return t.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (t *Tracer) send(ctx context.Context, cfg Config, spans []spanData) error {
	body, err := encodeOTLP(cfg, spans)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := t.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("collector answered %s", res.Status)
	}
	return nil
}

// ParseHeaders parses "key=value" pairs separated by commas (the format of
// OTEL_EXPORTER_OTLP_HEADERS); values may be percent-encoded.
func ParseHeaders(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		k, v, ok := strings.Cut(p, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" || !validToken(k) || strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("tracing: header %q is not key=value", k)
		}
		if dv, err := unescape(v); err == nil {
			v = dv
		}
		out[k] = v
	}
	return out, nil
}

func validToken(s string) bool {
	for _, c := range s {
		if !(c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// unescape decodes %XX sequences.
func unescape(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			x, err := hex.DecodeString(s[i+1 : i+3])
			if err != nil {
				return "", err
			}
			b.Write(x)
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String(), nil
}
