package api

// Observability: the Prometheus endpoint /metrics, OpenTelemetry tracing
// of requests (and, through the store, of database queries and, through
// the outbound client, of outgoing calls), and the readiness check.
//
// /metrics is off by default (metrics.enabled). When on it answers only
// requests that pass every configured protection: the bearer token
// (metrics.token) and the address allowlist (metrics.allowed_ips); with
// neither, only the loopback address. With metrics.listen it is served on
// a listener of its own (MetricsHandler) and the main listener answers 404.

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/metrics"
	"github.com/rom/qtestframe/internal/settings"
	"github.com/rom/qtestframe/internal/tracing"
)

// countsTTL is how long the business gauges (projects, requirements, …)
// are cached: counting is a query per table.
const countsTTL = 60 * time.Second

// observability holds the metrics and the tracer of a server.
type observability struct {
	reg      *metrics.Registry
	requests *metrics.CounterVec
	duration *metrics.HistogramVec
	inflight *metrics.Gauge
	jobs     *metrics.CounterVec
	logins   *metrics.CounterVec
	tracer   *tracing.Tracer
	start    time.Time

	countsMu  sync.Mutex
	counts    map[string]int64
	countsAt  time.Time
	countsErr error

	memMu sync.Mutex
	mem   runtime.MemStats
	memAt time.Time

	// migrated is set once /readyz found every migration applied.
	migrated atomic.Bool
}

