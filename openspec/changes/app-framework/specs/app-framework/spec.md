## ADDED Requirements

### Requirement: The golden-path app is under a page and runs both modes
The complete small app of ADR 0041 (server + data routes + middleware +
config + db query + transactional background job + cache + one AI call)
SHALL be expressible in under one page of cljgo, SHALL run unmodified
through the interpreter (`cljgo run`) and as an AOT-compiled static
binary, and SHALL be a conformance artifact (divergence = release
blocker, ADR 0007).

#### Scenario: dual harness
- **WHEN** the golden-path app runs interpreted and compiled against
  the same services
- **THEN** observable behavior is identical

### Requirement: keel never calls user code except through handed-in fns
keel SHALL expose only plain namespaces of plain functions. It SHALL
NOT scan, register by convention, proxy, or instantiate user code; the
only inversion is an adapter invoking a handler/job fn the user passed
explicitly (directly or as a var).

#### Scenario: boot order is main
- **WHEN** an app boots
- **THEN** every framework effect traces to a call in the user's
  `-main` (or file top-level), in source order

### Requirement: HTTP is the Ring contract on stdlib routing
A handler SHALL be a fn of request-map → response-map. Routes SHALL be
a data vector of `[pattern handler]` with Go ServeMux pattern strings;
`{name}` segments SHALL bind into `:params`. Middleware SHALL be
handler → handler fns applied in the order of an explicit vector.
`(keel.http/serve routes opts)` SHALL return a stop fn and expose the
underlying mux/server as escape hatches.

#### Scenario: routing without a router
- **WHEN** routes `[["GET /users/{id}" #'show]]` are served
- **THEN** `GET /users/7` invokes `show` with `{:params {:id "7"}}`
  and `POST /users/7` yields 405 — both from the stdlib matcher

#### Scenario: live handlers
- **WHEN** a route references `#'handler` and the var is re-`def`ed
  from a REPL/nREPL session against the live process
- **THEN** the next request observes the new definition without
  restart (var deref per request; plain-fn handlers skip the deref)

### Requirement: Configuration is four layers into one plain map
`(keel.config/load!)` SHALL merge, in increasing precedence: schema
defaults → conf.edn → conf.<profile>.edn (profile from APP_PROFILE) →
`APP_*` environment variables (`APP_DB_HOST` → `[:db :host]`, coerced
per schema), yielding one plain map. A declared-required key that is
missing or ill-typed SHALL abort boot with a diagnostic naming the key
and layer.

#### Scenario: misconfigured deploy must not boot
- **WHEN** a required key is absent from all layers
- **THEN** `load!` throws before any server/pool/worker starts

### Requirement: Data layer is SQL and maps, never an ORM
`keel.db` SHALL provide connect/query/one/insert/update/delete/tx over
pgx: SQL as strings or data, rows as plain maps, results as Result
values composing with `let?`; `!` variants unwrap-or-throw (the
language-wide convention). Schema casts SHALL return
`(ok row)`/`(err {field message})`. Migrations SHALL be SQL files with
UTC-timestamp names driven by `cljgo migrate`.

#### Scenario: railway signup
- **WHEN** an insert violates a constraint inside a `let?` chain
- **THEN** the chain short-circuits with `(err e)` and the http error
  funnel renders it (e.g. 422), no exception ceremony

### Requirement: Jobs are transactional rows, workers are goroutines
`keel.jobs` SHALL store jobs in the application's Postgres as
state-of-record; `enqueue` on a tx handle SHALL commit atomically with
the caller's domain writes. Workers SHALL be goroutines woken by
LISTEN/NOTIFY with polling fallback; handlers SHALL be a map sealed at
`(jobs/start pg handlers)`; retries, per-type concurrency, unique
jobs, and cron SHALL ride the same table. A `:memory` backend SHALL
offer the identical API for dev/tests.

#### Scenario: no lost jobs
- **WHEN** a tx inserts a user and enqueues :email/welcome and then
  rolls back
- **THEN** neither the user row nor the job exists

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
`(ok {:text … :usage …})`/`(err e)`. Models SHALL be resolved by step
key from config — application code SHALL NOT contain provider names.
Declared fallback models SHALL be tried on provider failure; JSON
output SHALL use native provider modes; every call SHALL pass through
one user-suppliable interaction-log fn.

#### Scenario: provider swap is config-only
- **WHEN** the :summarizer step's provider changes in conf.edn
- **THEN** no application source changes
