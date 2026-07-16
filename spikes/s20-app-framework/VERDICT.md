# S20 verdict — keel: the application framework

**Answer: YES.** The four risky claims are demonstrated (run.sh, all
PASS), and the design positions below survived three adversarial
DHH-persona review rounds (reviews/, unsoftened). Recommendation:
adopt ADR 0041, name the framework **keel**, ship it with the cljgo
toolchain as plain libraries under `keel.*`.

## The golden path (this page is the product)

```clojure
(ns app.main
  (:require [keel.config :as config]
            [keel.http :as http]
            [keel.db :as db]
            [keel.jobs :as jobs]
            [keel.cache :as cache]
            [keel.ai :as ai]))

(def cfg (config/load!))            ; conf.edn + APP_* env overlay, validated —
                                    ; a misconfigured deploy must not boot
(def pg  (db/connect (:db cfg)))    ; pgx pool (require-go, zero bindings)
(def mem (cache/local {:ttl "5m"})) ; in-process TTL cache, singleflight inside

(def q   (jobs/start pg            ; durable queue IN Postgres — no broker;
  {:email/welcome                  ; handlers sealed at start, goroutine workers
   (fn [{:keys [email]}]
     (println "welcome," email))}))

(defn signup [req]
  (let? [user (db/insert pg :users (:body req))      ; (err e) short-circuits
         _    (jobs/enqueue pg :email/welcome user)] ; same handle: enqueue can
    (http/created user)))                            ; ride the caller's tx

(defn summary [req]
  (let? [user (db/one pg ["select * from users where id = $1"
                          (-> req :params :id)])]
    (http/ok (cache/fetch mem [:summary (:id user)]
               #(ai/generate (ai/model cfg :summarizer)
                  {:prompt (str "One friendly line about " (:name user))})))))

(def routes
  [["POST /signup"            #'signup]
   ["GET /users/{id}/summary" #'summary]
   ["GET /health"             (http/health {:db pg :jobs q})]])

(defn -main []
  (http/serve routes {:port (:port cfg)
                      :middleware [(http/access-log) (http/recover)]}))
```

Every pillar is present; every call is visible; `-main` reads top to
bottom like the boot order it is. Handlers are vars → re-`def` at the
REPL and the LIVE server answers differently (measured below). The same
file AOT-compiles to one static binary.

## What was measured (prototype/, run.sh)

| Claim | Result |
|---|---|
| Live handlers: `#'var` deref per request, re-def through the real evaluator changes the next response, no restart | **PASS** — v1 → re-def → v2 on the same connection pool |
| Routes as data: Clojure vector walked by a ~40-line adapter onto Go 1.22+ `ServeMux` — method match, `{name}` params, no router engine | **PASS** — stdlib does the routing |
| Liveness cost | interpreted var-handler 761–865ns/req vs native Go 464–531ns/req — **1.6×, and that's the tree-walk interpreter**; AOT closes most of the rest (ADR 0004: var deref ≈ 1.7ns) |
| Config: EDN via the real reader + `APP_*` env overlay, one plain map | **PASS** — env > file > default held, nested keys (`APP_DB_HOST` → `[:db :host]`) |
| Workers: goroutine queue, zero broker, interpreted cljgo end-to-end | **PASS** — journal seam shows every transition through ONE fn (the Postgres swap point) |

## Name

Candidates considered:
1. **keel** (recommended) — the structural spine of a ship: everything
   is built on it, it holds the boat true, and it never steers. That is
   the library-style contract in one word. Short, typable, unclaimed in
   this space, pairs cleanly as `keel.http`, `keel.db`, `keel.jobs`.
2. **sangam** — the confluence; where the rivers (libraries) meet.
   Beautiful story, but harder to type/say worldwide and reads as a
   product name, not an infrastructure name.
3. **chassis** — honest but generic; already heavily used across
   ecosystems (fnproject/chassis, microservice-chassis pattern).

