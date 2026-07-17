## ADDED Requirements

### Requirement: The first fifteen minutes are generated, not assembled
`cljgo new <name>` SHALL generate a runnable app in the blessed layout
— `src/app/main.cljg`, `src/app/`, `conf.edn` (and optionally
`conf.schema.edn`), `public/` (with a real stylesheet), `test/` (with
one passing test), `build.cljgo`, and (from T2) `migrations/` with a
first migration — as plain files the user owns; nothing SHALL scan or
implicitly load them. The generated `main.cljg` SHALL be the golden
page of ADR 0041 trimmed only of pillars whose tier has not shipped —
the generator's output IS the advertised page, every generated verb
SHALL have a same-tier implementation, and each tier SHALL update the
generator in the same change that ships the pillar. `cljgo dev` SHALL
start the app and attach an nREPL; from T2 it SHALL also provision the
embedded-Postgres dev database when `APP_DB_URL` is unset and apply
pending migrations. `cljgo new --with-auth` SHALL copy a complete
session-based password auth implementation (code + tests) into the
app.

#### Scenario: new to styled page
- **WHEN** a user runs `cljgo new myapp && cd myapp && cljgo dev`
- **THEN** a styled HTML page (markup from keel.html, CSS served from
  `public/`) is served locally and an editor can connect to the
  printed nREPL port, with no file authored by hand

#### Scenario: zero-install database
- **WHEN** `cljgo dev` runs (T2+) with no `APP_DB_URL` set and no
  Postgres installed on the machine
- **THEN** an embedded Postgres is provisioned under `.dev/pg/`,
  migrations apply, and the app's queries work — dev/prod parity with
  nothing installed

#### Scenario: page and generator never diverge
- **WHEN** a tier ships a pillar shown on the golden page
- **THEN** the same change updates `cljgo new` so the generated app
  includes it

### Requirement: Templates are real files, embedded, and CI runs them
The app `cljgo new` generates SHALL exist in the repository as a
TEMPLATE: a directory of real, runnable source files (`templates/<name>/`),
never as string literals in the generator. The template tree SHALL be
embedded in the `cljgo` binary, so generating is offline, zero-install,
and version-matched to the toolchain (no first-run fetch). Templates
SHALL be valid source WITHOUT substitution — the app name is a real
default name (`newapp`) that the generator renames, in file contents and
in path names, and that rename SHALL be the only substitution mechanism.
`cljgo new --template <name|path>` SHALL accept a built-in template name
or a local template directory; a git URL SHALL be refused with an honest
error until it is implemented. CI SHALL generate the built-in template,
run its test, boot it, and fetch its pages inside the normal test gates.

#### Scenario: the template cannot rot
- **WHEN** keel's API changes in a way the generated app does not follow
- **THEN** the gate test that generates, `cljgo test`s, boots and curls
  the template fails — the breakage cannot ship silently

#### Scenario: generating is offline
- **WHEN** a user runs `cljgo new myapp` on a machine with no network
- **THEN** the app is generated from the binary's embedded template

#### Scenario: an alternate template
- **WHEN** a user runs `cljgo new myapp --template ./our-template`
- **THEN** the app is generated from that directory, with the same
  rename applied

#### Scenario: a git URL is refused, not half-done
- **WHEN** a user passes `--template https://github.com/x/y.git`
- **THEN** `cljgo new` refuses with an error naming the supported forms

### Requirement: The golden-path app is under a page and runs both modes
The complete small app of ADR 0041 (server + data routes + default
middleware + config + cast + db query + transactional background job +
cache + a rendered page + the migrate/serve entrypoint) SHALL be
expressible in under one page of cljgo, SHALL run unmodified through
the interpreter (`cljgo run`) and as an AOT-compiled static binary,
SHALL satisfy every requirement in this spec (including
no-I/O-at-load and visible shutdown wiring), and SHALL be a
conformance artifact (divergence = release blocker, ADR 0007).

#### Scenario: dual harness
- **WHEN** the golden-path app runs interpreted and compiled against
  the same services
- **THEN** observable behavior is identical

### Requirement: No hidden call graph, no I/O at namespace load
keel SHALL expose only plain namespaces of plain functions. It SHALL
NOT scan, register by convention, proxy, or instantiate user code; the
only inversion is an adapter invoking a handler/job fn the user passed
explicitly (directly or as a var). Handles (pools, queue registries,
caches) SHALL be ordinary values. Framework constructors SHALL perform
no I/O: `config/load!` reads only its file; `db/connect!` validates
configuration and dials on first use; `jobs/queue` builds a registry
value. Effectful starts (`jobs/start!`, `http/serve`) belong in
`-main`; readiness I/O (pinging the pool) happens in `http/serve`
before accepting traffic. Shutdown SHALL be composed visibly:
`http/serve` drains the handles passed in `:drain` after in-flight
requests — there SHALL be no ambient shutdown registry.

