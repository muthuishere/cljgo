# bri.core.security — API-first security

Security in one blessed way (ADR 0069): HS256 JWTs (algorithm PINNED
server-side — the token's own `alg` is never trusted), argon2id passwords,
a composable guard family that is plain Ring middleware, and escalating
abuse protection. Every decision (401/403/ban/token-issued) is audited via
`bri.core.audit`.

Guards, rate-limit, CORS, CSRF, sessions, metrics, request-ids and logs
are default-on in the bri.web.http stacks; this is how to reach each piece.

Full guide on the site: https://muthuishere.github.io/cljgo/bri/auth/

## JWT

```clojure
(require '[bri.core.security :as auth])

(auth/sign {:sub "u" :role "admin"})            ; iat/exp injected; exp default 3600s
(auth/verify token)     ; → claims, or nil (bad sig / wrong alg / malformed / expired)
(auth/verify! token)    ; → claims, or throws :auth/unauthorized (→ 401)
(auth/subject claims)   ; the :sub

(auth/issue "user-42" {:role "admin"} {:exp-seconds 900})  ; sign + AUDIT the login
```

Secret from `APP_AUTH__SECRET` (secrets-are-env); unset → per-process
random key (dev-grade — prod MUST set it).

## Passwords

```clojure
(auth/hash-password "s3cret")   ; argon2id PHC string (deliberately ~16 ms)
(auth/check-password pw hash)   ; constant-time; argon2id or legacy bcrypt
```

## Guards (composable Ring middleware)

Each is a middleware value (1 arg) or a handler wrapper (2 args). No/invalid
token → 401; authenticated but predicate false → 403. The subject is marked
on the request so logs/audit show who — even on a 403.

```clojure
(auth/logged-in-only)     ; any valid token
(auth/role-only :editor)  ; token :role must equal "editor"
(auth/user-only)          ; (role-only :user)
(auth/admin-only)         ; (role-only :admin)

(auth/guard #(= (get-in % [:auth/claims :org])       ; the general seam (req → bool)
                (get-in % [:path-params :org])))
(auth/guard-claims (fn [c] (contains? (:scopes c) "notes:write")))
```

On a route:

```clojure
(GET    "/api/notes"      (auth/logged-in-only) #'index)
(DELETE "/api/notes/{id}" (auth/admin-only)     #'delete-one)
```

## Abuse protection: auto-ban

Escalating denial-abuse guard (distinct from `http/rate-limit`'s throughput
cap). After `:threshold` denials (401/403) inside `:window-ms`, ban for
`:cooldown-ms` → 429 + Retry-After before the handler. Default-on in
`(http/api-defaults)`.

```clojure
(auth/auto-ban)                                    ; 5 / 60s / 5min
(auth/auto-ban {:threshold 10 :key :subject})      ; key on :sub (or :ip, or a fn)
```

In-process store (single-instance); swap `:store` for a shared backing.

## From bri.web.http (default-on in the relevant stack)

- `(http/rate-limit n opts)` — plain throughput cap → 429 + Retry-After.
- CORS — `(http/cors {:origins […]})`; permissive `*` in dev, allowlist via
  `:origins` / `APP_HTTP__CORS_ORIGINS` in prod.
- CSRF — gates session-bearing mutating requests; sessionless JSON passes.
- Sessions — signed cookies (HMAC-SHA256; key from `APP_SESSION_KEY`).

## See also

- `docs/guides/bri-http.md` · `docs/guides/bri-db.md` · `docs/guides/bri-otel.md`
- `examples/web-api/` — JWT login, role guards, rate-limit, reverse routing
