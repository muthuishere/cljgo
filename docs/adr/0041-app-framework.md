# ADR 0041 — keel: a batteries-included application framework, library style
Date: 2026-07-17 · Status: proposed (owner mandate 2026-07-17; evidence: spike S20)

## Context

Clojure's historical weakness is that anything past the language is
assembly-required: pick a router, pick a lifecycle library, pick a SQL
wrapper, wire them yourself. The JVM removed the pain enough (Java libs
everywhere) and the culture removed the will (libraries-over-frameworks
doctrine, the Component/mount/Integrant wars) that no "Rails of Clojure"
ever converged — the classic essays agree the missing 20% was curation
and conventions, not code. Owner mandate (2026-07-17): cljgo SUPPLIES
this — the capability set of Spring Boot, the simplicity of
Rails/Elixir, **library style**: you call it, it never calls you; no
classpath scanning, no annotation magic; Clojure style, not Go style;
simplicity as the core value. People should choose cljgo BECAUSE of it.

cljgo's substrate already made the hard calls a framework needs: any Go
module with zero bindings (require-go, design/05), Result/Option +
`let?` (ADR 0014), real goroutines/channels (design/05 §4), live vars at
1.7ns/deref (ADR 0004), single static binaries, build.cljgo (ADR 0021).
Spike S20 demonstrated the risky UX claims against the real runtime:
handlers behind vars re-`def`ed live on a running server; routes as
plain data mounted on Go 1.22+ ServeMux (no router engine); EDN + env
config; goroutine workers with a one-fn persistence seam. Liveness
costs 1.6× native Go interpreted — and AOT emission closes most of
that. Three adversarial DHH-persona review rounds (S20 reviews/)
reshaped the spec: round 1's central finding — a framework without a
generator, guides, and a visible first page is "a very good library
collection", the exact failure being diagnosed — produced tier 0;
round 2 hardened the golden page itself (casts on day one, default-on
security middleware, no I/O at namespace load, AI out of request
handlers, a documented error table); round 3 made the page pass its
own spec (workers start in -main, visible :drain wiring, CSRF posture
with html/form, http/render as the Result bridge, http/param!), gave
the beginner a zero-install dev database (embedded Postgres), added
the names doctrine and the single-binary deployment requirement,
re-sorted the tiers, and cut AI out of this change's scope.

## Decision

1. **Name: `keel`** — the spine you build the ship on. Namespaces
   `keel.http`, `keel.html`, `keel.config`, `keel.db`, `keel.jobs`,
   `keel.cache`, `keel.ai` — NOT `cljgo.*` (precedence principle: the
   language's namespace stays the language's).
2. **Tier 0 — the scaffold carries the conventions.** `cljgo new myapp`
   generates the blessed layout (`src/app/main.cljg` — the golden
   page's current-tier edition; **the generator's output IS the golden
   page**, trimmed only of unshipped tiers), `conf.edn` (+ optional
   `conf.schema.edn`), `migrations/`, `public/` with a real
   stylesheet, `test/` with one passing test, `build.cljgo`; `cljgo
   dev` boots it (provisions the embedded-Postgres dev database when
   `APP_DB_URL` is unset — zero install, dev/prod parity; applies
   migrations; server; nREPL attached; warns on non-var handlers;
   db verbs appear at T2 — every T0 verb has a T0 implementation).
   `cljgo new --with-auth` copies a complete
   session-based password auth implementation into the app (Phoenix
   phx.gen.auth model): the user owns the code, the framework owns
   the pattern. Convention over configuration WITHOUT inversion of
   control: conventions come from generation and guides, never from
   scanning or containers.
3. **Shipping shape: keel is a FRAMEWORK, shipped as plain
   libraries with the toolchain.** One install gives the batteries;
   everything is a plain namespace of plain fns; "library style"
   means no hidden call graph — nothing scanned, nothing ambient,
   adapters only invoke what the user handed them — not swappability
   theater. No container, no lifecycle protocol, no DI. Top-level
   defs construct VALUES with no I/O (config/load! reads a file;
   db/connect! validates now, dials on first use; jobs/queue is a
   registry value), so requiring an app namespace is side-effect-free
   and tests load it under `APP_PROFILE=test`; `-main` starts the
   world (`jobs/start!` there, not at the top level), `http/serve`
   pings dependencies before accepting traffic and drains the handles
   listed in `:drain` on SIGTERM — shutdown wiring is visible on the
   page, never an ambient registry. Bad config refuses to boot,
   loudly.
