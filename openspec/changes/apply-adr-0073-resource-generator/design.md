# Design — resource generator (ADR 0073)

## A code generator, not a file copy

`cljgo new` renders fixed project templates with one substitution
(`newapp` → name). A resource scaffold is field-parametrized, so it cannot
be a pure copy. It stays honest to ADR 0047 ("templates are real source")
two ways: the emission templates are REAL FILES (`resource_tmpl/*.tmpl`,
`text/template`, embedded in `cmd/cljgo` — NOT in `templates.FS`, so the
project-template invariants in `templates_test.go` are untouched), and
`generate_test.go` reader-validates every emitted file, so the generator's
OUTPUT cannot rot silently.

## The three-layer generated slice

- `db/migrations/<ts>_create_<plural>.sql` — the table; timestamped so
  migrations are append-only.
- `src/app/<plural>.cljg` — model (the only bri.db caller, `ds`-parametrized)
  + handlers (bri.http, JSON, guarded) + a `routes` value.
- `src/app/db.cljg` (once) — the single datasource `(def ds (delay
  (db/connect …)))`. `delay` keeps the no-I/O-at-load contract that
  `require` / `cljgo test` depend on; the pool opens on first deref.
- `test/app/<plural>_test.cljg` — in-process bri.http CRUD suite.

## Splice without a parser

Editing `main.cljg` by parsing its s-expressions is fragile. Instead the
`web` template carries two comment markers; `generate` inserts each new
line above its marker with a string splice (`insertAboveMarker`), keeping
the marker for the next resource. Markers are comments, so the file is
valid source with or without them, and `notes/routes` composes because
`http/routes` concatenates route values (ADR 0069). A missing marker is a
named error listing the lines to add — never a guessed edit.

## bri.db is a one-place reconciliation surface

The generated model targets the documented bri.db surface (`connect`,
`query`, `one`, `exec`, `insert!`) which ADR 0072 finalizes in parallel.
Every call site sits in the resource model + `app.db`, so aligning exact
names is a mechanical pass in known files. Until bri.db resolves in a
project, the generated `cljgo test` cannot execute the DB path; the
generator's own gate (reader-validity + splice) is the CI signal today,
which is the mandate's explicit fallback.

## Safety

Idempotent splice (no duplicate require/routes), `--force`-gated overwrite
of an existing resource ns, `app.db` created only when absent, and the
whole splice validated before any file is written — a failure leaves no
half-generated tree.
