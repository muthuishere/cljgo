# ADR 0068 — `.cljc` joins the accepted source extensions; the REPL resolves namespaces against the cwd

Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23) · Refines
**ADR 0055** (extension set) and the resolver contract of **ADR 0042**.

## Context

A multi-host library targets cljgo through `.cljc` files gated with reader
conditionals (`#?(:clj … :cljs … :cljgo …)`) — the exact pattern ADR 0036
ratified feature keys for, and the pattern the jank clojure-test-suite (ADR
0022) is written in. Three gaps broke that story in practice (found reproducing
a user question, 2026-07-23):

1. **`require` could not load a `.cljc` namespace.** `ResolveLibPath`
   (`pkg/eval/libload.go`) probed `.cljgo`/`.cljg`/`.clj` only — ADR 0055
   ratified those three and never mentioned `.cljc`. `cljgo run demo.cljc`
   worked (direct file read); `(require 'demo)` of the same file failed. A
   library consumable via `require` is the whole point of the multi-host
   claim.
2. **The interactive REPL could not `require` ANY cwd namespace.** The REPL
   leaves `*file*` at its root `NO_SOURCE_FILE`, and `ResolveLibPath`
   early-returned `""` when `*file*` was unset — before dependency roots
   (ADR 0052 §2) and `$CLJGO_PATH` were even consulted. JVM Clojure serves
   this case via the cwd on the classpath (`:paths ["."]`).
3. **The CLI's source-file test disagreed with the resolver.**
   `isSourceFile` (cmd/cljgo/main.go) accepted `.clj`/`.cljg` only — so
   `cljgo build demo.cljgo` / `demo.cljc` fell through to build-step
   dispatch, and `defaultBinaryName` stripped only `.clj` (a `demo.cljg`
   build would be named `demo.cljg`).

## Decision

1. **Source resolution accepts four extensions, most-specific-first:
   `.cljgo` > `.cljg` > `.clj` > `.cljc`.** `.cljc` is the portable
   multi-host fallback and ranks last, mirroring the JVM's own `.clj` >
   `.cljc` preference. Because `ResolveLibPath` is the single shared
   resolver (ADR 0053 invariant), interpreter and emitter inherit this
   identically — dual-mode parity by construction.
2. **When `*file*` provides no requiring-file context** (unset,
   `NO_SOURCE_FILE`, `NO_SOURCE_PATH`, or the interactive REPL), the
   resolver roots at the process cwd (`.`) instead of failing — the moral
   equivalent of the JVM's cwd-on-classpath. Dependency roots and
   `$CLJGO_PATH` still append after, unchanged in order.
3. **The CLI recognizes all four extensions as source files**, and derives
   binary names by stripping whichever accepted extension is present.
4. **The build file is unchanged** — `build.cljgo`/`build.cljg`/`build.clj`
   (ADR 0055 §2). A `.cljc` build file makes no sense; a build file is
   host-specific by nature.

## Consequences

- Purely additive: every name that resolved before still resolves to the
  same file; `.cljc` only wins when no more-specific extension exists.
- A multi-host library can now be consumed as-is: `(require 'lib)` in the
  REPL, from `cljgo run`, and inside an AOT build all find `lib.cljc` and
  read only its `:cljgo`/`:default` branches (ADR 0036).
- The REPL gains cwd-relative `require` — and, transitively, its first
  access to dependency roots and `$CLJGO_PATH` when no file context exists.
- Conformance: a dual-harness test freezes `.cljc` require + reader-
  conditional selection so REPL and compiled binaries cannot diverge here.