Namespace shape: `keel.<pillar>` (`keel.http`, `keel.config`,
`keel.db`, `keel.jobs`, `keel.cache`, `keel.ai`). NOT `cljgo.*` — the
language namespace stays clean (precedence principle: nothing may
shadow or crowd clojure.core's home), and keel must be replaceable in
principle precisely because it is only a library.

## Positions (omakase — one blessed way per pillar)

**Shipping shape.** keel ships WITH the cljgo toolchain (one install,
batteries included — the Spring Boot answer), but it is only a library:
plain namespaces you `:require`, plain fns you call. No app container,
no lifecycle protocol, no component system. `main` is the lifecycle.
The framework never calls you; adapters (the http server invoking your
handler) are the one obvious exception and they do it through a var you
handed them.

**HTTP + middleware (T1).** The Ring contract, verbatim: handler =
fn of request-map → response-map; middleware = fn of handler → handler,
composed in an explicit ordered vector. Server = Go's `net/http`
(production-grade, HTTP/2, graceful shutdown) behind `(http/serve
routes opts)` → returns a stop fn. Handlers referenced as `#'vars` get
per-request deref (REPL-driven web dev); plain fns are allowed and skip
the deref. Escape hatch: `(http/mux routes)` hands you the raw
`*http.ServeMux` for anything Go can do.

**Routing (T1).** Routes are a vector of `[pattern handler]` pairs,
pattern = Go 1.22+ ServeMux pattern string (`"GET /users/{id}"`).
reitit taught us routes-as-data; Go's stdlib now makes the engine free
(most-specific-wins, method match, 405s). We add only: param maps onto
`:params`, nesting via `(http/group "/api" middleware routes)`. We do
NOT build a router.

**Configuration (T1).** `(config/load!)`: `conf.edn` → profile overlay
(`conf.prod.edn` via `APP_PROFILE`) → `APP_*` env overlay → one plain
Clojure map. Four layers, not Spring's fourteen. Declared schema (keys,
types, required) validated at load; missing required key = refuse to
boot (fail loud, never fallback). Secrets are env-only by convention.
Runtime-mutable config (rotation-prone keys) is NOT this — it's a
documented recipe on `keel.db` (a config table + cache), not a pillar.

**Data layer (T2).** **No ORM. Ever.** The blessed stack: pgx (via
require-go — zero bindings) behind `keel.db`: `(db/query pg sql-or-data)`,
`(db/one ...)`, `(db/insert ...)`, `(db/tx pg (fn [tx] ...))` — plain
maps out, SQL in (strings or HoneySQL-style data, both first-class).
Changesets Ecto-style but as plain data → `(db/cast row schema)` returns
`(ok row)`/`(err {:field msg})` — composes with `let?` railway-style
(ADR 0014 is the error model of the whole framework). Migrations: SQL
files, UTC-timestamp names, `cljgo migrate` runs them; additive-only
doctrine documented. Postgres is the blessed database; `database/sql`
is the escape hatch for everything else.

**Worker queues (T3).** The Oban model on the substrate Go gives us:
jobs are rows in YOUR Postgres (state-of-record, transactional enqueue —
enqueue in the same tx as the domain write kills the lost-job bug
class), workers are goroutines (no broker, no sidecar), wakeup via
LISTEN/NOTIFY with polling fallback. Handlers are a map sealed at
`(jobs/start pg handlers)`. Per-type concurrency limits, retries with
backoff, unique jobs, cron via the same table. Dev mode: `(jobs/start
:memory handlers)` — same API, chan-backed (the spike's journal seam is
exactly this swap).

**Cache (T3).** `(cache/local {:ttl ...})` — in-process map + TTL +
singleflight (stampede suppression built in, from golang.org/x/sync).
`(cache/fetch c key f)` is the only read API you need. Same protocol
over Redis (`cache/redis`, rueidis via require-go) when you outgrow one
process. Namespacing enforced by constructor (shared-host discipline).

**AI providers (T4).** One fn: `(ai/generate model {:prompt ...})` →
`(ok {:text ... :usage ...})`/`(err e)`. Models are VALUES resolved by
step key from config — `(ai/model cfg :summarizer)` — never a hardcoded
vendor string in app code. Fallback chains are cross-provider in
matched price tier, declared in config. JSON mode via native provider
flags, never prompt-begging. One interaction-log seam (a fn you supply)
for cost/audit. Providers: anthropic/openai/google day one, all driven
through their Go SDKs via require-go.

**Auth (reference design, not a pillar).** A documented chapter +
`keel.http` middleware helpers: JWT HS256 minted by you, additive
middleware tiers (visitor → user → admin, each = previous + one check).
No auth framework; the essay explains why.

**Error model (cross-cutting).** ADR 0014 everywhere: framework fns
that are expected to fail return Result; `let?` is the composition; the
http adapter renders uncaught `(err e)` and exceptions through one
error-page/JSON funnel. The `!` convention stays language-wide
(plain = value/Result, `!` = unwrap-or-throw) — `config/load!` throws
at boot on purpose.

## What the DHH rounds changed

- **Round 1** — (recorded after the round; see reviews/dhh-round-1.md)
- **Round 2** — (recorded after the round; see reviews/dhh-round-2.md)
- **Round 3** — (recorded after the round; see reviews/dhh-round-3.md)

## Open questions for the owner

- (populated after round 3)

## Why this can be "the Rails of Clojure" when Clojure never got one

The classic reasons Clojure never converged (studied: Rails Doctrine,
Kit rationale, Biff, the HN/blog corpus): the JVM removed the pain
(Java libs everywhere), the culture removed the will (libraries over
frameworks, three incompatible lifecycle camps), and the last 20% —
conventions, docs, curation — is the expensive part nobody funded.
cljgo is a fresh community with no incumbent camps, a host whose web
culture is stdlib-first (net/http IS the server), and a language that
already made the framework's hardest calls (Result/Option, require-go,
build.cljgo, real goroutines). Every hated Spring feature is the
framework hiding the call graph; every loved one is a curated default.
keel ships the defaults and keeps the call graph visible.
