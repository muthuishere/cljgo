# bri.core.data — the data layer

The one blessed data layer (ADR 0072): connect, query, transact, migrate
over pure-Go SQLite (zero-install default, ADR 0057) or Postgres via pgx
(ADR 0058) — one API, a driver swap. Injection-safe by construction (SQL
string + positional `?` params, never concatenated values) and identical
interpreted (`cljgo dev`) and AOT-compiled to a `CGO_ENABLED=0` binary.

Full guide on the site: https://muthuishere.github.io/cljgo/bri/db/

## Connect

```clojure
(require '[bri.core.data :as db])

(def conn (db/connect {:driver :sqlite :database "app.db"}))  ; ":memory:" for tests
(db/connect {:driver :postgres :url "postgres://localhost/app"})
(db/connect)   ; APP_DB_URL=postgres://… → pgx, else SQLite
(db/close! conn)
```

With no `:driver`, an `APP_DB_URL` starting `postgres` selects pgx, else
SQLite. `:database` defaults to `.dev/app.db`. Placeholders (`?` → `$n`)
and column names (snake_case ↔ kebab-case) normalize; SQL dialect does
not (the ADR 0057 seam).

## Read

```clojure
(db/query conn "select id, text from notes where id = ?" 7)  ; VARIADIC params
;; => [{:id 7 :text "hi"}]   (snake columns → kebab keyword keys)

(db/one  conn "select * from notes where id = ?" 7)   ; first row, or nil
(db/one! conn "select * from notes where id = ?" 7)   ; or throws :bri.core.data/not-found
```

`db/one!` throws `:bri.core.data/not-found`, which the bri.web.http error funnel maps
to a 404 — no handler code.

## Write

```clojure
(db/exec!   conn "update notes set text = ? where id = ?" "hi" 7)  ; {:rows-affected :last-insert-id}
(db/insert! conn :notes {:text "buy milk" :created-at (db/now)})   ; kebab → snake
(db/update! conn :notes {:text "…"} {:id 7})                       ; set-map, where-map
(db/delete! conn :notes {:id 7})
```

`(db/now)` is the current UTC instant as an RFC3339 string.

## Transactions

```clojure
(db/tx conn (fn [t]                         ; commit on return, roll back on throw
              (db/insert! t :notes {:text "a"})
              (db/insert! t :notes {:text "b"})))

(db/with-rollback conn (fn [t] …))          ; ALWAYS rolled back — the per-test sandbox
```

## Migrations

Files `<utc-timestamp>_<slug>.sql`, forward-only, version order, each in
its own tx, tracked in `schema_migrations` — idempotent.

```clojure
(db/migrate! conn "migrations")        ; apply pending; returns the status
(db/migrate-status conn "migrations")  ; => {:applied [...] :pending [...]}
```

## The `delay` pattern

`cljgo build`'s discovery pass evaluates top-level forms (compile = eval,
ADR 0002), so wrap `connect` (+ `migrate!`) in a `delay` — it opens once,
on first use, at run time, not build time.

## See also

- `docs/guides/resource-generator.md` — scaffold a full CRUD slice over bri.core.data
- `docs/guides/bri-http.md` — the error funnel that turns `:bri.core.data/not-found` into 404
- `examples/notes-db/` — a runnable persistent CRUD service
