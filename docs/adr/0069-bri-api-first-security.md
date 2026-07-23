# ADR 0069 — bri's API-first security tier (auth, guards, observability, routing)

Status: accepted — owner-directed 2026-07-23. Supersedes nothing; extends
ADR 0041 (bri framework) with the security/observability pillar and the
Compojure-style router surface.

## Context

The owner directed (2026-07-23, verbatim intent): *"http should be so
fast; middleware security and all should be simple enough; honestly it's
okay to have html but predominantly API; default CORS; authentication JWT
in a single way; middleware like userOnly adminOnly loggedInOnly
composable; so simple for API development; JWT creation should be simple;
use SIMD-based stuff for hash or whatever if possible to make it fast."*
Then, across the session: *"but i also need express kind of"* → *"Clojure
but a better old way"* → the final call: **modernized Compojure — bare
verb one-liners**, plus composition at every level, dynamic/runtime-mutable
routes, named/reverse routing, custom-middleware as a first-class seam,
observability + audit baked in, and abuse protection.

Constraints: ADR 0056 (pure Go, `CGO_ENABLED=0`; Go asm is fine, cgo is
not; `bri.*` namespacing; dual-harness + perf budget), ADR 0041 (ONE
blessed way per pillar; library-style — you call it, it never calls you;
no scanning/DI; routes are DATA on Go 1.22+ ServeMux — no router of our
own), and the precedence principle (nothing shadows clojure.core).

Evidence: **spike s44** (`spikes/s44-jwt-guard-perf/`, VERDICT.md).

## Decision

### 1. API-first posture

JSON is the default: a handler returning a map/vector/string is
auto-encoded (JSON for data, HTML for strings) by the `negotiate`
middleware. HTML stays fully supported (bri.html, the `defaults` cookie
stack) but is secondary. The blessed API middleware stack is
`(bri.http/api-defaults)`.

### 2. One JWT way — HS256, algorithm pinned

