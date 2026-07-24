---
title: "bri.core.security"
description: "API-first security in one blessed way: pinned HS256 JWTs, argon2id passwords, a composable guard family that is plain Ring middleware, and escalating abuse protection — every decision audited."
---

`bri.core.security` is API-first security in one blessed way (ADR 0069): HS256 JWTs (the algorithm is **pinned** server-side — the token's own `alg` header is never trusted), argon2id passwords, a composable **guard** family that is plain Ring middleware (`handler → handler`, so `->` composes them), and escalating abuse protection. Every security decision — 401, 403, ban, token issued — is audited through [`bri.core.audit`](/cljgo/bri/http/).

Guards, rate-limiting, CORS, CSRF, sessions, metrics, request-ids and structured logs are **default-on** in the [bri.web.http](/cljgo/bri/http/) API stack — this page is how you reach for each piece explicitly.

## JWT: sign, verify, issue

```clojure
(require '[bri.core.security :as auth])

(auth/sign {:sub "u" :role "admin"})           ; → an HS256 token string
(auth/sign claims {:exp-seconds 900 :secret s}) ; iat/exp injected; exp default 3600s

(auth/verify token)     ; → the claims map, or nil (bad sig / wrong alg / malformed / expired)
(auth/verify! token)    ; → claims, or throws :auth/unauthorized (funnels to 401)

(auth/subject claims)   ; the token subject (:sub) — the audit actor
```

`(auth/issue sub claims opts)` signs a token for a subject **and audits the issuance** (a login success) — the blessed login call:

```clojure
(auth/issue "user-42" {:role "admin"} {:exp-seconds 900})
```

The signing secret comes from `APP_AUTH__SECRET` (the secrets-are-env doctrine). With none set you get a per-process random key: sign and verify agree within a run, but tokens do not survive a restart — dev-grade, and **prod must set it**. Perf (spike s44): sign ~350 ns, verify ~760 ns.

## Passwords: argon2id

```clojure
(auth/hash-password "s3cret")     ; → an argon2id PHC string (OWASP params)
(auth/check-password pw hash)     ; constant-time; verifies argon2id or legacy bcrypt
```

argon2id is **deliberately** slow (~16 ms) — that is the anti-brute-force feature, not a bug; never SIMD-fast. `check-password` also verifies legacy `$2a$`/`$2b$` bcrypt hashes so you can migrate.

## Guards: composable authorization

Guards are plain Ring middleware. Each is both a middleware value (one arg) and a handler wrapper (two args), so they drop into a route or compose with `->`. No/invalid token → **401**; authenticated but the predicate is false → **403** (RFC 6750). Every deny is audited, and the authenticated subject is marked on the request so the access log and audit trail show *who* — even on a 403.

```clojure
(auth/logged-in-only)        ; any valid token
(auth/role-only :editor)     ; token :role must equal "editor"
(auth/user-only)             ; sugar for (role-only :user)
(auth/admin-only)            ; sugar for (role-only :admin)
```

`guard` is the general seam — a predicate `(req → boolean)` with `:auth/claims` populated:

```clojure
;; a one-line tenant check
(auth/guard #(= (get-in % [:auth/claims :org])
                (get-in % [:path-params :org])))

;; the predicate can see the claims map directly
(auth/guard-claims (fn [c] (contains? (:scopes c) "notes:write")))
```

`logged-in-only` / `role-only` / `user-only` / `admin-only` are all thin specializations of `guard`. On a route (Compojure-style, from [bri.web.http](/cljgo/bri/http/)):

```clojure
(http/routes
  (GET    "/api/notes"      (auth/logged-in-only) #'index)
  (DELETE "/api/notes/{id}" (auth/admin-only)     #'delete-one))
```

## Abuse protection: auto-ban

`auto-ban` is the escalating **denial-abuse** guard (distinct from `http/rate-limit`, which is a plain throughput cap). After `:threshold` denials (401/403) inside `:window-ms`, a client is banned for `:cooldown-ms`; banned requests short-circuit **429 + Retry-After** *before* the handler runs, and every ban is audited. It is **default-on** in `(http/api-defaults)`.

```clojure
(auth/auto-ban)                                    ; defaults: 5 / 60s window / 5min cooldown
(auth/auto-ban {:threshold 10 :window-ms 30000
                :cooldown-ms 600000 :key :subject}) ; key on the authenticated :sub
```

Keys on client IP by default (proxy-aware `http/client-ip`), or `:subject`, or a custom `(fn [req] key)`. The store is in-process (single-instance); swap `:store` for a shared backing to make it cluster-wide. Tune or drop it from the API stack via `{:auto-ban {…}}` / `{:auto-ban false}` on `api-defaults`.

## Rate-limit, CORS, CSRF, sessions

These live in [bri.web.http](/cljgo/bri/http/) and are on by default in the relevant stack:

- **`(http/rate-limit n opts)`** — plain throughput: at most `n` requests per `:window-ms` per client key; over → 429 + Retry-After. Raw throughput, not denial-abuse.
- **CORS** — `(http/cors {:origins […]})`; permissive (`*`) and loud in dev, allowlist in prod via `:origins` or `APP_HTTP__CORS_ORIGINS`. Default-on in `api-defaults`.
- **CSRF** — gates session-bearing mutating requests on a token `bri.web.html/form` mints (or the `x-csrf-token` header). Sessionless JSON requests pass — a curl with no cookie has nothing to forge. Default-on in `(http/defaults)`.
- **Sessions** — signed cookies (HMAC-SHA256; key from `APP_SESSION_KEY`, else per-process random). Read as `:session`; attach with `(http/start-session res {…})`.

## Everything a token does — one login handler

```clojure
(defn login [req]
  (let [{:keys [user pass]} (:json req)
        u (get users user)]
    (if (and u (auth/check-password pass (:pass u)))
      (http/ok {:token (auth/issue user {:role (:role u)})})   ; signs + audits
      (http/json 401 {:error "bad credentials"}))))
```

## Where next

- [bri.web.http](/cljgo/bri/http/) — the middleware stack, rate-limit, CORS, CSRF, the error funnel
- [bri.core.data](/cljgo/bri/db/) — the model layer your guarded handlers call
- [bri.core.telemetry](/cljgo/bri/otel/) — opt-in tracing that records the authenticated subject on each span
- [bri.core.config](/cljgo/bri/config/) — where `APP_AUTH__SECRET` and `APP_SESSION_KEY` come from
