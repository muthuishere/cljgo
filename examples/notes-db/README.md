# notes-db — a persistent CRUD API on bri.core.data

A real, persistent notes service in cljgo: `bri.web.http` routes over **`bri.core.data`**
(ADR 0072), the one blessed data layer. GET/POST/DELETE persist to **SQLite**
— the zero-install default (ADR 0057, pure-Go `modernc.org/sqlite`, no cgo) —
and the *identical code* drives **Postgres** via pgx (ADR 0058) when you set
`APP_DB_URL=postgres://…`. It runs both interpreted (`cljgo dev`) and
AOT-compiled to one static `CGO_ENABLED=0` binary, byte-identical.

```
cljgo test         # in-process suite over a hermetic in-memory SQLite
cljgo dev          # live server + nREPL; re-def a handler, refresh
cljgo build run    # compile to ONE static binary and serve
```

## The API (all JSON)

```
GET    /api/notes            -> {"notes": [ {id,text,created-at}, … ]}
GET    /api/notes/{id}       -> the note, or 404
POST   /api/notes  {"text"…} -> 201 + Location, the created note
DELETE /api/notes/{id}       -> 204
```

## The data layer, in one screen

```clojure
(require '[bri.core.data :as db])

;; connect — SQLite file by default, ":memory:" for tests, pgx via APP_DB_URL
(def conn (db/connect {:database "notes.db"}))

;; migrate — forward-only <utc>_<slug>.sql files, idempotent, tracked
(db/migrate! conn "migrations")

;; parametrized queries only (injection-safe); rows are maps, snake→kebab keys
(db/query conn "select id, text, created_at from notes where id = ?" 7)
;; => [{:id 7 :text "hi" :created-at "2026-…"}]

(db/one  conn "select * from notes where id = ?" 7)   ; first row or nil
(db/one! conn "select * from notes where id = ?" 7)   ; or throws :bri.core.data/not-found

;; data-shaped writers build parametrized SQL from a map (kebab → snake)
(db/insert! conn :notes {:text "buy milk" :created-at (db/now)})  ; :last-insert-id
(db/update! conn :notes {:text "…"} {:id 7})
(db/delete! conn :notes {:id 7})

;; transactions: commit on return, roll back on throw
(db/tx conn (fn [t]
              (db/insert! t :notes {:text "a"})
              (db/insert! t :notes {:text "b"})))
```

## Why the `delay`

`src/app/main.cljg` wraps `connect` + `migrate!` in a `delay` and exposes
`(conn)`. `cljgo build`'s discovery pass *evaluates* top-level forms (compile =
eval, ADR 0002) — opening a database at the top level would run at **build**
time. The delay forces the connection once, on first use, at **run** time.
