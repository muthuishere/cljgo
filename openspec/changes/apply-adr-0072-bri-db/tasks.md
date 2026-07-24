# Tasks — apply-adr-0072-bri-db

## 1. The Go shims (pure Go, no pkg/eval)

- [ ] 1.1 `pkg/bri/db.go`: `installDBShims` interning the private primitives —
  `-db-open` `-db-close` `-db-query` `-db-exec` `-db-begin` `-db-commit`
  `-db-rollback` `-db-migration-files`. Imports `database/sql`,
  `modernc.org/sqlite`, `github.com/jackc/pgx/v5/stdlib`. No import of pkg/eval.
- [ ] 1.2 Handle model: `*dbHandle` wraps `*sql.DB`, `*txHandle` wraps
  `*sql.Tx`; both expose one `querier` (Query/Exec). Placeholder `?`→`$n`
  rewrite for Postgres (quote-aware). Params + rows convert Clojure↔driver
  (int64/float64/string/bool/nil, []byte→string, time.Time→RFC3339).
- [ ] 1.3 Rows→maps: `snake_case` column → `kebab-case` keyword key.

## 2. The Clojure API

- [ ] 2.1 `core/bri/db.cljg` (ns `bri.db`): `connect` `close!` `query` `one`
  `one!` `exec!` `insert!` `update!` `delete!` `tx` `with-rollback` `migrate!`
  `migrate-status`. `one!` throws `ex-info` tagged `:bri.db/not-found`.
- [ ] 2.2 Embed the source in `core/bri.go` (`//go:embed bri/db.cljg` →
  `BriDBSource`).

## 3. Wire into the AOT mechanism

- [ ] 3.1 Add the `bri.db` Spec to `pkg/bri.Specs()` (after bri.config; nothing
  bri depends on it at load time) with `install: installDBShims`.
- [ ] 3.2 Add `modernc.org/sqlite` + `pgx/v5` to the root `go.mod`; regenerate
  `pkg/briaot` via `go generate ./pkg/briaot` (adds `pkg/briaot/bridb`).
  `SynthGoMod` carries the new requires automatically.

## 4. Tests

- [ ] 4.1 `pkg/bri/db_test.go` (interpreter, in-memory SQLite): connect+query
  +params, insert!/last-insert-id, exec! rows-affected, tx commit, tx rollback
  on throw, migrate! apply + idempotent + status, one!/not-found, snake↔kebab.
- [ ] 4.2 Dual-mode parity: a small bri.db app run interpreted AND compiled
  returns identical results (mirror `spikes/s45-bri-aot-docker/bri/parity/`).
- [ ] 4.3 `pkg/briaot` drift test stays green (regenerated bridb committed).

## 5. Worked example

- [ ] 5.1 A notes/todo CRUD wiring bri.db into bri.http (GET list, POST create,
  DELETE) persisting to SQLite — proving GET/POST/DELETE persist, both modes.

## 6. Gates

- [ ] 6.1 `go build ./... && go vet ./... && gofmt -l pkg cmd conformance
  templates && go test ./pkg/bri/ ./pkg/briaot/... ./pkg/emit/ ./cmd/cljgo/
  -count=1`. Emitter/genbri touched ⇒ also `go test ./conformance/ -count=1
  -timeout 1800s`.
