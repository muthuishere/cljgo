package bri

// Where a bri request actually spends its time (spike s72).
//
// The published Docker table has bri at 78,126 req/s against clj-httpkit's
// 82,837 on the same two routes, and the obvious question is WHICH LAYER the
// gap is in — Go's HTTP server, bri's adapter, or the Clojure handler. A
// container benchmark cannot answer that; it measures all three plus the
// kernel plus Docker.
//
// So this measures ONE layer at a time, through the production adapt() path,
// with the Go server removed (httptest.NewRecorder, no socket):
//
//	BenchmarkRawGoHandler   the floor: a net/http handler that writes the same
//	                        bytes. Everything above this is framework cost.
//	BenchmarkBriAdapt*      the real bri path, at four request shapes.
//
// Allocation is the number to read, not ns/op. At 78k req/s a per-request
// allocation is 78k allocations/second of pressure the GC has to walk, and it
// is what separates "fast on an idle laptop" from "fast under sustained load".

import (
	"io"
	"net/http"
	"net/http/httptest"
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

// The corpus route, exactly as the Docker benchmark drives it: no query, no
// path params, and oha's own small header set.
func BenchmarkBriAdaptBare(b *testing.B) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "oha/1.4.5")
	benchAdapt(b, "/", req)
}

// A realistic browser request — 8 headers rather than 2. This is the scale
// question: does per-request cost track HEADER COUNT, which the handler never
// reads?
func BenchmarkBriAdaptRealHeaders(b *testing.B) {
	req := httptest.NewRequest("GET", "/", nil)
	for k, v := range map[string]string{
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Encoding": "gzip, deflate, br",
		"Accept-Language": "en-GB,en;q=0.9",
		"Cache-Control":   "no-cache",
		"Cookie":          "session=abc123; theme=dark",
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
		"Referer":         "https://example.com/page",
		"Sec-Fetch-Mode":  "navigate",
	} {
		req.Header.Set(k, v)
	}
	benchAdapt(b, "/", req)
}

// With a query string the handler also ignores.
func BenchmarkBriAdaptQuery(b *testing.B) {
	req := httptest.NewRequest("GET", "/?a=1&b=2&c=3", nil)
	req.Header.Set("Accept", "*/*")
	benchAdapt(b, "/", req)
}

// With a typed path param — the /api/n/{id} route.
func BenchmarkBriAdaptPathParam(b *testing.B) {
	req := httptest.NewRequest("GET", "/api/n/42", nil)
	req.Header.Set("Accept", "*/*")
	req.SetPathValue("id", "42")
	benchAdapt(b, "/api/n/{id}", req)
}

// The pieces, so the profile is attributable rather than a single total.
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
