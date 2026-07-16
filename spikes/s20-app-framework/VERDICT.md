# S20 verdict — keel: the application framework

**Answer: YES.** The four risky claims are demonstrated (run.sh, all
PASS) and the design went through three adversarial DHH-persona review
rounds — each by a fresh reviewer with no memory of the others, each
committed verbatim under reviews/, each followed by the substantive
revision recorded below. Unresolved objections are logged as owner
questions, not smoothed over. Recommendation: adopt ADR 0041, name the
framework **keel**, ship it with the cljgo toolchain, with a
`cljgo new` scaffold as tier ZERO.

**Read this first (honesty).** The prototypes prove the *mechanisms* —
live var handlers, the routes→ServeMux adapter, config merging,
goroutine workers — against the real evaluator/reader/runtime. But the
live-handler demo runs through a Go bridge embedding the evaluator,
because the interpreter's stdlib seed registry does not yet expose
net/http: `cljgo new && cljgo dev` boots NOTHING today. The terminal
transcript below is the T0 exit criterion, not the present tense.
Round 3's sequencing demand is adopted: nothing below T1 gets further
design until a generated app actually boots (T4/AI is cut from this
change entirely).

## The first fifteen minutes (T0/T2 exit criterion)

```
$ cljgo new myapp        # layout, conf.edn, first migration, a styled
$ cd myapp && cljgo dev  # page, one passing test — dev database
                         # provisioned (embedded Postgres, zero install),
                         # migrations applied, server up, nREPL attached
```

`cljgo new` is a generator, not a container: it writes plain files you
own into the blessed layout (`src/app/` · `conf.edn` · `migrations/` ·
`public/` · `test/`). Nothing scans them; the app calls everything,
visibly. **The generator's output IS the golden page** — trimmed only
of pillars whose tier hasn't shipped, growing tier by tier until they
are byte-identical (the T0 edition is designed in the tasks: config +
routes + a styled page + a passing test; no db verbs before the db
tier exists). And the beginner installs no database: with no
`APP_DB_URL` set, `cljgo dev` provisions an **embedded Postgres**
(pure require-go module, data under `.dev/pg/`) — real Postgres in
dev, dev/prod parity kept, nothing to install.

## The golden path (this page is the product — full page = end of T3)

```clojure
(ns app.main
  (:require [keel.http :as http]   [keel.config :as config]
            [keel.db :as db]       [keel.jobs :as jobs]
            [keel.cache :as cache] [keel.html :as html]))

(def cfg (config/load!))            ; conf.edn + APP_* env. Reads a file, no more.
(def pg  (db/connect! (:db cfg)))   ; validates now, dials on first use.
(def mem (cache/local {:ttl 300}))  ; seconds. No I/O above this line —
                                    ; requiring app.main is side-effect free.
(defn send-welcome [{:keys [email]}]
  (println "welcome," email))

(def q (jobs/queue {:email/welcome #'send-welcome}))  ; a VALUE: the job
                                                      ; registry. Vars => live.
(def user-schema
  {:email [:string :required]
   :name  [:string :required]})

(defn signup [req]
  (db/tx! pg
    (fn [tx]
      (let [user (db/insert! tx :users (db/cast! (:body req) user-schema))]
        (jobs/enqueue! tx q :email/welcome user)   ; commits WITH the insert —
        (http/created user)))))                    ; a lost job can't exist

(defn show-user [req]
  (let [id   (http/param! req :id :int)            ; bad input → 400, funnel-mapped
        user (cache/fetch mem [:user id]
               #(db/one! pg ["select * from users where id = $1" id]))]
    (http/ok (html/page                            ; db/one! miss → 404, same table
               [:h1 (:name user)]
               [:p "with us since " (:created-at user)]))))

(def routes
  [["POST /signup"    #'signup]
   ["GET /users/{id}" #'show-user]
   ["GET /static/"    (http/dir "public")]
   ["GET /health"     (http/health {:db pg :jobs q})]])

(defn -main [& args]
  (if (= (first args) "migrate")
    (db/migrate! pg)                          ; the same binary migrates & serves
    (let [workers (jobs/start! pg q)]         ; I/O starts HERE, in main
      (http/serve routes {:port (:port cfg)
                          :drain [workers]}))))
;; No :middleware given = (http/defaults) — a plain VECTOR you can conj
;; onto: access-log, recover (the error table), sessions, CSRF (gates
;; session-bearing requests; html/form mints the token; sessionless JSON
;; passes), JSON negotiation. serve pings pg, then blocks; SIGTERM drains
;; in-flight requests, then everything in :drain. Nothing ambient.
```