func newObservability(s *Server) *observability {
	r := metrics.NewRegistry()
	o := &observability{reg: r, tracer: tracing.New(), start: time.Now()}
	o.requests = r.NewCounterVec("qtf_http_requests_total", "HTTP requests by route template, method and status class.", "route", "method", "status")
	o.duration = r.NewHistogramVec("qtf_http_request_duration_seconds", "Duration of HTTP requests (without event streams).", nil, "route", "method")
	o.inflight = r.NewGauge("qtf_http_requests_in_flight", "HTTP requests being served (without event streams).")
	o.jobs = r.NewCounterVec("qtf_jobs_total", "Finished background jobs by kind (script_job, req_sync, report_mail) and result.", "kind", "result")
	o.logins = r.NewCounterVec("qtf_logins_total", "Sign-in attempts by method and result (success, failure).", "method", "result")

	r.GaugeFunc("qtf_build_info", "Version of the server (always 1).", []string{"version", "goversion"}, func(e metrics.Emit) {
		e(1, Version, runtime.Version())
	})
	r.GaugeFunc("process_start_time_seconds", "Start time of the process since the Unix epoch in seconds.", nil, func(e metrics.Emit) {
		e(float64(o.start.UnixNano()) / 1e9)
	})
	r.GaugeFunc("go_goroutines", "Number of goroutines.", nil, func(e metrics.Emit) { e(float64(runtime.NumGoroutine())) })
	r.GaugeFunc("go_memstats_alloc_bytes", "Bytes of allocated heap objects.", nil, func(e metrics.Emit) { e(float64(o.memStats().HeapAlloc)) })
	r.GaugeFunc("go_memstats_heap_inuse_bytes", "Bytes in in-use heap spans.", nil, func(e metrics.Emit) { e(float64(o.memStats().HeapInuse)) })
	r.GaugeFunc("go_memstats_heap_objects", "Number of allocated heap objects.", nil, func(e metrics.Emit) { e(float64(o.memStats().HeapObjects)) })
	r.GaugeFunc("go_memstats_sys_bytes", "Bytes of memory obtained from the operating system.", nil, func(e metrics.Emit) { e(float64(o.memStats().Sys)) })
	r.GaugeFunc("go_gc_next_bytes", "Heap size of the next garbage collection.", nil, func(e metrics.Emit) { e(float64(o.memStats().NextGC)) })
	r.CounterFunc("go_gc_cycles_total", "Completed garbage collection cycles.", nil, func(e metrics.Emit) { e(float64(o.memStats().NumGC)) })
	r.CounterFunc("go_gc_pause_seconds_total", "Total stop-the-world pause time of the garbage collector.", nil, func(e metrics.Emit) {
		e(float64(o.memStats().PauseTotalNs) / 1e9)
	})

	// the database pool (database/sql)
	r.GaugeFunc("qtf_db_connections", "Connections of the database pool by state.", []string{"state"}, func(e metrics.Emit) {
		st := s.Store.DB.Stats()
		e(float64(st.InUse), "in_use")
		e(float64(st.Idle), "idle")
	})
	r.GaugeFunc("qtf_db_max_open_connections", "Most open connections of the database pool.", nil, func(e metrics.Emit) {
		e(float64(s.Store.DB.Stats().MaxOpenConnections))
	})
	r.CounterFunc("qtf_db_wait_total", "Times a query waited for a free connection.", nil, func(e metrics.Emit) {
		e(float64(s.Store.DB.Stats().WaitCount))
	})
	r.CounterFunc("qtf_db_wait_duration_seconds_total", "Total time queries waited for a free connection.", nil, func(e metrics.Emit) {
		e(s.Store.DB.Stats().WaitDuration.Seconds())
	})
	r.CounterFunc("qtf_db_closed_connections_total", "Connections closed by the pool by reason.", []string{"reason"}, func(e metrics.Emit) {
		st := s.Store.DB.Stats()
		e(float64(st.MaxIdleClosed), "max_idle")
		e(float64(st.MaxIdleTimeClosed), "max_idle_time")
		e(float64(st.MaxLifetimeClosed), "max_lifetime")
	})

	r.GaugeFunc("qtf_live_connections", "Open live-update event streams (Server-Sent Events).", nil, func(e metrics.Emit) {
		if s.live != nil {
			e(float64(s.live.Count()))
		}
	})
	r.GaugeFunc("qtf_script_jobs_running", "Automated tests (scripts and web tests) running on this process.", nil, func(e metrics.Emit) {
		s.jobs.mu.Lock()
		n := s.jobs.running
		s.jobs.mu.Unlock()
		e(float64(n))
	})

	// the exporter of traces
	r.CounterFunc("qtf_tracing_spans_total", "Spans handed to the trace exporter by outcome.", []string{"outcome"}, func(e metrics.Emit) {
		st := o.tracer.Stats()
		e(float64(st.Exported), "exported")
		e(float64(st.Failed), "failed")
		e(float64(st.Dropped), "dropped")
	})

	// business gauges, counted at most once a minute
	for _, g := range []struct{ name, key, help string }{
		{"qtf_tenants", "tenants", "Organisations."},
		{"qtf_users", "users", "User accounts."},
		{"qtf_projects", "projects", "Projects."},
		{"qtf_requirements", "requirements", "Requirements (current versions)."},
		{"qtf_tests", "tests", "Test cases."},
		{"qtf_runs", "runs", "Test runs."},
	} {
		key := g.key
		r.GaugeFunc(g.name, g.help+" Counted at most once a minute.", nil, func(e metrics.Emit) {
			if c := o.businessCounts(s); c != nil {
				e(float64(c[key]))
			}
		})
	}
	return o
}

// memStats returns the memory statistics, read at most once a second
// (reading stops the world briefly).
func (o *observability) memStats() *runtime.MemStats {
	o.memMu.Lock()
	defer o.memMu.Unlock()
	if time.Since(o.memAt) > time.Second {
		runtime.ReadMemStats(&o.mem)
		o.memAt = time.Now()
	}
	return &o.mem
}

// businessCounts returns the cached counts of the installation (nil when
// they cannot be read).
func (o *observability) businessCounts(s *Server) map[string]int64 {
	o.countsMu.Lock()
	defer o.countsMu.Unlock()
	if o.countsAt.IsZero() || time.Since(o.countsAt) > countsTTL {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		o.counts, o.countsErr = s.Store.SystemCounts(ctx)
		cancel()
		o.countsAt = time.Now()
		if o.countsErr != nil {
			s.Log.Warn("metrics: counting failed", "err", o.countsErr)
		}
	}
	return o.counts
}

