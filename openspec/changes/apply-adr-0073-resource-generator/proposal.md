# apply-adr-0073-resource-generator

## Why

`cljgo new --template web` hands the author a runnable bri app, but every
new endpoint after that is the same hand-written boilerplate — a migration,
a model, five handlers, a routes value, a test. bri's pitch is "one-person
framework" (ADR 0041); the missing piece is the DHH-style scaffold that
turns a resource description into that whole vertical slice in one command.
ADR 0073 decides `cljgo generate resource <Name> <field:type>...`.

## What Changes

- New CLI verb `cljgo generate` (alias `g`) with one kind, `resource`,
  which scaffolds an authenticated CRUD resource into an existing bri web
  app (ADR 0073 §1).
- A field-type grammar — `string text int bool uuid timestamp references` —
  maps each field to a SQLite column and a Clojure coercion expression
  (ADR 0073 §2).
- The command CREATES a timestamped migration, a resource ns (model calling
  bri.db + handlers + a routes value), a one-time `app.db` datasource ns,
  and an in-process test; it EDITS `src/app/main.cljg` by splicing the
  require and routes value at two documented comment markers (ADR 0073
  §3/§4). It is idempotent and never clobbers user edits without `--force`.
- The `web` template's `src/app/main.cljg` gains the two markers
  (`;; cljgo:resource-requires`, `;; cljgo:resource-routes`) so the splice
  needs no s-expression parser.
- The emission templates are REAL FILES (`cmd/cljgo/resource_tmpl/*.tmpl`,
  `text/template`); a test generates a canonical resource, reader-validates
  every emitted file, and asserts the splice — the ADR 0047 anti-rot
  guarantee applied to the generator's output.

## Capabilities

### New Capabilities
- `resource-generator`: the `cljgo generate resource` command — its
  argument surface, the field-type grammar and its column/coercion mapping,
  which files are created vs edited, the marker-based route splice, the
  naming/pluralization rules, and the idempotency/no-clobber guarantees.

### Modified Capabilities

(none — no existing spec governs code scaffolding; `cljgo new` in the
app-framework spec covers project templates, not resource generation.)

## Impact

- `cmd/cljgo/generate.go` — the command, the field grammar, the splice.
- `cmd/cljgo/resource_tmpl/*.tmpl` — the emission templates (real files).
- `cmd/cljgo/main.go` — `generate`/`g` dispatch + usage line.
- `templates/web/src/app/main.cljg` — the two splice markers (comments;
  the page and routes are otherwise unchanged).
- `cmd/cljgo/generate_test.go` — the anti-rot gate.
- The generated model calls bri.db (ADR 0072), which lands in parallel; the
  generated `cljgo test` goes green once bri.db resolves. All bri.db call
  sites are confined to the resource model + `app.db` for a one-place
  reconciliation.
