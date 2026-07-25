# ADR 0090 — `bri.web.openapi`: an OpenAPI-driven typed HTTP client

Date: 2026-07-25 · Status: accepted (owner: *"authentication with some openapi
default"*). Realizes the `bri.openapi` battery reserved in **ADR 0075** under the
**ADR 0085** taxonomy (→ `bri.web.openapi`), and lays the client groundwork ADR 0080's
automatic-login CLI builds on. Composes **`cljg.net.http`** (ADR 0087).

## Context

A large class of programs are **API clients**: they talk to a backend described by an
OpenAPI document. Hand-rolling one function per endpoint — substituting path params,
placing query vs header vs body, attaching auth — is exactly the boilerplate an
OpenAPI spec exists to erase. bri already owns the *server* side (bri.web.http +
bri.core.security, ADR 0069); the client side is the missing half.

The spec *is* the contract: operations, parameters (and where each goes), and the
security scheme are all declared. A client built from the spec validates the request
shape and routes parameters correctly with no per-endpoint code.

## Decision

### 1. `bri.web.openapi` — a client from a spec

Pure Clojure over `cljg.net.http` — **no Go shims** (JSON decoding reuses
`cljg.net.http/json-body`, which decodes any JSON string). A namespace like
`bri.web.html`: `install: nil`.

```clojure
(require '[bri.web.openapi :as api])

(def c (api/client spec {:base-url "https://api.example.com"   ; overrides spec servers
                         :token    "abc"                        ; → Authorization: Bearer abc
                         :headers  {"X-Env" "prod"}
                         :timeout  30000}))

(api/operations c)          ; sorted vector of operation-id keywords
(api/operation  c :getUser) ; {:method :path :params …} — the spec's declared shape

(api/call c :getUser   {:id 42})        ; {id} substituted into the path
(api/call c :listNotes {:limit 20})     ; declared query param → query string
(api/call c :createNote {:body {…}})    ; request body (JSON)
;; → a cljg.net.http response {:status :headers :body :ok?}; api/result decodes JSON
```

`spec` is a map (already parsed), a JSON string, or a URL/file path (fetched with
`cljg.net.http` / read with `slurp`, then decoded). `call` resolves each supplied
param by the operation's declared `in` (path / query / header), substitutes `{name}`
path templates, sends `:body` as JSON, applies auth, and calls `cljg.net.http/request`.

### 2. Auth as data, with a per-request seam

Client `:auth` options, attached to every call:
- `:token "…"` → `Authorization: Bearer …`
- `:api-key {:name "X-API-Key" :in :header :value "…"}` (or `:in :query`)
- `:auth-fn (fn [] "Bearer …")` → called **per request** for the `Authorization` value.

The `:auth-fn` seam is the composition point for **ADR 0080**: passing
`#(str "Bearer " (bri.cli.auth/token {:service …}))` gives login-backed auth with a
refresh-on-each-call token, **without** `bri.web.openapi` hard-requiring
`bri.cli.auth` (so the opt-in keychain links only when an app actually uses it). The
full device-flow / auto-login *command* (ADR 0080) is a later `bri.cli` increment
that composes this client + `bri.cli.auth`; this ADR ships the client + the seam.

### 3. Non-OptIn, load order

`bri.web.openapi` requires `cljg.net.http`, so it is registered **after** it in
`bri.Specs()` (its vars must exist when openapi's top-level require resolves). No new
dependency (net/http is stdlib), so `CGO_ENABLED=0` + `cljgo dist` hold.

## Consequences

- cljgo gains a spec-driven API client — no per-endpoint code, request shape checked
  against the spec — reusing the whole `cljg.net.http` stack (retry, timeout, encoding).
- The `:auth-fn` seam makes ADR 0080's automatic-login CLI a composition, not a rewrite.
- Pure-Clojure keeps the namespace shim-free and the static binary intact.

## Not chosen

- **A codegen step** (emit a `.clj` client from a spec) — a runtime client keeps the
  spec as live data, no build phase, and matches Clojure's data-first grain.
- **Full request/response schema validation** — path/param routing + required-path
  checks now; deep JSON-schema validation is additive later (spec data is retained).
- **Hard-requiring `bri.cli.auth`** — would link the opt-in keychain into every
  openapi user; the `:auth-fn` seam composes it opt-in instead (ADR 0074 discipline).
- **YAML specs** — JSON only for now (pure stdlib); YAML is a dependency, deferred.
