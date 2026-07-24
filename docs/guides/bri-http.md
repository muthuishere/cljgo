# bri.web.http — the server guide

The Ring contract on stdlib routing (ADR 0041 §4): a handler is a fn
of request-map → response-map; middleware is handler → handler; routes
are data on Go 1.22+ `ServeMux` pattern strings. There is no router of
our own and no hidden call graph — the adapter only invokes what you
handed it.

## Requests and responses

```clojure
;; request                                   ;; response
{:request-method :get                        {:status  200
 :uri            "/users/7"                   :headers {"content-type" "..."}
 :query-string   "page=2"                     :body    "..."}   ; string, or
 :headers        {"accept" "text/html"}                         ; map/vector → JSON
 :params         {:id "7"}       ; {name} segments, AS STRINGS
 :query-params   {:page "2"}
 :body           "..."
 :session        {...}           ; from the signed cookie (middleware)
 :json           {...}           ; parsed JSON body (middleware)
 :form-params    {...}}          ; parsed form body (middleware)
```

`:params` bind as strings; the blessed, VISIBLE coercion is

```clojure
(http/param! req :id :int)   ; :string :int :uuid :keyword
```

Failure throws `:http/bad-param` — the funnel maps it to a 400. No
error-handling code in the handler.

## Routes are data

```clojure
(def routes
  [["POST /signup"    #'signup]
   ["GET /users/{id}" #'show-user]
   ["GET /static/"    (http/dir "public")]
   ["GET /health"     (http/health {})]])
```

- Patterns are the stdlib's own syntax (`METHOD /path`, `{name}`
  segments, `{$}` for exact-root, trailing `/` for subtree).
- `#'var` handlers deref **per request** — re-`def` at the REPL updates
  the live server. A plain fn works but is not live; `cljgo dev` warns.
- `(http/group "/admin" [require-admin] routes)` prefixes a group and
  wraps extra middleware.
- `(http/dir "public")` serves static files (Go's `http.FileServer`).

## serve

```clojure
(http/serve routes {:port (:port cfg)
                    :drain [workers]})     ; blocks; SIGTERM drains
```

- Pings anything in `:ping` before accepting traffic.
- Production timeouts are DEFAULT ON (read/write/idle).
- SIGTERM/Ctrl-C: in-flight requests finish (deadline), then each
  handle in `:drain` is invoked — shutdown wiring is on the page,
  never an ambient registry.
- Tests/REPL: `{:block? false}` returns `{:port n :stop (fn)}`.

## Middleware: the safe stack is what you didn't type

Omitting `:middleware` applies `(http/defaults)`:

```clojure
[access-log (recover) sessions negotiate csrf]   ; outermost first
```

It is a plain VECTOR of `{:name kw :wrap fn}` entries — inspect it,
`conj` onto it, `(http/without stack :csrf)` to remove by name.
`cljgo routes` prints the effective stack; dev mode warns when a
custom stack lacks `recover` or `csrf`.

- **access-log** — one line per request.
- **recover** — THE error funnel (below).
- **sessions** — signed cookies (HMAC-SHA256; key from
  `APP_SESSION_KEY`, or per-process random in dev). Read as
  `:session`; attach one with `(http/start-session res {...})`.
- **negotiate** (`:json` in the stack) — JSON bodies → `:json`, form
  bodies → `:form-params`; map/vector response bodies → JSON; string
  bodies default to `text/html`.
- **csrf** — gates SESSION-BEARING mutating requests on the token
  `bri.web.html/form` mints (or the `x-csrf-token` header). Sessionless
  requests pass: the documented API posture — a JSON curl with no
  cookie has nothing to forge.

## Errors: one blessed surface, one documented funnel

App handlers use `!` forms and let the funnel answer (ADR 0014 under
bri's doctrine). The funnel's mapping is shipped DATA:

| `:bri/error` in ex-data | status |
|---|---|
| `:http/bad-param` | 400 |
| `:cast/invalid`   | 422 |
| `:db/not-found`   | 404 |
| `:db/constraint`  | 409 |
| anything else     | 500 |

Override rows: `(http/recover {:error-map {:app/teapot 418}})` in a
custom stack. Error bodies name the kind; messages appear in dev only.

Result values cross the http boundary ONLY through the visible bridge:

```clojure
(defn signup [req]
  (http/render
    (let? [input (validate (:json req))
           user  (create input)]
      (http/created user))))
```

`(ok resp)` → the response; `(err e)` → the funnel (an err payload map
may carry `:bri/error` to pick its row). A handler that returns a
bare Result WITHOUT `http/render` is a loud 500 explaining exactly
that — never a silently laundered status.

## Testing

The in-process client runs the same mount + middleware path with no
socket:

```clojure
(http/request routes {:method "GET" :path "/users/7"})
(http/request routes {:method "POST" :path "/e"
                      :headers {"content-type" "application/json"}
                      :body "{\"a\":1}"})
```

## Escape hatch

The adapter is a thin shim over `net/http` (pkg/bri). When you
outgrow the blessed path, mount your own patterns — routes are just
data and the mux is the stdlib's.
