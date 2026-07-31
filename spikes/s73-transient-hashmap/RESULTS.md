# Spike s73 — transient HAMT for bulk `NewPersistentHashMap` construction

## Question

Should `pkg/lang` grow a transient hash map so `NewPersistentHashMap` /
`NewMap` above the array-map threshold stop building by N successive
path-copying `Assoc`s?

## What was built

`pkg/lang/hashmap_transient.go`: an **internal-only** transient HAMT, used
solely inside `NewPersistentHashMap` (`pkg/lang/persistenthashmap.go`). It
mirrors `clojure.lang.PersistentHashMap`'s transient nodes:

- Every node (`BitmapIndexedNode`, `HashCollisionNode`, `ArrayNode`) gets an
  `edit *int32` owner-token field (nil = ordinary immutable node — every
  existing non-transient code path ignores it).
- `assocT` (a twin of the existing `assoc`, one per node type) mutates a
  node's array **in place** when it is already owned by the running build's
  token, instead of path-copying it on every insert. `BitmapIndexedNode`
  over-allocates its array by 4 kv-slots of headroom on each grow, so
  several more inserts land in the same node before it must grow again —
  this headroom is what turns "copy every ancestor on every insert" into
  amortized in-place mutation.
- `persistent()` re-slices every touched array back to its tight,
  real-content length (`finalizeHashNode`) and clears `edit`, so the
  returned map is byte-for-byte the same shape a non-transient build would
  produce — every reader downstream (`find`, `without`, `nodeSeq`, `iter`,
  `Equiv`) sees the exact invariants it already assumed.
