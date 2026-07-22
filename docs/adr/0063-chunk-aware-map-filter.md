# ADR 0063 — Chunk-aware map / filter (JVM realization parity)
Date: 2026-07-23 · Status: accepted (evidence: core audit 2026-07-23 + prototype,
measured) · Continues the native-hotpath line of ADR 0045 and the chunked-reduce
fast path (PR #66); supersedes the "documented chunking divergence" note the
reduce fast path left behind.

## Context

cljgo's `reduce` already walks chunked sources a chunk at a time (PR #66:
`range`/vectors implement `IChunkedSeq`, `reduce` consumes chunks). But `map` and
`filter` (native, `pkg/corelib/hotpath_builtins.go`) **threw chunking away** —
they built a plain `Cons`/`LazySeq` node per element. So any pipeline
`range → map/filter → reduce/count` degraded to one seq node + one boxed step
per element, even though the source was chunked.

A core audit (2026-07-23, answering the owner's "make core fast") measured the
cost: `(count (filter even? (range 2e6)))` = 0.23 s vs `(reduce + (range 2e6))`
= 0.07 s — a **3.3×** penalty attributable entirely to lost chunking. The audit
also confirmed cljgo's laziness is otherwise correct and, being *unchunked*, was
actually realizing *fewer* elements than the JVM — a correctness win that was
simultaneously the throughput loss, because JVM realizes (and processes) 32 at a
time.

## Decision

1. **`map`/`filter` (and `remove`, which is `filter` in core.clj) take a
   chunk-aware fast path** when the source is `IChunkedSeq`: transform/scan the
   whole chunk in a tight loop into a fresh `ChunkBuffer`, then `ChunkedCons` it
   onto a lazy map/filter of `ChunkedMore()` — mirroring `clojure.core`'s
   `(chunked-seq? s)` branch. An all-reject `filter` chunk drops to the lazy
   tail (matching `chunk-cons`, which never conses an empty chunk). The
   non-chunked path (unchunked sources like `iterate`/`repeatedly`) is unchanged.
2. **`LongRange`'s chunk is capped at 32** (`longRangeChunkSize`), matching JVM
   Clojure's `LongRange`. Previously `ChunkedFirst` returned the *entire* range
   as one chunk; with chunk-preserving map/filter that would force the whole
   range on any partial consumption (e.g. `(take 5 (map f (range 2e6)))`). The
   32-cap keeps realization granularity **identical to the JVM**.
3. **This is a deliberate, observable semantic move toward JVM parity:**
   `map`/`filter` over a chunked source now realize a *chunk* (up to 32) at a
   time, not element-at-a-time. `(take 5 (map side-effect (range 1000)))` now
   realizes 32 — exactly what JVM Clojure 1.12.5 does (it was 5 unchunked).
   Unchunked sources still realize element-at-a-time in both cljgo and JVM.

## Evidence (AOT-compiled, hyperfine 3 warmup/10 runs, startup included)

| expression | before | after | JVM 1.12.5 |
|---|---|---|---|
| `(count (filter even? (range 2e6)))` | 243.7 ms | **180.5 ms** (1.35×) | 341.7 ms |
| `(reduce + (map inc (range 2e6)))` | 319.8 ms | **138.9 ms** (2.30×) | 323.7 ms |
| `(count (filter odd? (map inc (range 2e6))))` | 482.0 ms | **272.6 ms** (1.77×) | 336.3 ms |

The filter-vs-raw-reduce excess-gap fell from 3.04× to 2.33×; **cljgo AOT now
beats JVM wall-clock on all three** (JVM pays ~300 ms boot). 73 LOC, two files
(`pkg/corelib/hotpath_builtins.go`, `pkg/lang/longrange.go`); all chunk
infrastructure (`ChunkBuffer`, `ChunkedCons`, `LongChunk`) pre-existed.

## Consequences

- **Conformance / dual-harness:** the full suite passes unchanged (REPL + AOT).
  The one realization probe (`lazy-map-filter-no-over-realization.clj`) uses an
  *unchunked* source (`repeatedly`) and still asserts `[1 1 1]` — the fallback
  path is untouched. A new test freezes the *chunked* realization count (32) as
  the JVM-matching contract, so a future regression to element-wise or
  whole-range chunking is caught.
- **Perf gate:** a new `TestCorePipelinePerfBudget`
  (`(count (filter odd? (map inc (range 2e6)))`), wall-clock total, ADR 0024
  host-relative) closes the CI blind spot the range-only reduce gate left — no
  gate previously exercised `map`/`filter`.
- **The residual 2.33×** is `filter`'s per-element predicate call plus `count`
  walking the result via `Next()` (one `ChunkedCons` per element — `lang.Count`
  is not yet chunk-aware). A **chunk-aware `count`** (advance by `ChunkedNext`)
  is the named follow-up; `keep` (a separate `lazy-seq` in core.clj) is likewise
  left unchunked for now.
- Sits on the ADR 0045 line and does not touch the deeper 35× per-element boxing
  campaign (that is the emitter type-inference / native-reduce work, tracked
  separately) — this is the cheap structural win, banked.
- Not chosen: an uncapped whole-range chunk (2.03× — faster, but breaks JVM
  realization parity); leaving map/filter unchunked (the 3.3× penalty).
