# S40 — streaming, fast file & byte I/O (`bri.io`)

Owner ask: *"streaming fast file input output, fastest we need."* A battery
for reading and writing files that (a) never slurps the whole file into
memory, (b) plugs straight into cljgo's existing `reduce`/`transduce`/`into`
via the **reducible** contract, and (c) is only a thin, measured tax over
raw Go `bufio`.

## The blessed form (position under test)

A file line-stream is a **reducible** — a value that implements cljgo's
`IReduceInit` (`ReduceInit(f, init)`, `pkg/lang/interfaces.go:52`). It opens
the file lazily at reduce time, drives a `bufio` loop internally, feeds each
line through the step fn, honours `reduced`/`Reduced` early-termination, and
closes the file when the reduction ends. Because `reduce`, `transduce`, and
`into` all bottom out on `ReduceInit`, `(into [] (comp (filter ...) (take 5))
(io/lines path))` works with ZERO changes to core — and holds constant memory
because nothing is ever materialised except what the transducer chooses to
keep.

This mirrors what `LongRange`/`Iterate`/`Cycle` already do in `pkg/lang`
(each is an `IReduce`), so the emitter and interpreter treat an `io/lines`
value exactly like a range.

## Exit criteria (frozen before code)

1. **Line-stream a large file as a reducible, constant memory.** Generate
   ~200 MB of text; reduce over its lines (count + a numeric sum) via the
   reducible. Prove peak Go heap / RSS stays flat as file size grows 50→100→
   200 MB (nothing holds all lines). Report read MB/s.
2. **Tax ratio (ADR 0024 discipline).** The identical reduction (a) as a raw
   `bufio.Scanner` loop in straight Go vs (b) through the reducible +
   boxed step-fn that models the cljgo `IReduceInit`/`IFn.Invoke`/`Reduced`
   overhead. Report wrapped-vs-raw ratio, characterised like S19's channel
   tax.
3. **Composability without materialising.** A transducer pipeline
   (`filter`→`map`→`take`) over the stream, modelled in Go with a step fn,
   proving `(into [] (comp (filter even?) (map inc) (take 5)) (io/lines p))`
   semantics: early `take` stops the file read (reduced short-circuits the
   scan — verified by lines-read count << file lines).
4. **Codec pass-through.** The SAME `lines` API streams a gzip file
   (`compress/gzip`) and a plain file — codec chosen by extension / explicit
   wrap. Plus `io/copy` (Reader→Writer) and `io/tee` (fan a Reader to a
   second Writer while reducing).
5. **Writer side, constant memory.** A streaming sink (`io/spit-lines` /
   `io/writer`) that writes a large seq to disk through a `bufio.Writer`,
   flat memory. Report write MB/s.

## What "done" means

A runnable Go prototype (`probe/`) with its OWN `go.mod` (`cljgospike/s40`),
real numbers on this Mac (Apple M5 Pro, go1.26.3), a `.cljg` sketch of the
blessed `bri.io` surface, and a `VERDICT.md` that takes a position, gives the
throughput / tax / constant-memory evidence, lists UN-PROVEN risks, and routes
owner calls.

## Non-goals

Not building `bri.io` for real. No `pkg/`, `core/`, `cmd/`, or conformance
changes. Probe is throwaway (ADR 0027). Big generated files are deleted by
the run script.

## Layout

- `probe/main.go` — generator, reducible, raw baseline, tax bench,
  transducer pipeline, gzip/copy/tee, writer sink; prints PASS/FAIL + numbers.
- `sketch.cljg` — the blessed `bri.io` API surface (not loaded; illustration).
- `run.sh` — driver: builds, runs, cleans up temp files.
- `VERDICT.md` — written last.