Every pillar on the page is called, visibly, in boot order — and the
page passes its own spec: no I/O at namespace load (`jobs/queue` is a
value; workers start in `-main`), shutdown wiring is on the page
(`:drain`), bad input on `/users/abc` is a funnel-mapped 400, a
missing row is a 404, and the same binary migrates and serves (the
single-binary deploy story, embedded `public/` + `migrations/`
included). Handlers — http AND job — are vars: re-`def` at the REPL
and the LIVE process updates (measured below); `cljgo dev` warns if a
route or job holds a plain fn. Input is cast against a declared schema
before it touches a table.

**The blessed error surface for app code is `!` + the funnel — the
chef picked the fish.** Result/`let?` is the escape hatch for domain
and library code with real failure branches, and using it at the http
boundary requires the visible bridge — which is what makes the funnel
legal and a *bare* Result a loud dev-mode 500 (no coin flip):

```clojure
(defn signup [req]
  (http/render                    ; Result-aware: (ok resp) → resp, (err e) → table
    (let? [input (db/cast (:body req) user-schema)
           user  (db/insert pg :users input)]
      (http/created user))))
```

**AI (owner pillar, sequenced out of this change).** `keel.ai` is a
first-party, independently versioned satellite, specced in its own
OpenSpec change AFTER T1 ships and the generated app boots (round 3,
point 13). Its fixed positions carry: models resolved by step key from
config, never vendor strings in app code; Result surface;
cross-provider fallbacks; one interaction-log seam; blessed calling
context is a JOB, never inline in a handler.

## What was measured (prototype/, run.sh)

