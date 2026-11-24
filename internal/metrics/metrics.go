// Package metrics is a small implementation of Prometheus metrics:
// counters, gauges and histograms with labels, values read by a function
// at scrape time, and the text exposition format (version 0.0.4) served
// on /metrics. It has no dependencies outside the standard library.
package metrics

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ContentType is the media type of WriteText.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// Types of a metric family.
const (
	TypeCounter   = "counter"
	TypeGauge     = "gauge"
	TypeHistogram = "histogram"
)

var nameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
var labelRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Registry holds metric families and writes them in the text format.
type Registry struct {
	mu   sync.Mutex
	fams map[string]family
}

// family is one metric (all its label combinations).
type family interface {
	desc() *Desc
	// samples appends the samples of the family to out.
	samples(out []Sample) []Sample
}

// Desc describes a metric family.
type Desc struct {
	Name   string
	Help   string
	Type   string
	Labels []string
}

// Sample is one line of the exposition: the name (with a suffix for
// histograms), the label values and the value.
type Sample struct {
	Name   string
	Labels [][2]string
	Value  float64
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{fams: map[string]family{}} }

func (r *Registry) register(f family) {
	d := f.desc()
	if !nameRE.MatchString(d.Name) {
		panic("metrics: invalid metric name " + strconv.Quote(d.Name))
	}
	for _, l := range d.Labels {
		if !labelRE.MatchString(l) || strings.HasPrefix(l, "__") || l == "le" {
			panic("metrics: invalid label name " + strconv.Quote(l))
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.fams[d.Name]; dup {
		panic("metrics: duplicate metric " + d.Name)
	}
	r.fams[d.Name] = f
}

// ---------------------------------------------------------------- values

// value is a float64 updated atomically.
type value struct{ bits atomic.Uint64 }

func (v *value) add(d float64) {
	for {
		old := v.bits.Load()
		if v.bits.CompareAndSwap(old, math.Float64bits(math.Float64frombits(old)+d)) {
			return
		}
	}
}

func (v *value) set(x float64) { v.bits.Store(math.Float64bits(x)) }
func (v *value) get() float64  { return math.Float64frombits(v.bits.Load()) }

// vec keeps the series of a family by their label values.
type vec[T any] struct {
	d     Desc
	mu    sync.RWMutex
	items map[string]*entry[T]
	make  func() *T
}

type entry[T any] struct {
	labels []string
	v      *T
}

func newVec[T any](d Desc, mk func() *T) *vec[T] {
	return &vec[T]{d: d, items: map[string]*entry[T]{}, make: mk}
}

func (v *vec[T]) desc() *Desc { return &v.d }

func (v *vec[T]) with(lv []string) *T {
	if len(lv) != len(v.d.Labels) {
		panic(fmt.Sprintf("metrics: %s takes %d label values, got %d", v.d.Name, len(v.d.Labels), len(lv)))
	}
	key := strings.Join(lv, "\xff")
	v.mu.RLock()
	e, ok := v.items[key]
	v.mu.RUnlock()
	if ok {
		return e.v
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if e, ok := v.items[key]; ok {
		return e.v
	}
	e = &entry[T]{labels: slices.Clone(lv), v: v.make()}
	v.items[key] = e
	return e.v
}

// sorted returns the entries in the order of their label values.
func (v *vec[T]) sorted() []*entry[T] {
	v.mu.RLock()
	out := make([]*entry[T], 0, len(v.items))
	for _, e := range v.items {
		out = append(out, e)
	}
	v.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return slices.Compare(out[i].labels, out[j].labels) < 0 })
	return out
}

func pairs(names, values []string) [][2]string {
	out := make([][2]string, len(names))
	for i := range names {
		out[i] = [2]string{names[i], values[i]}
	}
	return out
}

// ---------------------------------------------------------------- counter and gauge

// Counter is a value that only goes up.
type Counter struct{ v value }

// Inc adds 1.
func (c *Counter) Inc() { c.v.add(1) }

// Add adds d (negative values are ignored: a counter never goes down).
func (c *Counter) Add(d float64) {
	if d > 0 {
		c.v.add(d)
	}
}

// Value returns the current value.
func (c *Counter) Value() float64 { return c.v.get() }

// CounterVec is a counter with labels.
type CounterVec struct{ *vec[Counter] }

// NewCounterVec registers a counter with the label names labels.
func (r *Registry) NewCounterVec(name, help string, labels ...string) *CounterVec {
	c := &CounterVec{newVec(Desc{Name: name, Help: help, Type: TypeCounter, Labels: labels}, func() *Counter { return &Counter{} })}
	r.register(c)
	return c
}

// With returns the counter of the label values lv.
func (c *CounterVec) With(lv ...string) *Counter { return c.with(lv) }

func (c *CounterVec) samples(out []Sample) []Sample {
	for _, e := range c.sorted() {
		out = append(out, Sample{Name: c.d.Name, Labels: pairs(c.d.Labels, e.labels), Value: e.v.Value()})
	}
	return out
}

// Gauge is a value that goes up and down.
type Gauge struct{ v value }

// Set sets the value.
func (g *Gauge) Set(x float64) { g.v.set(x) }

// Add adds d (which may be negative).
func (g *Gauge) Add(d float64) { g.v.add(d) }

// Inc adds 1, Dec subtracts 1.
func (g *Gauge) Inc() { g.v.add(1) }

// Dec subtracts 1.
func (g *Gauge) Dec() { g.v.add(-1) }

// Value returns the current value.
func (g *Gauge) Value() float64 { return g.v.get() }

// GaugeVec is a gauge with labels.
type GaugeVec struct{ *vec[Gauge] }

// NewGaugeVec registers a gauge with the label names labels.
func (r *Registry) NewGaugeVec(name, help string, labels ...string) *GaugeVec {
	g := &GaugeVec{newVec(Desc{Name: name, Help: help, Type: TypeGauge, Labels: labels}, func() *Gauge { return &Gauge{} })}
	r.register(g)
	return g
}

// With returns the gauge of the label values lv.
func (g *GaugeVec) With(lv ...string) *Gauge { return g.with(lv) }

func (g *GaugeVec) samples(out []Sample) []Sample {
	for _, e := range g.sorted() {
		out = append(out, Sample{Name: g.d.Name, Labels: pairs(g.d.Labels, e.labels), Value: e.v.Value()})
	}
	return out
}

// NewGauge registers a gauge without labels.
func (r *Registry) NewGauge(name, help string) *Gauge { return r.NewGaugeVec(name, help).With() }

// NewCounter registers a counter without labels.
func (r *Registry) NewCounter(name, help string) *Counter { return r.NewCounterVec(name, help).With() }

// ---------------------------------------------------------------- histogram

// DefBuckets are the default upper bounds of a latency histogram, in
// seconds.
var DefBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Histogram counts observations in buckets.
type Histogram struct {
	bounds []float64
	counts []atomic.Uint64 // per bucket (not cumulative), last is +Inf
	sum    value
	count  atomic.Uint64
}

// Observe records x.
func (h *Histogram) Observe(x float64) {
	i := sort.SearchFloat64s(h.bounds, x) // first bound >= x
	h.counts[i].Add(1)
	h.sum.add(x)
	h.count.Add(1)
}

// Count returns the number of observations.
func (h *Histogram) Count() uint64 { return h.count.Load() }

// HistogramVec is a histogram with labels.
type HistogramVec struct{ *vec[Histogram] }

// NewHistogramVec registers a histogram with the upper bounds buckets
// (nil: DefBuckets) and the label names labels.
func (r *Registry) NewHistogramVec(name, help string, buckets []float64, labels ...string) *HistogramVec {
	if buckets == nil {
		buckets = DefBuckets
	}
	b := slices.Clone(buckets)
	sort.Float64s(b)
	h := &HistogramVec{newVec(Desc{Name: name, Help: help, Type: TypeHistogram, Labels: labels}, func() *Histogram {
		return &Histogram{bounds: b, counts: make([]atomic.Uint64, len(b)+1)}
	})}
	r.register(h)
	return h
}

// With returns the histogram of the label values lv.
func (h *HistogramVec) With(lv ...string) *Histogram { return h.with(lv) }

func (h *HistogramVec) samples(out []Sample) []Sample {
	for _, e := range h.sorted() {
		base := pairs(h.d.Labels, e.labels)
		var cum uint64
		for i, b := range e.v.bounds {
			cum += e.v.counts[i].Load()
			out = append(out, Sample{Name: h.d.Name + "_bucket", Labels: append(slices.Clone(base), [2]string{"le", formatFloat(b)}), Value: float64(cum)})
		}
		cum += e.v.counts[len(e.v.bounds)].Load()
		out = append(out, Sample{Name: h.d.Name + "_bucket", Labels: append(slices.Clone(base), [2]string{"le", "+Inf"}), Value: float64(cum)},
			Sample{Name: h.d.Name + "_sum", Labels: base, Value: e.v.sum.get()},
			Sample{Name: h.d.Name + "_count", Labels: base, Value: float64(cum)})
	}
	return out
}

// ---------------------------------------------------------------- functions

// Emit reports one value of a function metric with its label values.
type Emit func(v float64, lv ...string)

type funcFamily struct {
	d  Desc
	fn func(Emit)
}

func (f *funcFamily) desc() *Desc { return &f.d }

func (f *funcFamily) samples(out []Sample) []Sample {
	f.fn(func(v float64, lv ...string) {
		if len(lv) != len(f.d.Labels) {
			panic(fmt.Sprintf("metrics: %s takes %d label values, got %d", f.d.Name, len(f.d.Labels), len(lv)))
		}
		out = append(out, Sample{Name: f.d.Name, Labels: pairs(f.d.Labels, lv), Value: v})
	})
	return out
}

// GaugeFunc registers a gauge whose values fn reports at each scrape.
func (r *Registry) GaugeFunc(name, help string, labels []string, fn func(Emit)) {
	r.register(&funcFamily{d: Desc{Name: name, Help: help, Type: TypeGauge, Labels: labels}, fn: fn})
}

// CounterFunc registers a counter whose values fn reports at each scrape
// (a total kept elsewhere, e.g. by database/sql).
func (r *Registry) CounterFunc(name, help string, labels []string, fn func(Emit)) {
	r.register(&funcFamily{d: Desc{Name: name, Help: help, Type: TypeCounter, Labels: labels}, fn: fn})
}

// ---------------------------------------------------------------- exposition

// Gather returns the descriptions and samples of every family, by name.
func (r *Registry) Gather() []Family {
	r.mu.Lock()
	fams := make([]family, 0, len(r.fams))
	for _, f := range r.fams {
		fams = append(fams, f)
	}
	r.mu.Unlock()
	sort.Slice(fams, func(i, j int) bool { return fams[i].desc().Name < fams[j].desc().Name })
	out := make([]Family, 0, len(fams))
	for _, f := range fams {
		out = append(out, Family{Desc: *f.desc(), Samples: f.samples(nil)})
	}
	return out
}

// Family is a gathered metric family.
type Family struct {
	Desc
	Samples []Sample
}

// WriteText writes every family in the Prometheus text format.
func (r *Registry) WriteText(w io.Writer) error {
	bw := bufio.NewWriter(w)
	for _, f := range r.Gather() {
		fmt.Fprintf(bw, "# HELP %s %s\n# TYPE %s %s\n", f.Name, escapeHelp(f.Help), f.Name, f.Type)
		for _, s := range f.Samples {
			bw.WriteString(s.Name)
			if len(s.Labels) > 0 {
				bw.WriteByte('{')
				for i, l := range s.Labels {
					if i > 0 {
						bw.WriteByte(',')
					}
					bw.WriteString(l[0])
					bw.WriteString(`="`)
					bw.WriteString(escapeLabel(l[1]))
					bw.WriteByte('"')
				}
				bw.WriteByte('}')
			}
			bw.WriteByte(' ')
			bw.WriteString(formatFloat(s.Value))
			bw.WriteByte('\n')
		}
	}
	return bw.Flush()
}

var helpEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`)
var labelEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)

func escapeHelp(s string) string  { return helpEscaper.Replace(s) }
func escapeLabel(s string) string { return labelEscaper.Replace(s) }

func formatFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	case math.IsNaN(f):
		return "NaN"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}
