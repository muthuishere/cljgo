# ADR 0080 — bri.cli built-in API auth: OpenAPI-default client that logs in automatically

Date: 2026-07-24 · Status: proposed (roadmap; owner-directed: *"some way for
authentication with some openapi default with login automatically"*). Depends on
ADR 0078 (bri.cli), ADR 0069 (bri.auth — HS256 JWT/guards), and the `bri.openapi`
battery (ADR 0075). Third of the bri.cli block.

## Context

A large class of CLIs are **API clients** — they talk to a backend, and the first
friction a user hits is authentication: obtain a token, store it, attach it,
refresh it. The owner wants this **built in** to bri.cli, with an **OpenAPI
default** contract and **automatic login** — a command that needs auth should just
work: if there's no valid credential, log in, cache it, proceed; the developer
should not hand-roll an OAuth dance or a token store. Because bri already owns the
*server* side (bri.http routes + bri.auth JWTs, ADR 0069) and can describe itself
via **OpenAPI** (bri.openapi, ADR 0075), a bri.cli client paired with a bri.http
server should get end-to-end auth with near-zero config.

## Decision

### 1. A built-in API client, defined from an OpenAPI spec

```clojure
(require '[bri.cli :as cli])

(def api
  (cli/api {:base-url "https://api.example.com"
            :openapi  "https://api.example.com/openapi.json"   ; the default contract
            :auth     :device}))                                ; automatic login strategy

;; calls are checked against the spec (path, params, shapes); auth is transparent
(cli/call api :list-notes {:limit 20})
```

`cli/api` loads the OpenAPI document (a bri.http server serves it by default via
bri.openapi), so operations, parameters, and the security scheme are **known from
the spec** — `cli/call` validates the request against it and the developer writes
no per-endpoint client code. A spec is the default, not a requirement: a `:routes`
map works for a server that doesn't publish one.

### 2. Automatic login + token lifecycle

`:auth` selects a strategy; the default suits a bri.auth backend:

- **`:device`** — OAuth 2.0 **device authorization flow**: on the first
  authenticated call with no valid token, bri.cli prints/opens the verification
  URL + user code, polls for completion, and proceeds — no secret typed into a
  flag.
- **`:token`** — a personal token from `--token`/`$APP_TOKEN` (the non-interactive
  / CI path).
- **`:password`** — interactive username + masked password (reusing ADR 0078
  inputs) exchanged for a JWT against the bri.auth login route.

In every strategy the flow is **automatic**: a call needing auth ensures a valid
credential first (login if absent, **refresh** if expired using a stored refresh
token), attaches it as the `Authorization: Bearer …` the OpenAPI security scheme
declares, and only surfaces a prompt/error when it genuinely cannot proceed. An
explicit `todo login` / `todo logout` command is generated for good measure.

### 3. Credential storage — secrets stay secrets

Tokens cache at `~/.config/<app>/credentials.json` (XDG), `0600`, one entry per
`base-url`. The value is **use-only**: attached to a request at the point of the
call, never printed, logged, echoed, or placed in argv/`--help`/error text — the
same discipline bri.auth enforces server-side. An OS-keychain backend is a noted
future option; the file store is the portable default.

### 4. Pairs with the bri server by default

Point a bri.cli client at a bri.http server and it works end-to-end with no glue:
bri.http serves the OpenAPI doc (bri.openapi), bri.auth issues/verifies the JWT
and runs the device/login routes, and bri.cli consumes exactly that contract. A
non-bri / third-party OpenAPI API works too — this is a general client, defaulting
to the bri contract.

## Consequences

- Writing an authenticated API CLI stops being boilerplate: declare the base URL +
  OpenAPI spec + strategy; login, caching, refresh, and header attachment are the
  framework's job.
- The bri client/server halves compose: one team ships a bri.http API and a
  bri.cli that talks to it with auth working out of the box, both pure-Go static
  binaries, both cross-compiled by `cljgo dist`.
- Secret hygiene is structural (use-only file store, never in context) — matching
  the owner's standing rule and bri.auth's server-side posture.
- Roadmap ADR: ratifies the shape (OpenAPI-driven client + automatic
  device/token/password login + safe cache + bri-default pairing); the OpenAPI
  loader, each strategy, and the store land on their own spec/gates, and this
  leans on bri.openapi (ADR 0075) shipping the server-side spec.
- Not chosen: making the developer wire an HTTP client + token store by hand (the
  automation is the point); baking in one identity provider (strategies are
  pluggable, defaulting to the bri.auth contract); storing tokens in plaintext env
  files or anywhere they could enter logs/argv.
