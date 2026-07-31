# s77 — the response path: writeResponse / jsonEncode / toJSONValue at scale

darwin/arm64, Apple M5 Pro, `go test -bench ... -benchmem ./pkg/bri/`. Code:
`pkg/bri/http_response_bench_test.go` (new). Fix: `pkg/bri/http.go` `toJSONValue`.

## The question

s72 measured the REQUEST side (~1700-1900 ns / ~84 allocs for a bare GET
through `adapt`). The RESPONSE side — `writeResponse`, `jsonEncode`,
`toJSONValue` — was unmeasured. Does it scale, and is it a problem?

## What EXCLUDES what

- No network (httptest.Recorder, no socket — same floor discipline as s72).
- No gzip/compression (not part of writeResponse).
- **`writeResponse` never sees a Clojure map/vector body in the shipped
  path.** `core/bri/http.cljg`'s `negotiate` middleware and the `json`
  helper both call `-json-encode` (→ `jsonEncode`) BEFORE the response map
  reaches `writeResponse`:
  `(assoc :body (-json-encode (:body res)))`. So the "JSON route" numbers
  below are two separate, correctly-ordered measurements: `jsonEncode`
  converts the Clojure value to a JSON *string* first; `writeResponse` then
  writes that string, exactly like any other string body. The
  `WriteResponse_JSON*` benchmarks pre-encode outside the timed loop to
  match this.

## 1. writeResponse, at realistic body sizes (string bodies, as shipped)

| body | ns/op | B/op | allocs/op |
|---|---|---|---|
| small string ("hello\n") | 251.2 | 320 | 8 |
| encoded 5-key map (~46 B) | 251.1 | 352 | 8 |
| encoded 30-key map (~460 B) | 357.0 | 896 | 8 |
| encoded 10-map vector (~460 B) | 379.6 | 896 | 8 |
| encoded 1000-map vector (~73 KB) | 4,933 | 73,984 | 8 |
| 256 KiB string | 12,452 | 262,400 | 8 |

**Scaling: linear in bytes, flat in allocs.** allocs/op is 8 at every size
from a 6-byte body to a 256 KiB body — the loop over headers/status/body-type
switch is fixed cost; the only thing that grows is the buffer copy, 1:1 with
payload size (320 B base + ~1 B/byte of body). No superlinear behavior at
any scale tested. **writeResponse is not a scaling risk.**

## 2. jsonEncode / toJSONValue in isolation — BEFORE the fix

| input | jsonEncode ns/op | B/op | allocs/op | toJSONValue ns/op | B/op | allocs/op |
|---|---|---|---|---|---|---|
| map, 5 keys | 2,481 | 4,660 | 45 | — | — | — |
| map, 30 keys | 19,737 | 33,485 | 282 | — | — | — |
| vector, 10 maps (5 keys ea.) | 26,276 | 46,995 | 448 | 19,214 | 41,960 | 336 |
| vector, 100 maps | 270,405 | 471,255 | 4,413 | 198,754 | 418,890 | 3,309 |
| vector, 1,000 maps | 2,520,317 | 4,924,394 | 44,037 | 1,863,481 | 4,179,252 | 33,012 |
| vector, 10,000 maps | 23,048,321 | 49,297,974 | 440,057 | 15,600,692 | 42,106,269 | 330,021 |

**Scaling: linear across 3 orders of magnitude** (10→10,000 elements: ns,
bytes, and allocs each grow ~10x per 10x input, at every step — no
superlinear blowup). So the earlier concern ("does JSON encoding scale?")
is answered **yes, linearly** — but the constant factor is large:
`toJSONValue` alone accounts for ~85% of jsonEncode's bytes and ~75% of its
allocs at 10,000 elements, confirming the task's suspicion: **it does build
a full parallel copy** (a Go `map[string]any` + `[]any` mirroring every
Clojure map/vector) before `json.Marshal` ever runs.

## 3. Profile attribution (worst row: jsonEncode on a 10,000-map vector)

`go tool pprof -top -alloc_space`, before the fix:

```
1629.30MB 51.55% flat  github.com/muthuishere/cljgo/pkg/lang/internal/persistent/vector.NewTransient
 257.74MB  8.15% flat  github.com/muthuishere/cljgo/pkg/bri.toJSONValue
 185.51MB  5.87% flat  github.com/muthuishere/cljgo/pkg/lang.newAPVSeq
 161.51MB  5.11% flat  github.com/muthuishere/cljgo/pkg/lang.NewVector          (cum 61.03%)
 149.51MB  4.73% flat  github.com/muthuishere/cljgo/pkg/lang.(*MapSeq).First
 138.01MB  4.37% flat  github.com/muthuishere/cljgo/pkg/lang/.../vector.(*Transient).Persistent
 126.02MB  3.99% flat  encoding/json.mapEncoder.encode                          (cum 11.50%)
```

**Attribution:** over half the allocation (51.55% flat, 61% cumulative
through `NewVector`) is `toJSONValue`'s map-entry loop calling
`k := lang.First(entry)` to read a `MapEntry`'s key. `lang.First(x)` always
goes through `Seq(x)`; `MapEntry.Seq()` (`amapentrySeq`) is generic and
implements itself as `NewVector(a.Key(), a.Val()).Seq()` — **building a
throwaway 2-element persistent vector, tree node and all, just to read
index 0**, on every single map key in every element. `json.Marshal` itself
(`mapEncoder.encode` + friends) is a much smaller slice of the pie.

