# ADR 0072 — bri.db is the one blessed data layer

Date: 2026-07-24 · Status: accepted (owner-directed; realizes ADR 0041 §4 Data
tier T2). Builds on ADR 0057 (SQLite pure-Go default), ADR 0058 (Postgres via
pgx), ADR 0069 (bri API-first), ADR 0071 (bri AOT static binary).

## Context

ADR 0057/0058 blessed the data *drivers* (pure-Go `modernc.org/sqlite` as the
zero-install default, `github.com/jackc/pgx/v5` as the production pillar) and
sketched the `bri.db` verb surface, but left it design-only ("un-proven" in
0058: `tx`/`insert!`/`update`/`delete`, migrations, the test sandbox). bri went
AOT in PR #116 (ADR 0071): its Clojure half lives in `core/bri/*.cljg`, its
pure-Go shims in `pkg/bri`, AOT-generated into `pkg/briaot` by `cmd/genbri`, and
its external deps flow into every emitted binary through `SynthGoMod`. bri.db
must plug into that SAME mechanism so `(require '[bri.db])` works identically
interpreted (`cljgo dev`) and AOT-compiled (`CGO_ENABLED=0` static binary) —
REPL↔binary divergence is the release blocker.

## Decision

1. **One connection API, two drivers, no fork.** `(db/connect opts)` returns a
   plain map handle `{:bri.db/handle H :driver :sqlite|:postgres}`. The driver
   is chosen by `opts` or, when omitted, by `APP_DB_URL`: unset ⇒ pure-Go SQLite
   at `.dev/app.db` (WAL); `postgres://…` ⇒ pgx. Zero app-code change to swap
   (ADR 0057 dec 1). `database/sql` is the host underneath both — `*sql.DB` is a
   pool, so `connect` IS the pool.

2. **Parametrized only — injection-safe by construction.** Every verb takes a
   SQL string plus positional params; there is no string-concatenation surface
   on the blessed path. Users always write `?` placeholders; bri.db rewrites
   `?`→`$n` for Postgres (quote-aware scan), so the same SQL string runs on both
   drivers. SQL *dialect* is NOT rewritten (the ADR 0057 seam).

3. **Query surface** (kebab verbs, plain data out):
   - `(query db sql & params)` → vector of maps, `snake_case`→`kebab-case`
     keyword keys (ADR 0058 names doctrine, both directions).
   - `(one db sql & params)` → first row map, or `nil`.
   - `(one! db sql & params)` → first row, or throws `ex-info` tagged
     `:bri.db/not-found` (the ADR 0041 funnel → 404).
   - `(exec! db sql & params)` → `{:rows-affected n :last-insert-id id}`.

4. **Data-shaped writers** (build the SQL from a map, still parametrized):
   `(insert! db :table row-map)`, `(update! db :table set-map where-map)`,
   `(delete! db :table where-map)`. `insert!` returns the row map echoed with
   its `:last-insert-id` (SQLite) — the one blessed insert. Bulk/`RETURNING`/
   `ON CONFLICT` stay hand-written SQL through `exec!`/`query`.

   **Precedence note:** ADR 0058 sketched `db/update`/`db/delete`. This ADR
   renames them to **`update!`/`delete!`** (bang = mutation), so bri.db reuses
   NO `clojure.core` name at all — strengthening the precedence principle
   beyond 0058's namespace-qualified allowance. `insert!` already had the bang.

5. **Transactions are a function boundary.** `(tx db (fn [t] …))` begins,
   binds a tx-handle map `{:bri.db/handle TX :driver … :tx true}`, runs the
   body, **commits on normal return, rolls back on any throw** (re-raising).
   Every read/write verb accepts either a db handle or a tx handle (both
   `*sql.DB` and `*sql.Tx` satisfy the same querier interface), so the body
   uses the identical API. Nested `tx` on a tx-handle runs inline (no
   savepoints on the blessed path).

6. **Migrations: versioned `.sql` files, forward-only, tracked.** Files in a
   `migrations/` dir named `<utc-timestamp>_<slug>.sql` (e.g.
   `20260724120000_create_notes.sql`). `(migrate! db "migrations")` ensures a
   `schema_migrations(version, applied_at)` table, applies every pending file
   **in ascending version order, each in its own transaction**, and is
   **idempotent** (re-running applies nothing). `(migrate-status db "migrations")`
   → `{:applied [...] :pending [...]}`. Deployment: `./app migrate && ./app`
   (ADR 0058 dec 3). No down-migrations on the blessed path (additive-only).

7. **Test sandbox = in-memory SQLite per test.** `(db/connect {:driver :sqlite
   :database ":memory:"})` gives a private, zero-file database; a fresh handle
   per test is the isolation. `(with-rollback db (fn [t] …))` runs a body in a
   transaction that is ALWAYS rolled back (the Ecto-Sandbox shape, ADR 0058
   dec 4) — same var, no `with-redefs`.

8. **AOT parity by construction.** `bri.db` is a normal bri namespace:
   `core/bri/db.cljg` + `installDBShims` in pure-Go `pkg/bri` (importing
   `database/sql`, `modernc.org/sqlite`, `pgx/v5/stdlib`), added to
   `bri.Specs()` and regenerated into `pkg/briaot/bridb`. `SynthGoMod` already
   carries the runtime's external requires into every emitted module, so the
   drivers link into a `CGO_ENABLED=0` binary with no extra wiring. The one Go
   shim implementation both modes share makes dual-mode parity structural.

## Consequences

- **Result mapping is fixed and simple:** DB scalars → Clojure data as
  `int64`/`float64`/`string`/`bool`/`nil`; `[]byte`→`string`; `time.Time`→
  RFC3339 string (JVM-free, deterministic across modes). No driver types leak.
- **Every bri binary links the SQLite engine (+~7 MB, ADR 0057) and pgx**,
  because `bri.Specs()` references `installDBShims` unconditionally and the
  briaot loader registers `bri.db` for all bri apps. This is the ADR 0057
  signed-off cost; a build-tag opt-out for db-less bri apps is a future
  optimization, not part of this ADR.
- **The dialect seam stays (ADR 0057):** placeholder + name normalization is
  handled; SQL dialect is not. `cljgo test --db=postgres` remains the gate that
  proves prod parity (tracked, not delivered here).
- **No JVM oracle:** bri.db does not exist in Clojure 1.12.5, so its behavior
  suite lives as Go tests in `pkg/bri` (like the rest of bri) plus a dual-mode
  parity harness (a bri.db app run interpreted AND compiled, identical output),
  not in `conformance/tests`.
- Not chosen: an ORM; exposing `*sql.DB` directly; a query-builder DSL on the
  blessed path; down-migrations; cgo `mattn/go-sqlite3` (breaks the static
  binary — the whole point).
