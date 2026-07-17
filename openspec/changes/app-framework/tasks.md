## 0. T0 — scaffold and dev loop (the 15 minutes; no db verbs)

- [x] 0.1 `cljgo new <name>`: blessed layout (`src/app/main.cljg`,
      `src/app/`, `conf.edn` + minimal `conf.schema.edn`, `public/`
      with a real stylesheet, `test/` with one passing test,
      `build.cljgo`, `.gitignore`); generated files are plain code
      the user owns — no scanning, no hidden registry. The T0
      `main.cljg` IS the golden page trimmed to shipped pillars:
      `config/load!` + routes + `html/page` + the static route +
      `-main`/`http/serve` — designed here, reviewed, byte-covered by
      tests; NO migrations dir, NO db calls until T2 ships them
      (every generated verb has a same-tier implementation). Each
      later tier updates the generator in the same change
      (generator/page contract, spec).
- [x] 0.2 `cljgo dev`: starts the app, attaches nREPL (ADR 0031),
      watches nothing (the REPL is the reload story), warns loudly
      when a route/job handler is a plain fn (non-live). Ctrl-C =
      graceful drain. Docs: the 15-minute tutorial IS this flow,
      gated with this task. (Dev-database provisioning and migration
      application join `cljgo dev` in T2.)
      *(Applied 2026-07-17: also grew `cljgo test` — the generated
      test needs a runner (test/ requires can't resolve src/ across
      roots), and `cljgo config` / `cljgo routes` per tasks 1.6/1.3.
      `--with-auth` is deferred to T2: the copied auth implementation
      needs keel.db + password hashing — every generated verb must
      have a same-tier implementation, and T1 has no db verbs.)*

- [x] 0.3 Templates are REAL FILES, embedded, CI-run: the generated app
      lives at `templates/web/` as runnable source (not Go string
      literals — a literal is never compiled, never linted, never run,
      and is unreviewable in a diff); `//go:embed all:web` keeps
      `cljgo new` offline/zero-install/version-matched (a first-run
      fetch is disqualifying in the first 15 minutes); the app name is a
      real default (`newapp`) that the generator renames in contents and
      path names — ONE substitution, so the template is runnable in
      place; `--template <name|path>` takes a built-in name or a local
      directory. `cmd/cljgo/keel_test.go` (generate → `cljgo test` →
      boot → curl landing page + /health + nREPL re-def) is the
      anti-rot gate; `cmd/cljgo/templates_test.go` the fast guards.
      *(Applied 2026-07-17. FOLLOW-UP: `--template <git-url>` — refused
      with an honest error today; wants a clone step (git binary or a
      Go client) plus a test story, and half-doing it is worse than the
      error.)*

## 1. T1 — server, html, routes, middleware, config

- [x] 1.1 Seed-registry growth: net/http, io, os, time, context
      members the T1 surface needs, with the S17-style regen note;
      conformance tests for each new package (dual-harness where
      oracle-skippable). Gates green. NOTHING below T1 proceeds until
      a generated app boots through `cljgo dev` (round 3 sequencing
      rule).
      *(Applied 2026-07-17 as a THIN GO SHIM instead: pkg/keel is
      keel.http's Go half — the routes→ServeMux adapter, server with
      default-on timeouts + SIGTERM drain, in-process test client,
      JSON/form/HMAC/env primitives — interned as :private vars into
      the keel namespaces, which load lazily via the lib-provider
      registry (pkg/eval/libload.go). This closes S20's honesty gap
      (a generated app now boots through `cljgo dev`, live re-def
      proven over the nREPL wire in cmd/cljgo/keel_test.go) WITHOUT
      reflect-seeding net/http wholesale; general-purpose seeding of
      net/http · io · os · time · context for user interop remains
      open and should land with its own conformance files.)*
- [x] 1.2 `keel.http`: `serve` (pings the pool before accepting;
      blocks; SIGTERM graceful drain with deadline, then drains the
      handles in `:drain`; production timeouts DEFAULT ON;
      returns/accepts a stop handle for tests), routes-as-data →
      ServeMux compiler, `#'var` per-request deref (plain fns skip
      it; dev warns), request/response map contract (Ring shape,
      `:params` strings, `param!` typed accessor → :http/bad-param),
      `group`, `dir` (static files),
      `created`/`ok`/`not-found`/`json` helpers, `render` (the
      Result bridge), `health`. Escape hatch: `mux`/`server`
      accessors. Conformance: S20 prototype behaviors as frozen
      tests, incl. live-redef via nREPL.
      *(Applied 2026-07-17. keel behaviors have no JVM oracle, so the
      frozen tests live as Go suites against the real interpreter —
      pkg/keel/keel_test.go (every spec scenario incl. live re-def on
      a running server) and cmd/cljgo/keel_test.go (new→dev→curl→
      nREPL-wire re-def, the T0/T1 exit transcript). Raw mux/server
      escape-hatch accessors are not yet exposed to Clojure code —
      the shim (pkg/keel/http.go) is the documented hatch for now.)*
- [x] 1.3 Middleware: `defaults` (applied when :middleware omitted;
      returns inspectable DATA — conj/remove-by-name; `cljgo routes`
      prints the effective stack; dev warns when a custom stack
      lacks recover/csrf), `access-log`, `recover` (THE error
      funnel: shipped DATA table — :http/bad-param 400,
      cast/validation 422, not-found 404, constraint 409, else 500 —
      overridable via :error-map; bare Result in a response = loud
      dev-mode 500), `json` (negotiated bodies); ordering tested.
