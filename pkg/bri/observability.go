// observability.go — the Go half of bri's default-on observability
// stack (ADR 0069 §"Observability & audit"): a lock-light metrics
// registry rendered as Prometheus text, plus the audit/log sink
// primitive (structured stderr). Kept off the hot path — an observe is
// one map lookup + a handful of atomic adds (S44 proved the guard chain
// is ~10 ns; metrics stays in that class, see metricsObserve).
package bri

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// latencyBuckets are the Prometheus histogram upper bounds in
// milliseconds — a web-request-shaped spread (sub-ms to 10 s).
var latencyBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// routeMetric is one route's counters. Counts are atomic so observe
// never takes the registry lock on the hot path once the entry exists.
type routeMetric struct {
	count      atomic.Uint64
	sumMillis  atomic.Uint64 // fixed-point: millis*1000, so sub-ms sums keep precision
	statusClss [6]atomic.Uint64
	buckets    []atomic.Uint64 // len(latencyBuckets); cumulative-le rendered at scrape
}

type metricsRegistry struct {
	mu     sync.RWMutex
	routes map[string]*routeMetric
}

var metrics = &metricsRegistry{routes: map[string]*routeMetric{}}

func (reg *metricsRegistry) get(route string) *routeMetric {
	reg.mu.RLock()
	m := reg.routes[route]
	reg.mu.RUnlock()
	if m != nil {
		return m
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if m = reg.routes[route]; m == nil {
		m = &routeMetric{buckets: make([]atomic.Uint64, len(latencyBuckets))}
		reg.routes[route] = m
	}
	return m
}

// metricsObserve records one request. Hot path: one RLock'd map lookup
// (amortized, entry created once) + atomic adds — no allocation.
func metricsObserve(route string, status int, millis float64) {
	m := metrics.get(route)
	m.count.Add(1)
	m.sumMillis.Add(uint64(millis * 1000))
	cls := status / 100
	if cls < 1 || cls > 5 {
		cls = 0
	}
	m.statusClss[cls].Add(1)
	for i, b := range latencyBuckets {
		if millis <= b {
			m.buckets[i].Add(1)
		}
	}
}

func metricsReset() {
	metrics.mu.Lock()
	metrics.routes = map[string]*routeMetric{}
	metrics.mu.Unlock()
}

// metricsRender emits Prometheus text format v0.0.4.
func metricsRender() string {
	metrics.mu.RLock()
	names := make([]string, 0, len(metrics.routes))
	for k := range metrics.routes {
		names = append(names, k)
	}
	snap := metrics.routes
	metrics.mu.RUnlock()
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# HELP bri_http_requests_total Total HTTP requests by route and status class.\n")
	b.WriteString("# TYPE bri_http_requests_total counter\n")
	for _, name := range names {
		m := snap[name]
		for cls := 1; cls <= 5; cls++ {
			if v := m.statusClss[cls].Load(); v > 0 {
				fmt.Fprintf(&b, "bri_http_requests_total{route=%q,status=\"%dxx\"} %d\n", name, cls, v)
			}
		}
	}
	b.WriteString("# HELP bri_http_request_duration_ms Request duration histogram in milliseconds.\n")
	b.WriteString("# TYPE bri_http_request_duration_ms histogram\n")
	for _, name := range names {
		m := snap[name]
		for i, ub := range latencyBuckets {
			fmt.Fprintf(&b, "bri_http_request_duration_ms_bucket{route=%q,le=%q} %d\n",
				name, strconv.FormatFloat(ub, 'g', -1, 64), m.buckets[i].Load())
		}
		total := m.count.Load()
		fmt.Fprintf(&b, "bri_http_request_duration_ms_bucket{route=%q,le=\"+Inf\"} %d\n", name, total)
		fmt.Fprintf(&b, "bri_http_request_duration_ms_sum{route=%q} %g\n", name, float64(m.sumMillis.Load())/1000)
		fmt.Fprintf(&b, "bri_http_request_duration_ms_count{route=%q} %d\n", name, total)
	}
	return b.String()
}

// installAuditShims interns bri.audit's private Go primitives — the
// structured-line sink (stderr, v1) plus JSON/clock/env helpers.
func installAuditShims(def func(name string, fn func(args ...any) any)) {
	def("-eprintln", func(args ...any) any {
		fmt.Fprintln(os.Stderr, asString(one("-eprintln", args)))
		return nil
	})
	def("-json-encode", func(args ...any) any { return jsonEncode(one("-json-encode", args)) })
	def("-now-millis", func(args ...any) any { return nowMillis() })
	def("-getenv", getenvShim)
}

func asInt(v any) int {
	switch t := v.(type) {
	case int64:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	}
	panic(fmt.Errorf("expected an int, got: %s", lang.PrintString(v)))
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	panic(fmt.Errorf("expected a number, got: %s", lang.PrintString(v)))
}
