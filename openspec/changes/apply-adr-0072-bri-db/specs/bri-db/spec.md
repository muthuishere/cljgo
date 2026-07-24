## ADDED Requirements

### Requirement: one connection API over two drivers

`(bri.db/connect opts)` SHALL return a plain-map handle usable by every other
bri.db verb, backed by a `database/sql` pool. The driver SHALL be pure-Go
`modernc.org/sqlite` by default (zero install) and `github.com/jackc/pgx/v5`
when a Postgres URL is given, selected by `opts` or by `APP_DB_URL` when `opts`
omits a driver — with no application-code change to swap. The SQLite path SHALL
support an in-memory database (`":memory:"`). No cgo driver is permitted (the
`CGO_ENABLED=0` static-binary constraint).

#### Scenario: connect defaults to zero-install SQLite

- **WHEN** `(bri.db/connect {:driver :sqlite :database ":memory:"})` is called
- **THEN** a usable handle is returned and subsequent `query`/`exec!` run
  against a private in-memory SQLite database

#### Scenario: the same code targets Postgres by URL

- **WHEN** `APP_DB_URL=postgres://…` is set and `(bri.db/connect)` is called
  with no explicit driver
- **THEN** the handle drives pgx, and the identical query/write verbs apply

### Requirement: parametrized, injection-safe query surface

Every read/write verb SHALL take a SQL string plus positional parameters; there
SHALL be no string-concatenation query surface on the blessed path.
Placeholders SHALL be written as `?` and bri.db SHALL rewrite them to `$n` for
Postgres (quote-aware) so one SQL string runs on both drivers. `query` SHALL
return a vector of maps; `one` SHALL return the first row map or `nil`; `one!`
SHALL return the first row or throw an `ex-info` tagged `:bri.db/not-found`;
`exec!` SHALL return `{:rows-affected n :last-insert-id id}`. Column names SHALL
map `snake_case` → `kebab-case` keyword keys.

#### Scenario: params bind, never interpolate

- **WHEN** `(bri.db/query db "select * from t where name = ?" "a'; drop table t")`
  runs
- **THEN** the value is bound as a parameter (no SQL injection) and rows whose
  `name` equals that literal string are returned

#### Scenario: snake_case columns become kebab-case keys

- **WHEN** a row has a `created_at` column
- **THEN** the returned map has key `:created-at`

#### Scenario: one! signals absence as a tagged error

- **WHEN** `(bri.db/one! db "select * from t where id = ?" 999)` matches no row
- **THEN** it throws an `ex-info` whose data is tagged `:bri.db/not-found`
  (fundable to a 404 by the ADR 0041 funnel)

### Requirement: data-shaped writers

`(bri.db/insert! db :table row-map)`, `(bri.db/update! db :table set-map
where-map)`, and `(bri.db/delete! db :table where-map)` SHALL build parametrized
SQL from the maps (kebab keys → snake columns), never string-concatenating
values. These names carry a trailing `!` and SHALL NOT reuse any `clojure.core`
name (precedence principle).

#### Scenario: insert! returns the last insert id

- **WHEN** `(bri.db/insert! db :notes {:title "hi" :body "there"})` runs against
  SQLite
- **THEN** a row is inserted and the result carries its `:last-insert-id`

### Requirement: transactions are a function boundary

`(bri.db/tx db (fn [t] …))` SHALL begin a transaction, bind a tx handle usable
by the identical read/write verbs, COMMIT on normal return, and ROLL BACK on any
thrown error (re-raising it). `(bri.db/with-rollback db (fn [t] …))` SHALL run
its body in a transaction that is ALWAYS rolled back (the per-test sandbox).

#### Scenario: tx commits on success

- **WHEN** a `tx` body inserts two rows and returns normally
- **THEN** both rows are visible after the transaction

#### Scenario: tx rolls back on throw

- **WHEN** a `tx` body inserts a row then throws
- **THEN** the row is NOT visible afterward and the error propagates

### Requirement: versioned forward-only migrations

Migration files named `<utc-timestamp>_<slug>.sql` in a directory SHALL be
applied by `(bri.db/migrate! db dir)` in ascending version order, each in its
own transaction, recorded in a `schema_migrations` table, and the operation
SHALL be idempotent (re-running applies nothing new). `(bri.db/migrate-status
db dir)` SHALL return `{:applied [...] :pending [...]}`.

#### Scenario: migrate applies pending then is idempotent

- **WHEN** `migrate!` runs against a dir with two unapplied files, then runs again
- **THEN** the first run applies both (schema present); the second run applies
  nothing and `migrate-status` reports both as `:applied`, none `:pending`

### Requirement: interpreted and compiled bri.db are byte-identical

A bri.db application run interpreted (`cljgo dev`) and AOT-compiled
(`CGO_ENABLED=0`) SHALL return identical results for the same operations —
connect, query, insert, transaction, migration. A divergence SHALL fail the
build (dual-harness discipline; a REPL↔binary divergence is a release blocker).

#### Scenario: a bri.db app agrees across modes

- **WHEN** the same connect → migrate → insert → query program is run
  interpreted and compiled
- **THEN** the query results are byte-identical between the two modes
