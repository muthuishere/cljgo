# s76 — which layer of bri's zero-config default stack costs the 42%?

**Question:** Docker/oha measured `bri.web.http/listen`'s `api-defaults` stack
(request-id, logging, recover, cors, metrics, auto-ban, negotiate) at
32,927 req/s vs 57,085 req/s for `recover` only — a ~42% throughput cost and
2.5x p99. Which layer(s) cost what, and what should the zero-config default
be?

**Method:** in-process, via `bri.web.http/request` (no socket), driven
directly from Go (`pkg/bri/middleware_bench_test.go`). One `repl.Driver`
compiles a `bench-call` fn once; it reads the `stack` var per invocation
(the documented liveness model in `core/bri/http.cljg`'s `base-handler`
comment), so redefining `stack` between table rows changes what runs without
re-parsing per iteration — the per-op ns/B/allocs are the middleware
execution cost, not reader/analyzer overhead. Route: `GET /api/hello` →
`{:status 200 :body "hello"}`, matching the Docker leg. darwin/arm64, Apple
M5 Pro, Go bench (`-count 3 -benchtime 500ms`, medians reported below).

## 1. Cumulative (adding one layer at a time, api-defaults order)

| stack (+layer)   | ns/op  | Δns    | B/op    | ΔB     | allocs | Δallocs |
|-------------------|-------:|-------:|--------:|-------:|-------:|--------:|
| `[]`              | 32,200 |    —   |  53,304 |    —   |    621 |     —   |
| `+recover`        | 36,800 | +4,600 |  57,642 | +4,338 |    681 |    +60  |
| `+request-id`     | 41,400 | +4,600 |  68,431 |+10,789 |    817 |   +136  |
| `+logging`        | 53,800 |+12,400 |  84,703 |+16,272 |  1,029 |   +212  |
| `+cors`           | 64,200 |+10,400 |  99,369 |+14,666 |  1,208 |   +179  |
| `+metrics`        | 64,100 |   -100 | 105,678 | +6,309 |  1,293 |    +85  |
| `+auto-ban`       | 70,700 | +6,600 | 116,664 |+10,986 |  1,439 |   +146  |
| `+negotiate`      | 86,800 |+16,100 | 131,651 |+14,987 |  1,628 |   +189  |

(negotiate was the noisiest row across repeats, 82.7–94.1k ns; 86.8k is the
3-run median.)

## 2. Each layer alone (`recover` + that one layer)

| layer         | ns/op  | Δns vs recover-only | B/op   | ΔB     | allocs | Δallocs |
|---------------|-------:|---------------------:|-------:|-------:|-------:|--------:|
| recover-only  | 34,050 |          —            | 57,642 |    —   |    681 |    —    |
| request-id    | 40,700 |        +6,650         | 68,431 |+10,788 |    817 |   +136  |
| logging       | 42,730 |        +8,680         | 73,631 |+15,987 |    890 |   +209  |
| cors          | 40,630 |        +6,580         | 72,547 |+14,903 |    864 |   +183  |
| metrics       | 40,050 |        +6,000 (noisy) | 63,950 | +6,307 |    766 |    +85  |
| auto-ban      | 42,130 |        +8,080          | 68,616 |+10,973 |    827 |   +146  |
| negotiate     | 48,770 to 59,660 (noisy, 2s-sample vs 500ms-sample) | +14,700 to +25,600 | 72,847 |+15,204 |    871 |   +190  |

Alone-order confirms cumulative's ranking: **negotiate and logging are the
two most expensive layers**, cors close behind; metrics is the cheapest
add by a wide margin.

## 3. Parallel (18-way `b.RunParallel`, one shared interpreter/Driver — same
process-global state a live server hits from every core: pkg/bri's atomic
metrics registry, cljg.security's `ban-store` atom)

| layer (alone, Δ vs recover-only) | serial Δns | parallel Δns | contention? |
|-----------------------------------|-----------:|-------------:|:-----------:|
| request-id                        |      6,650 |        5,900 | no          |
| logging                           |      8,680 |        4,570 | no          |
| cors                               |      6,580 |        5,360 | no          |
| metrics                            |      6,000 |        1,680 | no          |
| auto-ban                           |      8,080 |        6,310 | no          |
| negotiate                          | 14,700–25,600 |     6,460 | no          |

**No layer's parallel delta exceeds its serial delta** — metrics' atomic
counters + RWMutex-guarded map and auto-ban's shared `ban-store` atom show
**no contention blowup** at 18-way in-process concurrency; if anything
GC amortizes better under parallel load. This directly answers the "shared
mutable state" question: read-path cost is flat, not superlinear.
**Caveat:** the fixture always returns 200, so auto-ban's *write* path
(`reset!` on a 401/403, which is where a real CAS/lock race would show) was
never exercised — only its read/no-op path is measured here.

## 4. Profile: the two most expensive layers (negotiate, logging)

CPU + `-memprofile`/`go tool pprof -top` on both show the **same** hot path,
and it is not layer-specific business logic:

