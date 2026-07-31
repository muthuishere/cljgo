// middleware_bench_test.go — spike s76: per-layer cost of bri's
// zero-config api-defaults middleware stack, measured IN-PROCESS via
// bri.web.http/request (no socket) so the numbers are ns/op + B/op +
// allocs/op with no network noise. Drives the SAME interpreter path a
// live server uses: one repl.Driver, one compiled `bench-call` var
// whose body reads the `stack` var at each invocation (the documented
// REPL-liveness model — see core/bri/http.cljg's base-handler comment),
// so redefining `stack` between benchmarks changes what `bench-call`
// runs without re-parsing per iteration. That isolates middleware
// execution cost from reader/analyzer overhead, which would otherwise
// dominate and mask the per-layer deltas we're after.
package bri_test

import (
	"io"
	"os"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

// benchPrelude sets up one route (mirrors /api/hello from the Docker
// leg) and a `bench-call` fn that re-reads the `stack` var per call.
const benchPrelude = `
(require '[bri.web.http :as http] '[cljg.security :as sec])
(defn hello [_] {:status 200 :body "hello"})
(def routes [["GET /api/hello" #'hello]])
(def req {:method "GET" :path "/api/hello"})
(def stack [])
(defn bench-call [] (http/request routes req {:middleware stack}))
`

// newBenchDriver boots a fresh interpreter and points *out* (what
// println / the default-log-sink write through) at out. corelib.Out is
// a process-level package var (pkg/lang's *out* root is always
// os.Stdout — see corelib.outWriter), so it must be restored on
// cleanup or it leaks across benchmarks sharing this test binary.
func newBenchDriver(tb testing.TB, out io.Writer) *repl.Driver {
	tb.Helper()
	if err := os.Setenv("BRI_DEV", ""); err != nil {
		tb.Fatal(err)
	}
	prevOut := corelib.Out
	corelib.Out = out
	tb.Cleanup(func() { corelib.Out = prevOut })
	d := repl.New(nil, io.Discard, io.Discard)
	if _, err := d.EvalString(benchPrelude, "bench_prelude"); err != nil {
		tb.Fatalf("prelude: %v", err)
	}
	return d
}

func setStack(tb testing.TB, d *repl.Driver, expr string) {
	tb.Helper()
	if _, err := d.EvalString("(def stack "+expr+")", "bench_stack"); err != nil {
		tb.Fatalf("set stack %s: %v", expr, err)
	}
}

func benchVar(tb testing.TB, d *repl.Driver) *lang.Var {
	tb.Helper()
	ns := lang.FindNamespace(lang.NewSymbol("user"))
	if ns == nil {
		tb.Fatal("user namespace not found")
	}
	v := ns.FindInternedVar(lang.NewSymbol("bench-call"))
	if v == nil {
		tb.Fatal("bench-call var not found")
	}
	return v
}

// assertOK sanity-checks one call doesn't panic before timing — a
// middleware layer wired up wrong would otherwise "benchmark" a stack
// trace, not the layer.
func assertOK(tb testing.TB, v *lang.Var) {
	tb.Helper()
	v.Invoke()
}

// stacks — cumulative, in api-defaults order (recover first so it
// funnels every subsequent layer's own errors too; negotiate last, so
// it stays innermost/adjacent to the handler exactly as in production
// where it is conj'd on last).
var cumulative = []struct {
	name string
	expr string
}{
	{"00-none", `[]`},
	{"01-recover", `[(http/recover)]`},
	{"02-+request-id", `[(http/recover) http/request-id]`},
	{"03-+logging", `[(http/recover) http/request-id http/logging]`},
	{"04-+cors", `[(http/recover) http/request-id http/logging (http/cors {})]`},
	{"05-+metrics", `[(http/recover) http/request-id http/logging (http/cors {}) http/metrics]`},
	{"06-+auto-ban", `[(http/recover) http/request-id http/logging (http/cors {}) http/metrics (sec/auto-ban)]`},
	{"07-+negotiate", `[(http/recover) http/request-id http/logging (http/cors {}) http/metrics (sec/auto-ban) http/negotiate]`},
}

var alone = []struct {
	name string
	expr string
}{
	{"recover-only", `[(http/recover)]`},
	{"request-id", `[(http/recover) http/request-id]`},
	{"logging", `[(http/recover) http/logging]`},
	{"cors", `[(http/recover) (http/cors {})]`},
	{"metrics", `[(http/recover) http/metrics]`},
	{"auto-ban", `[(http/recover) (sec/auto-ban)]`},
	{"negotiate", `[(http/recover) http/negotiate]`},
}

func BenchmarkCumulative(b *testing.B) {
	d := newBenchDriver(b, io.Discard)
	v := benchVar(b, d)
	for _, tc := range cumulative {
		setStack(b, d, tc.expr)
		assertOK(b, v)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				v.Invoke()
			}
		})
	}
}

func BenchmarkAlone(b *testing.B) {
	d := newBenchDriver(b, io.Discard)
	v := benchVar(b, d)
	for _, tc := range alone {
		setStack(b, d, tc.expr)
		assertOK(b, v)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				v.Invoke()
			}
		})
	}
}

// BenchmarkCumulativeParallel re-runs the cumulative ladder under
// b.RunParallel, on ONE shared Driver/interpreter — exactly what a
// live server does (every core hits the same compiled middleware
// closures + the same Go-side shared state: pkg/bri's atomic metrics
// registry, cljg.security's ban-store atom). Contention is invisible
// single-threaded; this is the only way to see it.
func BenchmarkCumulativeParallel(b *testing.B) {
	d := newBenchDriver(b, io.Discard)
	v := benchVar(b, d)
	for _, tc := range cumulative {
		setStack(b, d, tc.expr)
		assertOK(b, v)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					v.Invoke()
				}
			})
		})
	}
}

// BenchmarkAloneParallel — same, for each layer alone (recover +
// layer), to catch a layer that is cheap single-threaded but
// contends under concurrency (the auto-ban / metrics suspicion).
func BenchmarkAloneParallel(b *testing.B) {
	d := newBenchDriver(b, io.Discard)
	v := benchVar(b, d)
	for _, tc := range alone {
		setStack(b, d, tc.expr)
		assertOK(b, v)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					v.Invoke()
				}
			})
		})
	}
}

// BenchmarkLoggingRealSink — logging's default-log-sink writes to
// *out* via println; io.Discard hides the real syscall cost of that
// write. Point stdout at a real file (like a prod container's stdout)
// to measure it, isolating logging alone (recover+logging) so the
// delta against BenchmarkAlone/logging (Discard) is the I/O cost.
func BenchmarkLoggingRealSink(b *testing.B) {
	f, err := os.CreateTemp(b.TempDir(), "bench-log-*.ndjson")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	d := newBenchDriver(b, f)
	v := benchVar(b, d)
	setStack(b, d, `[(http/recover) http/logging]`)
	assertOK(b, v)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v.Invoke()
	}
}