4. **One blessed way per pillar** (alternatives possible, not
   documented as equals):
   - **HTTP/middleware/routing (T1):** the Ring contract (handler:
     request-map → response-map; middleware: handler → handler);
     routes are data — `[["GET /users/{id}" #'handler] ...]` — on
     stdlib `net/http.ServeMux` patterns (method+path in one string
     is the stdlib's own syntax); no router of our own; `:params`
     bind as strings, `(http/param! req :id :int)` is the blessed
     visible coercion (failure → funnel-mapped 400). `#'var`
     handlers deref per request (live REPL web dev); `cljgo dev`
     warns on plain-fn handlers (silent non-liveness is a trap).
     **Omitting `:middleware` applies `(http/defaults)`** —
     access-log, recover, sessions (signed cookies), CSRF, JSON
     negotiation: the safe stack is what you didn't type — and the
     defaults are inspectable DATA (a vector: conj onto it, remove by
     name; `cljgo routes` prints the effective stack; dev warns when
     a custom stack lacks recover/csrf). CSRF gates session-bearing
     requests; `keel.html/form` mints the token; sessionless JSON
     requests pass (the documented API posture). The recover funnel's
     mapping is a shipped, documented DATA table (`{:http/bad-param
     400, :cast/invalid 422, :db/not-found 404, :db/constraint 409,
     else 500}`), overridable per app; Result values cross the http
     boundary only through `http/render` — a bare Result response
     fails loudly in dev, never silently laundered. Production
     timeouts and graceful SIGTERM drain are DEFAULTS (`:drain` lists
     what serve drains after in-flight requests). Static files:
     `(http/dir "public")`, one scaffold route — vanilla CSS from
     disk, no pipeline. Escape hatch: raw mux/server.
   - **HTML (T1):** `keel.html`, hiccup-style data→escaped-HTML fns —
     a function, not a template language; `html/form` mints the CSRF
     token (and is the deliberate outer boundary of the HTML surface:
     no layouts, no partials, no assets); no DSL, no asset pipeline
     (owner constraint honored) — but the first 15 minutes ends with
     a styled page. JSON equally first-class.
   - **Config (T1):** TWO layers — `conf.edn` → `APP_*` env — into
     one plain map; profiles are a `:profiles` section in conf.edn
     selected by `APP_PROFILE`, not a file family; env mapping is
     deterministic (`__` separates path segments, `_` joins words:
     `APP_DB__POOL_SIZE` → `[:db :pool-size]`); durations/sizes are
     numbers, not strings; the schema (`conf.schema.edn`) is
     optional, scaffold-generated, enforced when present (violations
     abort boot naming key and layer); `cljgo config` prints the
     resolved map with each key's winning layer; secrets env-only.
   - **Data (T2):** no ORM. pgx behind `keel.db` (query/one/insert/
     update/delete/tx; plain maps out; **SQL strings are THE blessed
     form** — data-SQL composers are unblessed libraries); a written
     NAMES DOCTRINE, conformance-tested: snake_case columns ↔
     kebab-case keywords, both directions, and nothing else renamed;
     Ecto-style casts returning Result, and **casting is day one**:
     the golden page casts request bodies before insert (mass
     assignment is structurally off the blessed path); `db/one!`
     throws `:db/not-found` (funnel 404); SQL-file migrations
     (UTC-timestamped, additive-only doctrine) via `cljgo migrate`.
     **Dev database: embedded Postgres**, provisioned by `cljgo dev`
     when `APP_DB_URL` is unset (zero install, real parity, data in
     `.dev/pg/`). **Deployment is ONE binary**: `cljgo build` embeds
     `public/` + `migrations/` (ADR 0021 comptime embed) and the
     generated `-main` answers `migrate` — `./myapp migrate &&
     ./myapp`. Tests run the Ecto-Sandbox model: under
     `APP_PROFILE=test` the pool wraps each test in a rolled-back
     transaction (same var, no with-redefs). Postgres blessed;
     database/sql is the hatch.
   - **Jobs (T3):** the Oban model — jobs are rows in YOUR Postgres,
     `(jobs/queue handlers)` is a pure registry VALUE (vars, derefed
     at dispatch — live like http handlers), `(jobs/start! pg q)`
     runs in `-main`, `enqueue!` takes the queue value and validates
     the job type (typos fail at the call site, not in a worker
     log); enqueue on a tx handle commits atomically with domain
     writes; workers are goroutines, LISTEN/NOTIFY + poll fallback;
     retries/backoff, unique jobs, per-type concurrency, cron on the
     same table. **Dev runs the real Postgres backend (parity);
     `:memory` (ADR 0040 core.async channels) is tests-only.** No
     broker, no sidecar. Shutdown drains via `http/serve`'s `:drain`.
   - **Cache (T3):** in-process TTL + singleflight behind
     `(cache/fetch c k f)`; same protocol over Redis when outgrown.
   - **AI (owner pillar; OUT of this change's scope):** `keel.ai` is
     a first-party satellite, **independently versioned**, specced in
     its own OpenSpec change after T1 boots a generated app
     (round 3: foundation before cathedral windows). Fixed positions
     carry: `(ai/generate model opts)` → Result; models resolved by
     step key from config (never vendor strings in app code);
     cross-provider fallback chains; native JSON modes; per-call
     timeout defaults; one interaction-log seam; blessed calling
     context is a JOB — docs never show inline AI in a request
     handler.
   - **App testing (T1+):** in-process http test client, `:memory`
     jobs drain-and-assert, per-test tx rollback fixtures — riding
     ADR 0012's first-class testing.
5. **Error model is ADR 0014 under one doctrine:** app handlers use
   `!` variants + the funnel — THE blessed surface (the chef picked);
   Result/`let?` serves domain/library code and crosses the http
   boundary only through the visible `http/render` bridge; a bare
   Result response is a loud dev-mode failure. One language-wide
   naming rule: plain = value/Result, `!` = throws.
6. **Guides are a gated deliverable:** the 15-minute tutorial, a guide
   per pillar, the auth chapter, and the production checklist ship
   with their tiers and gate releases like code (docs-as-product —
   the funded 20%). Framework diagnostics meet ADR 0015's bar.
7. **Process:** OpenSpec change `app-framework` carries tiered tasks
   T0 (scaffold/dev — no db verbs) → T1 (server/html/routes/
   middleware/config) → T2 (data + dev database + migrations +
   deployment) → T3 (jobs/cache). AI is a separate later change. The
   tiers topologically sort: every generated verb has a same-tier
   implementation, and each tier updates the generator. Each tier
   lands dual-harness conformance tests and perf budgets (ADR 0024).

## Consequences

- cljgo grows a product-defining surface: the under-a-page golden-path
  app (S20 VERDICT) is the demo, the doc, and a conformance artifact.
- The interpreter's stdlib seed registry must grow (net/http, io, os,
  time, context at minimum) or T1's REPL story stays AOT-only — S20's
  live demo needed a Go bridge precisely because of this gap; closing
  it is inside T1, and the honesty note stays in the VERDICT.
- keel rides require-go for pgx/redis/SDKs: framework quality now
  depends on interop breadth/perf — aligned incentives, one more
  reason interop stays priority #1.
- `cmd/cljgo` grows `new`, `dev`, `migrate`, `config` subcommands —
  the CLI is part of the framework's surface area and its docs bar.
- Not chosen: template DSLs/asset pipelines, Go-style APIs, any
  lifecycle/DI system, ORM, broker-backed queues, `cljgo.*` namespace,
  annotation/scanning magic of any kind, ambient global handles.
- Unresolved review objections are logged as owner questions in the
  S20 VERDICT.
