# ADR 0061 — Streaming file & byte I/O (bri.io)
Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23; evidence: spike
S40) · New bri battery; rides cljgo's reduce/transduce machinery (transducer
conformance is covered by the ADR 0022 compliance suite).

## Context

The owner wants "streaming fast file I/O, fastest we need." cljgo already has
reducers/transducers; the blessed streaming form should let
`reduce`/`transduce`/`into` and the transducer library work over a file
**without slurping it into memory**. Spike S40 proved a reducible reader.

## Decision

1. **The blessed reader is a reducible** — it implements `IReduceInit`
   (`pkg/lang/interfaces.go`), `bufio`-driven internally — so `reduce`,
   `transduce`, `into`, and the transducer library apply to files with **zero
   new machinery**, exactly the bet `LongRange`/`Iterate`/`Cycle` already make.
   Lazy-open at reduce time; re-reducible, so a `def`'d source stays REPL-live.
   (Names `io/lines`/`io/copy`/`io/writer` intentionally echo
   `clojure.java.io`'s vocabulary — a familiarity choice, not a clojure.core
   shadow, and always namespace-qualified.)
2. **`bri.io` surface:** `io/lines`, `io/byte-chunks` (**renamed** from the
   spike's `io/bytes` — precedence principle: `clojure.core/bytes` exists),
   `io/copy`, `io/tee`, `io/writer`, `io/spit-lines`. Codec auto-selected by
   extension.
3. **Codecs v1: plain + gzip** (`compress/gzip`, pure-Go) through the same
   `lines` API. `zstd` deferred (needs a pure-Go dep, tracked like S31–33).
4. **`take` short-circuits the disk read** — the `reduced` wrap propagates out
   of `ReduceInit` and closes the file (proven: 11 of 11.5M lines read, stop).

## Evidence (S40 — M-series, 200 MB / 11.5M-line file, stdlib-only, measured)

- Read **~320 MB/s**; write (`spit-lines`, buffered) **~801 MB/s**.
- **Constant memory: peak live heap flat at 3.8 MB** across 50→100→200 MB —
  one line live at a time, nothing accumulates.
- **Tax ratio** vs raw `bufio.Scanner`: 2.72–3.16× (the Clojure boxed-step-fn
  convention, S19-class — *not* the I/O); absolute throughput still ~320 MB/s.
- gzip streams through the same API at 172 MB/s; `copy`+`tee` fan 210 MB to two
  sinks in one pass.

## Consequences

- Big-file processing joins the language for free via `reduce`/`transduce`.
- **The tax is MODELED, not final:** real `pkg/eval` reduce + long-boxing + the
  current chunked-reduce fast path must **re-measure** before the perf budget
  (ADR 0024) freezes a number. The budget lands *with the implementation*, not
  this ADR.
- The error surface must route through `diag.Diagnostic` (ADR 0015), never raw
  `panic` (the spike used raw panic).
- JVM `line-seq` edge semantics (CRLF, no trailing newline, non-UTF8) are frozen
  against the `clojure` oracle in conformance.
- Not chosen: lazy-seq `line-seq` as the blessed form (the reducible composes
  better *and* short-circuits); a raw-`bufio` fast lane (revisit only if the tax
  proves too high in practice); `zstd` in v1.