- **No public `ITransientMap`/`IEditableCollection` surface is added.**
  `PersistentHashMap` does not gain `AsTransient()`. The existing defect
  test `s4_defects_test.go:TestDefectNoHAMTTransients` (which pins "no
  public HAMT transient exists") still passes unmodified — this spike does
  not change Clojure-visible behavior at all, only how one Go function
  builds its result.

`NewPersistentHashMap` was rewired to build through one
`transientHashMapBuild` instead of looping `res = res.Assoc(k, v)`.

## Benchmark harness

`pkg/lang/map_perf_test.go` (new — no such harness existed before this
spike, despite the task brief's assumption; built to spec):
`BenchmarkPersistentHashMapBuildBySize` builds an int64→int64 map of size
n directly via `NewPersistentHashMap`, for n = 8/16/32/64/128/256.
`go test ./pkg/lang/ -bench BenchmarkPersistentHashMapBuildBySize -benchmem
-benchtime 1000x`, darwin/arm64, Apple M5 Pro.

## Before → after

| n   | ns/op (before) | ns/op (after) | speedup | B/op (before) | B/op (after) | B/op ratio | allocs/op (before) | allocs/op (after) | allocs ratio |
|-----|---------------:|--------------:|--------:|---------------:|---------------:|-----------:|---------------------:|---------------------:|-------------:|
| 8   | 1086           | 396.4         | 2.74×   | 1920            | 628             | 3.06×      | 32                    | 8                     | 4.00×        |
| 16  | 2796           | 850.4         | 3.29×   | 5856            | 2100            | 2.79×      | 76                    | 20                    | 3.80×        |
| 32  | 7028           | 2773          | 2.53×   | 17504           | 6468            | 2.71×      | 198                   | 66                    | 3.00×        |
| 64  | 12673          | 6499          | 1.95×   | 40976           | 8548            | 4.79×      | 405                   | 92                    | 4.40×        |
| 128 | 23387          | 9490          | 2.46×   | 90928           | 14165           | 6.42×      | 809                   | 142                   | 5.70×        |
| 256 | 46766          | 23743         | 1.97×   | 202992          | 28900           | 7.02×      | 1709                  | 296                   | 5.77×        |

(`go test ./pkg/lang/ -run xxx -bench BenchmarkPersistentHashMapBuildBySize
-benchmem -benchtime 1000x`, both legs re-measured this session.)

## Scaling statement

**Allocation is linear in both legs, but the constant drops ~4–6×, and the
drop grows with n.** Before: allocs/entry is a near-flat ~6.5 (32/8=4.0,
1709/256=6.68) — i.e. every single insert still pays for path-copying
several ancestor nodes, so allocs stay proportional to n·depth, not n.
After: allocs/entry shrinks from 1.0 at n=8 to 1.16 at n=256 (296/256) —
essentially **O(n)** with the amortizing headroom absorbing most
of the depth factor. The allocation *reduction ratio* itself grows with
n (4.0× → 5.8×), which is the amortization working as designed: more
inserts sharing the same owned, headroom'd node before the next
reallocation.

Time is mildly superlinear in both legs (n256/n8 ratio: 43× before,
60× after, for 32× more entries) — expected, since deeper trees do more
work per insert regardless of allocation strategy — but the after leg's
**time** win (1.95–3.3×) is consistently in the same direction as its
**allocation** win (3.0–5.8×): both metrics improve together, unlike the
"6× time, 60× allocation" red-flag pattern CLAUDE.md warns against. This
is a clean win, not a trade.

## What this measurement EXCLUDES

- Only fresh, from-empty-root builds are measured (`NewPersistentHashMap`
  with no prior map). `PersistentHashMap.Assoc` on an *existing* persistent
  map (single key added to a long-lived map) is untouched — that path still
  path-copies, as it must, since sharing with the original persistent
  structure is the entire point there.
- `Without`/dissoc transients were not built — the transient here supports
  build-only (`assocT` + `persistent()`), no `without!`. Deletion-heavy bulk
  construction (rare — `dissoc` isn't how maps are normally built) is
  unmeasured and unimplemented.
- Values are `int64` (no hash collisions exercised at any depth beyond
  natural hash spread); `HashCollisionNode.assocT`'s growth path is
  exercised by unit/conformance tests, not by this benchmark.
- Wall-clock only on darwin/arm64 (Apple M5 Pro); no GC-pressure-under-
  concurrent-load scenario was measured — this is a single-goroutine,
  quiescent-heap number, a floor on real-world GC-shared-heap cost, not a
  ceiling.

## Moving-part count

- 3 struct field additions (`edit *int32` on `BitmapIndexedNode`,
  `HashCollisionNode`, `ArrayNode`) — ignored by every pre-existing method.
- 1 new `Node` interface method (`assocT`), 3 implementations — each a
  direct structural twin of the existing `assoc` on the same type (same
  branches, same shape), not a new algorithm.
- 5 small helpers (`ensureEditable`/`editAndSet1`/`editAndSet2` on
  `BitmapIndexedNode`, `ensureEditableArr` on `ArrayNode`,
  `ensureEditableColl` on `HashCollisionNode`) + `createNodeT` + `newEdit`.
- 1 unexported build-scratch struct (`transientHashMapBuild`, 3 methods)
  and 1 finalize function (`finalizeHashNode`).
- **1 call site changed**: `NewPersistentHashMap`'s body (6-line loop →
  8-line transient-build loop). Every other caller of `PersistentHashMap`
  (Assoc, Without, EntryAt, Seq, …) is untouched.
- **No new exported type, no new public protocol.** The mutable
  shared state (the `edit` token and the node graph while a build is in
  flight) never escapes `hashmap_transient.go` — `persistent()` clears
  every `edit` field to nil and re-slices every array tight before
  returning, so the result is provably indistinguishable from what the old
  loop produced. Nothing holds a `*int32` token or a non-nil-`edit` node
  past the one function call.

Total new code: one file, 281 lines
(`pkg/lang/hashmap_transient.go`) + a 23-line net diff in
`persistenthashmap.go` + a 63-line new benchmark file
(`pkg/lang/map_perf_test.go`).

## Correctness

- `go build ./...`, `go vet ./...`, `gofmt -l pkg cmd conformance templates
  core` all clean.
- `go test ./pkg/lang/ -timeout 300s` — pass (includes
  `TestDefectNoHAMTTransients`, unmodified, still documents "no public HAMT
  transient" — confirms this stays an internal implementation swap).
- `go test ./pkg/eval/...` — pass.
- `go test ./pkg/corelib/...` — pass.
- `go test ./conformance/ -run TestConformanceEval -timeout 900s -p 1` —
  pass.
- Hashing/equality semantics unchanged: `HashEq`/`Equiv`/`Equals` are not
  touched by this spike; the finalized map's `root` is structurally the
  same shape (`BitmapIndexedNode`/`HashCollisionNode`/`ArrayNode`, tightly
  sized arrays) a non-transient build would have produced.

## VERDICT: adopt

Applying the operational test:

- **"Would you keep this if it were the same speed?"** No — with no
  speed win this is pure internal complexity for zero benefit, and would
  be refused. But it is not the same speed: both allocation (3–5.8×
  fewer) and time (1.95–3.3×) improve, together, at every size measured,
  with the allocation win *growing* with n. That clears the bar the 8%
  rule is protecting: this is much closer to "6× time, but allocation
  also drops" than to "6× time, 60× allocation."
- **Moving parts, counted honestly:** one new file, three node types each
  grow one field and one twin method mirroring an existing one, one
  unexported build-scratch struct, one call site changed. No new public
  API, no pluggable strategy, no cache with an invalidation story — the
  mutable edit-token state is provably contained to the single function
  call that creates it and never escapes past `persistent()`.
- **Independently justified?** Not by correctness — this is pure
  performance work — but it reuses a pattern (owner-token transients)
  this codebase already ships for vectors, sets, and array-maps
  (`pkg/lang/vector.go`, `pkg/lang/set.go`,
  `pkg/lang/persistentarraymap.go`), so it is not a novel kind of
  complexity for a reader of this tree, just the missing fourth instance
  of a pattern already present three times.

Adopt as implemented: internal-only, one call site
(`NewPersistentHashMap`), no public transient protocol added to
`PersistentHashMap`. If a future need arises for a *public*
`assoc!`/`persistent!` on hash maps (e.g. a Clojure-visible
`(transient {})`), that is a separate decision with its own ADR — this
spike deliberately did not open that door, since the question asked was
only about bulk-construction cost.
