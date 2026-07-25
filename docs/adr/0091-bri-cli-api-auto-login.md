# ADR 0091 — `bri.cli.api`: the OpenAPI client that logs in automatically

Date: 2026-07-25 · Status: accepted (owner: *"some way for authentication with some
openapi default with login automatically"*). Realizes **ADR 0080** by composing
**`bri.web.openapi`** (ADR 0090, the spec-driven client + `:auth-fn` seam),
**`bri.cli.auth`** (ADR 0080 credential core → OS keychain), and **`bri.cli`**
prompts. Completes the bri.cli auth block.

## Context

ADR 0080 asked for a bri.cli **API client with automatic login**: a command that
needs auth just works — obtain a credential, cache it, attach it, refresh it — with
no hand-rolled token store. ADR 0090 shipped the spec-driven client and, crucially, a
per-request **`:auth-fn`** seam so login-backed auth composes in. ADR 0080 also owns
the credential core (`bri.cli.auth`: prompt → OS keychain → header). This ADR ties
the three together.

## Decision

### 1. Placement — a new `bri.cli.api`, not `bri.cli`

ADR 0080 sketched `cli/api` / `cli/call` under `bri.cli`. Putting them there would
make **every** `bri.cli` app `require` `bri.cli.auth` → transitively the **opt-in**
`bri.core.secrets` keychain client (ADR 0074/0076) — linking the keychain into CLIs
that never authenticate. So the auto-login client is its own namespace,
**`bri.cli.api`**, requiring `bri.cli` + `bri.web.openapi` + `bri.cli.auth`. An app
opts into keychain-backed auth by requiring `bri.cli.api`; a plain `bri.cli` app stays
lean. (Same opt-in discipline as ADR 0090's refusal to hard-require `bri.cli.auth`.)

### 2. Automatic login over the `:auth-fn` seam

`api` builds a `bri.web.openapi` client whose `:auth-fn` is *ensure-a-credential*:
on each call it checks `bri.cli.auth/authed?`; if absent it **acquires** one per
strategy, caches it in the OS keychain, and returns the `Authorization` value. So
`call` is just `bri.web.openapi/call` on that client — the login is transparent.

```clojure
(require '[bri.cli.api :as api])

(def a (api/api spec {:service "my-api" :auth :token}))    ; API key, prompted once, cached
(api/call a :list-notes {:limit 20})                        ; logs in on first need, attaches
(api/login a)   (api/logout a)   (api/authed? a)            ; explicit lifecycle
```

Strategies:
- **`:token`** (default) — the API-key / PAT path: prompt once with echo off
  (`bri.cli/ask-secret` via `bri.cli.auth/login`), cache, attach as `Bearer`.
- **`:password`** — prompt username + masked password, exchange them at a spec
  operation (`:login {:op … :username-field … :password-field … :token-path …}`) for
  a token via a **no-auth bootstrap client** (avoids `:auth-fn` recursion), cache the
  token. Non-interactive callers pass `:key`/creds to skip the prompt (CI path).
- **`:device`** (OAuth 2.0 device flow) — *reserved*; the `:auth-fn` seam already
  supports it, but it needs a real device/token endpoint to exercise, so it is
  deferred rather than shipped untested.

### 3. Refresh on 401

`call` inspects the response: a **401** with a cached credential drops it
(`bri.cli.auth/logout`) and retries once — the retry's `:auth-fn` re-acquires (re-login
/ re-exchange). Expiry heals without the caller noticing; a second 401 surfaces.

### 4. Non-OptIn machinery, load order

No Go shims (pure composition). Registered **after** `bri.web.openapi` in
`bri.Specs()` (its top-level requires must resolve). It transitively links the opt-in
keychain — correct, because an app that requires `bri.cli.api` uses auth.

## Consequences

- The owner's ask lands: a spec-driven CLI API client where auth is automatic —
  login once (or never, in CI with a supplied key), cached in the OS keychain,
  attached on every call, refreshed on expiry.
- `bri.cli` stays lean; keychain links only into apps that opt into `bri.cli.api`.
- Credentials remain masked-by-default (`bri.core.secrets`); the only unmask is at
  the `auth-fn` attach point (`bri.cli.auth/auth-header`).

## Not chosen

- **`cli/api` under `bri.cli`** — would link the opt-in keychain into every CLI;
  `bri.cli.api` preserves opt-in (refines ADR 0080's sketch).
- **Shipping `:device` now** — no CI-exercisable endpoint; the seam is ready,
  implementation waits for a tested target rather than shipping dark code.
- **A bespoke token store** — `bri.cli.auth` + `bri.core.secrets` already own the OS
  keychain; `bri.cli.api` composes them, it does not reimplement storage.
