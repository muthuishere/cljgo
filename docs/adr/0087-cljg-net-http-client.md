# ADR 0087 — `cljg.net.http`: the outbound HTTP client (first `cljg.*` stdlib namespace)

Date: 2026-07-25 · Status: accepted (unblocks ADR 0080's full OpenAPI auto-login;
the client half of the taxonomy's `cljg.net` tier, ADR 0085). Establishes the
first namespace in the **`cljg.*`** stdlib tier.

## Context

cljgo has a *server* HTTP stack (`bri.web.http`, ADR 0069) but no *client* — a CLI
that talks to an API (the `bri.cli.auth` case, ADR 0080) has no blessed way to make
a request. ADR 0085 placed the outbound client at **`cljg.net.http`**: it is general
**mechanism** (any program wants an HTTP client), not framework **policy**, so it
belongs in the `cljg.*` stdlib tier, not under `bri.*`.

This is the **first `cljg.*` namespace**, so it also settles how that tier is wired.

## Decision

### 1. `cljg.*` rides the existing embedded-namespace registry

The namespace registry (`bri.Specs()` + `briloader` + `genbri` → `pkg/briaot`) is
**name-generic** — every seam keys off a Spec's `Name`/`File`/`Pkg`/`Source`, with
no `bri.` prefix assumption. So `cljg.*` namespaces reuse it unchanged: a Spec named
`cljg.net.http`, source at `core/cljg/net_http.cljg`, AOT sub-package
`pkg/briaot/cljgnethttp`. The `pkg/bri` / `pkg/briaot` package *names* are a legacy
of bri being the first tenant; a future rename to a tier-neutral name
(`pkg/nsembed`) is noted but **out of scope** — it would be a mechanical,
behavior-preserving move, and the taxonomy that matters (the `cljg.net.http`
namespace a user types) is already correct. No parallel registry is built.

### 2. Pure-Go on `net/http`, `CGO_ENABLED=0`

The client is Go `net/http` (stdlib, pure Go, no dependency), so the static-binary +
`cljgo dist` guarantee holds. The Go half is one shim — `-http-do` (method, url,
headers, body, timeout) → `{:status :headers :body}` — plus JSON encode/decode
reused from the existing bri JSON shaping. Everything else (method sugar, body
encoding, query strings, retry) is **portable Clojure**, so it runs byte-identically
interpreted and AOT-compiled, and is testable against a local `net/http/httptest`
server.

### 3. The surface

```clojure
(require '[cljg.net.http :as http])
(http/get  "https://api.x/things")                       ; => {:status 200 :headers {…} :body "…"}
(http/get  url {:headers {"Authorization" "Bearer …"} :query {"q" "x"} :timeout 5000})
(http/post url {:json {:name "x"}})                       ; JSON-encodes + sets content-type
(http/post url {:form {"a" "b"}})                         ; x-www-form-urlencoded
(http/request {:method :put :url url :edn {…} :retry 2})  ; the core; the verbs are sugar
(http/json-body resp)                                     ; parse a JSON response body
```

- **Verbs**: `get` `post` `put` `delete` `patch` `head` — sugar over `request`.
  `get` is defined in-namespace (users call `http/get`); it is excluded from the
  `clojure.core` refer per the precedence principle — it never shadows core globally.
- **Body**: one of `:body` (raw string), `:json` (encode + `application/json`),
  `:edn` (`pr-str` + `application/edn`), `:form` (url-encode). `:query` builds the
  query string.
- **Response**: `{:status :headers :body :ok?}` — `:body` a string, `:ok?` = 2xx.
  `json-body`/`edn-body` decode it.
- **Reliability**: `:timeout` ms (default 30000); `:retry` n (default 0) with
  exponential backoff, retrying transport errors + 5xx (idempotent by default —
  the caller opts a non-idempotent method into retry knowingly). A circuit breaker
  is a later addition, not v1.

### 4. Not opt-in

`net/http` is stdlib (no heavy dependency to isolate), so `cljg.net.http` is a
normal (non-OptIn) namespace — its shims install directly, like `bri.web.http`.

## Consequences

- `cljg.net.http` makes cljgo a real HTTP client, unblocking `bri.cli.auth`'s full
  flow (attach `auth/auth-header` to a `http/get`) and any API-client CLI.
- The `cljg.*` stdlib tier is now real and proven to ride the shared registry;
  `cljg.io.*`, `cljg.os`, etc. follow the same path with zero new mechanism.
- Pure-Go keeps `CGO_ENABLED=0` + `cljgo dist`; dual-mode parity is tested against
  an in-process `httptest` server (no network in CI).

## Not chosen

- **A third-party client** (clj-http shape, or a Go HTTP library) — `net/http` is
  enough, dependency-free, and keeps the binary pure-Go.
- **Building a parallel `cljg` registry** — the existing one is name-generic; a
  second would be duplicated mechanism. The package *rename* is deferred, not the
  tier.
- **A streaming/async client in v1** — synchronous request/response first; streaming
  bodies and async are additive later.
