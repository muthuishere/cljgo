# S20 verdict — keel: the application framework

**Answer: YES.** The four risky claims are demonstrated (run.sh, all
PASS), and the design was hardened by three adversarial DHH-persona
review rounds (reviews/, committed verbatim, unsoftened).
Recommendation: adopt ADR 0041, name the framework **keel**, ship it
with the cljgo toolchain as plain libraries under `keel.*`, with a
`cljgo new` scaffold as tier ZERO.

## The first fifteen minutes

```
$ cljgo new myapp        # layout, conf.edn + schema, first migration,
$ cd myapp && cljgo dev  # first test, a rendered page — server up,
                         # nREPL attached, migrations applied
```

`cljgo new` is a generator, not a container: it writes plain files you
own into a blessed layout (`src/app/` · `conf.edn` · `migrations/` ·
`test/`). Nothing scans them; `-main` calls everything, visibly.

## The golden path (this page is the product)

```clojure
(ns app.main
  (:require [keel.http :as http]   [keel.config :as config]
            [keel.db :as db]       [keel.jobs :as jobs]
            [keel.cache :as cache] [keel.ai :as ai]
            [keel.html :as html]))

(def cfg (config/load!))            ; conf.edn + APP_* env, schema-checked —
(def pg  (db/connect! (:db cfg)))   ; a bad deploy refuses to boot.
(def mem (cache/local {:ttl "5m"})) ; pool/server/job timeouts: sane defaults.

(defn send-welcome [{:keys [email]}]
  (println "welcome," email))

(def q (jobs/start! pg {:email/welcome #'send-welcome}))
                                    ; job handlers are VARS — live at the
                                    ; REPL, exactly like http handlers

(defn signup [req]
  (db/tx! pg
    (fn [tx]
      (let [user (db/insert! tx :users (:body req))]
        (jobs/enqueue! tx :email/welcome user)   ; commits WITH the insert —
        (http/created user)))))                  ; the lost-job bug can't exist

(defn summary [req]
  (let [user (db/one! pg ["select * from users where id = $1"
                          (-> req :params :id)])]
    (http/ok (html/page
               [:h1 "About " (:name user)]
               [:p (:text (cache/fetch mem [:summary (:id user)]
                            #(ai/generate! (ai/model cfg :summarizer)
                               {:prompt (str "One line about " (:name user))})))]))))

(def routes
  [["POST /signup"            #'signup]
   ["GET /users/{id}/summary" #'summary]
   ["GET /health"             (http/health {:db pg :jobs q})]])

(defn -main []
  (http/serve routes {:port (:port cfg)
                      :middleware [(http/access-log) (http/recover)]}))
  ;; serve blocks; SIGTERM = graceful drain (in-flight requests AND jobs)
```

Every pillar is present; every call is visible; `-main` reads top to
bottom like the boot order it is. Handlers — http AND job — are vars:
re-`def` at the REPL and the LIVE process updates (measured below).
The same file AOT-compiles to one static binary. Errors here are plain
exceptions (`!` forms) rendered by one funnel — a constraint violation
becomes a 422 without ceremony.

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
| Live handlers: `#'var` deref per request, re-def through the real evaluator changes the next response, no restart | **PASS** — v1 → re-def → v2 on the same running server |
| Routes as data: Clojure vector walked by a ~40-line adapter onto Go 1.22+ `ServeMux` — method match, `{name}` params, no router engine | **PASS** — stdlib does the routing |
| Liveness cost | interpreted var-handler 761–865ns/req vs native Go 464–531ns/req — **1.6×, on the tree-walk interpreter**; AOT closes most of the rest (ADR 0004: var deref ≈ 1.7ns) |
| Config: EDN via the real reader + `APP_*` env overlay, one plain map | **PASS** — env > file > default held, nested keys (`APP_DB_HOST` → `[:db :host]`) |
| Workers: goroutine queue, zero broker, interpreted cljgo end-to-end | **PASS** — journal seam shows every transition through ONE fn (the Postgres swap point) |

Honesty note (surfaced by review round 1): the live-handler demo runs
through a Go bridge embedding the real evaluator, because today's
interpreter seed registry does not yet expose net/http. The mechanism
(vars, deref cost, adapter) is proven; making `cljgo dev` itself serve
it is T1 work and is scoped as such. No claim in the table depends on
unshipped magic — but the 15-minute experience DOES, and T0/T1 exist
precisely to close that.