## 4. The fix (free — no new mechanism)

`toJSONValue` already reads the value with `lang.Get(entry, int64(1))`
(the cheap `ValAtDefault` → `Nth` path, no vector built). The key read used
the expensive path for no reason — `lang.First(entry)` instead of
`lang.Get(entry, int64(0))`. One-line change, same semantics
(`MapEntry.Nth(0) == Key()`), no new abstraction, no cache, no strategy
object — just stopped doing unnecessary work on a path correctness never
asked for.

```go
// before
k := lang.First(entry)
// after
k := lang.Get(entry, int64(0))
```

### Before / after, same scales

| input | jsonEncode ns/op (before → after) | B/op | allocs/op |
|---|---|---|---|
| vector, 10 | 26,276 → 15,613 (1.68x) | 46,995 → 14,140 (3.32x) | 448 → 248 (1.81x) |
| vector, 100 | 270,405 → 155,331 (1.74x) | 471,255 → 141,424 (3.33x) | 4,413 → 2,411 (1.83x) |
| vector, 1,000 | 2,520,317 → 1,464,331 (1.72x) | 4,924,394 → 1,478,744 (3.33x) | 44,037 → 24,020 (1.83x) |
| vector, 10,000 | 23,048,321 → 12,915,677 (1.78x) | 49,297,974 → 16,404,711 (3.01x) | 440,057 → 240,053 (1.83x) |

| input | toJSONValue ns/op (before → after) | B/op | allocs/op |
|---|---|---|---|
| vector, 10 | 19,214 → 7,322 (2.62x) | 41,960 → 9,160 (4.58x) | 336 → 136 (2.47x) |
| vector, 100 | 198,754 → 71,595 (2.78x) | 418,890 → 90,889 (4.61x) | 3,309 → 1,309 (2.53x) |
| vector, 1,000 | 1,863,481 → 704,656 (2.65x) | 4,179,252 → 899,217 (4.65x) | 33,012 → 13,012 (2.54x) |
| vector, 10,000 | 15,600,692 → 6,942,131 (2.25x) | 42,106,269 → 9,306,077 (4.52x) | 330,021 → 130,019 (2.54x) |

The ratio is flat across 10→10,000 (~1.7-1.8x time / ~3.3x bytes / ~1.83x
allocs for jsonEncode; ~2.3-2.8x time / ~4.5x bytes / ~2.5x allocs for
`toJSONValue` alone) — this is a genuine per-element constant-factor win,
not a fixed setup cost being amortized differently at scale. `TestJSONNegotiation`
(the one correctness test exercising this exact path) still passes.

The remaining allocation (map[string]any construction, the []any output
slices, json.Marshal's own reflection-based encode) is the real, structural
cost of "convert an immutable Clojure collection into stdlib `encoding/json`'s
input shape" — that IS the full-copy design `encoding/json` requires, and
removing it would mean a hand-rolled encoder (a second code path) or a
custom `json.Marshaler` per shape, both bigger mechanisms this spike's
doctrine says not to reach for on a measured-but-not-huge remaining number.

## 5. Header-write loop (1 / 5 / 15 headers)

| headers | ns/op | B/op | allocs/op | Δns vs. 1-header baseline | per-header cost |
|---|---|---|---|---|---|
| 1 | 1,029 | 1,808 | 18 | — | ~778 ns/hdr (net of ~251 ns fixed cost) |
| 5 | 2,577 | 4,944 | 46 | 2.5x | ~465 ns/hdr |
| 15 | 9,085 | 16,936 | 130 | 8.83x | ~589 ns/hdr |

**Scaling: linear in header count**, ~500-780 ns and ~8-10 allocs per
header regardless of position — no evidence of quadratic behavior in the
`lang.Seq`/`lang.Get(entry, 1)` walk at realistic header counts (real
responses rarely exceed ~15).

## VERDICT

**The response path was a real, measurable problem, and it had a free fix.**

- `writeResponse` itself: **not a problem at any scale tested** — linear in
  body bytes, flat 8 allocs regardless of size (6 B to 256 KiB).
- The header-write loop: **not a problem** — linear, ~500-780 ns/header.
- `jsonEncode`/`toJSONValue`: **scales linearly, no blowup** — but the
  constant factor was inflated by an accidental allocation
  (`lang.First(entry)` building a throwaway vector per map key via
  `MapEntry.Seq()`'s generic implementation). Fixed by using the same cheap
  `lang.Get(entry, 0)` path the value read already used. Measured
  **1.7-1.8x faster, ~3.3x less garbage** on `jsonEncode`, consistent from
  10 to 10,000 elements. No new mechanism, no strategy object, no cache —
  qualifies as free under the "would you keep this if it were the same
  speed" test (yes: it is strictly less code doing strictly less work).
- What's left in `toJSONValue`/`jsonEncode` after the fix is the structural
  cost of building `encoding/json`'s expected `map[string]any`/`[]any`
  shape from an immutable Clojure collection — real, linear, and not
  cheaply removable without a second code path (a hand-rolled encoder).
  Not recommended by this spike's numbers: the remaining cost is a
  constant-factor conversion tax, not a scaling problem, and doctrine says
  a second code path isn't earned by "it would be somewhat faster."

**Smallest fix, already applied:** `pkg/bri/http.go`, `toJSONValue`'s map
branch, `k := lang.First(entry)` → `k := lang.Get(entry, int64(0))`.
