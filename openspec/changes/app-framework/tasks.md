## 0. T0 — scaffold and dev loop (the 15 minutes)

- [ ] 0.1 `cljgo new <name>`: blessed layout (`src/app/main.cljg`
      golden-path subset, `src/app/`, `conf.edn` + `conf.schema.edn`,
      `migrations/0001_users.sql`, `test/` with one passing test,
      `build.cljgo`, `.gitignore`); generated files are plain code the
      user owns — no scanning, no hidden registry. `--with-auth`
      copies the session-based password auth implementation (code +
      tests) into the app. Golden output covered by tests.
- [ ] 0.2 `cljgo dev`: applies pending migrations, starts the app,
      attaches nREPL (ADR 0031), watches nothing (the REPL is the
      reload story). Ctrl-C = graceful drain. Docs: the 15-minute
      tutorial IS this flow, gated with this task.

## 1. T1 — server, html, routes, middleware, config

- [ ] 1.1 Seed-registry growth: net/http, io, os, time, context
      members the T1 surface needs, with the S17-style regen note;
      conformance tests for each new package (dual-harness where
      oracle-skippable). Gates green.
- [ ] 1.2 `keel.http`: `serve` (blocks; SIGTERM graceful drain with
      deadline; production timeouts DEFAULT ON; returns/accepts a stop
      handle for tests), routes-as-data → ServeMux compiler, `#'var`
      per-request deref (plain fns skip it), request/response map
      contract (Ring shape, `:params` from pattern), `group`,
      `created`/`ok`/`not-found`/`json` helpers, `health`. Escape
      hatch: `mux`/`server` accessors. Conformance: S20 prototype
      behaviors as frozen tests, incl. live-redef via nREPL.
- [ ] 1.3 Middleware: `access-log`, `recover` (THE error funnel: maps
      exceptions and stray `(err e)` to responses — constraint
      violation → 422 — one place), `json` (negotiated bodies);
      composition is an explicit vector, order tested.
- [ ] 1.4 Sessions (signed cookies), CSRF protection, secure-cookie
      helpers — code in keel.http, on by default in the scaffold.
- [ ] 1.5 `keel.html`: hiccup-style data→escaped-HTML, `html/page`;
      XSS-safe by construction (escaping opt-out is explicit and
      ugly). No template DSL, no asset pipeline.
- [ ] 1.6 `keel.config`: `load!` (schema defaults → conf.edn →
      conf.<profile>.edn → APP_* env → map), schema in
      conf.schema.edn (required/type/coerce), refuse-to-boot naming
      key and layer; `cljgo config` prints the resolved map with each
      key's winning layer; secrets-are-env doctrine documented.
- [ ] 1.7 App-testing helpers: in-process http test client; the
      scaffold's generated test uses it. Per-pillar guide (http,
      html, config) lands with this tier and gates it. Perf budget
      recorded (interpreted handler ≤ 2× native Go, CI-checked seam).

## 2. T2 — data layer

- [ ] 2.1 `keel.db`: `connect!` (pgx pool via require-go, sane pool
      sizing + timeouts default), `query`/`one`/`insert`/`update`/
      `delete`/`tx` returning Result, `!` variants throwing; plain
      maps out; SQL strings THE blessed input form.
- [ ] 2.2 Casts: `(db/cast row schema)` → `(ok row)`/`(err {field
      msg})`; `let?` composition test end-to-end (the day-two signup
      from the golden page).
- [ ] 2.3 Migrations: `cljgo migrate [new|up|status]`, SQL files,
      UTC-timestamp names, applied-table; additive-only doctrine doc.
- [ ] 2.4 Per-test tx rollback fixtures. Conformance vs a real
      Postgres (compose harness), skipped under -short; golden-path
      db portion dual-harness. Data guide gates the tier.

## 3. T3 — jobs + cache

- [ ] 3.1 `keel.jobs`: Postgres backend — jobs table (state-of-
      record), transactional `enqueue!` (rides caller's tx handle),
      goroutine workers, LISTEN/NOTIFY + poll fallback, retries/
      backoff, unique jobs, per-type concurrency, cron rows; handler
      map values are VARS derefed at dispatch (live like http);
      SIGTERM drains in-flight jobs.
- [ ] 3.2 `:memory` backend: same API on chans (S20 seam), used by
      the scaffold in dev and by tests (drain-and-assert helper).
- [ ] 3.3 `keel.cache`: `local` (TTL + singleflight), `fetch`/`put`/
      `evict`, constructor-enforced namespace; `redis` impl of the
      same protocol (rueidis via require-go). Stampede test (N
      concurrent fetches, one fill). Jobs + cache guides gate the
      tier; production checklist (drain, pool sizing, timeouts)
      lands here.

## 4. T4 — AI providers

- [ ] 4.1 `keel.ai`: `generate`/`generate!` → Result/throwing, `model`
      (step-key resolution from config, cross-provider fallback
      chains), native JSON mode flags, per-call timeout defaults,
      interaction-log seam fn; anthropic + openai + google via their
      Go SDKs (require-go); no vendor strings in app code (doc-tested
      example).
- [ ] 4.2 Golden-path app complete: the full S20 VERDICT page runs
      `cljgo run` and as a compiled binary against real services
      (compose harness); becomes `examples/keel-app/` and the site
      demo. AI guide gates the tier. Final gates + full conformance +
      perf budgets green.
