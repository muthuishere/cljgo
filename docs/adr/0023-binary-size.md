# ADR 0023 — Binary size: strip by default now, AOT-core.clj structurally
Date: 2026-07-15 · Status: accepted (strip); proposed (AOT-core structural fix)

## Context
Measured 2026-07-15: the cljgo **tool** is 10.4MB and a **hello-world emitted
binary** is 6.6MB. Two independent causes:
1. **Tool (10.4MB):** dominated by `golang.org/x/tools/go/packages` +
   `go/types`/`go/parser` (the AOT interop signature resolver, spike S2) atop
   Go's static-runtime baseline. Comparable to let-go's 12MB tool.
2. **Emitted binary (6.6MB for a trivial program):** links the **entire
   interpreter** — `go tool nm` shows 381 `pkg/eval` symbols (analyzer, reader,
   all builtins) plus the embedded `core/*.cljg` sources — because `rt.Boot()`
   calls `eval.New()` at startup to load `core.clj` (design/04 §7). The Go
   linker's DCE keeps it all: `main → rt.Boot → eval.New` is a live edge, so a
   hello-world drags in the whole tree-walker. It does NOT link net/http/fips
   (0 syms) — the bloat is purely the interpreter + core sources.

## Decision
1. **Strip release artifacts by default (DONE).** `cljgo build` now runs
   `go build -trimpath -ldflags="-s -w"` — no DWARF, no symbol table, no
   absolute paths. Measured: hello 6.6MB → **4.4MB (−33%)**, identical behavior.
   A future `--debug` flag keeps symbols for profiling.
2. **The structural fix is AOT-compiling `core.clj` (proposed).** So emitted
   binaries link only `pkg/lang` (the runtime) + the emitted program, NOT
   `pkg/eval`/`pkg/analyzer`/`pkg/reader`. This is the design/04 §7 / M5 item
   and the same decoupling that removes the interpreter from binaries. Expected:
   emitted binaries approach the raw-Go static baseline (~2MB) once the
   `main → eval.New` edge is cut. This is the single biggest lever and the
   long-standing "rt → eval" coupling flagged since M3.1.
3. **Tool size is accepted for now.** 10.4MB for a full compiler+REPL is fine
   (go itself is ~60MB). If a REPL-only / no-AOT-interop build is ever wanted,
   `x/tools` can go behind a build tag or a subprocess resolver — deferred, low
   priority. Stripping the tool too (`-s -w`) is a cheap ~30% option for
   releases.

## Consequences
- Users get a 1/3 smaller binary today with no work.
- The real prize (2MB binaries, no interpreter) is bundled with AOT-core.clj —
  which also fixes REPL-boot cost (ADR 0019) and completes the "ClojureScript
  for Go" story (the emitted program is standalone Go, not a Go program that
  embeds a Clojure interpreter). Sequenced as an M5 OpenSpec change.
- Non-goal: dynamic linking, UPX, or other packaging tricks — a static,
  honestly-small Go binary is the target.
