## ADDED Requirements

### Requirement: The first fifteen minutes are generated, not assembled
`cljgo new <name>` SHALL generate a runnable app in the blessed layout
— `src/app/main.cljg`, `src/app/`, `conf.edn` (and optionally
`conf.schema.edn`), `migrations/` (with a first migration), `public/`
(with a real stylesheet), `test/` (with one passing test),
`build.cljgo` — as plain files the user owns; nothing SHALL scan or
implicitly load them. The generated `main.cljg` SHALL be the golden
page of ADR 0041 trimmed only of pillars whose tier has not shipped —
the generator's output IS the advertised page, and each tier SHALL
update the generator in the same change that ships the pillar. `cljgo
dev` SHALL apply pending migrations, start the app, and attach an
nREPL. `cljgo new --with-auth` SHALL copy a complete session-based
password auth implementation (code + tests) into the app.

#### Scenario: new to styled page
- **WHEN** a user runs `cljgo new myapp && cd myapp && cljgo dev`
- **THEN** a styled HTML page (markup from keel.html, CSS served from
  `public/`) is served locally and an editor can connect to the
  printed nREPL port, with no file authored by hand

#### Scenario: page and generator never diverge
- **WHEN** a tier ships a pillar shown on the golden page
- **THEN** the same change updates `cljgo new` so the generated app
  includes it

### Requirement: The golden-path app is under a page and runs both modes
The complete small app of ADR 0041 (server + data routes + default
middleware + config + cast + db query + transactional background job +
cache + a rendered page) SHALL be expressible in under one page of
cljgo, SHALL run unmodified through the interpreter (`cljgo run`) and
as an AOT-compiled static binary, and SHALL be a conformance artifact
(divergence = release blocker, ADR 0007).

#### Scenario: dual harness
- **WHEN** the golden-path app runs interpreted and compiled against
  the same services
- **THEN** observable behavior is identical

### Requirement: No hidden call graph, no I/O at namespace load
keel SHALL expose only plain namespaces of plain functions. It SHALL
NOT scan, register by convention, proxy, or instantiate user code; the
only inversion is an adapter invoking a handler/job fn the user passed
explicitly (directly or as a var). Handles (pools, queues, caches)
SHALL be ordinary values — no ambient globals. Framework constructors
SHALL perform no I/O at construction: `config/load!` reads only its
file; `db/connect!` validates configuration and dials on first use;
readiness I/O (pinging the pool) happens in `http/serve` before
accepting traffic.

#### Scenario: requiring the app is side-effect-free
- **WHEN** a test requires `app.main` under `APP_PROFILE=test`
- **THEN** no network connection is attempted and no server starts