#### Scenario: requiring the app is side-effect-free
- **WHEN** a test requires `app.main` under `APP_PROFILE=test`
- **THEN** no network connection is attempted, no worker starts, and
  no server starts

#### Scenario: shutdown is on the page
- **WHEN** SIGTERM arrives with `(http/serve routes {:drain
  [workers]})` running
- **THEN** in-flight requests complete (within the deadline), then
  `workers` drains — because the page said so, not because a registry
  knew

### Requirement: HTTP is the Ring contract on stdlib routing
A handler SHALL be a fn of request-map → response-map. Routes SHALL be
a data vector of `[pattern handler]` with Go ServeMux pattern strings;
`{name}` segments SHALL bind into `:params` as strings, and
`(http/param! req :name :int)` SHALL be the blessed typed accessor
(failure maps through the error table as 400). Middleware SHALL be
handler → handler fns. When `:middleware` is omitted,
`(keel.http/serve routes opts)` SHALL apply `(http/defaults)` —
access-log, recover, sessions (signed cookies), CSRF protection, JSON
negotiation. `(http/defaults)` SHALL return inspectable DATA (a
vector supporting conj/removal by name); `cljgo routes` SHALL print
the effective stack; dev mode SHALL warn when a supplied stack lacks
`recover` or `csrf`. Production timeouts SHALL default ON; SIGTERM
SHALL drain gracefully with a deadline. `(http/dir path)` SHALL serve
static files. The underlying mux/server SHALL be reachable as escape
hatches. In dev mode the server SHALL warn when a route or job
handler is a plain fn rather than a var (silent non-liveness).

#### Scenario: security is what you didn't type
- **WHEN** an app serves routes without passing `:middleware`
- **THEN** a session-bearing state-mutating POST without a valid CSRF
  token is rejected and responses carry the session cookie — with
  zero middleware code in the app

#### Scenario: the tutorial curl still works
- **WHEN** a client POSTs JSON with no session cookie
- **THEN** the request passes CSRF (nothing to forge without a
  session) and the handler runs — the documented API posture

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

#### Scenario: bad path param is a 400, not a stack trace
- **WHEN** `GET /users/abc` reaches a handler using
  `(http/param! req :id :int)`
- **THEN** the response is the error table's 400, with no
  error-handling code in the handler

### Requirement: HTML is a function over data
`keel.html` SHALL render hiccup-style vectors to escaped HTML
(`html/page` for full documents), XSS-safe by construction with an
explicit, visually loud unescape form. `html/form` SHALL emit the
CSRF token for session-bearing browsers and is the outer boundary of
the HTML surface: no layouts, no partials, no template language, no
asset pipeline — static assets are files served by
`(http/dir "public")`.

#### Scenario: page from data
- **WHEN** a handler returns `(http/ok (html/page [:h1 greeting]))`
- **THEN** the client receives a complete HTML document with
  `greeting` escaped

#### Scenario: forms carry the token
- **WHEN** a page renders `(html/form {:post "/signup"} ...)`
- **THEN** the emitted form includes the CSRF token and its browser
  POST passes the default middleware

### Requirement: Errors have one blessed surface and one documented funnel
App handlers SHALL use `!` variants (unwrap-or-throw) with the
`recover` funnel — THE blessed surface; all documentation entry paths
and generated code use it. keel fns that are expected to fail SHALL
also exist in plain form returning Result (the language-wide rule:
plain = value/Result, `!` = throws); Result values SHALL cross the
http boundary only through `(http/render result-expr)` — the visible
bridge that maps `(ok resp)` to the response and `(err e)` through
the error table. A handler that returns a bare Result WITHOUT
`http/render` SHALL fail loudly in dev mode (500 with an explanatory
message), never be silently coerced. The `recover` funnel SHALL map
errors through a SHIPPED, DOCUMENTED data table (default:
`:http/bad-param` → 400, cast/validation → 422, not-found → 404,
constraint violation → 409, otherwise 500), overridable via
`(http/recover {:error-map ...})`.

#### Scenario: constraint violation without ceremony
- **WHEN** `db/insert!` inside a handler hits a unique-constraint
  violation
- **THEN** the funnel renders the mapped status (JSON or HTML per
  negotiation) with no error-handling code in the handler

#### Scenario: the railway crosses on a visible bridge
- **WHEN** a handler wraps a `let?` chain in `http/render` and a
  binding yields `(err e)`
- **THEN** the funnel renders it through the same table

#### Scenario: the funnel does not launder type confusion
- **WHEN** a handler returns `(err e)` without `http/render` in dev
  mode
