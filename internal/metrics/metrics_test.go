package metrics

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestTextFormat(t *testing.T) {
	r := NewRegistry()
	req := r.NewCounterVec("http_requests_total", "Requests.\nSecond line \\ end", "route", "code")
	req.With("/a", "2xx").Inc()
	req.With("/a", "2xx").Add(2)
	req.With("/a", "2xx").Add(-5) // ignored
	req.With(`/b"q`+"\n", "5xx").Inc()
	g := r.NewGauge("in_flight", "In flight.")
	g.Inc()
	g.Inc()
	g.Dec()
	g.Add(0.5)
	h := r.NewHistogramVec("latency_seconds", "Latency.", []float64{1, 0.1}, "route")
	h.With("/a").Observe(0.05)
	h.With("/a").Observe(0.5)
	h.With("/a").Observe(3)
	r.GaugeFunc("up_things", "Things.", []string{"kind"}, func(e Emit) { e(2, "x"); e(math.Inf(1), "y") })
	r.CounterFunc("waits_total", "Waits.", nil, func(e Emit) { e(7) })
	gv := r.NewGaugeVec("special", "NaN and negative.", "k")
	gv.With("nan").Set(math.NaN())
	gv.With("neg").Set(math.Inf(-1))

	var b strings.Builder
	if err := r.WriteText(&b); err != nil {
		t.Fatal(err)
	}
	want := `# HELP http_requests_total Requests.\nSecond line \\ end
# TYPE http_requests_total counter
http_requests_total{route="/a",code="2xx"} 3
http_requests_total{route="/b\"q\n",code="5xx"} 1
# HELP in_flight In flight.
# TYPE in_flight gauge
in_flight 1.5
# HELP latency_seconds Latency.
# TYPE latency_seconds histogram
latency_seconds_bucket{route="/a",le="0.1"} 1
latency_seconds_bucket{route="/a",le="1"} 2
latency_seconds_bucket{route="/a",le="+Inf"} 3
latency_seconds_sum{route="/a"} 3.55
latency_seconds_count{route="/a"} 3
# HELP special NaN and negative.
# TYPE special gauge
special{k="nan"} NaN
special{k="neg"} -Inf
# HELP up_things Things.
# TYPE up_things gauge
up_things{kind="x"} 2
up_things{kind="y"} +Inf
# HELP waits_total Waits.
# TYPE waits_total counter
waits_total 7
`
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
	if req.With("/a", "2xx").Value() != 3 || g.Value() != 1.5 || h.With("/a").Count() != 3 {
		t.Fatal("values")
	}
}

func TestDefaultBucketsAndConcurrency(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogramVec("h", "h", nil)
	c := r.NewCounter("c_total", "c")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Inc()
				h.With().Observe(0.02)
			}
		}()
	}
	wg.Wait()
	if c.Value() != 5000 || h.With().Count() != 5000 {
		t.Fatal(c.Value(), h.With().Count())
	}
	fams := r.Gather()
	if len(fams) != 2 || fams[1].Name != "h" || len(fams[1].Samples) != len(DefBuckets)+3 {
		t.Fatal(fams)
	}
}

func TestPanics(t *testing.T) {
	mustPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: no panic", name)
			}
		}()
		f()
	}
	r := NewRegistry()
	r.NewCounter("a_total", "a")
	mustPanic("duplicate", func() { r.NewCounter("a_total", "a") })
	mustPanic("bad name", func() { r.NewCounter("1bad", "a") })
	mustPanic("bad label", func() { r.NewCounterVec("b_total", "b", "le") })
	mustPanic("reserved label", func() { r.NewCounterVec("c_total", "b", "__x") })
	v := r.NewCounterVec("d_total", "d", "x")
	mustPanic("label count", func() { v.With() })
	r.GaugeFunc("f", "f", []string{"x"}, func(e Emit) { e(1) })
	mustPanic("func label count", func() { _ = r.WriteText(&strings.Builder{}) })
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestWriteError(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("x_total", "x").Inc()
	if err := r.WriteText(failWriter{}); err == nil {
		t.Fatal("expected error")
	}
}