### Requirement: HTTP is the Ring contract on stdlib routing
A handler SHALL be a fn of request-map → response-map. Routes SHALL be
a data vector of `[pattern handler]` with Go ServeMux pattern strings;
`{name}` segments SHALL bind into `:params` as strings (coercion is
the app's visible call, e.g. `parse-long`). Middleware SHALL be
handler → handler fns. When `:middleware` is omitted,
`(keel.http/serve routes opts)` SHALL apply `(http/defaults)` —
access-log, recover, sessions (signed cookies), CSRF protection, JSON
negotiation; a supplied `:middleware` vector SHALL replace the
defaults wholesale (no merging). Production timeouts SHALL default ON;
SIGTERM SHALL drain gracefully with a deadline. `(http/dir path)`
SHALL serve static files. The underlying mux/server SHALL be reachable
as escape hatches. In dev mode the server SHALL warn when a route or
job handler is a plain fn rather than a var (silent non-liveness).

#### Scenario: security is what you didn't type
- **WHEN** an app serves routes without passing `:middleware`
- **THEN** a state-mutating POST without a valid CSRF token is
  rejected and responses carry the session cookie — with zero
  middleware code in the app

#### Scenario: routing without a router
- **WHEN** routes `[["GET /users/{id}" #'show]]` are served
- **THEN** `GET /users/7` invokes `show` with `{:params {:id "7"}}`
  and `POST /users/7` yields 405 — both from the stdlib matcher

#### Scenario: live handlers
- **WHEN** a route references `#'handler` and the var is re-`def`ed
  from a REPL/nREPL session against the live process
- **THEN** the next request observes the new definition without
  restart (var deref per request; plain-fn handlers skip the deref
  and dev mode warns about them)

### Requirement: HTML is a function over data
`keel.html` SHALL render hiccup-style vectors to escaped HTML
(`html/page` for full documents), XSS-safe by construction with an
explicit, visually loud unescape form. There SHALL be no template
language and no asset pipeline; static assets are files served by
`(http/dir "public")`.

#### Scenario: page from data
- **WHEN** a handler returns `(http/ok (html/page [:h1 greeting]))`
- **THEN** the client receives a complete HTML document with
  `greeting` escaped

### Requirement: Errors have one beginner surface and one documented funnel
keel fns that are expected to fail SHALL return Result; their `!`
variants SHALL unwrap-or-throw (the language-wide rule: plain =
value/Result, `!` = throws). All documentation entry paths and
generated code SHALL use `!` forms; `let?`/Result SHALL be documented
as the day-two upgrade. The `recover` middleware SHALL map errors to
responses through a SHIPPED, DOCUMENTED data table (default:
cast/validation → 422, not-found → 404, constraint violation → 409,
otherwise 500), overridable via `(http/recover {:error-map ...})`. A
handler that returns a raw Result value SHALL fail loudly in dev mode
(500 with an explanatory message), never be silently coerced.

#### Scenario: constraint violation without ceremony
- **WHEN** `db/insert!` inside a handler hits a unique-constraint
  violation
- **THEN** the funnel renders the mapped status (JSON or HTML per
  negotiation) with no error-handling code in the handler

#### Scenario: the funnel does not launder type confusion
- **WHEN** a handler returns `(err e)` as its response in dev mode
- **THEN** the response is a 500 explaining that a Result must be
  unwrapped, not a quietly derived status

### Requirement: Configuration is two layers into one plain map
`(keel.config/load!)` SHALL merge `conf.edn` (whose `:profiles`
section, selected by `APP_PROFILE`, overlays the base map) and then
`APP_*` environment variables (`APP_DB_HOST` → `[:db :host]`),
yielding one plain map — two layers, file then env. A
`conf.schema.edn`, when present, SHALL declare defaults, required
keys, and types/coercions; a violation SHALL abort boot with a
diagnostic naming the key and layer. `cljgo config` SHALL print the
resolved map annotated with each key's winning layer.

#### Scenario: misconfigured deploy must not boot
- **WHEN** a schema-required key is absent from file and env
- **THEN** `load!` throws before any server/pool/worker starts

#### Scenario: 2 a.m. debugging
- **WHEN** an operator runs `cljgo config` in the app directory
- **THEN** every key shows its effective value and the layer that won

### Requirement: Data layer is SQL and maps, never an ORM
`keel.db` SHALL provide connect!/query/one/insert/update/delete/tx
over pgx with sane pool and timeout defaults: SQL as strings (THE
blessed form), rows as plain maps, results as Result values with `!`
variants. Schema casts SHALL return `(ok row)`/`(err {field message})`
and SHALL be the blessed path for ALL external input — the golden page
and every doc SHALL cast request bodies before insert. Migrations
SHALL be SQL files with UTC-timestamp names driven by `cljgo migrate`.

#### Scenario: mass assignment is off the blessed path
- **WHEN** the golden page's signup handler receives a body with an
  undeclared `:admin` key
- **THEN** `db/cast!` drops/rejects it per the declared schema before
  any SQL runs

#### Scenario: railway signup (day two)
- **WHEN** an insert violates a constraint inside a `let?` chain
- **THEN** the chain short-circuits with `(err e)` and the funnel
  renders it, no exception ceremony

### Requirement: Jobs are transactional rows, workers are live goroutines
`keel.jobs` SHALL store jobs in the application's Postgres as
state-of-record; `enqueue!` on a tx handle SHALL commit atomically
with the caller's domain writes, and SHALL validate the job type
against the queue's registered handlers so a typo fails at the enqueue
site. Workers SHALL be goroutines woken by LISTEN/NOTIFY with polling
fallback; handler-map values SHALL accept vars, derefed at dispatch,
so job handlers are as live as http handlers; retries, per-type
concurrency, unique jobs, and cron SHALL ride the same table. A
`:memory` backend SHALL offer the identical API for dev/tests, built
on core.async channels per ADR 0040. SIGTERM SHALL drain in-flight
jobs before exit.

#### Scenario: no lost jobs
- **WHEN** a tx inserts a user and enqueues :email/welcome and then
  rolls back
- **THEN** neither the user row nor the job exists

#### Scenario: enqueue typo fails at the call site
- **WHEN** code enqueues `:email/welcom` and no such handler is
  registered
- **THEN** `enqueue!` throws immediately, naming the known job types

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

### Requirement: AI is a config-resolved, independently versioned satellite
`keel.ai` SHALL ship with the toolchain but version independently of
the other keel namespaces. `(keel.ai/generate model opts)` SHALL
return `(ok {:text … :usage …})`/`(err e)` (`generate!` throws).
Models SHALL be resolved by step key from config — application code
SHALL NOT contain provider names. Declared fallback models SHALL be
tried on provider failure; JSON output SHALL use native provider
modes; calls SHALL carry timeout defaults and pass through one
user-suppliable interaction-log fn. Documentation SHALL show AI calls
in jobs, never inline in request handlers.

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
