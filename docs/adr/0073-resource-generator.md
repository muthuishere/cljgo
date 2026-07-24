# ADR 0073 — `cljgo generate resource` scaffolds a CRUD resource into a bri web app

Date: 2026-07-24 · Status: accepted (owner-directed — the DHH-style
scaffold that makes bri a genuine one-person framework, capstone of ADR
0041/0047). Extends ADR 0041 (bri) and ADR 0047 (`cljgo new` is a
scaffolder); depends on ADR 0069 (bri.http/auth surface), ADR 0071 (bri is
AOT), and ADR 0072 (bri.db data layer) for the DB seam.

## Context

`cljgo new --template web` hands the author a runnable bri app. From there
every new endpoint is hand-written boilerplate: a migration, a model, five
handlers, a routes value, and a test — the same shape every time. Rails
answered this with `rails generate scaffold`; Phoenix with `mix phx.gen`.
bri's pitch is "one-person framework"; the missing piece is the generator
that turns a resource description into that whole vertical slice.

The owner's mandate: `cljgo generate resource <Name> <field:type>...`
scaffolds a complete, working, authenticated CRUD resource — migration +
model + handlers + routes + tests — so a user goes from nothing to a
running CRUD endpoint in one command, done functionally and minimally.

Constraints:

- **Templates are real source (ADR 0047).** The generated files must be
  valid `.cljg` that compile and run, not string-mangled fragments. But a
  resource scaffold is *field-parametrized* (unlike the fixed project
  templates), so it cannot be a pure name-substitution file copy — it is a
  code generator, like every scaffold generator (Rails' ERB templates carry
  logic). We keep the emission templates as **real embedded template
  files** (`cmd/cljgo/resource_tmpl/*.tmpl`, `text/template`), and a CI test
  generates a canonical resource, reader-validates every emitted file, and
  splices it into a real web project — the same anti-rot guarantee ADR 0047
  buys for project templates, applied to the generator's *output*.
- **The generated code calls bri.db (ADR 0072).** This generator does NOT
  implement bri.db; it emits code that calls it. Until ADR 0072's exact
  names are frozen, we target the documented surface: `(db/query ds sql
  args)` → vector of keyword-keyed maps, `(db/one ds sql args)`, `(db/exec
  ds sql args)`, `(db/insert! ds :table row)`, `(db/connect opts)`,
  sqlite-default (ADR 0057). Every db call site is confined to the model
  section of the generated resource ns plus one generated `app.db` ns, so a
  reconciliation pass aligns exact names in one place.
- **Routes/handlers use the existing bri.http surface (ADR 0069)** — verb
  forms, `param!`, guards, reverse routing — which exists and is emitted
  fully correct now.
- **Both modes.** The generated project must work interpreted (`cljgo
  dev`/`cljgo test`) and AOT-compile (ADR 0071); nothing generated reaches
  back into the interpreter.
- **Precedence principle.** Nothing generated shadows clojure.core.

## Decision

### 1. Command surface

`cljgo generate resource <Name> <field:type>...` (alias `cljgo g`). `Name`
is the singular resource (`Note`); fields are `name:type` pairs. It runs
from a bri web project root (requires `src/app/main.cljg`). Unknown types,
a missing name, or a non-web directory are named errors, not panics.

### 2. Field-type grammar → SQL column + Clojure coercion

| type         | SQLite column                    | coercion (from JSON body)              |
|--------------|----------------------------------|----------------------------------------|
| `string`     | `TEXT`                           | `str`                                  |
| `text`       | `TEXT`                           | `str`                                  |
| `int`        | `INTEGER`                        | number \| `parse-long`                 |
| `bool`       | `INTEGER`                        | boolean (truthy / `"true"`)            |
| `uuid`       | `TEXT`                           | `str` (validated `parse-uuid`)         |
| `timestamp`  | `TEXT`                           | `str` (ISO-8601)                       |
| `references` | `<name>_id INTEGER` + index      | number \| `parse-long`                 |

Every table gets an implicit `id INTEGER PRIMARY KEY AUTOINCREMENT`; the
resource's path-param `{id}` is coerced `:int` via `http/param!` (a bad id
is a funnel 400, no handler code). Unknown type ⇒ named error listing the
seven.

