# S20 verdict — keel: the application framework

**Answer: YES.** The four risky claims are demonstrated (run.sh, all
PASS) and the design was hardened by three adversarial DHH-persona
review rounds (reviews/, committed verbatim, unsoftened).
Recommendation: adopt ADR 0041, name the framework **keel**, ship it
with the cljgo toolchain, with a `cljgo new` scaffold as tier ZERO.

**Read this first (honesty).** The prototypes prove the *mechanisms* —
live var handlers, the routes→ServeMux adapter, config merging,
goroutine workers — against the real evaluator/reader/runtime. But the
live-handler demo runs through a Go bridge embedding the evaluator,
because the interpreter's stdlib seed registry does not yet expose
net/http: `cljgo new && cljgo dev` boots NOTHING today. The terminal
transcript below is the T0 exit criterion, not the present tense.
Every tier exists to close exactly that gap; no other claim in this
file depends on unshipped work.

## The first fifteen minutes (T0 exit criterion)

```
$ cljgo new myapp        # layout, conf.edn, first migration, a styled
$ cd myapp && cljgo dev  # page, one passing test — server up, nREPL
                         # attached, migrations applied
```

`cljgo new` is a generator, not a container: it writes plain files you
own into the blessed layout (`src/app/` · `conf.edn` · `migrations/` ·
`public/` · `test/`). Nothing scans them; the app calls everything,
visibly. **The generator's output IS the golden page** — trimmed only
of pillars whose tier hasn't shipped yet, growing tier by tier until
they are byte-identical. The page is never ahead of the product in
what it *shows shipping*; the full page below is the T3 target and is
labeled as such.

## The golden path (this page is the product — full page = end of T3)

```clojure
(ns app.main
  (:require [keel.http :as http]   [keel.config :as config]
            [keel.db :as db]       [keel.jobs :as jobs]
            [keel.cache :as cache] [keel.html :as html]))

(def cfg (config/load!))            ; conf.edn + APP_* env. Reads a file, no more.
(def pg  (db/connect! (:db cfg)))   ; validates the URL now, dials on first use —
(def mem (cache/local {:ttl "5m"})) ; requiring this namespace does no I/O.

(def user-schema                    ; what signup may write — nothing else gets in
  {:email [:string :required]
   :name  [:string :required]})

(defn send-welcome [{:keys [email]}]
  (println "welcome," email))

(def q (jobs/start! pg {:email/welcome #'send-welcome}))  ; jobs live in Postgres;
                                                          ; workers are goroutines
(defn signup [req]
  (db/tx! pg
    (fn [tx]
      (let [user (db/insert! tx :users (db/cast! (:body req) user-schema))]
        (jobs/enqueue! tx :email/welcome user)   ; commits WITH the insert —
        (http/created user)))))                  ; a lost job can't exist

(defn show-user [req]
  (let [id   (parse-long (-> req :params :id))   ; params are strings; say so
        user (cache/fetch mem [:user id]
               #(db/one! pg ["select * from users where id = $1" id]))]
    (http/ok (html/page
               [:h1 (:name user)]
               [:p "with us since " (:created-at user)]))))

(def routes
  [["POST /signup"    #'signup]
   ["GET /users/{id}" #'show-user]
   ["GET /static/"    (http/dir "public")]
   ["GET /health"     (http/health {:db pg :jobs q})]])

(defn -main []
  (http/serve routes {:port (:port cfg)}))
;; No :middleware given = (http/defaults): access-log, recover (the error
;; table), sessions, CSRF, JSON negotiation — the security is what you
;; DIDN'T type. serve pings pg, then blocks; SIGTERM drains requests AND
;; jobs. Pass :middleware to replace the stack — explicitly.
```

Every pillar on the page is called, visibly, in boot order. Handlers —
http AND job — are vars: re-`def` at the REPL and the LIVE process
updates (measured below); `cljgo dev` warns loudly if a route or job
holds a plain fn (liveness must not fail silently). The same file
AOT-compiles to one static binary. Input is cast against a declared
schema before it touches a table — mass assignment is not the lesson.
Errors are `!` exceptions rendered by the recover middleware's
*documented, overridable* error table.

