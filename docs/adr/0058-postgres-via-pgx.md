# ADR 0058 — Postgres via pgx is the production data pillar (bri.db)
Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23; evidence: spike
S25) · Realizes ADR 0041 §4 Data (T2). Paired with ADR 0057 (SQLite default).

## Context

ADR 0041 blessed "pgx + inline SQL + plain maps" for the data pillar; spike S25
proved it: pgx v5 is 2.18× `database/sql` on the single-row read, inline SQL
stays REPL-live (re-`def` a query on a running pool), and the snake↔kebab names
doctrine round-trips at +27% marshalling. ADR 0057 makes SQLite the zero-install
*default*; this ADR ratifies **Postgres as the production database** behind the
same `bri.db` API, closing S25's now-moot dev-provisioning question (0057
answers it).

## Decision

1. **`bri.db` over pgx v5** is the blessed production data layer. Verbs:
   `query`/`one`/`insert`/`update`/`delete`/`tx`; `one!` throws `:db/not-found`
   (funnel → 404, ADR 0041 error model). Plain maps in/out. **SQL strings are
   THE blessed form**; data-SQL composers are unblessed libraries.
2. **Names doctrine** (conformance-tested): snake_case columns ↔ kebab-case
   keywords, both directions, nothing else renamed.
3. **Casts return Result and run day-one** (mass assignment is off the blessed
   path). SQL-file migrations, UTC-timestamped, additive-only, via `cljgo
   migrate`; deployment embeds migrations (ADR 0021) — `./app migrate && ./app`.
4. Reached via a `require-go` Go-shim (like `net/http` in `pkg/bri`). Tests run
   the Ecto-Sandbox model: under `APP_PROFILE=test` each test is wrapped in a
   rolled-back transaction (same var, no `with-redefs`).

## Un-proven (S25 — design-only, not yet running code)

S25 measured the read path and the names round-trip; it did NOT build as running
code: `tx`/`cast`/`insert!`/`update`/`delete` (verbs designed, not exercised),
the per-test tx-rollback (Ecto-Sandbox) fixture, prepared-statement caching and
pool behaviour under concurrency, and JSON/array/enum marshalling. This ADR
blesses their *shape*; each is proven at implementation, dual-harness.

## Consequences

- Framework quality rides interop breadth/perf (ADR 0041) — aligned incentives.
- SQLite (0057) and Postgres share `bri.db` by construction; the **dialect seam
  (0057) is the price**, mitigated by `cljgo test --db=postgres`.
- S26's un-proven crash-mid-job at-least-once redelivery belongs to Jobs (T3),
  not here.
- `db/update` reuses `clojure.core/update`'s name but only as a namespace-
  qualified var (the `clojure.string/replace` convention, ADR 0056 filter #3) —
  not a shadow; the unqualified core `update` is unchanged.
- Not chosen: an ORM; `database/sql` as the default (it is the documented
  escape hatch); a broker-backed data layer.

**Constraint-filter #4 commitment (ADR 0056):** the names doctrine and every
blessed verb land with dual-harness conformance (`.clj` tests passing
interpreted AND AOT-compiled) and a perf budget (ADR 0024) on a representative
query workload, calibrated with the implementation.
