# ADR 0101 — `cljg.*` native stdlib: leaf layout + four capability gaps

Date: 2026-07-27 · Status: **proposed** (owner-directed, 2026-07-27). Refines the
`cljg.*` tier of **ADR 0085** (namespace taxonomy) with three new leaves and fills
four concrete, source-verified gaps. Rides the lazy + opt-in-linked doctrine
locked in by ADR 0096 decision-2 mandate 5 (available by default, zero cost when
unused, interpreter boot untouched — verified live 2026-07-27).

## Context

Four gaps in cljgo's own host stdlib, each verified against source (not memory):

| gap | evidence | Go closes it with |
|---|---|---|
| **No public env read** | `-getenv` exists only as a bri-internal private shim (`pkg/bri/auth.go:72`, `http.go:97`); `cljg.os` only *sets* a child's `:env` (`core/cljg/os.cljg:87`) | `os.Getenv` / `os.Environ` |
| **No streaming subprocess** | `io/exec`/`io/sh` → `-proc-exec` (`pkg/bri/io_proc.go:54`) is run-to-completion, buffered `{:out :err :exit}` | `exec.Cmd.StdinPipe`/`StdoutPipe` |
| **No streaming HTTP response** | `pkg/bri/net_http.go:63-64`: `defer resp.Body.Close()` + `io.ReadAll` — whole body buffered then closed | streamed `resp.Body` |
| **No public monotonic clock** | `-nano-time` is `defPrivate` (`pkg/corelib/macro_support_builtins.go:25`) | `time.Since` on a fixed instant |

## Decision

### 1. Leaf layout (refines ADR 0085's `cljg.*` bundles)

ADR 0085 sketched env/signals under `cljg.os` and subprocess under
`cljg.io.process`. The owner (2026-07-27) split them into dedicated leaves, which
read cleaner than overloading `cljg.os`/`cljg.io`:

- **`cljg.system`** (new) — process/runtime environment: `getenv`, `environ`
  (the full map), `exit`, `args`, and later `pid`/`hostname`/`arch`/`os-name`.
  (Supersedes ADR 0085's placement of env under `cljg.os`; `cljg.os` keeps
  signals/service/cron/ipc/notify/clipboard.)
- **`cljg.process`** (new) — subprocess as a first-class handle: the existing
  run-to-completion `exec`/`sh` **plus** streaming `spawn` (below). (Supersedes
  ADR 0085's `cljg.io.process` leaf; a process is not a filesystem concern.)
- **`cljg.date`** (new) — time: public `nano-time` (monotonic), `now`
  (epoch-ms/instant), `since`. (New leaf; not in 0085.)
- **`cljg.net.http`** (existing) — gains a streaming-response mode.

### 2. The four capabilities

1. **`cljg.system/getenv` + `cljg.system/environ`.** `getenv` reads one var
   (`nil` if unset, or a supplied default); `environ` returns the whole
   environment as an immutable map. Backed by `os.Getenv`/`os.Environ` — expose
   the existing internal shim as a public `cljg.system` var; **no secret value is
   ever logged or baked** (values flow through only at the call site).
2. **`cljg.process/spawn` — streaming subprocess.** Returns a process **handle**:
   `{:in <writable-stream> :out <readable-stream> :err <readable-stream> :wait
   (fn [] exit) :kill (fn [])}`. `:in` wraps `StdinPipe` (a writable stream —
   `write`/`close`); `:out`/`:err` wrap `StdoutPipe`/`StderrPipe` (readable
   streams). The run-to-completion `exec`/`sh` stay as the convenience layer over
   `spawn`.
3. **`cljg.net.http` streaming response.** A `:as :stream` request option returns
   the response with `:body` as a **readable stream** (over `resp.Body`, closed
   on drain/`close`) instead of `io.ReadAll`-buffering. The buffered mode stays
   the default (unchanged behavior); streaming is opt-in per request.
4. **`cljg.date/nano-time`.** Public monotonic nanoseconds since process start —
   promote the private `-nano-time` to a public `cljg.date` var. Plus `now`
   (wall-clock epoch-ms) and `since` (elapsed ms/ns between two `nano-time`s).

### 3. The stream abstraction (owner fork, resolved to reducible-handle)

Streams (subprocess `:in`/`:out`/`:err`, the HTTP `:as :stream` body) are a
small **readable/writable handle**, consistent with the s40 `io/byte-chunks`
reducible cljgo already ships for files — **not** core.async channels by default:

- **readable**: `reduce`-able over chunks, plus `read-line`/`read-bytes`/`close`.
- **writable**: `write`/`close`.
- A core.async channel adapter can wrap either, for code that wants async — but
  the base abstraction does not force async onto a simple streaming read.

One stream type, four call sites. (If the owner prefers channels as the base, that
flips the API — noted as the one reversible fork.)

### 4. Performance + laziness (ADR 0096 mandates, inherited)

Every leaf here is Go-host-backed where it touches the OS/network, lazy (loaded
on `require`, **not** a boot source), and opt-in-linked in AOT (`genbri`/`briaot`
machinery — a program that never requires `cljg.process` pays zero bytes). Hot
paths (stream copy, env map build) are Go, not interpreted Clojure — the
performance mandate holds.

## Consequences

- cljgo gains env, streaming subprocess, streaming HTTP, and a public clock — the
  everyday holes a real script hits — under clean dedicated leaves.
- **Refines ADR 0085** (env `cljg.os`→`cljg.system`, subprocess
  `cljg.io.process`→`cljg.process`, new `cljg.date`); 0085's three-umbrella
  doctrine is unchanged.
- The reducible stream handle becomes a reusable cljgo primitive (files,
  processes, HTTP all share it) — worth its own small spec.
- **Out of scope, deliberately:** `cljg.http` server (apache/nginx models),
  `cljg.grpc`, `cljg.socket` — larger libraries, same doctrine, sequenced later.

## Spikes

| spike | question | status |
|---|---|---|
| **s56** | Does the reducible stream-handle abstraction cleanly back subprocess pipes AND the HTTP `:as :stream` body over the existing s40 reducible, with correct backpressure/close? | pending |

Env read, `nano-time`, and `environ` are near-trivial exposures of existing
internal shims and do not need a spike; the streaming handle (s56) is the one real
design risk. Implementation follows via `/opsx:propose` once s56 closes.
