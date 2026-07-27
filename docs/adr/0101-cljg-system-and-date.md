# ADR 0101 — `cljg.system` + `cljg.date`: process/environment + time primitives

Date: 2026-07-27 · Status: accepted. Two leaf `cljg.*` stdlib namespaces
extending the tier ADR 0085 laid out (after ADR 0087 `cljg.net.http`, ADR 0088
`cljg.os`, ADR 0089 `cljg.io`). Small, pure-Go, non-OptIn.

## Context

Two capabilities every CLI / script reaches for are missing from what cljgo
ships as ergonomic Clojure:

- **Process + environment.** Reading a single env var, the whole environment,
  exiting with a status, and seeing the argument vector. `clojure.core` keeps
  `*command-line-args*` (the clojure.main contract) but nothing else — env
  reads today go through per-namespace private `-getenv` shims (auth/http/cli),
  never exposed as a public capability.
- **Time.** A monotonic nanosecond stopwatch and the wall clock. The `time`
  macro already rides a private `-nano-time` in `pkg/corelib` (`time.Since`
  over a fixed boot instant on Go's monotonic clock — the `System/nanoTime`
  analog), but that source is private to the macro; there is no public
  `now` / `nano-time` / elapsed helper.

Both are pure-Go over stdlib (`os`, `time`), `CGO_ENABLED=0`, cross-compilable,
with no heavy dependency — so both are **non-OptIn** and lazy+opt-in like every
`cljg.*` namespace (load on first `require` in the interpreter, opt-in-link in
AOT; a binary that never requires them pays zero bytes).

## Decision

### 1. `cljg.system` — process + environment

Go shims (`pkg/bri/cljg_system.go`, `installSystemShims`): `-getenv` (reuses the
shared `getenvShim`), `-environ`, `-exit`, `-args`. Ergonomic API is portable
Clojure (`core/cljg/system.cljg`):

```clojure
(require '[cljg.system :as sys])
(sys/getenv "HOME")          ; value, or nil when unset
(sys/getenv "PORT" "8080")   ; value, or the default when unset
(sys/environ)                ; {name value} — the whole environment, immutable
(sys/args)                   ; raw os.Args vector (element 0 = program path)
(sys/exit)   (sys/exit 1)    ; terminate the process (status 0 default)
```

`getenv`'s 1-arity matches `System/getenv` (nil when unset); the 2-arity default
is a cljgo convenience. **Security (owner doctrine): a shim never logs or bakes
an env VALUE — it only RETURNS it as data.** `environ` splits `os.Environ`'s
`K=V` on the first `=` (values may contain `=`) into an immutable map. It never
shadows `*command-line-args*` (the precedence principle) — `args` is the raw
process view alongside it.

### 2. `cljg.date` — time primitives

Go shims (`pkg/bri/cljg_date.go`, `installDateShims`): `-nano-time` (monotonic,
promoting the same `time.Since`-over-boot-instant technique the `time` macro's
private `-nano-time` uses — the private core builtin is untouched) and
`-now-millis` (wall clock). Ergonomic API (`core/cljg/date.cljg`):

```clojure
(require '[cljg.date :as date])
(date/now)                   ; wall-clock epoch millis (System/currentTimeMillis analog)
(date/nano-time)             ; monotonic nanos since process start (System/nanoTime analog)
(date/since t0)              ; elapsed nanos from a reading until now
(date/since t0 t1)           ; elapsed nanos between two readings
(date/since-ms t0 t1)        ; the same, in milliseconds (a double)
```

Only the DIFFERENCE of two `nano-time` readings is meaningful; the epoch is
arbitrary. `since`/`since-ms` are pure Clojure over `nano-time`.

### 3. Mechanism

Registered as two Spec rows placed LAST in `bri.Specs()` (so they do not shift
the gensym numbering of earlier-emitted namespaces), driven by the same
name-generic embedded-namespace registry `cljg.*` already rides (the `pkg/bri`
package name is a legacy of bri being the first tenant — ADR 0087 §1).
`go generate ./pkg/briaot` emits the AOT twins (`cljgsystem`, `cljgdate`).
Neither is added to `core.BootSources()`.

## Consequences

- Env reads become a first-class public capability without changing the private
  per-namespace `-getenv` shims that already exist.
- The `time` macro keeps its private `-nano-time`; `cljg.date` exposes an
  equivalent public one — no behavior change to the macro.
- Conformance: `cljg-system-getenv.clj` (getenv/environ semantics, frozen
  against cljgo output — `oracle: skip`, semantics verified against
  `System/getenv` at authoring time) and `cljg-date-monotonic.clj` (monotonic
  invariants, `oracle: skip` — no stable JVM byte-analog). Both run dual-harness
  (eval + compiled), enforcing REPL-vs-binary parity.
