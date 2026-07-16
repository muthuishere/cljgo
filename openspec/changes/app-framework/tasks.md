## 1. T1 — server, routes, middleware, config

- [ ] 1.1 Seed-registry growth: net/http, io, os, time, context members
      the T1 surface needs, with the S17-style regen note; conformance
      tests for each new package (dual-harness where oracle-skippable).
      Gates green.
- [ ] 1.2 `keel.http`: `serve` (returns stop fn), routes-as-data →
      ServeMux compiler, `#'var` per-request deref (plain fns skip it),
      request/response map contract (Ring shape, `:params` from
      pattern), `group`, `created`/`ok`/`not-found` helpers, `health`.
      Escape hatch: `mux`/`server` accessors. Conformance: S20
      prototype behaviors as frozen tests, incl. live-redef via nREPL.
- [ ] 1.3 Middleware: `access-log`, `recover` (one error funnel:
      renders (err e) and exceptions), `json` (negotiated bodies);
      composition is an explicit vector, order tested.
- [ ] 1.4 `keel.config`: `load!` (conf.edn → conf.<profile>.edn →
      APP_* env → map), declared schema (required/type/coerce),
      refuse-to-boot on violation; docs state the four layers and the
      secrets-are-env rule. Unit + conformance tests.
- [ ] 1.5 The 15-minute demo doc + `examples/keel-hello/`: golden-path
      T1 subset runs `cljgo run` and `cljgo build`, byte-identical
      output. Perf budget recorded (interpreted handler ≤ 2× native).

## 2. T2 — data layer

- [ ] 2.1 `keel.db`: `connect` (pgx pool via require-go), `query`/
      `one`/`insert`/`update`/`delete`/`tx` returning Result, plain
      maps out, SQL as string or HoneySQL-shaped data; `!` variants.
- [ ] 2.2 Casts: `(db/cast row schema)` → `(ok row)`/`(err {field msg})`,
      `let?` composition test end-to-end (signup path from the golden
      page).
- [ ] 2.3 Migrations: `cljgo migrate [new|up|status]`, SQL files,
      UTC-timestamp names, applied-table; additive-only doctrine doc.
- [ ] 2.4 Conformance vs a real Postgres (compose harness), skipped
      under -short; golden-path db portion dual-harness.

## 3. T3 — jobs + cache

- [ ] 3.1 `keel.jobs`: Postgres backend — jobs table (state-of-record),
      transactional `enqueue` (rides caller's tx handle), goroutine
      workers, LISTEN/NOTIFY + poll fallback, retries/backoff, unique
      jobs, per-type concurrency, cron rows; handlers sealed at
      `start`.
- [ ] 3.2 `:memory` backend: same API on chans (S20 seam), used by the
      dev golden path and the conformance suite.
- [ ] 3.3 `keel.cache`: `local` (TTL + singleflight), `fetch`/`put`/
      `evict`, constructor-enforced namespace; `redis` impl of the same
      protocol (rueidis via require-go). Stampede test (N concurrent
      fetches, one fill).

## 4. T4 — AI providers

- [ ] 4.1 `keel.ai`: `generate` → Result, `model` (step-key resolution
      from config, fallback chains cross-provider), native JSON mode
      flags, interaction-log seam fn; anthropic + openai + google via
      their Go SDKs (require-go), no vendor strings in app code
      (enforced by an example + doc test).
- [ ] 4.2 Golden-path app complete: the full S20 VERDICT page runs
      `cljgo run` and as a compiled binary against real services
      (compose harness); becomes `examples/keel-app/` and the site
      demo. Final gates + full conformance + perf budgets green.