- **THEN** the response is a 500 explaining that a Result must pass
  through `http/render`, not a quietly derived status

### Requirement: Configuration is two layers into one plain map
`(keel.config/load!)` SHALL merge `conf.edn` (whose `:profiles`
section, selected by `APP_PROFILE`, overlays the base map) and then
`APP_*` environment variables, yielding one plain map — two layers,
file then env. The env mapping SHALL be deterministic: `__` separates
path segments and `_` joins words (`APP_DB__POOL_SIZE` →
`[:db :pool-size]`). Durations and sizes SHALL be numbers (seconds,
bytes), not strings. A `conf.schema.edn`, when present, SHALL declare
defaults, required keys, and types/coercions; a violation SHALL abort
boot with a diagnostic naming the key and layer. `cljgo config` SHALL
print the resolved map annotated with each key's winning layer.

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
variants; `db/one!` SHALL throw `:db/not-found` on a missing row
(funnel: 404). A NAMES DOCTRINE SHALL be documented and
conformance-tested: snake_case columns ↔ kebab-case keywords in both
directions, unqualified table keywords in write fns, and no other
renaming ever. Schema casts SHALL return `(ok row)`/`(err {field
message})` and SHALL be the blessed path for ALL external input — the
golden page and every doc SHALL cast request bodies before insert.
Migrations SHALL be SQL files with UTC-timestamp names driven by
`cljgo migrate`.

#### Scenario: mass assignment is off the blessed path
- **WHEN** the golden page's signup handler receives a body with an
  undeclared `:admin` key
- **THEN** `db/cast!` drops/rejects it per the declared schema before
  any SQL runs

#### Scenario: names round-trip
- **WHEN** a row with column `created_at` is read and a map with
  `:created-at` is written
- **THEN** both map to the same column, per the tested doctrine

### Requirement: One binary deploys
`cljgo build` SHALL embed `public/` and `migrations/` into the
compiled binary (ADR 0021 comptime embed), and the generated `-main`
SHALL answer a `migrate` argument by applying pending migrations —
`./myapp migrate && ./myapp` SHALL be a complete deployment against a
configured database, with no files carried alongside the binary.

#### Scenario: the binary travels alone
- **WHEN** the compiled binary is copied to a clean host with only
  `APP_*` env set
- **THEN** `./myapp migrate` applies embedded migrations and
  `./myapp` serves, including static assets, with no other files
  present

### Requirement: Tests load the app and roll back
Under `APP_PROFILE=test`, the pool returned by `db/connect!` SHALL
operate in the sandbox model: one connection per test wrapping it in
a transaction rolled back at test end — the SAME top-level pool var
the app defines, no `with-redefs`. keel SHALL ship an in-process http
test client and a `:memory`-jobs drain-and-assert helper; `cljgo new`
SHALL generate one passing test using the http test client.

#### Scenario: rollback fixture through the app's own var
- **WHEN** two tests each insert a user through handlers closing over
  the app's `pg`
- **THEN** neither sees the other's row and the table is unchanged
  after the run

### Requirement: Jobs are transactional rows, workers are live goroutines
`(keel.jobs/queue handlers)` SHALL be a pure registry value whose
handler values accept vars, derefed at dispatch, so job handlers are
as live as http handlers. `(jobs/start! pg q)` — called in `-main` —
SHALL start goroutine workers woken by LISTEN/NOTIFY with polling
fallback and return a drainable handle. Jobs SHALL be rows in the
application's Postgres (state-of-record); `(jobs/enqueue! tx q type
payload)` SHALL commit atomically with the caller's domain writes and
SHALL validate `type` against the queue's registry so a typo fails at
the enqueue site. Retries, per-type concurrency, unique jobs, and
cron SHALL ride the same table. Dev SHALL run the real Postgres
backend (parity via the embedded dev database); a `:memory` backend
with the identical API (core.async channels per ADR 0040) SHALL exist
for tests only.

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
(singleflight), storing with the cache's TTL (a number of seconds).
The same protocol SHALL be implemented by `local` and `redis`
constructors.

#### Scenario: one fill under contention
- **WHEN** N goroutines fetch a cold key concurrently
- **THEN** `f` runs once and all N receive its value

### Requirement: Guides gate tiers like code
Each tier SHALL NOT ship without its guides: T0/T1 the 15-minute
tutorial and http/html/config guides; T2 the data guide (including
the names doctrine) and the deployment guide; T3 the jobs/cache
guides and the production checklist (drain, pool sizing, timeouts).
Framework error messages SHALL meet the ADR 0015 diagnostics bar.

#### Scenario: docs are a release blocker
- **WHEN** a tier's code is done but its guide is not
- **THEN** the tier does not ship
