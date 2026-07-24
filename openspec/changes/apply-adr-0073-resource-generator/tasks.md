# Tasks — apply ADR 0073 (resource generator)

## 1. Command + grammar
- [x] 1.1 `cmd/cljgo/generate.go`: `runGenerate` dispatch (`resource` kind) + `generate`/`g` wiring in `main.go` + usage line.
- [x] 1.2 Field-type grammar: `resolveField` maps `string/text/int/bool/uuid/timestamp/references` to a column decl + coercion expr; unknown types are named errors.
- [x] 1.3 `buildResourceData`: singular/plural, ns/table/route/keyword derivation, SET clause + args, sample body; `pluralize` + `validIdent`.

## 2. Emission templates (real files)
- [x] 2.1 `resource_tmpl/resource.cljg.tmpl` — model (bri.db) + handlers (bri.http, authenticated) + routes value.
- [x] 2.2 `resource_tmpl/migration.sql.tmpl` — `CREATE TABLE` + implicit id + `references` indexes.
- [x] 2.3 `resource_tmpl/db.cljg.tmpl` — the one-time `app.db` datasource (`delay` — no I/O at load).
- [x] 2.4 `resource_tmpl/resource_test.cljg.tmpl` — in-process bri.http CRUD suite.

## 3. Splice
- [x] 3.1 Two comment markers in `templates/web/src/app/main.cljg` (require + routes).
- [x] 3.2 `spliceMain`/`insertAboveMarker`: marker-based, idempotent, missing-marker named error; no s-expression parsing.
- [x] 3.3 Create vs edit: timestamped migration, resource ns, test, `app.db` once; `--force`/no-clobber for the resource ns.

## 4. Tests (gate)
- [x] 4.1 `generate_test.go`: generate a canonical resource into a rendered `web` app; assert files exist + reader-validate every emitted `.cljg`; assert migration SQL columns/index.
- [x] 4.2 Assert the splice (require + `notes/routes`), markers survive, spliced `main.cljg` reads as source.
- [x] 4.3 Idempotency: two resources, re-run no-clobber, `--force` no-duplicate splice.
- [x] 4.4 Outside-a-web-app error leaves no files; missing-marker named error; `resolveField`/`pluralize` unit tables.
- [x] 4.5 Gates green: `go build ./... && go vet ./... && gofmt -l && go test ./cmd/cljgo/ -count=1`.

## 5. Reconciliation (follow-up, owner/bri.db agent)
- [ ] 5.1 When bri.db (ADR 0072) freezes its API, align `db/connect|query|one|exec|insert!` names in `resource.cljg.tmpl` + `db.cljg.tmpl`, then flip the generated `cljgo test` to a green CI gate.
