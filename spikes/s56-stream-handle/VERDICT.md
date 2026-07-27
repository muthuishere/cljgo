# S56 VERDICT — the stream abstraction (reducible stream handle)

**VERDICT: SHIP `cljg.stream` — ONE stream handle, the readable half SEQABLE
(not core.async, not a new protocol), reused verbatim by `cljg.process/spawn`
and `cljg.net.http` `:as :stream`.** A readable stream wraps a Go `io.Reader`
and answers `Seq()` with a lazy seq of `[]byte` chunks pulled one-per-node, so
`reduce`/`into`/`doseq`/transducers and `take`/`reduced` short-circuit apply to
a live subprocess pipe or HTTP body with zero new machinery — the same bet
`pkg/lang`'s `LongRange`/`LazySeq` make. A writable stream wraps an `io.Writer`
with buffered flush-per-write + close (EOF to the peer). Proceed to ADR 0101 +
implementation.

Realized directly in-tree (this was the hard streaming half of the ADR 0101
task, not a throwaway probe): `pkg/bri/stream.go` (handle + shims),
`pkg/bri/proc_spawn.go` (spawn), `pkg/bri/net_http.go` (`-http-stream`),
`core/cljg/{stream,process}.cljg`, tests in `pkg/bri/stream_test.go` +
`cmd/cljgo/stream_compiled_test.go`.

## The design question this spike closed

s56 asked: what is a cljgo "stream"? Three candidates were on the table —
(a) core.async channels, (b) a new `IStream`/`IReduceInit`-style protocol,
(c) a Seqable reader. The deciding fact is **cljgo's native `reduce` is
seq-driven** (`pkg/corelib/hotpath_builtins.go`: it calls `lang.Seq(coll)` and
walks `First`/`Next`, with a chunked fast path for `IChunkedSeq` — it does NOT
dispatch to `IReduceInit`). So the s40 assumption ("reducible = implements
`IReduceInit`") does not actually make a value reduce-able in this runtime.

⇒ **A stream is reduce-able iff it is `Seqable`.** The readable handle
implements `lang.Seqable` (`Seq() lang.ISeq`), returning a `NewLazySeq` closure
that reads ONE chunk per node and `NewCons`es it onto the lazy tail. That single
method makes `(reduce f init handle)`, `(into [] handle)`, `(doseq [c handle])`,
and every transducer work — and `take`/`reduced` propagate through `LazySeq`,
stopping the underlying `Read` early.

core.async (a) was rejected: a go-block per stream is heavier and does NOT
compose with `reduce`/transducers/`into` without adapters. A bespoke protocol
(b) was rejected: `Seqable` + `read-line`/`read-bytes`/`read-all`/`close` +
`write`/`close` is the whole surface, and it rides the existing seq library.

## Backpressure & close semantics (the frozen contract)

- **Backpressure is inherent, no buffer to tune.** A chunk is produced only when
  the consumer asks (lazy-seq node realization / `read-line` / `read-bytes`). A
  slow consumer throttles the producer through the OS pipe buffer (subprocess)
  or the TCP receive window (HTTP) — no unbounded queue, constant memory.
- **Chunk size** default 64 KiB (one `bufio` fill); `st/chunks handle n` and
  `st/read-bytes handle n` override it. A chunk may be shorter than `n` (whatever
  is available now); EOF is a `nil` element that terminates the seq.
- **Single-pass, single-consumer.** Each element consumed advances the reader;
  seqing twice continues where the first left off (like the JVM `line-seq` over a
  `Reader`). Concurrent consumers are undefined and documented, not enforced.
- **Writable `write` flushes every call** so a line-at-a-time protocol works
  without waiting for a buffer to fill (proved by the bidirectional
  `upcaselines` test). `close` on a writable flushes then closes the pipe,
  sending **EOF** to the reader on the other end — this is how `cat` drains and
  exits. `close` is idempotent on both halves.
- **Readable `close`** releases the underlying `io.Closer` (the subprocess
  stdout pipe, or `resp.Body`). For HTTP `:as :stream` the body is NOT read and
  NOT closed by the shim — the handle owns it; the caller closes it. A streaming
  HTTP request therefore sets **no client timeout** (a download must not be
  killed mid-body) and is **not retried on 5xx** (the body handle is live).

## Evidence (green)

- `pkg/bri/stream_test.go` (interpreter): spawn `cat` echo-a-line through
  `:in`→`:out`; bidirectional live line streaming (no EOF) via `upcaselines`;
  `:kill` → non-zero `:wait`; `reduce`/`into`/`take` over `st/lines`; `reduce`
  byte-count over the raw Seqable handle; HTTP `:as :stream` lines + the default
  buffered body unchanged.
- `cmd/cljgo/stream_compiled_test.go` (AOT parity — the release blocker): a real
  `cljgo build` binary spawns `cat`, streams a line, and streams an httptest
  body via `:as :stream`; output matches the interpreter byte-for-byte, and the
  binary is `CGO_ENABLED=0` (bufio/io/os/exec/net/http are pure Go).

## Un-proven / deferred

1. **Consolidating `cljg.io/exec|sh|sh!` into `cljg.process`** — clean but would
   churn ADR 0089's surface + tests for no behavior change; deferred (ADR 0101 §4),
   not half-moved.
2. **Codec pass-through** (gzip/flate on a readable) — s40's territory; not wired
   into `cljg.stream` yet.
3. **`select`/multiplex over several readables** — no `alts!`-style combinator;
   a stream is a single blocking source. Revisit only if a real workload needs it.
4. **Zero-copy byte fast path** — each chunk allocates a fresh `[]byte`
   (idiomatic, immutable); a reused-buffer scan lane was not built.
