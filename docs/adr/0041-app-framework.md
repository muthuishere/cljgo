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
reshaped the spec; round 1's central finding — a framework without a
generator, guides, and a visible first page is "a very good library
collection", the exact failure being diagnosed — produced tier 0.

## Decision

1. **Name: `keel`** — the spine you build the ship on. Namespaces
   `keel.http`, `keel.html`, `keel.config`, `keel.db`, `keel.jobs`,
   `keel.cache`, `keel.ai` — NOT `cljgo.*` (precedence principle: the
   language's namespace stays the language's).
2. **Tier 0 — the scaffold carries the conventions.** `cljgo new myapp`
   generates the blessed layout (`src/app/main.cljg` golden path,
   `conf.edn` + `conf.schema.edn`, `migrations/`, `test/` with one
   passing test, `build.cljgo`) and a rendered first page; `cljgo dev`
   boots it (migrations, server, nREPL attached). `cljgo new
   --with-auth` copies a complete session-based password auth
   implementation into the app (Phoenix phx.gen.auth model): the user
   owns the code, the framework owns the pattern. Convention over
   configuration WITHOUT inversion of control: conventions come from
   generation and guides, never from scanning or containers.
3. **Shipping shape: with the toolchain, as a library.** One install
   gives the batteries; everything is a plain namespace of plain fns.
   No container, no lifecycle protocol, no DI: `-main` calls
   constructors top to bottom and that IS the boot order; handles are
   ordinary values, never ambient globals. Bad config or failed
   construction refuses to boot, loudly.
4. **One blessed way per pillar** (alternatives possible, not
   documented as equals):
   - **HTTP/middleware/routing (T1):** the Ring contract (handler:
     request-map → response-map; middleware: handler → handler in an
     explicit vector); routes are data — `[["GET /users/{id}"
     #'handler] ...]` — on stdlib `net/http.ServeMux` patterns; no
     router of our own. `#'var` handlers deref per request (live REPL
     web dev); production timeouts and graceful SIGTERM drain are
     DEFAULTS. Sessions (signed cookies), CSRF, and secure-cookie
     helpers are code in keel.http. Escape hatch: raw mux/server.
   - **HTML (T1):** `keel.html`, hiccup-style data→escaped-HTML fns —
     a function, not a template language; no DSL, no asset pipeline
     (owner constraint honored) — but the first 15 minutes ends with a
     rendered page. JSON equally first-class.
   - **Config (T1):** `(config/load!)` = schema defaults → conf.edn →
     conf.<profile>.edn → `APP_*` env → one plain map; schema in
     `conf.schema.edn`; violations abort boot naming key and layer;
     `cljgo config` prints the resolved map with each key's winning
     layer; secrets env-only.
   - **Data (T2):** no ORM. pgx behind `keel.db` (query/one/insert/
     update/delete/tx; plain maps out; **SQL strings are THE blessed
     form** — data-SQL composers are unblessed libraries); Ecto-style
     casts returning Result; SQL-file migrations (UTC-timestamped,
     additive-only doctrine) via `cljgo migrate`. Postgres blessed;
     database/sql is the hatch.
   - **Jobs (T3):** the Oban model — jobs are rows in YOUR Postgres,
     enqueue commits atomically with domain writes (tx handle),
     workers are goroutines, LISTEN/NOTIFY + poll fallback; handler
     maps hold VARS, derefed at dispatch — live like http handlers;
     retries/backoff, unique jobs, per-type concurrency, cron on the
     same table; `:memory` backend with the identical API for dev.
     No broker, no sidecar. SIGTERM drains in-flight jobs.
   - **Cache (T3):** in-process TTL + singleflight behind
     `(cache/fetch c k f)`; same protocol over Redis when outgrown.
   - **AI (T4):** `(ai/generate model opts)` → Result; models resolved
     by step key from config (never vendor strings in app code);
     cross-provider fallback chains; native JSON modes; per-call
     timeout defaults; one interaction-log seam.
   - **App testing (T1+):** in-process http test client, `:memory`
     jobs drain-and-assert, per-test tx rollback fixtures — riding
     ADR 0012's first-class testing.
5. **Error model is ADR 0014 end-to-end, beginner-first:** the
   documented/default surface is `!` variants (unwrap-or-throw) plus
   ONE error funnel in the http adapter; plain fns return Result and
   `let?` is the documented day-two upgrade. One language-wide rule:
   plain = value/Result, `!` = throws.
6. **Guides are a gated deliverable:** the 15-minute tutorial, a guide
   per pillar, the auth chapter, and the production checklist ship
   with their tiers and gate releases like code (docs-as-product —
   the funded 20%). Framework diagnostics meet ADR 0015's bar.
7. **Process:** OpenSpec change `app-framework` carries tiered tasks
   T0 (scaffold/dev) → T1 (server/html/routes/middleware/config) →
   T2 (data) → T3 (jobs/cache) → T4 (AI). Each tier lands
   dual-harness conformance tests and perf budgets (ADR 0024).

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