## Name

Candidates considered:
1. **keel** (recommended) — the structural spine of a ship: everything
   is built on it and it holds the boat true. Short, typable,
   unclaimed, pairs cleanly as `keel.http`, `keel.db`, `keel.jobs`.
   Round 1 needled the metaphor ("it never steers") — the steering is
   the scaffold + guides + omakase defaults; a spine that holds you
   true IS a position, not an abdication.
2. **sangam** — the confluence; where rivers meet. A soul, but harder
   to type/say worldwide.
3. **chassis** — honest but generic and heavily used elsewhere.

Namespace shape: `keel.<pillar>`. NOT `cljgo.*` — the language
namespace stays the language's (precedence principle), and keel must
be replaceable in principle precisely because it is only a library.

## Positions (omakase — one blessed way per pillar)

**Tier 0 — the scaffold IS the convention layer.** `cljgo new myapp`
generates the blessed layout: `src/app/main.cljg` (the golden path),
`src/app/` for your namespaces, `conf.edn` + `conf.schema.edn`,
`migrations/`, `test/`, `build.cljgo`. `cljgo dev` boots it: applies
migrations, starts the server, attaches nREPL. `cljgo new --with-auth`
copies a complete session-based password auth implementation INTO your
app (the Phoenix phx.gen.auth model): you own the code, the framework
owns the pattern. This is how "convention over configuration" coexists
with "it never calls you": conventions come from generation and
documentation, not from inversion of control.

**Shipping shape.** keel ships WITH the cljgo toolchain (one install,
batteries included), but it is only a library: plain namespaces, plain
fns. No container, no lifecycle protocol, no DI: `-main` is the
lifecycle. Handles (`pg`, `q`, `mem`) are ordinary values in vars —
explicitly NOT ambient globals the framework conjures; a second
database is a second `(db/connect! ...)`, not a YAML stanza. Failing
construction refuses to boot, loudly.

**HTTP + middleware (T1).** The Ring contract, verbatim: handler = fn
of request-map → response-map; middleware = fn of handler → handler in
an explicit ordered vector. Server = Go `net/http` behind
`(http/serve routes opts)`; graceful shutdown (SIGTERM → drain, with a
deadline) and production timeouts are DEFAULTS, not options you
discover after an outage. `#'var` handlers deref per request; plain
fns skip it. Sessions (signed cookies), CSRF protection, and secure
cookie helpers are code in `keel.http` — not an essay (round 1, point
9). Escape hatch: the raw mux/server.

**HTML (T1).** `keel.html`: hiccup-style — vectors and keywords in,
escaped HTML out, `html/page` for a full document. It is a function
over data, not a template language; there is nothing to "learn" beyond
the data structure already in your hands. The owner's "no templating
focus" holds: no template DSL, no asset pipeline — but the first
fifteen minutes ends with a PAGE, not curl (round 1, point 7). JSON
responses are equally first-class (`http/json`, content negotiation in
the `json` middleware).

**Routing (T1).** Routes are a vector of `[pattern handler]`, pattern
= Go 1.22+ ServeMux pattern string. reitit taught us routes-as-data;
Go's stdlib made the engine free (most-specific-wins, method match,
405s). We add only `:params` binding and `(http/group prefix mw
routes)` nesting. We do NOT build a router.

**Configuration (T1).** `(config/load!)`: schema defaults →
`conf.edn` → `conf.<profile>.edn` → `APP_*` env → one plain map. The
schema lives in `conf.schema.edn` (generated by `cljgo new`, next to
the data it validates); required/typed keys checked at load; missing =
refuse to boot naming the key and layer. `cljgo config` prints the
resolved map with each key's winning layer — the 2 a.m. debugging
story is a subcommand, not archaeology (round 1, point 8). Secrets are
env-only by convention. Runtime-mutable config is a documented recipe
on `keel.db`, not a pillar.

**Data layer (T2).** **No ORM. Ever.** pgx (require-go, zero bindings)
behind `keel.db`: `query`/`one`/`insert`/`update`/`delete`/`tx` —
plain maps out, **SQL strings in — THE blessed form** (round 1, point
10: "both first-class" was a menu; the menu is closed). SQL-as-data
composers remain possible as ordinary libraries; keel's docs write
SQL. Casts Ecto-style: `(db/cast row schema)` → `(ok row)`/`(err
{:field msg})`, composing with `let?`. Migrations: SQL files,
UTC-timestamp names, `cljgo migrate`, additive-only doctrine.
Postgres is blessed; `database/sql` is the hatch.

