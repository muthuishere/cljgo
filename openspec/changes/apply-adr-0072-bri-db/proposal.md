# apply-adr-0072-bri-db

## Why

ADR 0072 (docs/adr/0072-bri-db-data-layer.md) makes bri "first-class" on the
data tier (ADR 0041 §4 T2): a blessed, one-way data layer — connect, query,
transact, migrate — that works in BOTH modes, interpreted (`cljgo dev`) and
AOT-compiled to a single `CGO_ENABLED=0` static binary, byte-identical. ADR
0057 blessed pure-Go `modernc.org/sqlite` as the zero-install default and ADR
0058 blessed `github.com/jackc/pgx/v5` for production, but both left `bri.db`
design-only ("un-proven": `tx`/`insert!`/`update`/`delete`, migrations, the
test sandbox). bri went AOT in PR #116 (ADR 0071): its `.cljg` sources live in
`core/bri/*.cljg`, its pure-Go shims in `pkg/bri`, AOT-generated into
`pkg/briaot` by `cmd/genbri`, with external deps carried into every emitted
binary by `SynthGoMod`. bri.db MUST follow that pattern or it breaks the
flagship single-binary deploy story.

## What Changes

- **`bri.db` becomes a real bri namespace.** `core/bri/db.cljg` (the Clojure
  API) + `installDBShims` in pure-Go `pkg/bri` (the `database/sql` + modernc +
  pgx driving), added to `bri.Specs()` and AOT-generated into `pkg/briaot/bridb`
  by `cmd/genbri`. `(require '[bri.db])` resolves interpreted AND compiled.
- **One connection/pool API over two drivers.** `(db/connect opts)` →
  `{:bri.db/handle H :driver …}`; SQLite (`.dev/app.db` default, or `:memory:`)
  when `APP_DB_URL` unset, pgx when `postgres://…`. Zero app-code change to swap.
- **Injection-safe query surface.** `query`/`one`/`one!`/`exec!` — SQL string +
  positional `?` params only, no string-concat path. bri.db rewrites `?`→`$n`
  for Postgres (quote-aware). Rows → vector of maps, `snake_case`→`kebab-case`
  keyword keys.
- **Data-shaped writers + transactions.** `insert!`/`update!`/`delete!` build
  parametrized SQL from a map; `(tx db (fn [t] …))` commits on return, rolls
  back on throw; every verb accepts a db OR tx handle.
- **Migrations.** `<utc>_<slug>.sql` files, `(migrate! db "migrations")` applies
  pending in order each in its own tx, idempotent, tracked in
  `schema_migrations`; `(migrate-status db dir)` → `{:applied :pending}`.
- **Test sandbox.** In-memory SQLite per test; `(with-rollback db f)` runs a
  body in an always-rolled-back transaction (Ecto-Sandbox shape).
- **Wiring.** `SynthGoMod` carries `modernc.org/sqlite` + `pgx` into every
  emitted module (it already carries the runtime's external requires); the
  drivers link into a `CGO_ENABLED=0` binary with no extra emitter work.
- **A worked example.** A notes CRUD (GET/POST/DELETE persisting to SQLite)
  proves the layer serves data in both modes.

## Non-goals

- An ORM, a query-builder DSL, or mass-assignment on the blessed path (ADR
  0058: casts/composers are unblessed libraries).
- Down-migrations (additive-only per ADR 0058 dec 3).
- Rewriting SQL dialect across drivers — the ADR 0057 seam stays; placeholder +
  name normalization only.
- Delivering the `cljgo test --db=postgres` CI gate (tracked by ADR 0057/0058;
  the drivers and API land here, the CI matrix is a follow-up).
- A build-tag opt-out so db-less bri apps skip the SQLite engine (future
  size optimization; ADR 0072 consequences).