// countJob counts a finished background job.
func (s *Server) countJob(kind, result string) {
	if s.obs == nil {
		return
	}
	s.obs.jobs.With(kind, result).Inc()
}

// countLogin counts a sign-in attempt; method is reduced to the known
// methods (bounded label values).
func (s *Server) countLogin(method string, ok bool) {
	if s.obs == nil {
		return
	}
	m, _, _ := strings.Cut(method, " ")
	switch m {
	case "local", "oidc", "saml", "mfa", "invitation", "external":
	default:
		m = "other"
	}
	result := "failure"
	if ok {
		result = "success"
	}
	s.obs.logins.With(m, result).Inc()
}

// quietPath: probes and scrapes are not traced.
func quietPath(p string) bool {
	return p == "/healthz" || p == "/readyz" || p == "/metrics"
}

const eventStreamPath = "/api/v1/events/stream"

// routeLabel is the route template of a registered pattern without its
// method ("GET /api/v1/tests/{id}" → "/api/v1/tests/{id}").
func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if _, p, ok := strings.Cut(pattern, " "); ok {
		return p
	}
	return pattern
}

func statusClass(code int) string {
	if code == 0 {
		code = http.StatusOK
	}
	return fmt.Sprintf("%dxx", code/100)
}

// knownMethod bounds the method label.
func knownMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	}
	return "OTHER"
}

// observe counts and traces each request. It runs inside recoverer, so a
// panic is counted as a 500 before recoverer answers it.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		stream := r.URL.Path == eventStreamPath
		if !stream {
			s.obs.inflight.Inc()
		}
		ctx := tracing.Extract(r.Context(), r.Header)
		var span *tracing.Span
		if !quietPath(r.URL.Path) {
			ctx, span = s.obs.tracer.Start(ctx, r.Method, tracing.KindServer,
				tracing.Attr{Key: "http.request.method", Value: r.Method}, tracing.Attr{Key: "url.path", Value: r.URL.Path},
				tracing.Attr{Key: "client.address", Value: s.clientIP(r)}, tracing.Attr{Key: "user_agent.original", Value: r.UserAgent()})
		}
		r = r.WithContext(ctx)
		sw := &statusWriter{ResponseWriter: w}
		panicked := true
		defer func() {
			if !stream {
				s.obs.inflight.Dec()
			}
			status := sw.status
			if panicked {
				status = http.StatusInternalServerError
			}
			route, method := routeLabel(r.Pattern), knownMethod(r.Method)
			s.obs.requests.With(route, method, statusClass(status)).Inc()
			if !stream {
				s.obs.duration.With(route, method).Observe(time.Since(start).Seconds())
			}
			if span != nil {
				if status == 0 {
					status = http.StatusOK
				}
				span.SetName(r.Method + " " + route)
				span.SetAttr(tracing.Attr{Key: "http.route", Value: route}, tracing.Attr{Key: "http.response.status_code", Value: status})
				if status >= 500 {
					span.SetStatus(tracing.StatusError, http.StatusText(status))
				}
				span.End()
			}
		}()
		next.ServeHTTP(sw, r)
		panicked = false
	})
}

// tracingConfig is the tracer configuration of c.
func tracingConfig(c *config.Config, log func(string, ...any)) tracing.Config {
	t := c.Tracing
	ratio, err := config.ParseRatio(t.SampleRatio)
	if err != nil {
		ratio = 1
	}
	headers, err := tracing.ParseHeaders(t.Headers)
	if err != nil {
		log("tracing: headers ignored", "err", err)
		headers = nil
	}
	return tracing.Config{Enabled: t.Enabled, Endpoint: t.Endpoint, Headers: headers, SampleRatio: ratio,
		ServiceName: t.ServiceName, ServiceVersion: Version}
}

// ShutdownTracing sends the spans still queued and stops the exporter.
func (s *Server) ShutdownTracing(ctx context.Context) error { return s.obs.tracer.Shutdown(ctx) }

// Tracer returns the tracer of the server.
func (s *Server) Tracer() *tracing.Tracer { return s.obs.tracer }

