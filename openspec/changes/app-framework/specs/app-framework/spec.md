## ADDED Requirements

### Requirement: The first fifteen minutes are generated, not assembled
`cljgo new <name>` SHALL generate a runnable app in the blessed layout
— `src/app/main.cljg`, `src/app/`, `conf.edn`, `conf.schema.edn`,
`migrations/` (with a first migration), `test/` (with one passing
test), `build.cljgo` — as plain files the user owns; nothing SHALL
scan or implicitly load them. `cljgo dev` SHALL apply pending
migrations, start the app, and attach an nREPL. `cljgo new
--with-auth` SHALL copy a complete session-based password auth
implementation (code + tests) into the app.

#### Scenario: new to page
- **WHEN** a user runs `cljgo new myapp && cd myapp && cljgo dev`
- **THEN** a rendered HTML page is served locally and an editor can
  connect to the printed nREPL port, with no file authored by hand

### Requirement: The golden-path app is under a page and runs both modes
The complete small app of ADR 0041 (server + data routes + middleware
+ config + db query + transactional background job + cache + one AI
call + a rendered page) SHALL be expressible in under one page of
cljgo, SHALL run unmodified through the interpreter (`cljgo run`) and
as an AOT-compiled static binary, and SHALL be a conformance artifact
(divergence = release blocker, ADR 0007).

#### Scenario: dual harness
- **WHEN** the golden-path app runs interpreted and compiled against
  the same services
- **THEN** observable behavior is identical

### Requirement: keel never calls user code except through handed-in fns
keel SHALL expose only plain namespaces of plain functions. It SHALL
NOT scan, register by convention, proxy, or instantiate user code; the
only inversion is an adapter invoking a handler/job fn the user passed
explicitly (directly or as a var). Handles (pools, queues, caches)
SHALL be ordinary values — no ambient globals.

#### Scenario: boot order is main
- **WHEN** an app boots
- **THEN** every framework effect traces to a call in the user's
  `-main` (or file top-level), in source order

### Requirement: HTTP is the Ring contract on stdlib routing
A handler SHALL be a fn of request-map → response-map. Routes SHALL be
a data vector of `[pattern handler]` with Go ServeMux pattern strings;
`{name}` segments SHALL bind into `:params`. Middleware SHALL be
handler → handler fns applied in the order of an explicit vector.
`(keel.http/serve routes opts)` SHALL default production timeouts ON
and SHALL drain gracefully on SIGTERM with a deadline; the underlying
mux/server SHALL be reachable as escape hatches. Sessions (signed
cookies), CSRF protection, and secure-cookie helpers SHALL ship as
code in keel.http.

#### Scenario: routing without a router
- **WHEN** routes `[["GET /users/{id}" #'show]]` are served
- **THEN** `GET /users/7` invokes `show` with `{:params {:id "7"}}`
  and `POST /users/7` yields 405 — both from the stdlib matcher

#### Scenario: live handlers
- **WHEN** a route references `#'handler` and the var is re-`def`ed
  from a REPL/nREPL session against the live process
- **THEN** the next request observes the new definition without
  restart (var deref per request; plain-fn handlers skip the deref)

### Requirement: HTML is a function over data
`keel.html` SHALL render hiccup-style vectors to escaped HTML
(`html/page` for full documents), XSS-safe by construction with an
explicit, visually loud unescape form. There SHALL be no template
language and no asset pipeline.

#### Scenario: page from data
- **WHEN** a handler returns `(http/ok (html/page [:h1 greeting]))`
- **THEN** the client receives a complete HTML document with
  `greeting` escaped

### Requirement: Errors have one beginner surface and one funnel
keel fns that are expected to fail SHALL return Result; their `!`
variants SHALL unwrap-or-throw (the language-wide rule: plain =
value/Result, `!` = throws). All documentation entry paths and
generated code SHALL use `!` forms; `let?`/Result SHALL be documented
as the day-two upgrade. The `recover` middleware SHALL be the single
funnel mapping exceptions and stray `(err e)` values to responses.

#### Scenario: constraint violation without ceremony
- **WHEN** `db/insert!` inside a handler hits a unique-constraint
  violation
