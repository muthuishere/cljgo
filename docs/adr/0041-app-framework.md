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
costs 1.6× native Go interpreted — and AOT emission closes most of that.

## Decision

1. **Name: `keel`** — the spine you build the ship on; it holds true,
   it never steers. Namespaces `keel.http`, `keel.config`, `keel.db`,
   `keel.jobs`, `keel.cache`, `keel.ai` — NOT `cljgo.*` (precedence
   principle: the language's namespace stays the language's).
2. **Shipping shape: with the toolchain, as a library.** One install
   gives the batteries; everything is a plain namespace of plain fns.
   No container, no lifecycle protocol, no DI: `-main` calls
   constructors top to bottom and that IS the boot order. Failing
   construction (bad config, unreachable db when required) refuses to
   boot, loudly.
3. **One blessed way per pillar** (alternatives possible, not
   documented as equals):
   - **HTTP/middleware/routing (tier 1):** the Ring contract (handler:
     request-map → response-map; middleware: handler → handler in an
     explicit vector); routes are data — `[["GET /users/{id}" #'handler]
     ...]` — compiled onto stdlib `net/http.ServeMux` patterns.
     `#'var` handlers deref per request: REPL-driven live web dev.
     Escape hatch: the raw mux/server.
   - **Config (tier 1):** `(config/load!)` = conf.edn → profile file →
     `APP_*` env → one plain map; declared schema validated at load;
     four layers, not fourteen; secrets env-only.
   - **Data (tier 2):** no ORM. pgx behind `keel.db` (query/one/insert/
     tx, plain maps out, SQL as strings or data); Ecto-style casts
     returning Result composing with `let?`; SQL-file migrations
     (UTC-timestamped, additive-only doctrine) via `cljgo migrate`.
     Postgres is blessed; database/sql is the hatch.
   - **Jobs (tier 3):** the Oban model — jobs are rows in YOUR
     Postgres, enqueue is transactional with domain writes, workers are
     goroutines, LISTEN/NOTIFY wakeup, handlers sealed at
     `(jobs/start pg handlers)`; `:memory` backend, same API, for dev.
     No broker, no sidecar.
   - **Cache (tier 3):** in-process TTL + singleflight behind
     `(cache/fetch c k f)`; same protocol over Redis when outgrown.
   - **AI (tier 4):** `(ai/generate model opts)` → Result; models are
     config-resolved values by step key (never vendor strings in app
     code); cross-provider fallback chains; native JSON modes; one
     interaction-log seam.
   - **Auth:** a reference design (JWT HS256, additive middleware
     tiers), not a pillar.
4. **Error model is ADR 0014 end-to-end:** expected-failure fns return
   Result; `!` variants unwrap-or-throw (the language-wide convention);
   the http adapter renders stray errs/exceptions through one funnel.
5. **Process:** OpenSpec change `app-framework` carries tiered tasks
   T1 (server/routes/middleware/config) → T2 (data) → T3 (jobs/cache)
   → T4 (AI). Each tier lands dual-harness conformance tests and perf
   budgets like every other feature (ADR 0024 host-relative).

## Consequences

- cljgo grows a product-defining surface: the under-a-page golden-path
  app (S20 VERDICT) is the demo, the doc, and the conformance test.
- The interpreter's stdlib seed registry must grow (net/http, io, os,
  time, context at minimum) or T1's REPL story stays AOT-only — S20's
  adapter had to be Go precisely because of this gap; scoped in T1.
- keel rides require-go for pgx/redis/SDKs: framework quality now
  depends on interop breadth/perf — aligned incentives, one more reason
  interop stays priority #1.
- Not chosen: HTML templating focus (owner: minimal), Go-style APIs,
  any lifecycle/DI system, ORM, broker-backed queues, `cljgo.*`
  namespace, annotation/scanning magic of any kind.
- Three DHH-persona review rounds (S20 reviews/) shaped the spec; their
  unresolved objections are logged as owner questions in the VERDICT.
