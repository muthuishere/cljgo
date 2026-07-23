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

## Addendum (2026-07-23) — reader conditionals are NOT gated to `.cljc`

Systematic verification against the official Reader Conditionals guide
surfaced the one place cljgo deliberately diverges from the JVM here: on
the JVM, `#?`/`#?@` in a plain `.clj` file is a reader error
("Conditional read not allowed"); conditionals are permitted only in
`.cljc` files (and via `read-string` with `{:read-cond :allow}`).

**cljgo processes reader conditionals in ALL file and REPL reading,
regardless of source extension** (`.clj`, `.cljg`, `.cljgo`, `.cljc`).
This was already the shipped ADR 0036/0050 behavior; it is now RATIFIED
as a deliberate divergence rather than tightened to JVM parity, because:

1. The existing conformance suite itself uses `#?` in `.clj` files
   (`conformance/tests/reader-conditionals.clj` and others) — the
   harness only globs `tests/*.clj`, so `.cljc`-only enforcement would
   break the suite's ability to freeze conditional semantics at all.
2. cljgo has four source extensions, three of them host-specific;
   restricting conditionals to `.cljc` buys no safety on a
   single-host-per-binary platform.
3. Enforcement would break any existing cljgo code using `#?` in
   `.clj`/`.cljg` — a compatibility cost with no offsetting benefit.

`clojure.core/read-string`, by contrast, follows the JVM opts protocol
EXACTLY (verified against clojure 1.12.5, frozen in
`conformance/tests/read-string-read-cond.clj`): a bare `read-string`
refuses conditionals ("Conditional read not allowed", diagnostic R1011),
`{:read-cond :allow}` selects, `{:features #{…}}` adds selectable
features on top of the always-present platform feature `:cljgo`
(mirroring the JVM's always-present `:clj`), and `{:read-cond
:preserve}` reads the conditional as a `ReaderConditional` data value
with tagged literals inside preserved (closing ADR 0050's deferred
reader-wiring follow-up). Top-level `#?@` splicing is rejected in file
reading and under `:allow` ("Reader conditional splicing not allowed at
the top level.", diagnostic R1010) but is one whole data value under
`:preserve`, as on the JVM.
