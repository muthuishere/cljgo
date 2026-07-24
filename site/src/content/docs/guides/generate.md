---
title: "The resource generator"
description: "cljgo generate resource Note title:string scaffolds a migration, model, handlers, routes, and a green CRUD test over bri.core.data — and splices it into your app."
---

`cljgo generate resource` is the DHH-style scaffold: it turns a resource description into a whole authenticated CRUD slice inside a bri web app — a migration, a model, five handlers, a routes value, and a **green** test — then splices the routes into `src/app/main.cljg` at documented comment markers (ADR 0073). It is a real code generator, not a file copy: the output is field-parametrized, and every emitted file is validated on each build.

Run it from inside a bri web app (create one with `cljgo new --template web`):

```bash
cljgo generate resource Note title:string body:text
cljgo g Note title:string body:text            # `g` is the alias
```

## Field types

Each field is `name:type`. The grammar:

| type | column | notes |
|---|---|---|
| `string` | `TEXT NOT NULL` | |
| `text` | `TEXT NOT NULL` | |
| `int` | `INTEGER NOT NULL` | coerced through `->long` |
| `bool` | `INTEGER NOT NULL` | coerced through `->bool` |
| `uuid` | `TEXT NOT NULL` | |
| `timestamp` | `TEXT NOT NULL` | |
| `references` | `INTEGER NOT NULL` | column is `<name>_id`, gets an index |

```bash
cljgo generate resource Comment body:text author:references approved:bool
# → author_id INTEGER + idx_comments_author_id, approved coerced to a bool
```

## What it generates

```
generated resource note (/api/notes)
  create  db/migrations/20260724120000_create_notes.sql
  create  src/app/db.cljg            # the shared datasource (created once)
  create  src/app/notes.cljg         # model + handlers + routes
  create  test/app/notes_test.cljg   # a green CRUD suite
  splice  src/app/main.cljg  (require app.notes + routes)
```

- **The migration** — `CREATE TABLE notes (…)` with an `id` primary key and your columns, plus an index per `references` field.
- **`src/app/db.cljg`** — the app's single datasource, created **once** and never clobbered. It wires the [bri.core.data](/cljgo/bri/db/) connection (SQLite by default, `:memory:` under the `:test` profile, Postgres via `APP_DB_URL`).
- **The resource** (`src/app/notes.cljg`) — a `coerce` fn (JSON body → typed columns), a model of parametrized `bri.core.data` calls, five handlers (`index`/`show`/`create`/`update-one`/`delete-one`), and a `routes` value. Every route is authenticated ([bri.core.security](/cljgo/bri/auth/)); `delete` is `admin-only`. **This file is yours** — edit it freely; the generator never rewrites an existing resource.
- **The test** — the full CRUD over a fresh in-memory database, plus the guards and the typed-param funnel. It is green out of the box.

The model is the only place that touches `bri.core.data`, and every query is parametrized — the injection-safe seam kept in one file.

## The splice

The generator inserts the resource's `require` and `routes` value into `app.main` above two documented markers:

```clojure
(ns app.main
  (:require [bri.web.http :as http]
            ;; cljgo:resource-requires
            ))

(def routes
  (http/routes
    ;; cljgo:resource-routes
    ))
```

The splice is validated **before** any file is written (a missing marker is a clean, named error, not a half-generated resource) and is **idempotent** — re-running for an already-wired resource is a no-op. Pass `--force` to overwrite an existing resource file.

## After generating

```bash
cljgo routes    # see the new endpoints and their guards
cljgo test      # the generated CRUD suite — green
cljgo dev       # serve it
```

## Where next

- [bri.core.data](/cljgo/bri/db/) — the data layer the generated model calls
- [bri.core.security](/cljgo/bri/auth/) — the guards on the generated routes
- [bri.web.http](/cljgo/bri/http/) — the router the routes value plugs into
- [Deploy](/cljgo/guides/deploy/) — ship the finished app as one static binary