**HS256, hand-rolled on Go stdlib `crypto/sha256`, zero external JWT
dependency.** s44 proved this 2.8× faster to sign / 2.1× faster to verify
than `golang-jwt/jwt/v5`, at ⅓ the allocations, and cross-verified against
golang-jwt + the RFC 7519 example token (two independent implementations
agree). The **algorithm is pinned server-side**: the Go verifier compares
the token's header against one precomputed `{"alg":"HS256","typ":"JWT"}`
constant and never reads the token's own `alg` to choose verification —
the classic `alg:none` / alg-confusion forgery is structurally impossible
(frozen by `pkg/bri/auth_test.go` and the spike's `verify_test.go`).

Surface (`bri.auth`): `(sign claims)` / `(sign claims {:exp-seconds n
:secret s :now t})`, `(verify token)` → claims-or-nil, `(verify! token)`
(throws `:auth/unauthorized`, the ADR 0014 `!` convention), `(issue sub
claims opts)` (signs + audits the login). exp/iat injected; secret from
`APP_AUTH__SECRET` (secrets-are-env), per-process random in dev. **On the
SIMD directive:** stdlib `crypto/sha256` already dispatches to SHA-NI /
ARMv8-SHA2 instructions; s44 showed `minio/sha256-simd` is *slower* on
JWT-sized inputs on this hardware — the directive is satisfied by the
stdlib, and the extra dependency is declined.

### 3. Passwords — argon2id blessed, never fast

argon2id (OWASP m=19 MiB, t=2, p=1) is the default; bcrypt-verify is kept
for importing legacy hashes. `(hash-password pw)` / `(check-password pw
hash)`. Both are pure-Go `golang.org/x/crypto` (passes the ADR 0056
filter). These are DELIBERATELY slow (~16 ms argon2id, ~45 ms bcrypt) —
password hashing is the ONE place where "make it fast" is a vulnerability;
SIMD/SHA-NI must never touch a password.

### 4. Default CORS

`(bri.http/cors)` is in `api-defaults`. **The dev default is permissive
(origin `*`) — convenient and LOUD**; in production set `:origins` (or
`APP_HTTP__CORS_ORIGINS`) to an allowlist. Adds ACAO/ACAM/ACAH +
`Vary: Origin`; an `OPTIONS` preflight short-circuits 204.

### 5. Composition model — one ordered mechanism at every scope

The public contract: **middleware = `handler -> handler`; handler =
`ring-request -> ring-response`.** A middleware value is that bare fn OR a
`{:name kw :wrap fn}` map (so it names itself in `cljgo routes`). User
middleware and built-ins are the SAME type, composed by ORDER
(outer→inner), with no registration. Scopes, all the same mechanism:

- **route-level**: `(GET path mw handler)` / `(GET path [mw1 mw2] handler)`
- **group**: `(wrap routes-value mw…)` wraps every route in a value
- **context** (Compojure `context`): `(context "/api" [mw…] …values)` —
  prefix + shared middleware over nested values
- **global**: `:middleware [...]` at `serve`/`listen`, or `(api-defaults)`

Guards are plain middleware, so `->` composes them. Claims land under
`:auth/claims`. Ergonomic helpers: `(wrap-request f)` (pre-process /
short-circuit) and `(wrap-response f)` — a custom middleware is ~2 lines.

**Authz seam:** `(auth/guard req-pred)` is the general guard;
`logged-in-only` / `role-only` / `user-only` / `admin-only` are thin
specializations. A one-line tenant check:
`(auth/guard #(= (get-in % [:auth/claims :org]) (get-in % [:path-params :org])))`.

### 6. 401 vs 403 (RFC 6750)

No/invalid/expired token → **401**; valid token, predicate false (wrong
role/authz) → **403**. Guards emit these uniformly and audit each denial.

### 7. Router surface — modernized Compojure, sugar over routes-as-data

Bare verb forms `GET POST PUT PATCH DELETE HEAD OPTIONS ANY` (uppercase —
none shadow clojure.core; `:refer` them). Arities: `(GET path handler)`,
`(GET path mw handler)`, `(GET path [mw…] handler)`, plus an optional
`{:name :x}` opts map. **Each form is PURE SUGAR that lowers to the exact
`[["METHOD /path" handler] …]` data the low-level engine already mounts on
ServeMux — ONE router (ADR 0041), no second one.** `(defroutes name …)`
names a routes value; `(defroute name form)` names a single route;
`(routes …)` concatenates. `cljgo routes` prints the resolved table.
Conformance freezes that the one-liner, the generated, and the
hand-written data forms are byte-identical (`TestCompositionEquivalence`).

### 8. Named routes & reverse routing

A route may carry `{:name :item}`; `(path-for :item {:id 5})` →
`"/items/5"` and `(url-for :item {:id 5} {:q "x"})` fill + URL-encode
against the served table (Rails `url_for` / Django `reverse`, which
Express lacks). Unknown name or missing param throws a diagnostic
(`:http/bad-param`), never silent. Powers redirects/HATEOAS by name.

### 9. Dynamic routes (routes are data)

The framework adds NOTHING special for dynamic routes — because routes are
plain values, Clojure already does it: `(routes (for [e (config …)] (GET
(:path e) …)) …)` builds tables from data (`routes`/`defroutes` accept
seqs of route values). `add-route`/`add-routes` and `remove-route`
(by `:name` or method+path)/`remove-routes` are FIRST-CLASS, immutable,
returning new values that compose with `routes`/`wrap`/`context`. When
`serve` holds the app behind a `#'var`, re-`def`ing it hot-swaps the live
table (the REPL-liveness edge, same as live handler vars). Path patterns
pass through to ServeMux natively: named `/items/{id}` → `:path-params`,
catch-all `/files/{path...}`, the `{$}` exact anchor, method precedence;
typed coercion stays `http/param!` (`:int`/`:uuid` → funnel-mapped 400).
We invent no regex router (ADR 0041): a constraint not expressible in
ServeMux is a `param!` coercion or a guard, not route syntax.

### 10. Observability & audit (default-on, config-tunable)

Baked into `api-defaults` as ordinary inspectable middleware DATA (conj to
add, `(without stack :name)` to remove), zero-config out of the box:

- **request-id** — honor/mint `X-Request-Id`, thread `:request/id` + a
  shared `:bri/ctx` atom, echo on the response. The tracing floor.
- **logging** — one STRUCTURED line/request (JSON prod, readable dev):
  method, path, status, ms, request-id, and the authenticated subject
  (guards record it into `:bri/ctx` so even a 403 logs WHO). Overridable
  sink (`set-log-sink!` / `*log-sink*`).
- **metrics** — count + latency histogram + status-class per route
  (low-cardinality PATTERN label) in a lock-light atomic Go registry,
  rendered as **Prometheus text** at `GET /metrics`. Guardable
  (`:metrics-guard (auth/admin-only)`) — DEFAULT IS OPEN; guard it in
  prod. Off the hot path (`TestMetricsStackOverhead` ≤ 3×; s44 guard class
  ~10 ns).
- **health** — `GET /healthz` (liveness), `GET /readyz` (runs
  `:ready-checks` → 200/503), auto-appended by serve/listen.
- **audit** (`bri.audit`) — the security trail, DISTINCT from access logs,
  grounded in the owner's reqsume-kernel ch04 convention (audience-aware,
  severity-driven, `actor`/`action`/`target`/`ts` shape, ONE fan-out
  seam). v1 sink = structured stderr; `set-sink!` is the T2 seam to
  bri.db + `notify.Send` (NOT built now). Failed-auth (401/403), token
  issuance, and bans audit by default.