- [x] 1.4 Sessions (signed cookies), CSRF protection (gates
      session-bearing requests; sessionless JSON passes — documented
      API posture), secure-cookie helpers — code in keel.http,
      inside `(http/defaults)` so the safe stack is what you didn't
      type.
- [x] 1.5 `keel.html`: hiccup-style data→escaped-HTML, `html/page`,
      `html/form` (mints the CSRF token — and is the deliberate
      outer boundary: no layouts, no partials); XSS-safe by
      construction (escaping opt-out is explicit and ugly). No
      template DSL, no asset pipeline — CSS is a file under
      `public/`, served by `http/dir`.
- [x] 1.6 `keel.config`: `load!` — TWO layers: conf.edn (with a
      `:profiles` section selected by APP_PROFILE) → APP_* env
      (deterministic mapping: `__` nests, `_` joins words);
      durations/sizes are numbers; optional conf.schema.edn
      (defaults/required/type/coerce), refuse-to-boot naming key and
      layer; `cljgo config` prints the resolved map with each key's
      winning layer; secrets-are-env doctrine documented.
- [x] 1.7 App-testing helpers: in-process http test client; the
      scaffold's generated test uses it (shown byte-for-byte in the
      guide). Per-pillar guides (http, html, config) land with this
      tier and gate it. Perf budget recorded (interpreted handler ≤
      2× native Go, CI-checked seam). Generator updated: T1 page
      edition.
      *(Applied 2026-07-17: http/request is the client; guides at
      docs/guides/keel-{tutorial,http,html,config}.md. The perf seam
      is TestInterpretedHandlerOverhead (pkg/keel): S20 measured
      1.6–1.7× at the adapter; the end-to-end HTTP ratio jitters on
      shared runners, so the gate defaults to 6× with
      CLJGO_KEEL_PERF_MAX per host — it exists to catch adapter
      regressions (re-mounting, reflection), which read 10×+.)*

## 2. T2 — data layer, dev database, migrations, deployment

- [ ] 2.1 `keel.db`: `connect!` (pgx pool via require-go, sane pool
      sizing + timeouts default, validates-now/dials-on-first-use —
      the no-I/O-at-load contract), `query`/`one`/`insert`/`update`/
      `delete`/`tx` returning Result, `!` variants throwing
      (`one!` → :db/not-found, funnel 404); plain maps out; SQL
      strings THE blessed input form. NAMES DOCTRINE implemented and
      conformance-tested: snake_case ↔ kebab-case both directions,
      nothing else renamed.
- [ ] 2.2 Casts: `(db/cast row schema)` → `(ok row)`/`(err {field
      msg})`, `cast!` throwing — the DAY-ONE input gate (golden page
      casts before insert; undeclared keys dropped/rejected — mass
      assignment structurally off the path); `let?` + `http/render`
      composition test end-to-end (the railway signup from the
      golden page).
- [ ] 2.3 Migrations: `cljgo migrate [new|up|status]`, SQL files,
      UTC-timestamp names, applied-table; additive-only doctrine
      doc. `cljgo dev` gains migration application.
- [ ] 2.4 Dev database: embedded Postgres (require-go module, data
      under `.dev/pg/`) provisioned by `cljgo dev` when APP_DB_URL
      is unset — zero install, dev/prod parity. Documented
      alternative: point APP_DB_URL at your own server.
- [ ] 2.5 Deployment: `cljgo build` embeds `public/` + `migrations/`
      (ADR 0021 comptime embed); generated `-main` answers
      `migrate`; clean-host scenario tested (binary + env only).
      Deployment guide gates the tier.
- [ ] 2.6 Test sandbox: under APP_PROFILE=test the pool wraps each
      test in a rolled-back transaction (Ecto-Sandbox model, same
      pool var, no with-redefs); per-test fixture + generated db
      test. Conformance vs the embedded Postgres, skipped under
      -short; golden-path db portion dual-harness. Data guide
      (incl. names doctrine) gates the tier. Generator updated: T2
      page edition (schema, cast, db routes, migrate arm in -main).

## 3. T3 — jobs + cache

- [ ] 3.1 `keel.jobs`: `queue` (pure registry VALUE; handler values
      are vars, derefed at dispatch — live like http), `start!`
      (called in -main; goroutine workers, LISTEN/NOTIFY + poll
      fallback; returns a drainable handle for `:drain`),
      transactional `enqueue!` (takes tx + queue; validates the job
      type against the registry — typos fail at the call site),
      retries/backoff, unique jobs, per-type concurrency, cron rows.
      Jobs table = state-of-record in the app's Postgres.
- [ ] 3.2 `:memory` backend: same API on core.async channels per
      ADR 0040 (the S20 seam) — TESTS ONLY (dev runs the real
      Postgres backend on the embedded dev db: parity);
      drain-and-assert helper.
- [ ] 3.3 `keel.cache`: `local` (TTL in seconds + singleflight),
      `fetch`/`put`/`evict`, constructor-enforced namespace; `redis`
      impl of the same protocol (rueidis via require-go). Stampede
      test (N concurrent fetches, one fill). Jobs + cache guides
      gate the tier; production checklist (drain, pool sizing,
      timeouts) lands here. Generator updated: full golden page —
      generator and page now byte-identical; the complete app
      becomes `examples/keel-app/` + the site demo; full gates +
      conformance + perf budgets green.

## Out of scope (sequenced later, per round 3)

`keel.ai` — first-party, independently versioned satellite; its own
OpenSpec change AFTER T1 boots a generated app. Positions fixed in
ADR 0041 (step-key models, Result surface, fallbacks, log seam,
blessed context = a job).
