---
title: "bri.db"
description: "The one blessed data layer: connect, query, transact, and migrate over pure-Go SQLite or Postgres — parametrized by construction, plain maps out, identical interpreted and AOT-compiled."
---

`bri.db` is cljgo's one blessed data layer (ADR 0072). It is API-first and injection-safe by construction — the blessed form is a SQL string plus positional `?` params, never string-concatenated values — and it behaves **identically** interpreted (`cljgo dev`) and AOT-compiled to a `CGO_ENABLED=0` static binary. Two pure-Go drivers sit behind one API: [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (the zero-install default, ADR 0057) and [`pgx`](https://github.com/jackc/pgx) (production Postgres, ADR 0058) — a driver swap, not an API fork.

If you want the whole thing wired for you, `cljgo generate resource` scaffolds a migration + model + handlers + a green CRUD test over `bri.db` in one command — see [the resource generator](/cljgo/guides/generate/).

## Connect

```clojure
(require '[bri.db :as db])

(def conn (db/connect {:driver :sqlite :database "app.db"}))
```

- `:driver` — `:sqlite` (default) or `:postgres`.
- `:database` — the SQLite file path (default `.dev/app.db`), or `":memory:"` for a fresh, disposable database (tests).
- `:url` — the Postgres URL (or set `APP_DB_URL`).

With **no** `:driver`, an `APP_DB_URL` starting `postgres` selects pgx; otherwise you get SQLite, zero install. `(db/connect)` with no args reads `APP_DB_URL` / the defaults. Close a pool with `(db/close! conn)`.

```clojure
;; the same code, Postgres — a driver swap, not a rewrite
(db/connect {:driver :postgres :url "postgres://localhost/app"})
;; or, ambiently:
(db/connect)              ; APP_DB_URL=postgres://… → pgx
```

`bri.db` normalizes placeholders (`?` → `$n` for Postgres) and column names (snake_case ↔ kebab-case) — but **not** SQL dialect (the ADR 0057 seam), so DDL stays yours.

## Read

Queries are parametrized SQL — a string plus **variadic** positional params — and rows come back as vectors of maps with snake_case columns turned into kebab-case keyword keys.

```clojure
(db/query conn "select id, text, created_at from notes where id = ?" 7)
;; => [{:id 7 :text "hi" :created-at "2026-…"}]

(db/one  conn "select * from notes where id = ?" 7)   ; the first row, or nil
(db/one! conn "select * from notes where id = ?" 7)   ; or throws :bri.db/not-found
```

`db/one!` throws an ex-info tagged `:bri.db/not-found` — which the [bri.http error funnel](/cljgo/bri/http/) maps straight to a 404. That is the whole "row missing → 404" story: no `if-let`, no handler code.

## Write

`db/exec!` runs any parametrized write and returns `{:rows-affected n :last-insert-id id}`. The data-shaped helpers build the parametrized SQL from a map for you (kebab keys → snake columns):

```clojure
(db/exec!   conn "update notes set text = ? where id = ?" "hi again" 7)

(db/insert! conn :notes {:text "buy milk" :created-at (db/now)})  ; :last-insert-id
(db/update! conn :notes {:text "…"} {:id 7})                      ; set-map, where-map
(db/delete! conn :notes {:id 7})
```

`(db/now)` is the current UTC instant as an RFC3339 string — a portable `created-at`/`updated-at` value with no Java interop, identical across modes.

## Transactions

`db/tx` runs `(f tx-conn)` in a transaction: **commit on normal return, roll back on any throw** (re-raised). The `tx-conn` drives the identical read/write verbs — a db and a tx are the same API.

```clojure
(db/tx conn (fn [t]
              (db/insert! t :notes {:text "a"})
              (db/insert! t :notes {:text "b"})))   ; both, or neither
```

`db/with-rollback` runs `(f tx-conn)` in a transaction that is **always** rolled back — the per-test sandbox (the Ecto-Sandbox shape): exercise real writes inside a test and leave the database untouched.

```clojure
(db/with-rollback conn (fn [t]
                         (db/insert! t :notes {:text "scratch"})
                         (is (= 1 (count (db/query t "select * from notes"))))))
```

## Migrations

Migrations are files named `<utc-timestamp>_<slug>.sql`, applied **forward-only** in version order, each in its own transaction, tracked in a `schema_migrations` table — idempotent.

```clojure
(db/migrate! conn "migrations")       ; apply every pending file; returns the status
(db/migrate-status conn "migrations") ; => {:applied ["2026…"] :pending ["2026…"]}
```

```sql
-- migrations/20260724120000_create_notes.sql
CREATE TABLE notes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  text TEXT NOT NULL,
  created_at TEXT NOT NULL
);
```

`migrate!` is the same call a deploy runs; running it twice is a no-op.

## The `delay` pattern

Open the connection lazily. `cljgo build`'s discovery pass **evaluates** top-level forms (compile = eval, ADR 0002), so a `connect` at the top level would open a socket/file at *build* time. Wrap it in a `delay` (or a fn) so it forces once, on first use, at run time:

```clojure
(def ^:private conn*
  (delay
    (let [c (db/connect {:database (:db-path cfg "notes.db")})]
      (db/migrate! c "migrations")
      c)))

(defn conn [] @conn*)   ; opened + migrated on first call
```

## API at a glance

| fn | does |
|---|---|
| `(db/connect opts)` | open a pool — `:driver`, `:database`/`:url` (or `APP_DB_URL`) |
| `(db/close! conn)` | close the pool |
| `(db/query conn sql & params)` | parametrized SELECT → vector of kebab-keyword maps |
| `(db/one conn sql & params)` | first row, or `nil` |
| `(db/one! conn sql & params)` | first row, or throws `:bri.db/not-found` (→ 404) |
| `(db/exec! conn sql & params)` | write → `{:rows-affected :last-insert-id}` |
| `(db/insert! conn table row)` | data-shaped INSERT (kebab→snake) |
| `(db/update! conn table set-map where-map)` | data-shaped UPDATE |
| `(db/delete! conn table where-map)` | data-shaped DELETE |
| `(db/tx conn f)` | transaction: commit on return, roll back on throw |
| `(db/with-rollback conn f)` | always-rolled-back tx (per-test sandbox) |
| `(db/migrate! conn dir)` | apply pending `<utc>_<slug>.sql`, forward-only |
| `(db/migrate-status conn dir)` | `{:applied […] :pending […]}` |
| `(db/now)` | current UTC instant, RFC3339 string |

## Where next

- [The resource generator](/cljgo/guides/generate/) — scaffold a full CRUD slice over `bri.db` in one command
- [bri.http](/cljgo/bri/http/) — the error funnel that turns `:bri.db/not-found` into a 404
- [bri.auth](/cljgo/bri/auth/) — guarding the handlers that call your model
- [bri.config](/cljgo/bri/config/) — where `APP_DB_URL` and the `:db` key come from
- [Deploy](/cljgo/guides/deploy/) — ship the whole thing as one static binary