**AI is a pillar, not a page-one stunt.** The blessed pattern is a job
— retries, timeouts, and cost logging come from the queue, not from a
hung request:

```clojure
(defn summarize-user [{:keys [id]}]
  (let [user (db/one! pg ["select * from users where id = $1" id])
        res  (ai/generate! (ai/model cfg :summarizer)
               {:prompt (str "One friendly line about " (:name user))})]
    (db/update! pg :users {:id id :summary (:text res)})))
;; registered as :user/summarize in jobs/start!; enqueue from anywhere
```

**Day two — the railway.** When a flow has real failure branches, drop
the `!`: plain keel fns return Result, and `let?` short-circuits:

```clojure
(defn signup [req]
  (let? [input (db/cast (:body req) user-schema)   ; (err {:email "taken"})
         user  (db/insert pg :users input)]        ;   short-circuits → 422
    (http/created user)))
```

One rule language-wide: plain = value/Result, `!` = unwrap-or-throw.
Beginners live in `!`; the railway is there when you earn a reason.

## What was measured (prototype/, run.sh)

| Claim | Result |
|---|---|
| Live handlers: `#'var` deref per request, re-def through the real evaluator changes the next response, no restart | **PASS** — v1 → re-def → v2 on the same running server (via the embedding bridge — see honesty note) |
| Routes as data: Clojure vector walked by a ~40-line adapter onto Go 1.22+ `ServeMux` — method match, `{name}` params, no router engine | **PASS** — stdlib does the routing |
| Liveness cost | interpreted var-handler 761–865ns/req vs native Go 464–531ns/req — **1.6×, on the tree-walk interpreter**; AOT closes most of the rest (ADR 0004: var deref ≈ 1.7ns) |
| Config: EDN via the real reader + `APP_*` env overlay, one plain map | **PASS** — env > file > default held, nested keys (`APP_DB_HOST` → `[:db :host]`) |
| Workers: goroutine queue, zero broker, interpreted cljgo end-to-end | **PASS** — journal seam shows every transition through ONE fn (the Postgres swap point) |

## Name

Candidates considered:
1. **keel** (recommended) — the structural spine of a ship: everything
   is built on it and it holds the boat true. Short, typable,
   unclaimed, pairs cleanly as `keel.http`, `keel.db`, `keel.jobs`.
2. **sangam** — the confluence; where rivers meet. A soul, but harder
   to type/say worldwide.
3. **chassis** — honest but generic and heavily used elsewhere.

Namespace shape: `keel.<pillar>`. NOT `cljgo.*` — the language
namespace stays the language's (precedence principle). And plainly:
**keel is a framework** — fused to the toolchain, opinionated, the
generator writes its requires into your files. What "library style"
buys is not deniability; it is a framework with a library's manners:
no hidden call graph, nothing scanned, nothing ambient, adapters only
ever invoke what you handed them.

## Positions (omakase — one blessed way per pillar)