### 3. Files created vs edited (idempotent, never clobber)

Created (for `Note title:string body:text`):

- `db/migrations/<YYYYMMDDHHMMSS>_create_notes.sql` — the table (always a
  fresh timestamped file; migrations are append-only, never rewritten).
- `src/app/notes.cljg` — the resource ns: a **model** section (bri.db
  calls, one per CRUD op, `ds`-parametrized) + **handlers** (list/show/
  create/update/delete, JSON, authenticated) + a **routes** value.
- `src/app/db.cljg` — the app's single datasource, created **once** (skipped
  if it already exists). `(def ds (delay (db/connect …)))` — a `delay` so
  there is no I/O at load (the `cljgo test`/require contract), opened on
  first use. This is the ONE bri.db reconciliation point for connect.
- `test/app/notes_test.cljg` — in-process `bri.http` tests over the CRUD
  surface (login → list/create/show/update/delete), the same client the
  web template's own test uses.

Edited (splice, marker-based — see §4):

- `src/app/main.cljg` — adds `[app.notes :as notes]` to the ns `:require`
  and `notes/routes` to the app's `(http/routes …)` value.

**Safety:** re-running for an existing resource never clobbers user edits.
If `src/app/notes.cljg` already exists the command refuses (exit 1) unless
`--force`. The splice is idempotent: if `app.notes` is already required /
`notes/routes` already present, that edit is skipped. `app.db` is created
only when absent.

### 4. Splicing without a fragile parser — documented markers

The `web` template's `src/app/main.cljg` carries two comment markers:

```clojure
(ns app.main
  (:require [bri.http :as http :refer [GET POST]]
            …
            ;; cljgo:resource-requires
            ))
…
(def routes
  (http/routes
    (GET "/{$}" #'home)
    …
    ;; cljgo:resource-routes
    ))
```

`generate` inserts each new line *above* its marker (keeping the marker for
the next resource) with a plain string splice — no s-expression parsing.
The markers are comments, so the file is valid source with or without them,
and `notes/routes` composes because `http/routes` concatenates route values
and seqs (ADR 0069 §7/§9). If a marker is missing (a hand-edited app), the
command prints the exact two lines to add rather than guessing — an honest
error over a fragile edit.

### 5. Naming & pluralization

The given `Name` is the singular; the collection/table/route base is its
plural. Pluralization is the minimal English ruleset (`y`→`ies` after a
consonant, `s/x/z/ch/sh`→`es`, else `+s`); the ns is `app.<plural>`, the
table `<plural>`, routes `/api/<plural>`, the reverse-route names `:notes`
(collection) and `:note` (member). Names are lower-cased; the input must be
a valid identifier.

### 6. Generated code shape (bri-idiomatic, both-mode safe)

Handlers are `#'var` values in the routes table (live re-def in dev);
models take `ds` and are the only bri.db callers; the datasource is
`@app.db/ds`. All routes are guarded `(auth/logged-in-only)` and the
destroy route additionally `(auth/admin-only)` — "authenticated CRUD" per
the mandate, mirroring `examples/web-api`. Nothing runs at load; `-main`
(unchanged) serves the spliced routes.

## Consequences

- A user goes `cljgo new --template web blog && cd blog && cljgo generate
  resource Post title:string body:text && cljgo dev` to a live authenticated
  CRUD API in three commands.
- The DB layer is a named reconciliation surface: all bri.db calls live in
  the generated resource model + `app.db`, so ADR 0072's final names land
  with a mechanical pass. Until then, a generated resource's `cljgo test`
  goes green the moment bri.db resolves; the generator's own CI gate
  (reader-validity + splice) is green today without it.
- The generator emits source from `text/template` files (real files, CI
  round-tripped), not inline string literals — the ADR 0047 anti-rot intent
  applied to a code generator.
- Not chosen: a live-reflection scaffold (bri does not scan — ADR 0041);
  editing `main.cljg` by parsing its s-expressions (fragile — markers are
  the documented seam the owner asked for); generating HTML views (bri is
  API-first, ADR 0069 — JSON handlers are the blessed default; an HTML view
  generator is a possible later `--html` flag).
