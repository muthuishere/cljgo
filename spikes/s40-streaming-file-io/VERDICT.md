# S40 VERDICT — streaming, fast file & byte I/O (`bri.io`)

**VERDICT: SHIP a `bri.io` battery whose reader (`io/lines`, `io/bytes`) is a
REDUCIBLE (implements `IReduceInit`, `pkg/lang/interfaces.go:52`), driven by
`bufio` internally.** It gives constant-memory streaming, plugs into the
EXISTING `reduce`/`transduce`/`into` and the transducer library (ADR 0022)
with zero new protocol, honours `reduced` (so `take` stops the disk read), and
transparently spans plain + gzip files through one API. The writer side
(`io/spit-lines`, `io/writer`) is a buffered sink with the same shape. The one
real cost is a ~2.7–3.2× wrapper tax over a bare `bufio.Scanner` loop —
inherent to Clojure's boxed step-fn contract, in line with S19's channel tax,
and still ~320 MB/s / ~800 MB/s absolute. Recommend proceed to ADR + T-tier.

Machine: darwin/arm64 (Apple M5 Pro), go1.26.3, `CGO_ENABLED=0`-compatible
(stdlib only, no deps). Test file: 200 MB, 11,537,411 lines. All 5 criteria
PASS (`probe/`, re-run with `./run.sh`).

## Evidence

### C1 — line-stream as reducible, constant memory ✅
- Correct: `count=11,537,411 sum=5,763,237,787` over 200 MB via `reduce` on the
  `LinesReducible`.
- **Read throughput: ~320–332 MB/s** (0.60–0.62 s for 200 MB).
- **Constant-memory proof** — peak live heap sampled during the reduce, file
  size swept 4×:

  | file | peak heap during reduce | MB/s |
  |------|------------------------|------|
  |  50 MB | 3.8 MB | 298 |
  | 100 MB | 3.8 MB | 305 |
  | 200 MB | 3.8 MB | 315 |

  Heap is **flat as the file grows 4×** ⇒ nothing accumulates; only one line
  is live at a time. This is the headline property.

### C2 — tax ratio (ADR 0024) ⚖️
Identical count/sum, raw `bufio.Scanner` loop vs through the reducible + boxed
step-fn (models `IFn.Invoke(...any) any` + `reduced` check + accumulator
boxing). Two workloads separate the cost:

| workload | raw | wrapped | tax |
|----------|-----|---------|-----|
| count-only (pure dispatch) | 1451 MB/s | 459 MB/s | **3.16×** |
| sum+parse (dispatch + acc int-boxing) | 895 MB/s | 329 MB/s | **2.72×** |

Reading of the number: the tax is the Clojure calling convention, not the I/O.
Per line the wrapper pays an interface `Invoke`, boxing each line into `any`,
a `reduced` type-assertion, and (for numeric folds) re-boxing the `int64`
accumulator every step. The count-only case is *higher* ratio because the raw
baseline there is almost free (`count++`), so the fixed dispatch cost dominates
the ratio; once real per-line work (parse) is added the ratio drops to 2.72×.
Both are **the same order as S19's 2.7–5.6× channel-op tax** and are accepted
there. Absolute wrapped throughput (~320 MB/s) far exceeds any realistic
line-processing workload, which will itself dwarf the dispatch cost.

### C3 — composability, no materialisation ✅
`(into [] (comp (filter even?) (map inc) (take 5)) (io/lines path))` modelled
with real step fns → `[669 129 653 53 83]`, and **`take` short-circuited the
disk read: 11 of 11,537,411 lines read, then stopped** (the `reduced` wrap
propagates out of `ReduceInit` and closes the file). Transducers compose over
the stream for free because everything bottoms out on `IReduceInit`.

### C4 — codec pass-through ✅
Same `lines` API over a 53.1 MB gzip of the same data → identical
count/sum, **172 MB/s decompressed** (gzip-bound, not I/O-bound — expected).
`io/copy` (`io.Copy`) + `io/tee` (`io.TeeReader`) fanned all 209,715,206 bytes
to two sinks in one pass.