- **THEN** the funnel renders a 422 (JSON or HTML per negotiation)
  with no error-handling code in the handler

### Requirement: Configuration is four layers into one plain map
`(keel.config/load!)` SHALL merge, in increasing precedence: schema
defaults → conf.edn → conf.<profile>.edn (profile from APP_PROFILE) →
`APP_*` environment variables (`APP_DB_HOST` → `[:db :host]`, coerced
per schema), yielding one plain map. The schema SHALL live in
`conf.schema.edn`. A declared-required key that is missing or
ill-typed SHALL abort boot with a diagnostic naming the key and layer.
`cljgo config` SHALL print the resolved map annotated with each key's
winning layer.

#### Scenario: misconfigured deploy must not boot
- **WHEN** a required key is absent from all layers
- **THEN** `load!` throws before any server/pool/worker starts

#### Scenario: 2 a.m. debugging
- **WHEN** an operator runs `cljgo config` in the app directory
- **THEN** every key shows its effective value and the layer that won

### Requirement: Data layer is SQL and maps, never an ORM
`keel.db` SHALL provide connect!/query/one/insert/update/delete/tx
over pgx with sane pool and timeout defaults: SQL as strings (THE
blessed form), rows as plain maps, results as Result values with `!`
variants. Schema casts SHALL return `(ok row)`/`(err {field message})`
composing with `let?`. Migrations SHALL be SQL files with
UTC-timestamp names driven by `cljgo migrate`.

#### Scenario: railway signup (day two)
- **WHEN** an insert violates a constraint inside a `let?` chain
- **THEN** the chain short-circuits with `(err e)` and the funnel
  renders it (e.g. 422), no exception ceremony

### Requirement: Jobs are transactional rows, workers are live goroutines
`keel.jobs` SHALL store jobs in the application's Postgres as
state-of-record; `enqueue!` on a tx handle SHALL commit atomically
with the caller's domain writes. Workers SHALL be goroutines woken by
LISTEN/NOTIFY with polling fallback; handler-map values SHALL accept
vars, derefed at dispatch, so job handlers are as live as http
handlers; retries, per-type concurrency, unique jobs, and cron SHALL
ride the same table. A `:memory` backend SHALL offer the identical
API for dev/tests. SIGTERM SHALL drain in-flight jobs before exit.

#### Scenario: no lost jobs
- **WHEN** a tx inserts a user and enqueues :email/welcome and then
  rolls back
- **THEN** neither the user row nor the job exists

#### Scenario: live job handlers
- **WHEN** a job handler var is re-`def`ed at the REPL
- **THEN** the next dispatched job runs the new definition

### Requirement: Cache is fetch-through with stampede suppression
`(keel.cache/fetch c key f)` SHALL return the cached value or invoke
`f` exactly once across concurrent callers for the same key
(singleflight), storing with the cache's TTL. The same protocol SHALL
be implemented by `local` and `redis` constructors.

#### Scenario: one fill under contention
- **WHEN** N goroutines fetch a cold key concurrently
- **THEN** `f` runs once and all N receive its value

### Requirement: AI models are config-resolved values with fallbacks
`(keel.ai/generate model opts)` SHALL return
`(ok {:text … :usage …})`/`(err e)` (`generate!` throws). Models SHALL
be resolved by step key from config — application code SHALL NOT
contain provider names. Declared fallback models SHALL be tried on
provider failure; JSON output SHALL use native provider modes; calls
SHALL carry timeout defaults and pass through one user-suppliable
interaction-log fn.

#### Scenario: provider swap is config-only
- **WHEN** the :summarizer step's provider changes in conf.edn
- **THEN** no application source changes

### Requirement: Guides gate tiers like code
Each tier SHALL NOT ship without its guides: T0/T1 the 15-minute
tutorial and http/html/config guides; T2 the data guide; T3 the
jobs/cache guides and the production checklist (drain, pool sizing,
timeouts); T4 the AI guide. Framework error messages SHALL meet the
ADR 0015 diagnostics bar.

#### Scenario: docs are a release blocker
- **WHEN** a tier's code is done but its guide is not
- **THEN** the tier does not ship