// MetricsListen is the separate address of /metrics in use since the
// start ("" = the main listener).
func (s *Server) MetricsListen() string {
	if s.sys.boot != nil {
		return s.sys.boot.Metrics.Listen
	}
	return s.Cfg.Metrics.Listen
}

// MetricsHandler serves /metrics, /healthz and /readyz on the separate
// metrics listener.
func (s *Server) MetricsHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) { s.serveMetrics(w, r, true) })
	m.HandleFunc("GET /healthz", s.handleHealth)
	m.HandleFunc("GET /readyz", s.handleReady)
	return s.recoverer(m)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) { s.serveMetrics(w, r, false) }

func (s *Server) serveMetrics(w http.ResponseWriter, r *http.Request, separate bool) {
	c := s.conf().Metrics
	if !c.Enabled || (!separate && s.MetricsListen() != "") {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
		return
	}
	if status, msg := s.metricsAccess(r, c); status != 0 {
		if status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
		}
		writeError(w, status, map[int]string{http.StatusUnauthorized: "unauthorized", http.StatusForbidden: "forbidden"}[status], msg)
		return
	}
	w.Header().Set("Content-Type", metrics.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	if err := s.obs.reg.WriteText(w); err != nil {
		s.Log.Debug("metrics: write", "err", err)
	}
}

// metricsAccess checks the protections of /metrics; 0 = allowed.
func (s *Server) metricsAccess(r *http.Request, c config.Metrics) (int, string) {
	ip := net.ParseIP(s.clientIP(r))
	if len(c.AllowedIPs) > 0 && !ipAllowed(ip, c.AllowedIPs) {
		return http.StatusForbidden, "this address may not read the metrics"
	}
	if c.Token != "" {
		got := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			got = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		}
		a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(c.Token))
		if got == "" || subtle.ConstantTimeCompare(a[:], b[:]) != 1 {
			return http.StatusUnauthorized, "a valid bearer token is required"
		}
	}
	if len(c.AllowedIPs) == 0 && c.Token == "" && (ip == nil || !ip.IsLoopback()) {
		return http.StatusForbidden, "set metrics.token or metrics.allowed_ips to read the metrics from another host"
	}
	return 0, ""
}

func ipAllowed(ip net.IP, list []string) bool {
	if ip == nil {
		return false
	}
	for _, a := range list {
		if x := net.ParseIP(a); x != nil && x.Equal(ip) {
			return true
		}
		if _, n, err := net.ParseCIDR(a); err == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- status

// monitoringView is the state of metrics and tracing in the system
// settings (section monitoring).
type monitoringView struct {
	MetricsEnabled bool `json:"metrics_enabled"`
	// MetricsListen is the separate address in use ("" = main listener).
	MetricsListen string `json:"metrics_listen"`
	// MetricsProtection: "token", "allowlist" or "loopback" (neither set).
	MetricsProtection []string      `json:"metrics_protection"`
	TracingEnabled    bool          `json:"tracing_enabled"`
	TracingEndpoint   string        `json:"tracing_endpoint"`
	SampleRatio       float64       `json:"sample_ratio"`
	Spans             tracing.Stats `json:"spans"`
}

func (s *Server) monitoringView() *monitoringView {
	c := s.conf()
	v := &monitoringView{MetricsEnabled: c.Metrics.Enabled, MetricsListen: s.MetricsListen(), MetricsProtection: []string{}}
	if c.Metrics.Token != "" {
		v.MetricsProtection = append(v.MetricsProtection, "token")
	}
	if len(c.Metrics.AllowedIPs) > 0 {
		v.MetricsProtection = append(v.MetricsProtection, "allowlist")
	}
	if len(v.MetricsProtection) == 0 {
		v.MetricsProtection = append(v.MetricsProtection, "loopback")
	}
	tc := s.obs.tracer.Config()
	v.TracingEnabled, v.TracingEndpoint, v.SampleRatio = tc.Enabled, settings.RedactURL(tc.Endpoint), tc.SampleRatio
	v.Spans = s.obs.tracer.Stats()
	return v
}
