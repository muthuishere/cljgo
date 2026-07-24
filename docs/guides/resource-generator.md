# The resource generator

`cljgo generate resource` (ADR 0073) is the DHH-style scaffold: it turns a
resource description into a whole authenticated CRUD slice inside a bri web
app — a migration, a model, five handlers, a routes value, and a GREEN test
— then splices the routes into `src/app/main.cljg` at documented comment
markers. Field-parametrized code generation, not a file copy; every emitted
file is reader-validated on each build.

Run from inside a bri web app (`cljgo new --template web`).

Full guide on the site: https://muthuishere.github.io/cljgo/guides/generate/

```bash
cljgo generate resource Note title:string body:text
cljgo g Note title:string body:text        # `g` is the alias
```

## Field types (`name:type`)

| type | column | notes |
|---|---|---|
| `string` / `text` | `TEXT NOT NULL` | |
| `int` | `INTEGER NOT NULL` | coerced via `->long` |
| `bool` | `INTEGER NOT NULL` | coerced via `->bool` |
| `uuid` / `timestamp` | `TEXT NOT NULL` | |
| `references` | `INTEGER NOT NULL` | column is `<name>_id`, gets an index |

## What it generates

```
generated resource note (/api/notes)
  create  db/migrations/<utc>_create_notes.sql
  create  src/app/db.cljg            # the shared datasource — created ONCE, never clobbered
  create  src/app/notes.cljg         # coerce + model (parametrized bri.db) + handlers + routes
  create  test/app/notes_test.cljg   # a green CRUD suite (fresh in-memory DB)
  splice  src/app/main.cljg  (require app.notes + routes)
```

- Every route is authenticated (bri.auth); `delete` is `admin-only`.
- The model is the only place that touches bri.db (ADR 0072); every query is
  parametrized. `src/app/notes.cljg` is YOURS to edit — the generator never
  rewrites an existing resource (pass `--force` to overwrite).
- Persistence is the zero-install SQLite default; `:test` profile uses an
  in-memory DB; Postgres via `APP_DB_URL`, no code change.

## The splice

Inserts the `require` and `routes` value above two markers in `app.main`:

```clojure
;; cljgo:resource-requires    (in the :require vector)
;; cljgo:resource-routes      (inside (http/routes …))
```

Validated BEFORE any file is written (a missing marker is a clean named
error, not a half-generated resource) and idempotent (re-running an
already-wired resource is a no-op).

## After generating

```bash
cljgo routes    # the new endpoints + guards
cljgo test      # the generated CRUD suite — green
cljgo dev       # serve it
```

## See also

- `docs/guides/bri-db.md` — the data layer the model calls
- `docs/guides/bri-auth.md` — the guards on the routes
