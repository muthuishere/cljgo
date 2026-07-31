package bri

// spike s77 — the RESPONSE side of bri's HTTP path.
//
// s72 (http_perf_test.go) measured the REQUEST side: adapt() building the
// request map, at ~1700-1900 ns / ~84 allocs for a bare GET. The response
// side — writeResponse, jsonEncode, toJSONValue — was never isolated at
// realistic payload sizes. This file does that, through the production
// path (writeResponse / jsonEncode / toJSONValue themselves, not a toy).
//
// EXCLUDES: network (httptest.Recorder, no socket, same as s72) and gzip/
// compression (not part of writeResponse). Numbers are a floor, not a
// deployed cost.

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// --- body shape builders -----------------------------------------------------

func benchMapOfN(n int) any {
	kvs := make([]any, 0, n*2)
	for i := 0; i < n; i++ {
		kvs = append(kvs, "field"+strconv.Itoa(i), "value"+strconv.Itoa(i))
	}
	return lang.NewMap(kvs...)
}

func benchVectorOfMaps(n int) any {
	vals := make([]any, n)
	for i := 0; i < n; i++ {
		vals[i] = lang.NewMap(
			"id", int64(i),
			"name", "item"+strconv.Itoa(i),
			"active", true,
			"score", float64(i)*1.5,
			"tag", "row",
		)
	}
	return lang.NewVectorOwning(vals)
}

func benchString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + i%26)
	}
	return string(b)
}

// --- 1. writeResponse at realistic body shapes -------------------------------

func BenchmarkWriteResponse_SmallString(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwBody, "hello\n"))
}

// IMPORTANT: core/bri/http.cljg's `negotiate` middleware (the :json entry in
// the default stack) and the `json` response helper both call -json-encode
// BEFORE the response map reaches writeResponse — a map/vector :body is
// turned into a Go string upstream of writeResponse in every production
// path (see negotiate's `(assoc :body (-json-encode (:body res)))`).
// writeResponse's own `default:` branch (lang.ToString on a raw map/vector)
// is reachable only if a handler bypasses both `json` and negotiate, which
// is not the shipped path. So writeResponse is benchmarked here on the
// STRING it actually receives — pre-encoded once, outside the timed loop —
// and jsonEncode/toJSONValue are isolated separately below as the upstream
// cost that precedes it.

func BenchmarkWriteResponse_JSONMap5(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwBody, jsonEncode(benchMapOfN(5))))
}

func BenchmarkWriteResponse_JSONMap30(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwBody, jsonEncode(benchMapOfN(30))))
}

func BenchmarkWriteResponse_JSONVec10(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwBody, jsonEncode(benchVectorOfMaps(10))))
}

func BenchmarkWriteResponse_JSONVec1000(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwBody, jsonEncode(benchVectorOfMaps(1000))))
}

func BenchmarkWriteResponse_LargeString256KiB(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwBody, benchString(256*1024)))
}

func runWriteResponseBench(b *testing.B, res any) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		writeResponse(w, res)
	}
}

// --- 2. jsonEncode / toJSONValue in isolation, at collection scale -----------

func BenchmarkJSONEncode_Map5(b *testing.B) {
	runJSONEncodeBench(b, benchMapOfN(5))
}

func BenchmarkJSONEncode_Map30(b *testing.B) {
	runJSONEncodeBench(b, benchMapOfN(30))
}

func BenchmarkJSONEncode_Vec10(b *testing.B) {
	runJSONEncodeBench(b, benchVectorOfMaps(10))
}

func BenchmarkJSONEncode_Vec100(b *testing.B) {
	runJSONEncodeBench(b, benchVectorOfMaps(100))
}

func BenchmarkJSONEncode_Vec1000(b *testing.B) {
	runJSONEncodeBench(b, benchVectorOfMaps(1000))
}

func BenchmarkJSONEncode_Vec10000(b *testing.B) {
	runJSONEncodeBench(b, benchVectorOfMaps(10000))
}

func runJSONEncodeBench(b *testing.B, v any) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = jsonEncode(v)
	}
}

// toJSONValue alone (the conversion, no json.Marshal) — isolates whether the
// conversion itself is the parallel-copy cost or json.Marshal is.
func BenchmarkToJSONValue_Vec10(b *testing.B) {
	runToJSONValueBench(b, benchVectorOfMaps(10))
}

func BenchmarkToJSONValue_Vec100(b *testing.B) {
	runToJSONValueBench(b, benchVectorOfMaps(100))
}

func BenchmarkToJSONValue_Vec1000(b *testing.B) {
	runToJSONValueBench(b, benchVectorOfMaps(1000))
}

func BenchmarkToJSONValue_Vec10000(b *testing.B) {
	runToJSONValueBench(b, benchVectorOfMaps(10000))
}

func runToJSONValueBench(b *testing.B, v any) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = toJSONValue(v)
	}
}

// --- 3. header-write loop at 1 / 5 / 15 headers ------------------------------

func benchHeaderMap(n int) any {
	kvs := make([]any, 0, n*2)
	names := []string{
		"Content-Type", "Cache-Control", "X-Request-Id", "X-Trace-Id", "Vary",
		"ETag", "Access-Control-Allow-Origin", "X-Frame-Options", "X-Content-Type-Options",
		"Strict-Transport-Security", "X-RateLimit-Limit", "X-RateLimit-Remaining",
		"X-RateLimit-Reset", "Content-Language", "Set-Cookie",
	}
	for i := 0; i < n; i++ {
		kvs = append(kvs, names[i%len(names)], "value-"+strconv.Itoa(i))
	}
	return lang.NewMap(kvs...)
}

func BenchmarkWriteResponse_Headers1(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwHeaders, benchHeaderMap(1), kwBody, "hi\n"))
}

func BenchmarkWriteResponse_Headers5(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwHeaders, benchHeaderMap(5), kwBody, "hi\n"))
}

func BenchmarkWriteResponse_Headers15(b *testing.B) {
	runWriteResponseBench(b, lang.NewMap(kwStatus, int64(200), kwHeaders, benchHeaderMap(15), kwBody, "hi\n"))
}
