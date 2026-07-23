---
title: Status & roadmap
description: Where cljgo stands — milestones M0–M5 landed, what shipped in each, and the "Bun of Clojure" direction that is roadmap, not yet shipped.
---

cljgo today is a working REPL **and** a native compiler: the same source runs
interpreted at the prompt and AOT-compiles to a static Go binary, with
byte-identical output enforced by a dual-harness conformance suite on every
commit. Against the jank clojure-test-suite it measures **238/242 files
passing (98.3%)** — details in
[Compatibility](/cljgo/reference/compatibility/). Early, moving fast.

## Milestones landed

The original build-order roadmap
([`design/00-architecture.md`](https://github.com/muthuishere/cljgo/blob/main/design/00-architecture.md)
§6) ran M0 → M5; all of it has landed, plus several stages beyond it:

| Milestone | State | What landed |
|---|---|---|
| M0–M1 | ✅ | REPL: full syntax-quote reader, `loop*`/`recur`, dynamic vars, namespaces, macroexpansion, `defmacro` at the prompt, embedded `core.clj`, `clojure.test` |
| M2 | ✅ | `cljgo build` → native binary, <10 ms startup, fixed-arity calling convention |
| M3 | ✅ | Zero-ceremony Go interop, both modes — `require-go`, package fns/consts, members `(.Method r …)` / `(.-Field r)`, ctors, `(T,error)` → `[v err]`, `!` unwrap |
| M4 (core.async) | ✅ | Real `clojure.core.async` over goroutines — no CPS rewrite. 55 publics = every non-deprecated, non-internal var of JVM core.async 1.6.681 |
| M5 / AOT-core | ✅ | `clojure.core` AOT-compiled into the binary instead of evaluated at boot — compiled startup 28.9 → 6.5 ms at the time (5.0 ms today) |
| perf 07-23 | ✅ | The 2026-07-23 campaign (ADRs 0063–0067): emitted-vs-handwritten Go ~35× → ~5×, `fib` 975 → 24.7 ms, startup back to 5.0 ms, CI-gated |
| deps | ✅ | `(dep …)` in `build.cljgo`, content-addressed cache, committed `build.lock.edn`, one resolver feeding both legs |
| publish | ✅ | `cljgo publish go` (go-gettable module) + `publish clojars` (pure Clojure source), purity-gated at `file:line` |
| build.cljgo | ✅ | Zig-style build graph — `exe`/`install`/`run` + `go-require` for third-party Go modules, zero bindings |
| bri T0–T1 | ✅ | The app framework — `cljgo new`/`cljgo dev`, HTTP + hiccup HTML + routes/middleware, sessions + CSRF, layered config |
| core batches | ✅ | Numeric tower, transients, JVM-compatible hashing, `reify`, tagged literals/reader conditionals, richer error rendering, suite ratchet |
| Next | ◦ | See below |

Also shipped along the way: structured [diagnostics](/cljgo/reference/diagnostics/)
with `cljgo explain`, an nREPL server (Calva/CIDER connect), and real project
templates (`cljgo new` — lib/cli/web).

## What's next (planned, not shipped)

Per the README's "Next" row:

- **ADR 0067 follow-ups** — float64, multi-arity/variadic specialization,
  capturing-closure lift.
- **`reduce`/`transducers` vs babashka's core** — the two
  [benchmark](/cljgo/reference/benchmarks/) rows still lost.
- **App framework T2** (ADR 0041).
- **C FFI via purego** (ADR 0044 — proposed, spike S21).
- **The batteries direction** (ADRs 0056–0062, ratified on `feat/batteries` —
  decisions recorded, **not shipped**).

## Where it's headed — the "Bun of Clojure"

The direction — explicitly roadmap, not yet shipped: one fast native binary,
batteries included, zero-config — Bun's ergonomics with Go's delivery and no
runtime to distribute. Go is a near-perfect host for the list, so the
batteries stay native-fast and keep the single static binary:

- **Data** — `bri.db`: a pure-Go SQLite as the zero-install default DB,
  Postgres (pgx) for production, migrations (`cljgo migrate`). *(bri T2, spiked)*
- **Jobs & cache** — a durable Postgres queue (`FOR UPDATE SKIP LOCKED`) +
  in-process TTL/singleflight cache. *(bri T3, spiked)*
- **A curated Go-native stdlib** — secrets (OS keychain), streaming file I/O,
  crypto/hashing, http-client, websocket — there by default, no `require-go`
  ceremony.
- **Spring-Boot-style config** — one layered chain (defaults →
  `application.edn`/`.properties` → profiles → `APP_*` env → pluggable vault
  → overrides), plus i18n message bundles on the same infra.

## The Zig model

cljgo's "batteries" follow Zig's shape, not Leiningen's / deps.edn's
([`design/08-build-comptime-compat.md`](https://github.com/muthuishere/cljgo/blob/main/design/08-build-comptime-compat.md)):

- **`build.cljgo`** — the build is a program, not a data file (shipped).
- **Dependencies as code** — `(dep …)` in `build.cljgo`, content-addressed and
  lockfile-pinned, no `deps.edn` in either direction (shipped).
- **Publish both ways** — one pure-Clojure library reaches Go developers and
  JVM-Clojure developers, gated on purity at publish time (shipped).
- **comptime** — Zig-style compile-time value execution alongside macros:
  macros transform *syntax*, `comptime` computes *values* embedded as
  literals (planned).
- **Cross-compilation** — cljgo emits plain Go, so pure-Go + purego programs
  build for any OS/arch with no target toolchain.

Guardrail on everything above — the precedence principle: **Clojure is
first-class**. Nothing cljgo adds may shadow, rename, or change the semantics
of anything in `clojure.core` or the reader; when a new feature's natural name
collides with Clojure, the new feature renames.

Authority chain for how decisions get made:
[`docs/adr/`](https://github.com/muthuishere/cljgo/tree/main/docs/adr)
(decisions) ›
[`design/00-architecture.md`](https://github.com/muthuishere/cljgo/blob/main/design/00-architecture.md)
(contracts + roadmap) › `openspec/` (active change proposals).
