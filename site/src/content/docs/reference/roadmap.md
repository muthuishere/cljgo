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
| bri security | ✅ | API-first security ([bri.core.security](/cljgo/bri/auth/), ADR 0069): pinned HS256 JWT, argon2id passwords, composable guards, rate-limit + auto-ban, CORS, audited decisions, Compojure-style router |
| bri AOT + Docker | ✅ | bri AOT-compiles to one static `CGO_ENABLED=0` binary, byte-identical to interpreted; `cljgo new --template web` ships a scratch-image [Dockerfile](/cljgo/guides/deploy/) (~15 MB) (ADR 0071) |
| bri T2 — data | ✅ | [bri.core.data](/cljgo/bri/db/) (ADR 0072): pure-Go SQLite (zero-install default) + Postgres (pgx), parametrized queries, data-shaped writers, transactions, forward-only migrations — one API, driver swap |
| resource generator | ✅ | [`cljgo generate resource`](/cljgo/guides/generate/) (ADR 0073): migration + model + handlers + routes + a green CRUD test, spliced into `main` at markers |
| bri.core.telemetry | ✅ | Opt-in [OpenTelemetry tracing](/cljgo/bri/otel/) (ADR 0074): server span per request, W3C trace-context, OTLP exporter — linked only when required |
| bri.core.data opt-in | ✅ | bri.core.data linking is opt-in (ADR 0076): the SQLite/pgx drivers link only when an app requires `bri.core.data`, so a db-less web binary drops ~8 MB — the same zero-cost mechanism as bri.core.telemetry |
| `cljgo dist` | ✅ | [Cross-compile to every platform](/cljgo/guides/compile/) in one command (ADR 0077): pure-Go/`CGO_ENABLED=0` means no cross-toolchain — `dist/` gets a native binary per OS/arch + `checksums.txt` |
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
- **C FFI via purego** (ADR 0044 — proposed, spike S21).
- **The opt-in battery catalog + template composition** (ADR 0075 — roadmap):
  a curated set of pure-Go `bri.*` batteries (cache, jobs, mail, client,
  validate, openapi, ws, storage, cron…) each linked only when required, plus
  `cljgo new --template web-api` and `--with otel,db,jobs`. The *shape* is
  ratified; batteries land one ADR at a time.

## Where it's headed — the "Bun of Clojure"

The direction — explicitly roadmap, not yet shipped: one fast native binary,
batteries included, zero-config — Bun's ergonomics with Go's delivery and no
runtime to distribute. Go is a near-perfect host for the list, so the
batteries stay native-fast and keep the single static binary:

- **Data** — [`bri.core.data`](/cljgo/bri/db/): pure-Go SQLite as the zero-install
  default DB, Postgres (pgx) for production, forward-only migrations. *(shipped,
  ADR 0072)*
- **Observability** — Prometheus metrics + structured logs + request-ids
  default-on, opt-in [OpenTelemetry](/cljgo/bri/otel/) tracing. *(shipped, ADR
  0074)*
- **Deploy** — AOT to one static binary in a ~15 MB scratch image. *(shipped,
  ADR 0071 — see [Deploy](/cljgo/guides/deploy/))*
- **Jobs & cache** — a durable Postgres queue (`FOR UPDATE SKIP LOCKED`) +
  in-process TTL/singleflight cache. *(ADR 0075 catalog — `bri.jobs` /
  `bri.cache`, not yet shipped)*
- **A curated Go-native stdlib** — mail, outbound http-client, websockets,
  object storage, cron, validation — opt-in `bri.*` batteries, each linked
  only when required (ADR 0075).
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