### 11. Abuse protection — auto-ban + rate-limit, proxy-aware identity

`(auth/auto-ban)` (default-on in api-defaults, removable) tracks 401/403
denials per client in a sliding window; over `:threshold` in `:window-ms`
→ banned for `:cooldown-ms`, short-circuiting **429 + Retry-After** before
the handler, every ban audited. `(http/rate-limit n)` is the distinct
plain-throughput cap (N/window → 429). Both key on **client IP by
default**, `{:key :subject}` or a custom fn, and share one resolver.

**The client-IP gotcha is handled correctly:** `RemoteAddr` is the proxy's
IP behind a CDN/LB, so `(http/client-ip req)` — the ONE blessed resolver
used by rate-limit, auto-ban, logging, and audit alike — honors
`X-Forwarded-For` / `X-Real-IP` **ONLY when the immediate peer is inside a
configured trusted-proxy CIDR (`APP_HTTP__TRUSTED_PROXIES`)**, taking the
right-most untrusted hop; otherwise it uses the unspoofable socket peer.
Trusting XFF from an untrusted peer is the classic ban-evasion / spoofing
bypass and we never do it (`TestClientIPTrustedProxy`). In-process TTL
store (SINGLE-INSTANCE — each node counts independently behind an LB; swap
`:store` for a shared bri.cache/Redis backing to make it cluster-wide).

## Consequences

- One dependency added to the module: `golang.org/x/crypto` (pure Go,
  argon2id + bcrypt). No JWT library — HS256 is ~40 lines of stdlib.
- Two new lazily-loaded namespaces: `bri.auth`, `bri.audit` (+ Go shims in
  `pkg/bri`), boot budget untouched (ADR 0024) — nothing loads until the
  app `(require)`s it.
- `bri.http` gains the router/observability surface; `:params` kept for
  back-compat with `:path-params` added as the Compojure-ish alias.
- Behaviors are gated as Go tests in `pkg/bri` (bri has no JVM oracle and
  is not yet AOT-compiled, per the established `bri_test.go` precedent) —
  deterministic (injected clock/secret, captured sinks), covering
  round-trip, alg-pin, guard 401/403, composition equivalence, reverse
  routing, catch-all, live add/remove, custom-mw short-circuit, ops
  endpoints, trusted-proxy IP, rate-limit/auto-ban, and two perf budgets.
- Single-instance abuse-store and metrics registry are honest v1 limits
  with clean seams; the audit DB sink + SRE fan-out are a named T2.

## Perf evidence (spike s44, Apple M5 Pro, CGO_ENABLED=0)

| what | result |
|---|---|
| HMAC-SHA256, JWT-sized | stdlib **219 ns** vs minio-simd 241 ns → stdlib wins |
| JWT sign | hand-rolled **348 ns / 12 allocs** vs golang-jwt 987 ns / 32 |
| JWT verify | hand-rolled **763 ns / 16 allocs** vs golang-jwt 1581 ns / 53 |
| 3 composed guards | **20 ns, 0 allocs** (bare handler 10 ns) |
| per-request auth (verify + parse) | ~900 ns, paid once by the outer guard |
| bcrypt(10) / argon2id | 44 ms / 16 ms — correctly slow |