- `eval.(*Scope).Define`/`Push` — **40–50% of allocated bytes** in both
  profiles. Every middleware `(fn [handler] (fn [req] ...))` wrap is a
  Clojure-level closure call through the tree-walk interpreter; each
  invocation allocates a fresh lexical `Scope`. This is intrinsic
  interpreter-dispatch cost (cf. the standing "fn tax 1.17–1.28×
  interpreted" figure), not something wrong with logging or negotiate's own
  code.
- CPU samples are ~50–60% GC/runtime frames (`mallocgc`,
  `scanObjectsSmall`, `pthread_cond_wait`) — both layers are
  **allocation-bound**, matching their large ΔB (+15–16 KB/op), not
  compute- or lock-bound.
- **Does logging do per-request I/O?** Yes, but the default benchmark (via
  `io.Discard`) hides it. `BenchmarkLoggingRealSink` points `*out*` at a
  real temp file: **49,680 ns/op, 76,171 B/op, 931 allocs/op** vs
  42,730 ns/op / 73,631 B/op / 890 allocs/op on Discard — a further
  **~7µs, +41 allocs** for the actual `write(2)` syscall + JSON encode of
  the log line, on top of the interpreter cost above. In prod (stdout →
  container log driver / shipped over the network) this line item only
  gets worse; that cost is *not* in the docker numbers.
- **Does metrics/auto-ban touch shared mutable state?** Yes (metrics:
  atomic counters behind an amortized `RWMutex` map lookup, already
  documented as lock-light; auto-ban: one global `ban-store` atom read
  per request). Per §3, neither shows contention under parallel load.

## 5. A measurement artifact worth flagging

`bri.web.http/request`'s Go shim (`requestShim`, `pkg/bri/http.go:445`)
calls `buildMux(mounted)` — building a **fresh `http.ServeMux` and
re-parsing every route pattern on every call**, confirmed in the
alloc-profile (`net/http.(*ServeMux).registerErr` + `parsePattern` +
`routingNode.addChild` ≈ 0.6 GB / 4–5% of total allocation in both
profiles). A live `bri.web.http/listen` server builds its mux **once at
boot**; this per-call rebuild is a property of the in-process **test
client** convenience API, not of a deployed server's per-request path. It
is identical across every stack variant here (present even in `[]`), so it
does **not** skew the per-layer deltas this spike answers — but it does
mean these absolute ns/op numbers are not 1:1 comparable to the Docker
per-request numbers; they include ~1–2µs of route-table rebuild a live
server pays only once, ever.

## What this measurement excludes

- Network, TLS, socket syscalls, OS scheduler — same floor caveat as the
  Docker numbers (this is in-process, so it's a floor under an already-a-floor).
- The per-call ServeMux rebuild above (present, but common to every row).
- auto-ban's write path (ban-trigger CAS/lock behavior on repeated
  401/403) — not exercised by a fixture that always 200s.
- Cross-platform (linux/amd64 container) numbers — this ran darwin/arm64,
  same machine as the interpreter fn-tax figures already on file.

## VERDICT

**Negotiate and logging are the two most expensive layers** (Δ12–16µs
cumulative each), cors close behind (Δ6.6–10.4µs); metrics is effectively
free (Δ0–3µs) and, along with auto-ban, shows **no contention** even at
18-way parallelism. The dominant cost driver for every layer, cheap or
expensive, is the **same** thing: interpreter closure-call overhead
(`eval.Scope.Define/Push`), not each layer's own logic — so "which layer"
is somewhat the wrong frame; "the interpreter is the layer" is closer to
right, and that's the standing bri-AOT campaign's job (ADR 0074), not
this spike's.

Applying the owner's test ("would you keep this if it were the same
speed? is it independently justified?") layer by layer:

- **request-id, recover, metrics, auto-ban, negotiate → keep default-on.**
  Each is independently justified regardless of cost: recover is the
  documented non-negotiable safety net (`warn-custom-stack` already
  enforces its presence); request-id is the tracing floor other layers
  depend on; metrics and auto-ban are free-to-cheap and contention-free;
  negotiate (the single most expensive layer) *is* the "zero-config JSON
  API" value proposition itself — cutting it isn't optional, it's cutting
  the feature.
- **cors → move to opt-in.** It fails the test: a large fraction of bri
  APIs are server-to-server or CLI-consumed and never see a browser
  Origin header, so permissive CORS-by-default buys nothing for them while
  costing Δ6.6–10.4µs/request (one of the top 3 layers) on every request,
  browser or not. `cors` stays available as one `(conj (api-defaults) ...)`
  away — it's a one-line opt-in for the browser-facing case, not a design
  regression.
- **logging is the harder call, flag for owner sign-off, don't cut
  unilaterally.** It's the #2 (alone) / #2 (cumulative) most expensive
  layer AND the only one with a confirmed real per-request I/O cost
  (~7µs extra syscall, measured above, worse in a real container). But
  `api-defaults`'s own doc comment promises "zero-config gives logs... out
  of the box" as the headline feature, and this codebase's own test
  suite names "2 a.m. debugging" as the scenario it exists for — removing
  it from defaults reverses a documented product promise, which is an ADR
  decision, not a spike's to make. Recommend keeping it default-on unless
  the owner explicitly wants to trade "logs by default" for raw req/s.

Net: of the four candidate cuts named in the Docker leg's stack, this
spike found **one clean cut (cors)**, **one call for the ADR
(logging)**, and confirmed **no contention risk** in the two layers
(metrics, auto-ban) most likely to have one. The published 42%
comparison against clj-httpkit remains not like-for-like — no other
entrant in that corpus does per-request structured logging, metrics, or
abuse protection at all — but that context justifies *documenting* the
tradeoff, not *keeping every layer by default without measurement*, which
is what this spike was asked to settle.
