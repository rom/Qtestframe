package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTraceparent(t *testing.T) {
	const v = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	sc, err := ParseTraceparent(v)
	if err != nil || !sc.Sampled || sc.TraceID.String() != "4bf92f3577b34da6a3ce929d0e0e4736" || sc.SpanID.String() != "00f067aa0ba902b7" {
		t.Fatal(sc, err)
	}
	if Traceparent(sc) != v {
		t.Fatal(Traceparent(sc))
	}
	sc.Sampled = false
	if !strings.HasSuffix(Traceparent(sc), "-00") {
		t.Fatal(Traceparent(sc))
	}
	// a future version with more fields is read by its first four
	if _, err := ParseTraceparent("cc-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-x",
		"0x-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e47zz-00f067aa0ba902b7-01"} {
		if _, err := ParseTraceparent(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	h := http.Header{}
	ctx := Extract(context.Background(), http.Header{"Traceparent": {v}})
	if got, ok := SpanContextFromContext(ctx); !ok || !got.Remote {
		t.Fatal(got, ok)
	}
	Inject(ctx, h)
	if h.Get(Header) != v {
		t.Fatal(h)
	}
	if Extract(context.Background(), http.Header{"Traceparent": {"junk"}}) != context.Background() {
		t.Fatal("junk header changed the context")
	}
	h2 := http.Header{}
	Inject(context.Background(), h2)
	if len(h2) != 0 {
		t.Fatal(h2)
	}
}

func TestNilSpanAndOff(t *testing.T) {
	var s *Span
	s.SetName("x")
	s.SetAttr(Attr{"a", 1})
	s.SetError(errors.New("x"))
	s.SetStatus(StatusOK, "")
	s.End()
	if s.Context().IsValid() {
		t.Fatal("nil span has a context")
	}
	tr := New()
	ctx, sp := tr.Start(context.Background(), "x", KindServer)
	if sp != nil || ctx != context.Background() || tr.Enabled() {
		t.Fatal("off tracer started a span")
	}
	var nilTracer *Tracer
	if _, sp := nilTracer.Start(context.Background(), "x", KindServer); sp != nil {
		t.Fatal("nil tracer")
	}
	if _, sp := StartChild(context.Background(), "q", KindClient); sp != nil {
		t.Fatal("child without parent")
	}
	tr.Configure(Config{Enabled: true}) // no endpoint: stays off
	if tr.Enabled() {
		t.Fatal("enabled without endpoint")
	}
	if ContextWithSpan(context.Background(), nil) != context.Background() || ContextWithRemote(context.Background(), SpanContext{}) != context.Background() {
		t.Fatal("contexts")
	}
}

func TestTracesURLAndHeaders(t *testing.T) {
	for in, want := range map[string]string{
		"":                                   "",
		"http://otel:4318":                   "http://otel:4318/v1/traces",
		"http://otel:4318/":                  "http://otel:4318/v1/traces",
		"https://otel.example.com/v1/traces": "https://otel.example.com/v1/traces",
		"https://x/custom":                   "https://x/custom",
		"otel:4318":                          "otel:4318",
	} {
		if got := TracesURL(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	h, err := ParseHeaders(" Authorization=Bearer%20abc , x-scope=team-1,,")
	if err != nil || h["Authorization"] != "Bearer abc" || h["x-scope"] != "team-1" || len(h) != 2 {
		t.Fatal(h, err)
	}
	if h, _ := ParseHeaders("k=100%zz"); h["k"] != "100%zz" {
		t.Fatal(h) // an invalid escape is kept as it is
	}
	for _, bad := range []string{"novalue", "=x", "bad key=x", "k=a\nb"} {
		if _, err := ParseHeaders(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestSampling(t *testing.T) {
	var lo, hi TraceID
	hi[8] = 0xff
	if !sampled(lo, 0.5) || sampled(hi, 0.5) || !sampled(hi, 1) || sampled(lo, 0) {
		t.Fatal("ratio sampling")
	}
	tr := New()
	tr.Configure(Config{Enabled: true, Endpoint: "http://127.0.0.1:1", SampleRatio: -1})
	if tr.Config().SampleRatio != 0 || tr.Config().ServiceName != "qtestframe" {
		t.Fatal(tr.Config())
	}
	_, s := tr.Start(context.Background(), "root", KindServer, Attr{"a", 1})
	if s == nil || s.Context().Sampled || len(s.attrs) != 0 {
		t.Fatal("ratio 0 sampled")
	}
	// the parent's decision wins over the ratio
	ctx := ContextWithRemote(context.Background(), SpanContext{TraceID: TraceID{1}, SpanID: SpanID{2}, Sampled: true})
	_, s = tr.Start(ctx, "child", KindServer)
	if !s.Context().Sampled || s.parent != (SpanID{2}) || s.Context().TraceID != (TraceID{1}) {
		t.Fatal(s.Context())
	}
	tr.Configure(Config{Enabled: true, Endpoint: "http://127.0.0.1:1", SampleRatio: 5})
	if tr.Config().SampleRatio != 1 {
		t.Fatal(tr.Config())
	}
	_ = tr.Shutdown(context.Background())
}

type collector struct {
	mu     sync.Mutex
	bodies []otlpRequest
	header http.Header
	status int
}

func (c *collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var req otlpRequest
	_ = json.Unmarshal(b, &req)
	c.mu.Lock()
	c.bodies = append(c.bodies, req)
	c.header = r.Header.Clone()
	st := c.status
	c.mu.Unlock()
	if st != 0 {
		w.WriteHeader(st)
	}
}

func (c *collector) spans() []otlpSpan {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []otlpSpan
	for _, b := range c.bodies {
		for _, rs := range b.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				out = append(out, ss.Spans...)
			}
		}
	}
	return out
}

func TestExport(t *testing.T) {
	col := &collector{}
	srv := httptest.NewServer(col)
	defer srv.Close()
	tr := New()
	tr.Interval = 10 * time.Millisecond
	tr.BatchSize = 2
	tr.Configure(Config{Enabled: true, Endpoint: srv.URL, SampleRatio: 1, ServiceName: "svc", ServiceVersion: "1.2",
		Headers: map[string]string{"Authorization": "Bearer <token>"}})
	ctx, root := tr.Start(context.Background(), "GET", KindServer, Attr{"s", "x"}, Attr{"b", true}, Attr{"i", 3}, Attr{"i64", int64(4)},
		Attr{"f", 1.5}, Attr{"nan", math.NaN()}, Attr{"other", []int{1}}, Attr{"nil", nil})
	root.SetName("GET /x")
	_, child := StartChild(ctx, "db.query", KindClient)
	child.SetError(errors.New("boom"))
	child.End()
	root.SetStatus(StatusOK, "")
	root.End()
	root.End() // once only
	deadline := time.Now().Add(5 * time.Second)
	for len(col.spans()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	spans := col.spans()
	if len(spans) != 2 {
		t.Fatal(spans)
	}
	c, r := spans[0], spans[1]
	if c.Name != "db.query" || c.ParentSpanID != r.SpanID || c.Status.Code != StatusError || c.Status.Message != "boom" || c.Kind != int(KindClient) {
		t.Fatal(c)
	}
	if r.Name != "GET /x" || r.ParentSpanID != "" || r.TraceID != c.TraceID || len(r.Attributes) != 8 || r.Kind != int(KindServer) {
		t.Fatal(r)
	}
	col.mu.Lock()
	res := col.bodies[0].ResourceSpans[0].Resource.Attributes
	auth := col.header.Get("Authorization")
	col.mu.Unlock()
	if *res[0].Value.StringValue != "svc" || auth != "Bearer <token>" || len(res) != 4 {
		t.Fatal(res, auth)
	}
	st := tr.Stats()
	if st.Exported != 2 || st.LastExport == nil || st.QueueMaximum != 4096 {
		t.Fatal(st)
	}

	// errors of the collector are counted
	col.mu.Lock()
	col.status = 500
	col.mu.Unlock()
	_, s := tr.Start(context.Background(), "x", KindInternal)
	s.End()
	if err := tr.Shutdown(context.Background()); err == nil {
		t.Fatal("expected export error")
	}
	if st := tr.Stats(); st.Failed != 1 || st.LastError == "" || st.LastErrorAt == nil {
		t.Fatal(st)
	}
	// a full queue drops spans
	tr.MaxQueue = 1
	_, a := tr.Start(context.Background(), "a", KindInternal)
	_, b := tr.Start(context.Background(), "b", KindInternal)
	a.End()
	b.End()
	if tr.Stats().Dropped != 1 {
		t.Fatal(tr.Stats())
	}
	// spans queued while the endpoint is removed are dropped
	tr.cfg.Store(&Config{})
	if err := tr.Flush(context.Background()); err != nil || tr.Stats().Dropped != 2 {
		t.Fatal(err, tr.Stats())
	}
}

func TestTransport(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(Header)
		w.WriteHeader(404)
	}))
	defer srv.Close()
	tr := New()
	tr.Configure(Config{Enabled: true, Endpoint: "http://127.0.0.1:1", SampleRatio: 1})
	defer tr.Shutdown(context.Background())
	c := &http.Client{Transport: &Transport{}}
	// without a span: nothing is sent
	req, _ := http.NewRequest("GET", srv.URL+"/a?token=x", nil)
	res, err := c.Do(req)
	if err != nil || got != "" {
		t.Fatal(err, got)
	}
	res.Body.Close()
	ctx, root := tr.Start(context.Background(), "job", KindInternal)
	req, _ = http.NewRequestWithContext(ctx, "GET", srv.URL+"/a?token=x", nil)
	req.URL.User = nil
	res, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	sc, err := ParseTraceparent(got)
	if err != nil || sc.TraceID != root.Context().TraceID || sc.SpanID == root.Context().SpanID {
		t.Fatal(got, err)
	}
	if req.Header.Get(Header) != "" {
		t.Fatal("the caller's request was changed")
	}
	tr.mu.Lock()
	q := tr.queue
	tr.mu.Unlock()
	if len(q) != 1 || q[0].status != StatusError || strings.Contains(q[0].attrs[2].Value.(string), "token") {
		t.Fatal(q)
	}
	// a failing request
	req, _ = http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:1/", nil)
	if _, err := c.Do(req); err == nil {
		t.Fatal("expected error")
	}
	// tracing turned off while a span is open
	tr.Configure(Config{})
	req, _ = http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	res, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
}
