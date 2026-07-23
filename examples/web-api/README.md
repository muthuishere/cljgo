# web-api — a real JSON web API in cljgo

A JWT-secured "notes" service that shows the **whole API-first surface** of
bri (ADR 0069) in one small app: login that issues a JWT, role guards
(`logged-in-only` / `admin-only`), typed path params, an in-memory store,
reverse routing, a rate-limited admin group, and routes you add/remove at
runtime — all in plain Clojure, all as **values**, compiled toward a single
static binary.

Security is *what you didn't type*: `(http/listen …)` turns on request-ids,
structured logging, the error funnel, CORS, Prometheus metrics, abuse
protection (auto-ban), JSON negotiation, and `/healthz` + `/readyz`. The app
file has **zero security plumbing**.

```
web-api/
  build.cljgo               the build plan
  conf.edn                  {:port 3000} (+ :profiles, + APP_* env)
  src/app/main.cljg         the API — routes read as a flat column
  test/app/main_test.cljg   the in-process suite (no socket)
```

## Run it

```bash
cljgo test                        # the in-process suite: 9 tests, 27 assertions
APP_PORT=3999 cljgo dev           # a live server + an nREPL; re-def a handler, refresh
```

> AOT (`cljgo build`) of a bri app lands with a later app-framework tier —
> `bri.http` is a runtime lib provider today. The dev loop is `cljgo dev`
> and `cljgo test`.

## The API

| Method & path            | Guard            | What                                   |
|--------------------------|------------------|----------------------------------------|
| `POST /login`            | —                | `{"user","pass"}` → `{"token": …}` (JWT) |
| `GET  /api/me`           | logged-in        | whoami, from the token claims          |
| `GET  /api/notes`        | logged-in        | list notes                             |
| `GET  /api/notes/{id}`   | logged-in        | one note (`{id}` typed → 400 if not int) |
| `POST /api/notes`        | logged-in        | create → `201` + reverse-routed `Location` |
| `DELETE /api/notes/{id}` | **admin**        | delete (a plain user gets `403`)       |
| `GET  /api/admin/stats`  | **admin**, rate-limited | a guarded group under one prefix |
| `GET  /healthz` `/readyz` `/metrics` | (default-on) | ops endpoints                    |

## Try it with curl

```bash
APP_PORT=3999 cljgo dev &

# no token → 401
curl -s -o /dev/null -w '%{http_code}\n' localhost:3999/api/notes
#  → 401

# log in, keep the JWT
TOK=$(curl -s localhost:3999/login \
        -H 'content-type: application/json' \
        -d '{"user":"ada","pass":"s3cret"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')

curl -s localhost:3999/api/notes -H "authorization: Bearer $TOK"
#  → {"notes":[{"by":"ada","id":1,"text":"buy milk"}, …]}

# create — 201, and Location is built by reverse routing, not a hardcoded string
curl -s -D - -o /dev/null localhost:3999/api/notes \
     -H "authorization: Bearer $TOK" -H 'content-type: application/json' \
     -d '{"text":"from curl"}'
#  → HTTP/1.1 201 Created
#  → Location: /api/notes/4

# a plain user is authenticated but not authorized → 403
G=$(curl -s localhost:3999/login -H 'content-type: application/json' \
      -d '{"user":"grace","pass":"hopper"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE localhost:3999/api/notes/2 \
     -H "authorization: Bearer $G"
#  → 403

curl -s localhost:3999/api/admin/stats -H "authorization: Bearer $TOK"
#  → {"notes":3,"users":2}
```

## The one thing to read

`src/app/main.cljg` — the routes:

```clojure
(def routes
  (http/routes
    (POST   "/login"            #'login)
    (GET    "/api/me"           (auth/logged-in-only) #'whoami)
    (GET    "/api/notes"        (auth/logged-in-only) #'list-notes)
    (GET    "/api/notes/{id}"   {:name :note}
            (auth/logged-in-only) #'get-note)              ; named → path-for
    (POST   "/api/notes"        (auth/logged-in-only) #'create-note)
    (DELETE "/api/notes/{id}"   (auth/admin-only) #'delete-note)
    (http/context "/api/admin" [(auth/admin-only) (http/rate-limit 30)]
      (GET "/stats" #'admin-stats))))
```

Each `(GET …)` is a **value**, so you build routes with ordinary Clojure —
`(for …)` over data, `http/context` to prefix + guard a whole group,
`http/wrap` to layer middleware, and `http/add-route` / `http/remove-route`
to change the table at runtime (routes are just data; the server holds
`#'routes`, so a re-`def` hot-swaps it live). Guards are plain Ring
middleware — they compose with `->` and stack in any order. See
`test/app/main_test.cljg` for each of these exercised in-process.
