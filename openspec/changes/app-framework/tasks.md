## 0. T0 — scaffold and dev loop (the 15 minutes)

- [ ] 0.1 `cljgo new <name>`: blessed layout (`src/app/main.cljg`,
      `src/app/`, `conf.edn` + minimal `conf.schema.edn`,
      `migrations/0001_users.sql`, `public/` with a real stylesheet,
      `test/` with one passing test, `build.cljgo`, `.gitignore`);
      generated files are plain code the user owns — no scanning, no
      hidden registry. The T0 `main.cljg` IS the golden page trimmed
      to shipped pillars: `config/load!` + routes + `html/page` + the
      static route + `-main`/`http/serve` — designed here, reviewed,
      byte-covered by tests; each later tier updates the generator in
      the same change (generator/page contract, spec). `--with-auth`
      copies the session-based password auth implementation (code +
      tests) into the app.
- [ ] 0.2 `cljgo dev`: applies pending migrations, starts the app,
      attaches nREPL (ADR 0031), watches nothing (the REPL is the
      reload story), warns loudly when a route/job handler is a plain
      fn (non-live). Ctrl-C = graceful drain. Docs: the 15-minute
      tutorial IS this flow, gated with this task.

## 1. T1 — server, html, routes, middleware, config

- [ ] 1.1 Seed-registry growth: net/http, io, os, time, context
      members the T1 surface needs, with the S17-style regen note;
      conformance tests for each new package (dual-harness where
      oracle-skippable). Gates green.
- [ ] 1.2 `keel.http`: `serve` (pings the pool before accepting;
      blocks; SIGTERM graceful drain with deadline; production
      timeouts DEFAULT ON; returns/accepts a stop handle for tests),
      routes-as-data → ServeMux compiler, `#'var` per-request deref
      (plain fns skip it; dev warns), request/response map contract
      (Ring shape, `:params` from pattern — strings, no hidden
      coercion), `group`, `dir` (static files),
      `created`/`ok`/`not-found`/`json` helpers, `health`. Escape
      hatch: `mux`/`server` accessors. Conformance: S20 prototype
      behaviors as frozen tests, incl. live-redef via nREPL.
- [ ] 1.3 Middleware: `defaults` (applied when :middleware omitted;
      replacement is wholesale), `access-log`, `recover` (THE error
      funnel: a shipped DATA table — cast/validation 422, not-found
      404, constraint 409, else 500 — overridable via :error-map;
      stray Result in a response = loud dev-mode 500), `json`
      (negotiated bodies); ordering tested.
- [ ] 1.4 Sessions (signed cookies), CSRF protection, secure-cookie
      helpers — code in keel.http, inside `(http/defaults)` so the
      safe stack is what you didn't type.
- [ ] 1.5 `keel.html`: hiccup-style data→escaped-HTML, `html/page`;
      XSS-safe by construction (escaping opt-out is explicit and
      ugly). No template DSL, no asset pipeline — CSS is a file under
      `public/`, served by `http/dir`.
- [ ] 1.6 `keel.config`: `load!` — TWO layers: conf.edn (with a
      `:profiles` section selected by APP_PROFILE) → APP_* env;
      optional conf.schema.edn (defaults/required/type/coerce),
      refuse-to-boot naming key and layer; `cljgo config` prints the
      resolved map with each key's winning layer; secrets-are-env
      doctrine documented.
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
      msg})`, `cast!` throwing — the DAY-ONE input gate (golden page
      casts before insert; undeclared keys dropped/rejected — mass
      assignment structurally off the path); `let?` composition test
      end-to-end (the day-two signup from the golden page).
- [ ] 2.3 Migrations: `cljgo migrate [new|up|status]`, SQL files,
      UTC-timestamp names, applied-table; additive-only doctrine doc.
- [ ] 2.4 Per-test tx rollback fixtures. Conformance vs a real
      Postgres (compose harness), skipped under -short; golden-path
      db portion dual-harness. Data guide gates the tier.

## 3. T3 — jobs + cache

- [ ] 3.1 `keel.jobs`: Postgres backend — jobs table (state-of-
      record), transactional `enqueue!` (rides caller's tx handle;
      validates the job type against registered handlers — typos fail
      at the call site), goroutine workers, LISTEN/NOTIFY + poll
      fallback, retries/backoff, unique jobs, per-type concurrency,
      cron rows; handler map values are VARS derefed at dispatch
      (live like http); SIGTERM drains in-flight jobs.
- [ ] 3.2 `:memory` backend: same API on core.async channels per
      ADR 0040 (the S20 seam), used by the scaffold in dev and by
      tests (drain-and-assert helper).
- [ ] 3.3 `keel.cache`: `local` (TTL + singleflight), `fetch`/`put`/
      `evict`, constructor-enforced namespace; `redis` impl of the
      same protocol (rueidis via require-go). Stampede test (N
      concurrent fetches, one fill). Jobs + cache guides gate the
      tier; production checklist (drain, pool sizing, timeouts)
      lands here.

## 4. T4 — AI providers

- [ ] 4.1 `keel.ai` (first-party satellite, independently versioned):
      `generate`/`generate!` → Result/throwing, `model` (step-key
      resolution from config, cross-provider fallback chains), native
      JSON mode flags, per-call timeout defaults, interaction-log
      seam fn; anthropic + openai + google via their Go SDKs
      (require-go); no vendor strings in app code (doc-tested
      example); docs show AI in jobs only, never inline in handlers.
- [ ] 4.2 Golden-path app complete: the full S20 VERDICT page runs
      `cljgo run` and as a compiled binary against real services
      (compose harness); becomes `examples/keel-app/` and the site
      demo. AI guide gates the tier. Final gates + full conformance +
      perf budgets green.