**Worker queues (T3).** The Oban model: jobs are rows in YOUR
Postgres (state-of-record), `enqueue!` on a tx handle commits
atomically with domain writes, workers are goroutines, LISTEN/NOTIFY
wakeup with polling fallback. Handler map values are **vars, derefed
at dispatch — live at the REPL exactly like http handlers** (round 1,
point 6; "sealed" is gone). Retries/backoff, unique jobs, per-type
concurrency, cron — same table. `(jobs/start! :memory handlers)` for
dev/tests: same API on chans (the spike's journal seam is exactly this
swap). SIGTERM drains in-flight jobs before exit.

**Cache (T3).** `(cache/local {:ttl ...})` — in-process TTL +
singleflight. `(cache/fetch c key f)` is the read API. Same protocol
over Redis (rueidis via require-go) when you outgrow one process.

**AI providers (T4).** One fn: `(ai/generate model opts)` → Result
(`ai/generate!` throws). Models are values resolved by step key from
config — `(ai/model cfg :summarizer)` — never vendor strings in app
code. Cross-provider fallback chains in config; native JSON modes;
per-call timeout defaults; one interaction-log seam.

**App testing (cross-cutting, T1+).** cljgo already made testing
first-class (ADR 0012); keel adds the app-shaped helpers: an
in-process http test client (`(http.test/request app {:post "/signup"
...})`), `:memory` jobs with a drain-and-assert helper, per-test tx
rollback fixtures for db tests. `cljgo new` generates a first passing
test.

**Guides (deliverable, gated like code).** The 15-minute tutorial, one
guide per pillar, the auth chapter, and the production checklist ship
with their tiers and are release gates — docs-as-product, funded
(round 1, points 2/11). Framework error messages carry the same
diagnostic quality bar as the compiler (ADR 0015).

**Error model (cross-cutting).** ADR 0014 everywhere, one rule: plain
= value/Result, `!` = unwrap-or-throw. Beginner surface is `!` + the
one error funnel; the railway is the day-two upgrade, not the entry
fee (round 1, point 5).

## What the DHH rounds changed

**Round 1** (reviews/dhh-round-1.md) forced:
- **T0 exists now**: `cljgo new` (layout, conf.edn + schema, first
  migration, first test, rendered page) and `cljgo dev` (server +
  nREPL + migrations) — was entirely absent; the reviewer called that
  disqualifying, and was right.
- **Blessed project layout** written into the spec normatively.
- **Golden path flipped to `!` forms**; `let?`/Result demoted to a
  "day two" section. The beginner no longer meets the railway before
  their first route.
- **Job handlers unsealed**: vars, derefed at dispatch — live-redef is
  now uniform across http and jobs.
- **`keel.html` added** (hiccup-style data→HTML fn): the first 15
  minutes ends with a page, while honoring the owner's no-template-DSL
  constraint.
- **Sessions/CSRF/secure cookies became code** in keel.http; password
  auth became a copy-in generator (`cljgo new --with-auth`) plus the
  essay — not literature alone.
- **SQL strings blessed as THE form** (data-SQL demoted to "possible,
  not documented as equal") — closed the "both first-class" menu.
- **Production defaults**: timeouts on by default; graceful SIGTERM
  drain for server AND jobs; `cljgo config` explain subcommand;
  pool-sizing defaults.
- **Guides + app-testing helpers became tracked, gated deliverables.**
- **Honesty note** on the seed-registry gap added to the measurements.

Positions defended against round 1 (not changed): library style itself
(owner mandate — inversion of control is the disease; conventions come
from T0 generation + guides instead), explicit handles over ambient
globals (`User.create`'s hidden connection is exactly the magic cljgo
refuses; the golden path shows the cost is a few visible defs), and
the name.

**Round 2** — (recorded after the round; see reviews/dhh-round-2.md)

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
build.cljgo, real goroutines). Every hated Spring feature is the
framework hiding the call graph; every loved one is a curated default.
keel ships the defaults — including the generator and the guides that
carry the conventions — and keeps the call graph visible.
