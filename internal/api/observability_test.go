package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/tracing"
)

func scrape(t *testing.T, c *client, headers map[string]string) (int, string) {
	t.Helper()
	old := c.headers
	c.headers = headers
	res := c.raw("GET", "/metrics", nil)
	c.headers = old
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestMetricsEndpoint(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Server.TrustedProxies = []string{"127.0.0.1"} })
	anon := e.anon()
	// off by default
	if code, _ := scrape(t, anon, nil); code != 404 {
		t.Fatalf("off: %d", code)
	}
	a := e.login("admin@example.com")
	anon.m("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "wrong-password1"}, 401)
	anon.m("POST", "/api/v1/auth/login", map[string]string{"email": "nobody@example.com", "password": "wrong-password1"}, 401)
	a.m("GET", "/api/v1/projects", nil, 200)
	a.m("GET", "/api/v1/nope", nil, 404)

	// on, without protection: loopback only (the test client is local)
	putSettings(a, map[string]any{"metrics.enabled": true}, 200)
	code, body := scrape(t, anon, nil)
	if code != 200 {
		t.Fatalf("loopback: %d %s", code, body)
	}
	for _, want := range []string{
		`qtf_http_requests_total{route="/api/v1/projects",method="GET",status="2xx"} 1`,
		`qtf_http_requests_total{route="/api/",method="GET",status="4xx"} 1`,
		`qtf_http_request_duration_seconds_bucket{route="/api/v1/projects",method="GET",le="+Inf"} 1`,
		"qtf_http_requests_in_flight 1",
		`qtf_logins_total{method="local",result="failure"} 2`,
		`qtf_logins_total{method="local",result="success"} 1`,
		`qtf_db_connections{state="idle"}`,
		"qtf_db_max_open_connections",
		"go_goroutines ", "go_memstats_alloc_bytes ", "go_gc_cycles_total ", "process_start_time_seconds ",
		`qtf_build_info{version="`, "qtf_live_connections 0", "qtf_script_jobs_running 0",
		"qtf_projects 0", "qtf_tenants 1", "qtf_users 1", "qtf_requirements 0", "qtf_tests 0", "qtf_runs 0",
		`qtf_tracing_spans_total{outcome="exported"} 0`,
		"# TYPE qtf_http_request_duration_seconds histogram",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	// the business gauges are cached
	a.m("POST", "/api/v1/projects", map[string]any{"key": "OBS", "name": "Observed"}, 201)
	if _, body := scrape(t, anon, nil); !strings.Contains(body, "qtf_projects 0") {
		t.Error("counts not cached")
	}
	e.srv.obs.countsMu.Lock()
	e.srv.obs.countsAt = time.Now().Add(-2 * countsTTL)
	e.srv.obs.countsMu.Unlock()
	if _, body := scrape(t, anon, nil); !strings.Contains(body, "qtf_projects 1") {
		t.Error("counts not refreshed")
	}

	// a token: required
	putSettings(a, map[string]any{"metrics.token": "<token>"}, 200)
	if code, _ := scrape(t, anon, nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := scrape(t, anon, map[string]string{"Authorization": "Bearer nope"}); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	if code, _ := scrape(t, anon, map[string]string{"Authorization": "Bearer <token>"}); code != 200 {
		t.Fatalf("token: %d", code)
	}
	st := getSettings(t, a)
	for _, s := range st.Settings {
		if s.Key == "metrics.token" && (s.Value != nil || s.HasSecret == nil || !*s.HasSecret) {
			t.Fatalf("the token is not write-only: %+v", s)
		}
	}

	// an allowlist: the client address (X-Forwarded-For from a trusted proxy)
	putSettings(a, map[string]any{"metrics.token": "", "metrics.allowed_ips": []string{"10.0.0.0/8", "192.0.2.7"}}, 200)
	if code, _ := scrape(t, anon, nil); code != 403 {
		t.Fatalf("loopback not in the allowlist: %d", code)
	}
	if code, _ := scrape(t, anon, map[string]string{"X-Forwarded-For": "10.1.2.3"}); code != 200 {
		t.Fatalf("allowed range: %d", code)
	}
	if code, _ := scrape(t, anon, map[string]string{"X-Forwarded-For": "192.0.2.7"}); code != 200 {
		t.Fatalf("allowed address: %d", code)
	}
	putSettings(a, map[string]any{"metrics.allowed_ips": []string{}}, 200)
	if code, _ := scrape(t, anon, map[string]string{"X-Forwarded-For": "10.1.2.3"}); code != 403 {
		t.Fatalf("remote without protection: %d", code)
	}
	putSettings(a, map[string]any{"metrics.allowed_ips": []string{"not-an-ip"}}, 400)

	// the status in the settings
	var view struct {
		Monitoring monitoringView `json:"monitoring"`
	}
	a.call("GET", "/api/v1/admin/settings", nil, 200, &view)
	if !view.Monitoring.MetricsEnabled || strings.Join(view.Monitoring.MetricsProtection, ",") != "loopback" || view.Monitoring.TracingEnabled {
		t.Fatalf("%+v", view.Monitoring)
	}

	// the main listener does not serve it when a separate listener is set
	e.srv.sys.boot.Metrics.Listen = "127.0.0.1:9090"
	if code, _ := scrape(t, anon, nil); code != 404 {
		t.Fatalf("separate listener: %d", code)
	}
	ms := httptest.NewServer(e.srv.MetricsHandler())
	defer ms.Close()
	res, err := http.Get(ms.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatal(res.StatusCode, res.Header)
	}
}

func TestMetricsHelpers(t *testing.T) {
	if routeLabel("") != "unmatched" || routeLabel("GET /api/v1/x/{id}") != "/api/v1/x/{id}" || routeLabel("/") != "/" {
		t.Fatal("routeLabel")
	}
	if statusClass(0) != "2xx" || statusClass(503) != "5xx" || knownMethod("BREW") != "OTHER" || knownMethod("PATCH") != "PATCH" {
		t.Fatal("labels")
	}
	if ipAllowed(nil, []string{"10.0.0.0/8"}) {
		t.Fatal("nil ip")
	}
	s := &Server{}
	s.countJob("x", "y")     // no observability: ignored
	s.countLogin("x", false) // no observability: ignored
}

func TestJobAndLoginCounters(t *testing.T) {
	e := newEnv(t)
	e.srv.countLogin("oidc google", true)
	e.srv.countLogin("weird", false)
	e.srv.countJob("req_sync", "ok")
	var b strings.Builder
	if err := e.srv.obs.reg.WriteText(&b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`qtf_logins_total{method="oidc",result="success"} 1`, `qtf_logins_total{method="other",result="failure"} 1`,
		`qtf_jobs_total{kind="req_sync",result="ok"} 1`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}

// A request panicking is counted as a 500.
func TestObservePanic(t *testing.T) {
	e := newEnv(t)
	h := e.srv.recoverer(e.srv.observe(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != 500 || e.srv.obs.requests.With("unmatched", "GET", "5xx").Value() != 1 || e.srv.obs.inflight.Value() != 0 {
		t.Fatal(rec.Code)
	}
}

type spanSink struct {
	mu    sync.Mutex
	spans []map[string]any
}

func (k *spanSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []map[string]any `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			k.spans = append(k.spans, ss.Spans...)
		}
	}
}

func TestTracing(t *testing.T) {
	sink := &spanSink{}
	col := httptest.NewServer(sink)
	defer col.Close()
	// an outgoing call made inside a request carries the trace
	var gotParent string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gotParent = r.Header.Get("traceparent") }))
	defer hook.Close()

	e := newEnv(t)
	a := e.login("admin@example.com")
	putSettings(a, map[string]any{"tracing.enabled": true, "tracing.endpoint": col.URL, "tracing.sample_ratio": "1",
		"tracing.service_name": "qtf-test", "tracing.headers": "Authorization=Bearer <token>"}, 200)
	putSettings(a, map[string]any{"tracing.headers": "bad header"}, 400)
	putSettings(a, map[string]any{"tracing.sample_ratio": "2"}, 400)
	if !e.srv.Tracer().Enabled() || e.srv.Tracer().Config().ServiceName != "qtf-test" || e.srv.Tracer().Config().Headers["Authorization"] != "Bearer <token>" {
		t.Fatal(e.srv.Tracer().Config())
	}
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	a.headers = map[string]string{"traceparent": parent}
	a.m("GET", "/api/v1/projects", nil, 200)
	a.headers = nil
	a.m("GET", "/healthz", nil, 200) // not traced

	// the outbound client propagates the context of the request
	ctx, span := e.srv.Tracer().Start(context.Background(), "job", tracing.KindInternal)
	e.srv.HTTPClient = e.srv.outboundClient()
	req, _ := http.NewRequestWithContext(ctx, "GET", hook.URL, nil)
	res, err := e.srv.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	span.End()
	if sc, err := tracing.ParseTraceparent(gotParent); err != nil || sc.TraceID != span.Context().TraceID {
		t.Fatalf("outbound traceparent %q %v", gotParent, err)
	}

	if err := e.srv.ShutdownTracing(context.Background()); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	spans := sink.spans
	sink.mu.Unlock()
	var server, db bool
	serverID := ""
	for _, s := range spans {
		if s["name"] == "GET /api/v1/projects" {
			server = s["traceId"] == "4bf92f3577b34da6a3ce929d0e0e4736" && s["parentSpanId"] == "00f067aa0ba902b7"
			serverID, _ = s["spanId"].(string)
		}
		if s["name"] == "/healthz" || s["name"] == "GET /healthz" {
			t.Error("health check traced")
		}
	}
	for _, s := range spans {
		if strings.HasPrefix(s["name"].(string), "db ") && s["parentSpanId"] == serverID {
			db = true
		}
	}
	if !server || !db {
		t.Fatalf("server %v db %v: %v", server, db, spans)
	}
	var view struct {
		Monitoring monitoringView `json:"monitoring"`
	}
	a.call("GET", "/api/v1/admin/settings", nil, 200, &view)
	if !view.Monitoring.TracingEnabled || view.Monitoring.Spans.Exported == 0 || view.Monitoring.SampleRatio != 1 {
		t.Fatalf("%+v", view.Monitoring)
	}
	// headers that cannot be parsed are ignored with a warning
	var logged string
	cfg := config.Default()
	cfg.Tracing.Headers = "bad key=x"
	cfg.Tracing.SampleRatio = "x"
	tc := tracingConfig(&cfg, func(msg string, _ ...any) { logged = msg })
	if tc.Headers != nil || tc.SampleRatio != 1 || logged == "" {
		t.Fatal(tc, logged)
	}
}

func TestReadyzMigrations(t *testing.T) {
	e := newEnv(t)
	c := e.anon()
	if _, err := e.st.DB.Exec(`DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)`); err != nil {
		t.Fatal(err)
	}
	if m := c.m("GET", "/readyz", nil, 503); !strings.Contains(m["error"].(string), "1 database migrations") {
		t.Fatal(m)
	}
}