**Tier 0 — the scaffold IS the convention layer.** `cljgo new myapp`
generates the blessed layout: `src/app/main.cljg` (the golden page,
current-tier edition), `src/app/`, `conf.edn` (+ optional
`conf.schema.edn`), `migrations/`, `public/` (with a real stylesheet —
the first page is styled, #NOBUILD, CSS served from disk), `test/`
(one passing test), `build.cljgo`. `cljgo dev` boots it: applies
migrations, starts the server, attaches nREPL, and warns on non-var
handlers. `cljgo new --with-auth` copies a complete session-based
password auth implementation INTO your app (the Phoenix phx.gen.auth
model): you own the code, the framework owns the pattern. Convention
over configuration WITHOUT inversion of control: conventions come
from generation and guides, never from scanning or containers.

**Boot & values.** No container, no lifecycle protocol, no DI. Top
level constructs VALUES: `config/load!` reads a file; `db/connect!`
validates and returns a pool that dials on first use; `cache/local`
allocates a map — requiring `app.main` performs no I/O, so tests load
it freely under `APP_PROFILE=test`. `-main` is where the world starts:
`http/serve` pings the pool (readiness), starts accepting, and owns
shutdown. Bad config still can't reach production: `load!` throws at
load, the serve-time ping throws before traffic.

**HTTP + middleware (T1).** The Ring contract, verbatim: handler = fn
of request-map → response-map; middleware = fn of handler → handler.
When `:middleware` is not given, `(http/defaults)` applies:
access-log, recover, sessions (signed cookies), CSRF, JSON
negotiation — **the safe stack is what you didn't type**; passing
`:middleware` replaces it, explicitly and completely (no merge magic).
Production timeouts and graceful SIGTERM drain (requests and jobs,
with a deadline) are defaults. `#'var` handlers deref per request;
plain fns are allowed but `cljgo dev` warns that they are not live.
The recover middleware's error table is DATA, documented and shipped:
`{:cast/invalid 422, :db/not-found 404, :db/constraint 409, else 500}`
— override with `(http/recover {:error-map ...})`. A handler that
returns a raw Result is a loud 500 in dev ("you returned a Result —
unwrap it or use the funnel"), never silently laundered. Escape hatch:
the raw mux/server.

**HTML + static (T1).** `keel.html`: hiccup-style — vectors in,
escaped HTML out (`html/page` for documents), XSS-safe by
construction, explicit ugly opt-out for raw fragments. No template
DSL, no asset pipeline (owner constraint): CSS is a file in `public/`,
served by `(http/dir "public")`, one route in the scaffold. JSON
equally first-class.

**Routing (T1).** Routes are a vector of `[pattern handler]`, pattern
= Go 1.22+ ServeMux pattern string — method and path in one string is
the stdlib's own blessed syntax and we do not fork it. `:params` bind
as STRINGS (documented; the page shows `parse-long`). We add only
`:params` and `(http/group prefix mw routes)`. We do NOT build a
router.

**Configuration (T1).** TWO layers: `conf.edn` → `APP_*` env, into one
plain map. Profiles are a section (`:profiles {:prod {...}}` merged by
`APP_PROFILE`), not a file family. The schema (`conf.schema.edn`) is
OPTIONAL — generated minimal by the scaffold, enforced when present
(required/typed keys abort boot naming key and layer). `cljgo config`
prints the resolved map with each key's winning layer. Secrets are
env-only by convention. Runtime-mutable config is a `keel.db` recipe,
not a pillar.

**Data layer (T2).** **No ORM. Ever.** pgx (require-go, zero bindings)
behind `keel.db`: `query`/`one`/`insert`/`update`/`delete`/`tx` —
plain maps out, **SQL strings in — THE blessed form**. Casts are day
ONE, not day two: the golden page writes `(db/cast! body schema)`
before any insert — the blessed path is the security posture (round 2,
point 1). Plain variants return Result (`(err {:field msg})`)
composing with `let?`. Migrations: SQL files, UTC-timestamp names,
`cljgo migrate`, additive-only doctrine. Postgres blessed;
database/sql is the hatch.

**Worker queues (T3).** The Oban model: jobs are rows in YOUR
Postgres (state-of-record), `enqueue!` on a tx handle commits
atomically with domain writes, workers are goroutines, LISTEN/NOTIFY
wakeup with polling fallback. Handler map values are vars, derefed at
dispatch — live like http handlers. `enqueue!` validates the job type
against the queue's registered handlers — a typo fails at the enqueue
site, not asynchronously in a worker log (round 2, point 14).
Retries/backoff, unique jobs, per-type concurrency, cron — same
table. `(jobs/start! :memory handlers)` for dev/tests: same API on
channels with ADR 0040's core.async semantics (real Go chans — the
spike's journal seam is exactly this swap). SIGTERM drains in-flight
jobs.

**Cache (T3).** `(cache/local {:ttl ...})` — in-process TTL +
singleflight. `(cache/fetch c key f)` is the read API. Same protocol
over Redis (rueidis via require-go) when you outgrow one process.

**AI providers (T4 — first-party satellite).** `keel.ai` ships with
the toolchain but is **independently versioned**: provider churn revs
the satellite, never the keel (round 2, point 7). One fn:
`(ai/generate model opts)` → Result (`generate!` throws); models
resolved by step key from config — never vendor strings in app code;
cross-provider fallback chains; native JSON modes; per-call timeout
defaults; one interaction-log seam. The blessed calling context is a
JOB, and the docs never show an inline AI call in a request handler.

**App testing (T1+).** In-process http test client, `:memory` jobs
drain-and-assert, per-test tx rollback fixtures — riding ADR 0012.
`cljgo new` generates a first passing test; no-I/O-at-load makes
namespaces loadable under `APP_PROFILE=test` by construction.

**Guides (deliverable, gated like code).** The 15-minute tutorial, one
guide per pillar, the auth chapter, and the production checklist ship
with their tiers and gate releases. Framework error messages meet the
ADR 0015 diagnostics bar.

**Error model (cross-cutting).** ADR 0014, one rule: plain =
value/Result, `!` = unwrap-or-throw. Beginner surface is `!` + the
documented error table; the railway is the day-two upgrade. The funnel
never converts a type confusion silently (loud dev-mode 500).

## What the DHH rounds changed

**Round 1** (reviews/dhh-round-1.md) forced:
- **T0 exists now**: `cljgo new` + `cljgo dev` — absence called
  disqualifying, correctly.
- **Blessed project layout** written normatively into the spec.
- **Golden path flipped to `!` forms**; `let?`/Result demoted to day
  two.
- **Job handlers unsealed**: vars, derefed at dispatch, live like
  http.
- **`keel.html` added** (data→HTML fn; no DSL) so the 15 minutes ends
  with a page.
- **Sessions/CSRF/secure cookies became code**; password auth became
  `cljgo new --with-auth` copy-in + guide.
- **SQL strings blessed as THE form** (closed the "both first-class"
  menu).
- **Production defaults** (timeouts, SIGTERM drain, `cljgo config`,
  pool sizing); **guides + app-testing became gated deliverables**;
  honesty note added.

Defended in round 1: library style itself (owner mandate; conventions
come from T0 + guides, not IoC), explicit handles over ambient
globals, the name.

**Round 2** (reviews/dhh-round-2.md) forced:
- **Casts moved to day ONE**: the page now writes
  `(db/cast! (:body req) user-schema)` — mass assignment is no longer
  lesson one (point 1).
- **`(http/defaults)`**: no `:middleware` given = the full safe stack
  (sessions, CSRF, recover, access-log, JSON) — security is what you
  didn't type; the page's POST is now CSRF-protected as shown
  (point 2).
- **No-I/O-at-load contract**: constructors build values; the pool
  dials on first use; `http/serve` pings before traffic; the spec's
  flinching "(or file top-level)" replaced by a real requirement
  (point 4).
- **Liveness trap closed**: `cljgo dev` warns on plain-fn routes/jobs
  (point 5).
- **AI off the page**: golden page is signup/query/job/page; the AI
  snippet moved below it, blessed pattern = in a job, never inline in
  a handler (point 6); **keel.ai became an independently-versioned
  first-party satellite** (point 7).
- **Config cut to TWO layers** (conf.edn with a `:profiles` section →
  env); schema now optional (point 8).
- **The error funnel became a documented data table** with an
  override; stray Results fail loudly instead of being laundered
  (point 9).
- **Static files exist**: `public/` in the layout, `(http/dir
  "public")` on the page, scaffold ships a stylesheet (point 11).
- **Generator/page contract**: the generator's output IS the page,
  trimmed to shipped tiers; the T0 edition is specified in tasks
  (point 12).
- **Honesty note moved to the top** of this file (point 13).
- **`enqueue!` validates job types at the call site** (point 14).
- **Dropped the "replaceable in principle" hedge** — keel is a
  framework and says so (point 16).

Defended in round 2: the `!`/Result two-surface model (ADR 0014 is an
owner decision; the funnel no longer launders, and docs keep one
surface per chapter — logged as an open question below), pattern
strings (`"POST /signup"` is the stdlib's own syntax), `:ttl "5m"`
(Go duration literals; EDN has no duration), params-as-strings with
visible `parse-long` (coercion magic rejected), and "it never calls
you" reworded rather than retracted — the claim is now "no hidden call
graph", which adapters invoking handed-in handlers does not violate
(point 3).

**Round 3** — (recorded after the round; see reviews/dhh-round-3.md)

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
build.cljgo, core.async on real goroutines — ADR 0040). Every hated
Spring feature is the framework hiding the call graph; every loved one
is a curated default. keel ships the defaults — including the
generator and the guides that carry the conventions — and keeps the
call graph visible.