### C5 — writer sink, constant memory ✅
`io/spit-lines` streamed 11.5 M produced lines (358 MB) through a
`bufio.Writer`: **write throughput ~801 MB/s**, peak heap 3.9 MB (flat — the
producing seq is never materialised). Gzip sink works through the same call
(2.19 MB logical → 0.26 MB on disk).

## Recommended BLESSED `bri.io` surface

Full sketch in `sketch.cljg`. Shape:

- `(io/lines path)` / `(io/lines path opts)` → **reducible** of line strings.
  Codec auto by extension (`.gz`→gzip), lazy-open at reduce time, closes on
  normal end or `reduced`. Re-reducible (each `reduce` re-opens) ⇒ a var
  holding it stays REPL-live.
- `(io/bytes path)` → reducible of fixed-size `[]byte` chunks (binary streams).
- `(io/spit-lines path coll)` → stream a (lazy/huge) seq of strings to disk,
  buffered, constant memory; returns bytes written. Codec by extension.
- `(io/writer path opts)` → open buffered sink handle (escape hatch).
- `(io/copy src dst)` → Reader→Writer stream copy.
- `(io/tee src dst)` → read-through fan-out to a second sink while reducing.

Rationale: reader-as-reducible is the ONE decision that makes the whole
transducer/`into`/`reduce` library apply to files with no new machinery — the
same bet `LongRange`/`Iterate`/`Cycle` already make in `pkg/lang`.

## UN-PROVEN risks

1. **Real runtime boxing may differ from the model.** The probe models
   `IFn.Invoke`/`Reduced`/`int` boxing; cljgo's actual `reduce` path,
   long-boxing, and any fast-paths (e.g. the chunked-reduce work on the
   current perf branch) could move the 2.7× up or down. Must be re-measured
   against the REAL `pkg/eval` reduce before the tax is quoted in an ADR.
2. **`sc.Text()` allocates a string per line.** Fine and idiomatic (cljgo
   strings are immutable Go strings), but a zero-copy `bytes`-chunk fast path
   (`sc.Bytes()` reused buffer) for pure byte scanning was not built — a
   possible T2/T3 optimisation, unproven.
3. **AOT/interpreter reach.** Like pgx in S25, `bri.io` is Go-native; whether
   it ships as a `pkg/bri` shim in the seed registry or via self-rebuild
   (design/05) is unresolved — not this spike's question.
4. **Error surface not built.** Missing file / permission / mid-stream gzip
   corruption must route through `diag.Diagnostic` (ADR 0015), not the raw Go
   `panic` the probe uses. Un-built.
5. **No `race`/concurrent-consumer testing.** A single reducible reduced from
   two goroutines is undefined here; the JVM `LineSeq` is likewise not
   thread-shared. Left as documented single-consumer.
6. **Long-line / no-trailing-newline / CRLF / non-UTF8** edge semantics vs JVM
   `line-seq` not frozen against the oracle (spike used clean generated data).
7. **Only gzip tested.** `compress/flate`, `bzip2` (read-only in stdlib), zstd
   (needs a dep — breaks pure-Go/no-dep unless a pure-Go impl) not evaluated.

## Owner-gated questions

1. **Is 2.7–3.2× wrapper tax acceptable as the blessed default?** (It's the
   Clojure boxed-step-fn cost, matches S19, absolute is ~320 MB/s.) Ship as-is,
   OR also expose a raw-`bufio` fast lane (`io/scan` with a Go-typed callback)
   for hot paths that opt out of boxing? Recommendation: **ship the reducible as
   the one blessed form; revisit a fast lane only if a real workload proves I/O
   dispatch is its bottleneck.**
2. **`io/bytes` vs `clojure.core/bytes` name collision** (precedence
   principle). Keep `bri.io/bytes` (qualified use, different fn) or rename to
   `io/byte-chunks`? Recommendation: **`io/byte-chunks`** to avoid any shadow
   read.
3. **Codec scope for v1:** plain + gzip only (pure-Go, zero-dep), or commit to
   zstd (needs a pure-Go dep like klauspost/compress, which stays
   `CGO_ENABLED=0`)? Recommendation: **plain+gzip in v1; zstd behind an
   ADR-tracked impure-dep decision (cf. S31–S33).**
4. **`bri.io` namespace name** — `bri.io` vs `bri.file` vs folding readers into
   an existing `bri.*`? Recommendation: **`bri.io`.**