| Claim | Result |
|---|---|
| Live handlers: `#'var` deref per request, re-def through the real evaluator changes the next response, no restart | **PASS** — v1 → re-def → v2 on the same running server (via the embedding bridge — see honesty note) |
| Routes as data: Clojure vector walked by a ~40-line adapter onto Go 1.22+ `ServeMux` — method match, `{name}` params, no router engine | **PASS** — stdlib does the routing |
| Liveness cost | interpreted var-handler 761–888ns/req vs native Go 464–531ns/req — **1.6–1.7×, on the tree-walk interpreter**; AOT closes most of the rest (ADR 0004: var deref ≈ 1.7ns) |
| Config: EDN via the real reader + `APP_*` env overlay, one plain map | **PASS** — env > file > default held, nested keys |
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
`conf.schema.edn`), `migrations/` (from T2), `public/` (with a real
stylesheet — the first page is styled, #NOBUILD, CSS from disk),
`test/` (one passing test), `build.cljgo`. `cljgo dev` boots it —
and provisions the embedded-Postgres dev database when no
`APP_DB_URL` is set (from T2). `cljgo new --with-auth` copies a
complete session-based password auth implementation INTO your app
(the Phoenix phx.gen.auth model). Conventions come from generation
and guides, never from scanning or containers.

**Boot & values.** No container, no lifecycle protocol, no DI. Top
level constructs VALUES — `config/load!` reads a file; `db/connect!`
validates and dials on first use; `jobs/queue` is a registry value —
so requiring `app.main` performs no I/O and tests load it under
`APP_PROFILE=test`. `-main` starts the world: workers via
`(jobs/start! pg q)`, then `http/serve` pings the pool and owns
shutdown, draining `:drain` handles after in-flight requests. All
wiring is on the page.

**HTTP + middleware (T1).** The Ring contract, verbatim. When
`:middleware` is not given, `(http/defaults)` applies — and it is a
plain VECTOR: inspect it, `conj` onto it, remove by name; `cljgo
routes` prints the effective stack, and dev mode warns when a custom
stack lacks `recover` or `csrf`. CSRF gates session-bearing requests;
`html/form` mints the token into forms; sessionless JSON requests
pass (the documented API posture) — so the tutorial's curl works AND
the browser is protected. Production timeouts and graceful SIGTERM
drain are defaults. `#'var` handlers deref per request; `cljgo dev`
warns on plain fns. The recover error table is DATA, shipped and
documented: `{:http/bad-param 400, :cast/invalid 422, :db/not-found
404, :db/constraint 409, else 500}` — override via `(http/recover
{:error-map ...})`. `(http/param! req :id :int)` is the blessed typed
param accessor (visible coercion, funnel-mapped failure). Escape
hatch: the raw mux/server.

**HTML + static (T1).** `keel.html`: hiccup-style — vectors in,
escaped HTML out, `html/page` for documents, `html/form` for
CSRF-bearing forms. XSS-safe by construction, explicit ugly opt-out.
No template DSL, no asset pipeline: CSS is a file in `public/`,
served by `(http/dir "public")`. JSON equally first-class.

**Routing (T1).** Routes are a vector of `[pattern handler]`, pattern
= Go 1.22+ ServeMux pattern string (the stdlib's own syntax).
`:params` bind as strings; `http/param!` is the blessed coercion. We
add only `:params` and `(http/group prefix mw routes)`. We do NOT
build a router.

**Configuration (T1).** TWO layers: `conf.edn` → `APP_*` env.
Profiles are a `:profiles` section in conf.edn selected by
`APP_PROFILE`. Env mapping is deterministic: `__` (double underscore)
separates path segments, single `_` joins words —
`APP_DB__POOL_SIZE` → `[:db :pool-size]`. The schema
(`conf.schema.edn`) is OPTIONAL — generated minimal, enforced when
present (violations abort boot naming key and layer). Durations and
sizes are NUMBERS (seconds, bytes) — no stringly-typed `"5m"`.
`cljgo config` prints the resolved map with each key's winning layer.
Secrets are env-only.

**Data layer (T2).** **No ORM. Ever.** pgx (require-go, zero
bindings) behind `keel.db`: query/one/insert/update/delete/tx —
plain maps out, **SQL strings in — THE blessed form**. A written
NAMES DOCTRINE, conformance-tested: snake_case columns ↔ kebab-case
keywords both directions (`:created-at` ↔ `created_at`), table
keywords in `insert!` are unqualified table names, and the mapping is
documented in the data guide (nothing else is renamed, ever). Casts
are day ONE (`db/cast!` before any insert — mass assignment is
structurally off the path). `db/one!` throws `:db/not-found`
(funnel: 404); plain variants return Result. Migrations: SQL files,
UTC-timestamp names, `cljgo migrate`, additive-only doctrine.
**Dev database: embedded Postgres** provisioned by `cljgo dev` (real
parity, zero install); **deployment: ONE binary** — `cljgo build`
embeds `public/` and `migrations/` (ADR 0021 comptime embed), and the
generated `-main` answers `migrate` — `./myapp migrate && ./myapp`
is the whole ops story. Postgres blessed; database/sql is the hatch.

**Worker queues (T3).** The Oban model: jobs are rows in YOUR
Postgres (state-of-record), `enqueue!` on a tx handle commits
atomically with domain writes and validates the job type against the
queue value's registry (typos fail at the call site). Workers are
goroutines (LISTEN/NOTIFY + poll fallback), started in `-main`,
drained via `:drain`. Handler map values are vars, derefed at
dispatch — live like http. Retries/backoff, unique jobs, per-type
concurrency, cron — same table. **Dev runs the REAL Postgres backend
(parity — the embedded dev db is already there); `:memory` (on ADR
0040's core.async channels) is for TESTS only.**

**Cache (T3).** `(cache/local {:ttl secs})` — in-process TTL +
singleflight. `(cache/fetch c key f)` is the read API. Same protocol
over Redis (rueidis via require-go) when you outgrow one process.

**App testing (T1+).** In-process http test client; `:memory` jobs
drain-and-assert; db tests on the **Ecto-Sandbox model**: under
`APP_PROFILE=test` the pool `pg` (the same top-level var, no
with-redefs) hands out one connection wrapping each test in a
rolled-back transaction — the generated test is shown byte-for-byte
in the scaffold and the guide. Riding ADR 0012.

**Guides (deliverable, gated like code).** The 15-minute tutorial,
one guide per pillar, the auth chapter, the names doctrine, and the
production checklist ship with their tiers and gate releases.
Framework error messages meet the ADR 0015 diagnostics bar.

**Error model (cross-cutting).** ADR 0014 under one doctrine: app
handlers use `!` + the funnel (THE blessed surface); Result/`let?` is
for domain/library code and crosses the http boundary only through
`http/render`. One naming rule language-wide: plain = value/Result,
`!` = unwrap-or-throw.

## What the DHH rounds changed

**Round 1** (reviews/dhh-round-1.md) forced:
- **T0 exists now**: `cljgo new` + `cljgo dev` — absence called
  disqualifying, correctly.
- **Blessed project layout** written normatively into the spec.
- **Golden path flipped to `!` forms**; `let?`/Result demoted from the
  entry path.
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
- **Casts moved to day ONE** — mass assignment off the blessed path
  (point 1).
- **`(http/defaults)`** applied when `:middleware` is omitted —
  security is what you didn't type (point 2).
- **No-I/O-at-load contract** written into the spec; constructors
  build values (point 4).
- **Liveness trap closed**: dev warns on plain-fn routes/jobs
  (point 5).
- **AI off the page** (blessed context = a job) and **keel.ai
  became an independently-versioned satellite** (points 6–7).
- **Config cut to TWO layers**; schema optional (point 8).
- **The error funnel became a documented data table** with an
  override; stray Results fail loudly (point 9).
- **Static files exist** (`public/`, `http/dir`, scaffold stylesheet)
  (point 11).
- **Generator/page contract** — the generator's output IS the page
  (point 12); honesty note moved to the top (point 13); `enqueue!`
  validates job types (point 14); dropped the "replaceable in
  principle" hedge — keel is a framework (point 16).

Defended in round 2: the two-surface error model (ADR 0014, owner
decision — revisited in round 3), ServeMux pattern strings,
params-as-strings with visible coercion, explicit handles.

**Round 3** (reviews/dhh-round-3.md) forced:
- **A dev database**: `cljgo dev` provisions embedded Postgres when
  `APP_DB_URL` is unset — zero install, real parity (point 1).
- **The page now passes its own spec**: `jobs/queue` is a pure value;
  workers start in `-main` — no I/O at namespace load anywhere on the
  page (point 2).
- **Shutdown wiring is visible**: `http/serve` takes `:drain
  [workers]`; no ambient shutdown registry (point 3).
- **CSRF posture specified**: gates session-bearing requests;
  `html/form` mints tokens; sessionless JSON passes — the tutorial
  curl and the security scenario can both pass (point 4).
- **The Result contradiction resolved**: `http/render` is the visible
  bridge that makes the funnel legal; a bare Result stays a loud dev
  500 (point 5).
- **`(http/defaults)` is inspectable DATA** (conj/remove by name;
  `cljgo routes` prints the stack; dev warns when a custom stack
  lacks recover/csrf) (point 6).
- **One blessed error surface for app code**: `!` + funnel; railway =
  escape hatch via `http/render` — the chef picked (point 7).
- **Tiers re-sorted**: T0 generates no db verbs; migrations/dev-db/
  deployment land in T2 and the generator grows with each tier
  (point 8).
- **Dev/prod parity**: dev jobs run the REAL Postgres backend;
  `:memory` is tests-only (point 9).
- **The generated test is specified**: Ecto-Sandbox pool under
  `APP_PROFILE=test` — same `pg` var, no with-redefs (point 10).
- **A names doctrine**: snake_case↔kebab-case both directions,
  `__`-nesting for env vars, conformance-tested (point 11).
- **`/users/abc` no longer 500s**: `http/param!` → funnel-mapped 400;
  `db/one!` miss → 404 (point 12).
- **T4/AI cut from this change** — foundation before cathedral
  windows (point 13).
- **A Deployment requirement**: `cljgo build` embeds `public/` +
  `migrations/`; `./myapp migrate && ./myapp` (point 14).
- **Self-citation fixed**: the header now claims only what the file
  contains; durations became numbers (point 15).

Defended in round 3: top-level handle defs (values, no I/O — the
Clojure shape; the Sandbox pool answers the testing objection without
with-redefs), and keeping Result/`let?` in the language surface at
all (ADR 0014 is an owner decision; keel narrows it to `http/render`
at the boundary rather than removing it).

## Open questions for the owner

1. **AI sequencing.** The mandate lists AI providers as a first-class
   pillar; three review rounds converged on cutting it from this
   change (independently versioned `keel.ai` satellite, own OpenSpec
   change after T1 boots a generated app). Positions are fixed in ADR
   0041; only the sequencing needs your confirmation.
2. **The error-model dual surface.** All three rounds attacked
   `!`-vs-Result as "two dialects". The resolution: app handlers
   bless `!` + funnel; Result crosses the http boundary only through
   `http/render`. If you want ONE surface in keel docs end-to-end
   (dropping the day-two railway from the framework's story
   entirely), say so — it cuts against ADR 0014's reach but not its
   core.
3. **Embedded Postgres as the dev database.** Zero-install dev via an
   embedded-Postgres Go module (data in `.dev/pg/`) keeps dev/prod
   parity but adds a real dependency to `cljgo dev`. Alternative:
   SQLite behind keel.db (smaller, but parity and the Oban queue
   break). Recommendation: embedded Postgres. Confirm.
4. **`html/form` scope.** CSRF-bearing form helpers edge toward the
   templating focus you deprioritized. Recommendation: ship exactly
   `html/form` (the security requires it) and nothing more — no
   layouts, no partials, no asset story. Confirm the boundary.

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
generator, the dev database, and the guides that carry the
conventions — and keeps the call graph visible.
