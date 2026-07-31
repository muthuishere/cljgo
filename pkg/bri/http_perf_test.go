package bri

// Spike s72 — where a bri request spends its time, AT SCALE.
//
// The container benchmark (benchmark/web/) answers "how fast is the whole
// thing"; it cannot answer "which layer, and how does that layer grow". This
// file is the second question, measured through the production adapt() path
// with the Go server removed (httptest.NewRecorder, no socket), so the only
// thing varying is the framework.
//
// Read ALLOCATION first, not ns/op. At 50k req/s an allocation per request is
// 50k allocations/second the GC must walk, and it is what separates "fast on
// an idle laptop" from "fast under sustained load".
//
// WHAT THESE NUMBERS EXCLUDE, so nobody quotes them as throughput: no socket,
// no HTTP parsing, no TLS, no middleware stack, no Docker, no kernel. They are
// a FLOOR on framework cost and a shape, never a req/s figure.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// briEcho is the /api/hello handler from the benchmark corpus, as a plain
// Clojure fn: it ignores the request entirely and returns a literal map. So
// EVERY allocation the benchmarks below report is the framework's, not the
// application's.
var briEcho = lang.FnFunc(func(args ...any) any {
	return lang.NewMap(kwStatus, int64(200), kwBody, "hello\n")
})

// BenchmarkRawGoHandler is the floor — what the compare/go entrant does.
// Everything above this line is framework cost. Its ~9 allocs are
// httptest.NewRecorder's, present in every row below too.
func BenchmarkRawGoHandler(b *testing.B) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "hello\n")
	})
	req := httptest.NewRequest("GET", "/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

func benchAdapt(b *testing.B, pattern string, req *http.Request) {
	h := adapt(pattern, briEcho)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h(w, req)
	}
}

// --- scale 1: header count ---------------------------------------------------
//
// The handler reads NO headers. This asks whether a request the app ignores
// still costs in proportion to what the client sent — i.e. whether a browser
// (many headers) is systematically more expensive than a load generator (few).

func BenchmarkAdaptHeaders(b *testing.B) {
	for _, n := range []int{0, 2, 8, 20, 50} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			req := httptest.NewRequest("GET", "/", nil)
			for i := 0; i < n; i++ {
				req.Header.Set(fmt.Sprintf("X-Header-%d", i), "some header value here")
			}
			benchAdapt(b, "/", req)
		})
	}
}

// --- scale 2: query parameters ------------------------------------------------

func BenchmarkAdaptQueryParams(b *testing.B) {
	for _, n := range []int{0, 1, 5, 20} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			var q []string
			for i := 0; i < n; i++ {
				q = append(q, fmt.Sprintf("k%d=v%d", i, i))
			}
			u := "/"
			if len(q) > 0 {
				u += "?" + strings.Join(q, "&")
			}
			req := httptest.NewRequest("GET", u, nil)
			req.Header.Set("Accept", "*/*")
			benchAdapt(b, "/", req)
		})
	}
}

// --- scale 3: request body ----------------------------------------------------
//
// The body is read eagerly into a string. This prices that decision: an API
// taking a 64 KiB JSON payload pays for the copy whether or not it parses it.

func BenchmarkAdaptBody(b *testing.B) {
	for _, size := range []int{0, 1 << 10, 16 << 10, 256 << 10} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			payload := strings.Repeat("x", size)
			h := adapt("/", briEcho)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/", strings.NewReader(payload))
				w := httptest.NewRecorder()
				h(w, req)
			}
		})
	}
}

// --- scale 4: route count -----------------------------------------------------
//
// Does dispatch cost track the SIZE OF THE ROUTING TABLE? A framework that is
// fast with 2 routes and slow with 200 is not usable for a real API. This goes
// through http.ServeMux, the same mux buildMux produces.

func BenchmarkMuxRouteCount(b *testing.B) {
	for _, n := range []int{1, 10, 100, 500} {
		b.Run(fmt.Sprintf("routes=%d", n), func(b *testing.B) {
			mux := http.NewServeMux()
			for i := 0; i < n; i++ {
				mux.HandleFunc(fmt.Sprintf("GET /api/r%d/{id}", i), adapt(fmt.Sprintf("/api/r%d/{id}", i), briEcho))
			}
			// Hit the LAST route registered, so no ordering luck helps.
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/r%d/42", n-1), nil)
			req.Header.Set("Accept", "*/*")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)
			}
		})
	}
}

// --- the pieces, so the profile is attributable ------------------------------

func BenchmarkRequestMapOnly(b *testing.B) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "oha/1.4.5")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = requestMap(req, nil, "/")
	}
}

func BenchmarkWriteResponseOnly(b *testing.B) {
	res := lang.NewMap(kwStatus, int64(200), kwBody, "hello\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		writeResponse(w, res)
	}
}

// BenchmarkAdaptParallel is the contention question the single-goroutine rows
// cannot answer: bri serves on every core at once, so any shared mutable state
// on the request path (a global registry lock, a counter, an abuse-guard map)
// shows up HERE and nowhere else.
func BenchmarkAdaptParallel(b *testing.B) {
	h := adapt("/", briEcho)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "oha/1.4.5")
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h(w, req)
		}
	})
}
