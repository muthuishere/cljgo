# s74 — the shape of bri's 10-entry `requestMap`: results

**Question.** `requestMap` (`pkg/bri/http.go:276`) builds 9-10 keys
(`:request-method :uri :query-string :headers :params :path-params
:query-params :body :remote-addr :route-pattern`), which is over the
array-map→hash-map threshold (`hashmapThreshold = 16` slots = 8 entries,
`pkg/lang/persistentarraymap.go:8`). Is a forced array-map shape, a reduced
key set, or the current hash map the right build for a per-request map that
gets built once, read a handful of times, and sometimes grown by middleware?

**Measured 2026-07-31**, darwin/arm64, Apple M5 Pro, go1.26. Harness: a
temporary internal benchmark in `pkg/lang` (package-internal, so it could
construct a `*Map` directly and bypass `NewMap`'s threshold check the way
Clojure's own `(array-map ...)` constructor does) — **not committed**,
removed after this run. `go test ./pkg/lang/ -bench BenchmarkS74 -benchmem
-benchtime 100000x`.

## 0. Clojure ground truth (verified against the real `clojure` CLI, not memory)

```
user=> (class (array-map :a 1 :b 2 :c 3 :d 4 :e 5 :f 6 :g 7 :h 8 :i 9 :j 10))
clojure.lang.PersistentArrayMap          ; explicit array-map DOES permit >8 entries
user=> (class (assoc *1 :k 11))
clojure.lang.PersistentHashMap           ; but assoc past the threshold converts
user=> (class {:a 1 :b 2 :c 3 :d 4 :e 5 :f 6 :g 7 :h 8 :i 9 :j 10})
clojure.lang.PersistentHashMap           ; the {} literal builds via hash-map, not array-map
```

cljgo's `NewMap` matches the `{}`-literal behavior (converts at construction),
not the `array-map` behavior (which stays array past 8). Building an
over-threshold array map in cljgo therefore requires bypassing `NewMap` —
there is no public constructor for it today.

## 1. Build cost

| shape | entries | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| hash map (current `NewMap`, 10 entries, 20 slots) | 10 | 905.4 | 2768 | 47 |
| array map forced past threshold (10 entries) | 10 | 179.5 | 800 | 12 |
| array map, in-threshold (7 entries, 14 slots) | 7 | 42.4 | 272 | 2 |

Scaling: build cost is not linear in entry count alone — it is a step
function at the threshold. Forced-array 10 vs in-threshold array 7 is only
~4.2x for +3 entries (linear-ish, array build is O(n) copy); hash map 10 vs
forced-array 10 is **5.0x time / 3.5x memory / 3.9x allocs for the identical
10 entries** — that delta is pure shape cost, not size cost.

## 2. Read cost — 1 / 3 / 5 / 10 key lookups (realistic middleware/handler access)

| keys read | hash map ns/op (B/op, allocs) | array map (forced, 10-entry) ns/op (B/op, allocs) | array/hash ratio |
|---|---|---|---|
| 1  | 32.3  (48, 2)   | 12.6  (16, 1)   | 0.39x |
| 3  | 98.8  (144, 6)  | 47.5  (48, 3)   | 0.48x |
| 5  | 164.3 (240, 10) | 89.7  (80, 5)   | 0.55x |
| 10 | 328.4 (480, 20) | 229.4 (160, 10) | 0.70x |

Scaling: both are linear in keys read (expected — no caching between calls).
The array map's linear scan is **faster than the hash trie walk at every
scale measured, including reading all 10 keys** — the hash map's per-`ValAt`
cost includes an interface-boxing allocation on the key argument that the
array map's own `ValAt` pays too, but the trie descent adds real work on top.
At realistic access counts (2-5 keys per request, the stated middleware/
handler pattern) the array map is **2.1x-1.8x faster**, not slower. The read
side does NOT make the case for the hash map here —10 linear comparisons on
short keyword slices is still cheap on modern hardware, and this scan never
gets larger because request-map key count is bounded by the framework, not
by user data.

**Exclusion:** this isolates `ValAt` only — no HTTP parsing, no
`lang.Get`/protocol dispatch overhead layered on top, and does not model a
pathological handler that reads the same key in a hot loop (only Get, not
repeated destructuring).

## 3. Assoc cost — middleware adding a key (the case that could flip the verdict)

| onto shape | ns/op | B/op | allocs/op | note |
|---|---|---|---|---|
| hash map (10 entries) + assoc 1 key | 95.2  | 400  | 5  | HAMT path-copy, cheap |
| array map forced-over-threshold (10) + assoc 1 key | 1015 | 3168 | 52 | **converts to hash map every single assoc** |
| array map in-threshold (7) + assoc 1 key | 200.3 | 736  | 4  | stays array (7→8 entries, 16 slots = still `< hashmapThreshold`) |

This is the decisive number. `Map.Assoc` (`persistentarraymap.go:216`) checks
`len(m.keyVals) < hashmapThreshold` — an over-threshold array map re-derives
a full `PersistentHashMap` from scratch on **every** assoc, because the check
re-runs from the same over-threshold state each time; it never "becomes"
a hash map once and stays one across a chain of calls unless the caller keeps
the converted result. A forced 10-entry array map that goes through even one
middleware `assoc` costs **10.7x the time and 7.9x the allocation** of just
building the hash map directly. Two middleware layers each doing one `assoc`
(a realistic bri stack: auth then request-id) would pay that conversion
tax twice if each held the array-shaped value — this is the false economy
the task asked to check for, and it is real.

## 4. The ≤7-key alternative

To get under the 8-entry threshold without an assoc-conversion trap, 3 keys
of the current 9-10 must go. Candidates, in order of removability:

- **`:path-params`** — already a documented duplicate of `:params` ("Compojure-ish
  alias… kept for back-compat", `http.go:333`). Dropping it removes the
  duplication, not information. Lowest-cost removal.
- **`:route-pattern`** — only present when a mux pattern exists; used for
  metrics/logging labels, not typically read by handlers. Removable but loses
  a feature (route-pattern-based metrics) for anyone currently relying on it.
- **`:remote-addr`** or **`:query-string`** — both are real, commonly-read
  Ring/Compojure-convention keys. Removing either is a bigger break: it is
  not internal bri plumbing, it is public request-map surface every
  Ring-family framework ships.

Verdict on this option: dropping `:path-params` alone only gets to 9 keys
(18 slots, still over threshold) — reaching ≤7 requires cutting at least
**two** of the three real keys above (`:route-pattern` is the next cheapest,
then `:remote-addr` or `:query-string`). **That is a plain API break** for
any handler or middleware currently destructuring `:remote-addr`,
`:query-string`, or `:route-pattern` off the request map — this is not a free
optimization, it removes documented, back-compat-flagged surface for the sole
purpose of staying under an internal implementation threshold. Build cost of
the resulting 7-key array map is 42.4 ns / 272 B / 2 allocs — vs today's
905.4 ns / 2768 B / 47 allocs — a real 21x/10x/23x win, but earned by
deleting API, not by a shape change.

## VERDICT

**Keep the hash map. Do not force an over-threshold array-map shape for
`requestMap`, and do not cut keys to chase the 7-key array-map number.**

- The build-side win for a forced array map (5.0x time, 3.5x memory) is real
  but it is not free — §3 shows it evaporates and reverses (10.7x *worse*)
  the moment ANY middleware layer does a single `assoc`, which is bri's
  documented middleware pattern (auth, request-id, tracing all assoc onto
  the request map). A per-request object that gets `assoc`ed at least once
  in a typical request is the common case, not the edge case, for a web
  framework — measuring only the build cost was the trap the task asked to
  check for, and it is a trap.
- The read-side numbers (§2) do favor linear scan at 10 entries (1.4x-2.6x
  faster than hash-trie at 1-10 keys read) — but that gain is smaller than
  the assoc-conversion cost it risks, and it requires a NEW mechanism
  (bypassing `NewMap`'s threshold, i.e. a second array-map constructor path)
  to get at all. Per "Simplicity first, then performance": this is exactly
  the case where the price is a second code path and a cache-like
  invalidation story (when does the array shape convert, who pays for it,
  is it still array-shaped after middleware #1 or #2) for a gain that is
  smaller than its own downside case.
- Cutting to ≤7 keys (§4) buys the best absolute numbers but costs public,
  back-compat-flagged API surface (`:route-pattern`, and either
  `:remote-addr` or `:query-string`) for reasons entirely internal to
  cljgo's map representation — the user gets a smaller map because of how
  cljgo happens to implement `IPersistentMap`, not because they asked for
  fewer keys. That is optimizing the implementation detail by breaking the
  contract, which the precedence and simplicity doctrines both rule out.
- The one genuinely free win already sitting in the code: **`:path-params`
  is a literal duplicate of `:params`** (same `pm` value, `http.go:333`).
  Removing it drops 9→8 keys... which is still over the 8-*entries*-stays,
  9th-entry-converts boundary (`hashmapThreshold` triggers at the 16th
  *slot*, i.e. the 8th pair already returned by `NewMap`, so 8 real entries
  is still exactly at the hash-map boundary) — so even this dedup does not
  change the shape outcome. It is worth doing anyway (it is dead
  duplication, not a perf change) but should not be sold as a shape fix.

No shape change to `requestMap` is justified by this data. The 905 ns / 2768
B / 47 allocs it costs today is the correct trade for a value that gets
assoc'ed onto by middleware in the framework's own normal operation.
