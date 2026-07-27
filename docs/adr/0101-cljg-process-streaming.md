# ADR 0101 — cljg.stream + cljg.process: one reducible stream, streaming spawn, streaming HTTP

Status: Accepted (2026-07-27)
Supersedes: none. Extends ADR 0087 (cljg.net.http), ADR 0089 (cljg.io exec/sh).
Resolves: spike s56 (`spikes/s56-stream-handle/VERDICT.md`).

## Context

cljgo's process story (ADR 0089) is run-to-completion: `cljg.io/exec` and `sh`
start a subprocess, fully buffer its stdout/stderr, and return `{:out :err
:exit}`. That is the right shape for `git rev-parse` but wrong for the live-pipe
shape — `cat`, `less`, `ffmpeg`, a long-running child you feed a line at a time
and read incrementally. Likewise `cljg.net.http` (ADR 0087) does `io.ReadAll`
on every response body: fine for a JSON API, ruinous for a multi-gigabyte
download or a never-ending SSE/NDJSON stream.

Both gaps are the same missing primitive: a **streaming handle** over a Go
`io.Reader`/`io.Writer` that a cljgo program can pull from / push to in constant
memory. The open question (spike s56) was the *shape* of that handle — core.async
channels? a new protocol? — and how it plugs into the existing reduce/transduce
library without a parallel universe of stream combinators.

## Decision

**1. One stream abstraction — `cljg.stream` — reused by every call site.**
Not core.async. A **readable** stream wraps a Go `io.Reader` and is **Seqable**:
`(reduce f init readable)`, `(into [] readable)`, `(doseq [c readable] …)` and
every transducer walk it a chunk at a time, and `take`/`reduced` short-circuit
the underlying read. Element = a `[]byte` chunk (default 64 KiB); `st/lines`
gives a lazy seq of line strings instead. It also carries the direct verbs
`read-line` / `read-bytes` / `read-all` / `close`. A **writable** stream wraps a
Go `io.Writer` with buffered `write` (flushed each call) + `close` (flush + EOF
to the peer). The Go half (`pkg/bri/stream.go`) is thin shims over stdlib
`bufio`/`io`; the surface is portable Clojure (`core/cljg/stream.cljg`).

Making the readable handle **Seqable** (not a new `IReduceInit` — cljgo's native
`reduce` is seq-driven, `pkg/corelib/hotpath_builtins.go`) is THE decision: the
whole reduce/transduce/into library applies to a live stream with zero new
machinery, the same bet `pkg/lang`'s `LongRange`/`LazySeq` already make. The
seq bottoms out on a `lang.NewLazySeq` closure that reads one chunk per node —
constant memory, single-pass, single-consumer (like the JVM's `line-seq` over a
`Reader`).

**2. `cljg.process/spawn` — streaming subprocess.** `(spawn ["cat"])` returns a
live handle `{:in <writable> :out <readable> :err <readable> :wait (fn []→exit)
:kill (fn [])}`, backed by `exec.Cmd`'s `StdinPipe`/`StdoutPipe`/`StderrPipe`
(`pkg/bri/proc_spawn.go`), the pipes wrapped as the same `cljg.stream` handles.
`:wait` blocks and returns the exit code (a non-zero exit is a normal value, not
a throw); `:kill` force-terminates.

**3. `cljg.net.http` `:as :stream`.** `(http/request {… :as :stream})` returns
the response with `:body` as a `cljg.stream` readable over the LIVE
`resp.Body` — not read, not closed here; the caller closes it. The buffered
string body stays the default (unchanged); streaming is strictly opt-in. A
streaming request sets no client timeout (a download must not be killed
mid-body) and is not retried on 5xx (its body handle is already live).

**4. `exec`/`sh`/`sh!` stay in `cljg.io`.** ADR 0101 makes `cljg.process` the
home for the *streaming* capability. Folding `cljg.io`'s run-to-completion
`exec`/`sh`/`sh!` into `cljg.process` is a clean future consolidation but would
churn ADR 0089's documented surface and its test suite for no behavior change —
**deferred**, not half-moved (per the owner's no-half-move rule).

**5. Lazy + opt-in, both harnesses.** `cljg.stream` and `cljg.process` are two
new `bri.Specs()` rows placed LAST (so earlier gensym numbering is stable),
non-OptIn (bufio/io/os/exec are stdlib — nothing heavy to isolate). They load on
first `require` in the interpreter (`pkg/briloader`, automatic) and opt-in-LINK
in AOT via the regenerated `pkg/briaot` twins (`go generate ./pkg/briaot`). A
binary that never requires them links zero bytes of them. Never a boot source.

## Consequences

- One streaming vocabulary (`reduce`/`lines`/`chunks`/`write`/`close`) covers
  subprocess pipes and HTTP bodies alike; any future `io.Reader` source (file
  streams, sockets, decompressors) reuses it by handing back a `cljg.stream`
  handle.
- `read-line` in `cljg.stream` and `close` do not shadow `clojure.core`
  globally: `read-line` is `:exclude`d from the refer (precedence principle,
  same as `cljg.net.http/get`).
- Single-consumer / single-pass is documented, not enforced — a stream reduced
  from two goroutines is undefined (same as the JVM `line-seq`).
- Conformance: cljgo host behavior with no JVM analog, so expectations are
  cljgo's own frozen output — interpreter suite `pkg/bri/stream_test.go` (spawn
  echo-a-line, bidirectional stream, kill, reduce/into/take over a readable, http
  `:as :stream`) and the compiled-parity smoke test
  `cmd/cljgo/stream_compiled_test.go`.

## Alternatives rejected

- **core.async channels as the stream type** — heavier (a go-block per stream),
  and it would NOT compose with `reduce`/transducers/`into` without adapters. A
  Seqable reader gets all of that free.
- **A new `IStream` protocol** — unnecessary; `Seqable` + a handful of verbs is
  the whole surface, and it rides the existing seq library.
- **Auto-streaming HTTP** — would break the ubiquitous buffered-string API and
  leak open bodies. Streaming stays opt-in (`:as :stream`).
