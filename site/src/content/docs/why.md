---
title: Why cljgo
description: What cljgo is, the five priorities it is built against, where it is headed — and an honest list of what it is not yet.
---

cljgo is Clojure hosted on Go: a compiler, written in Go, that AOT-emits plain
Go source — the ClojureScript model, with Go playing the part of JavaScript —
plus a tree-walk evaluator that is the REPL and the macro engine. The same
source runs interpreted at the prompt and compiles to a single static native
binary, with byte-identical output on both paths.

That last clause is enforced, not aspired to: a dual-harness conformance suite
runs every semantic test both interpreted and compiled on every commit, and a
REPL↔binary divergence is a release blocker.

## Five priorities, in order

These are not a wishlist — they are the design contract. Everything in cljgo is
judged against them, Clojure-first.

1. **Universal interop.** Any Go module is importable and callable with zero
   hand-written bindings — the Go ecosystem *is* the standard library. C
   reaches in via cgo modules and purego FFI.
2. **Full REPL-driven development.** The tree-walk evaluator is a real Clojure
   REPL: live re-`def`, `defmacro` at the prompt, namespaces, `eval`,
   `resolve`, and nREPL for CIDER/Calva.
3. **Faithful Clojure principles.** Persistent data structures with real
   structural sharing, transients, a numeric tower, macros as plain fns, seqs,
   and vars as the indirection layer. Clojure is first-class: nothing cljgo
   adds may shadow or change `clojure.core` semantics.
4. **High performance in both modes.** A feature, gated in CI like tests, not
   asserted. A perf regression is treated like a conformance failure.
5. **Single-file deployment.** `cljgo build` produces one static binary
   (5.3 MB for hello, ~5 ms startup) — no JVM, no runtime install. cgo
   projects are supported, not tolerated: cgo-based Go modules (sqlite
   drivers, sensors, GUI/audio) import like anything else.

## Priority #1 in practice: zero-binding interop

`require-go` pulls in any Go package and calls it directly — no wrappers, no
generated stubs. The Go toolchain is the classpath, and it runs identically
interpreted and compiled:

```clojure
(require-go '[strings])
(require-go '[strconv])

(println (strings/ToUpper "hello"))
(println (strconv/Atoi "123"))    ; (T, error) → [v err] — errors as values
(println (strconv/Atoi! "456"))   ; ! suffix unwraps, or throws
```

Members (`(.Method r …)`, `(.-Field r)`), constructors (`(pkg/T. {…})`), and
core.async over **real goroutines** (no CPS rewrite) round it out. Third-party
modules are one line in `build.cljgo` —
`(go-require app "github.com/gorilla/websocket" "v1.5.3")` — with no
hand-written bindings. See the [interop guide](/cljgo/guides/interop/).

## Where it stands

Working REPL **and** native compiler. Against the jank
[clojure-test-suite](https://github.com/jank-lang/clojure-test-suite)
(unmodified upstream): **238/242 files passing (98.3%)**, 242/242 vars
resolved, 0 failures — the 4 errors are missing `:cljgo` reader-conditional
branches in the suite itself, not broken semantics. Run `cljgo suite` to
reproduce. Details on the [compatibility page](/cljgo/reference/compatibility/)
and [benchmarks](/cljgo/reference/benchmarks/).

## Where it's headed

**The Bun of Clojure** — one fast native binary, batteries included,
zero-config: Bun's ergonomics with Go's delivery and no runtime to distribute.
This is the direction (roadmap, not yet shipped): a data layer with pure-Go
SQLite as the zero-install default, a durable Postgres job queue plus
in-process cache, a curated Go-native stdlib, and Spring-Boot-style layered
config.

**The Zig model** — cljgo's "batteries" follow Zig's design, not
Leiningen's/deps.edn's: the build is a program (`build.cljgo` defines an
artifact DAG, not a data file), dependencies are code in that same file
(content-addressed, lockfile-pinned, one resolver for both the interpreted and
compiled legs), one library publishes to both the Go module ecosystem and
Clojars from one build description, and cross-compilation falls out of
emitting plain Go — pure-Go programs build for any OS/arch with no target
toolchain.

## What cljgo is NOT (yet)

Honest gaps, from the project's own status ledger:

- **`clojure.core` is not complete.** The suite score is 98.3% and climbing;
  the honest per-namespace ledger is `docs/fundamentals-audit-2026-07.md` in
  the repo. The satellite namespaces (`clojure.string`, `set`, `edn`, `walk`,
  `zip`, `data`, `repl`, `pprint`, `test`) are complete against the 1.12.5
  oracle; core itself is early and moving fast.
- **Not the fastest at everything.** Compiled cljgo wins the recursion and
  data-structure benchmark rows outright, but `reduce` and `transducers` are
  still honestly lost to babashka and let-go. The interpreter is a
  tree-walker and loses everywhere except against joker — that is what
  `cljgo build` is for.
- **The batteries are decisions, not code.** The Bun-direction items above
  (data layer, jobs/cache, curated stdlib, vault/i18n) are ratified ADRs and
  spikes, **not shipped**.
- **C FFI via purego is proposed** (ADR 0044, spiked), not landed. cgo-based
  Go modules work today; direct C FFI does not.
- **comptime** (Zig-style compile-time value execution) is on the roadmap,
  not implemented.
- **bri web apps don't AOT-compile yet.** The app framework's dev loop is
  `cljgo dev` / `cljgo test`; AOT compilation of bri apps lands with a later
  tier.

The full picture is on [status & roadmap](/cljgo/reference/roadmap/). Ready to
try it? [Install](/cljgo/install/), then the
[quickstart](/cljgo/quickstart/).
