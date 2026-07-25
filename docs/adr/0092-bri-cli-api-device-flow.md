# ADR 0092 — `bri.cli.api` `:device`: OAuth 2.0 Device Authorization Grant

Date: 2026-07-25 · Status: accepted (owner: *"security is hard across, we need a
better way"*). Realizes the **`:device`** strategy **ADR 0091 §2 reserved** (and
listed under *Not chosen — shipping `:device` now*, "no CI-exercisable endpoint").
This ADR does not reverse 0091; it lifts that deferral now that the flow is
exercised end-to-end by an in-process RFC-8628 server in the test suite.

## Context

`bri.cli.api` ships two login strategies: `:token` (paste an API key / PAT, echo
off) and `:password` (type a username + password, exchanged for a token). Both put
a **secret into the terminal** — the exact thing "security is hard, we need a better
way" pushes against. A shared machine, shell history, a screen-share, or a
scrollback buffer all leak it; a pasted long-lived PAT is a standing liability.

The industry answer for CLIs is the **OAuth 2.0 Device Authorization Grant (RFC
8628)** — what `gh auth login`, `aws sso`, and `gcloud` use. The CLI never sees a
password: it asks the authorization server for a short **user code**, shows the
user a **URL + code**, the user authorizes in a *browser* (with the provider's own
MFA), and the CLI **polls** a token endpoint until an access token drops out. The
credential that lands is a scoped, expiring token, not a reusable secret typed on a
terminal. ADR 0091's `:auth-fn` seam already supports it — it only needed the flow.

## Decision

### 1. `:device` as a third `bri.cli.api` strategy

`(api spec {:service … :auth :device :device {…}})`. The `:device` map carries the
endpoints (no secret):

```clojure
(def a (api/api spec
  {:service "my-api" :auth :device
   :device {:device-url "https://issuer/oauth/device/code"   ; device authorization endpoint
            :token-url  "https://issuer/oauth/token"         ; token endpoint
            :client-id  "cli-abc"                            ; public client id (not a secret)
            :scope      "openid profile api"}}))             ; optional
(api/call a :list-notes {:limit 20})   ; first need → device flow → cached token attached
```

### 2. The flow (RFC 8628), over `cljg.net.http`

`acquire!` dispatches `:device` to `device-acquire!`, which talks to the OAuth
endpoints directly with `cljg.net.http` (form-encoded, `:form`) — **not** through the
`bri.web.openapi` boot client, because the OAuth endpoints are the issuer's, not the
API spec's operations, and the grant is `application/x-www-form-urlencoded`:

1. **Device authorization request** — `POST device-url` `client_id` (+ `scope`) →
   `{device_code, user_code, verification_uri, verification_uri_complete?,
   interval, expires_in}`.
2. **Instruct the user** — print `verification_uri` + `user_code` (and the
   pre-filled `verification_uri_complete` when present). No prompt, no echo, no
   secret entered.
3. **Poll** — `POST token-url` `grant_type=urn:ietf:params:oauth:grant-type:device_code`
   + `device_code` + `client_id`, every `interval` seconds:
   - `access_token` present → **cache it** (`bri.cli.auth/login … {:key token}` → OS
     keychain), done.
   - `error=authorization_pending` → keep polling.
   - `error=slow_down` → add 5 s to the interval (RFC 8628 §3.5), keep polling.
   - any other `error`, or `expires_in` elapsed → a named `bri.cli.api/device-*`
     error.

`interval` and `expires_in` come from the server; `:poll-interval-ms` overrides the
wait (tests drive it to a few ms), and the poll count is bounded by
`expires_in / interval` so a stalled authorization cannot loop forever. The sleep is
`clojure.core/-sleep-ms` (the `time.Sleep` seam, present interpreted **and** AOT).

### 3. Unchanged everywhere else

The cached token is an ordinary `bri.cli.auth` credential: `authed?`/`logout`/the
`:auth-fn` attach point, the 401-refresh in `call` (a 401 drops it and re-runs the
device flow), and masked-by-default storage all work as for `:token`/`:password`.
Still **pure composition** — no new Go shims (`cljg.net.http` + `clojure.core/-sleep-ms`
are the only host calls); `bri.cli.api`'s Spec stays `install: nil`.

## Consequences

- A `bri.cli.api` command can authenticate with **no secret ever typed into the
  terminal** — browser-based, provider-MFA-capable, scoped-and-expiring tokens.
  This is the "better way" the owner asked for.
- Dual-harness: exercised interpreted (stubbed keychain + in-process RFC-8628
  server) and loaded AOT (the compiled binary constructs a `:device` client).
- `:token`/`:password` remain for issuers without a device endpoint; `:device` is
  the recommended path where the provider supports it.

## Not chosen

- **A browser auto-open** (`open`/`xdg-open`) — printing the URL + code is portable
  and headless-safe (SSH, CI); auto-open can be a later opt-in, it is not required
  by RFC 8628 and would add an OS shell-out.
- **Authorization Code + PKCE (a loopback redirect)** — needs a local HTTP listener
  and a real browser on the same host; the device grant is strictly better for a
  CLI that may run over SSH. PKCE stays a possible future strategy.
- **A refresh-token store** — the access token is cached like any credential; a 401
  re-runs the device flow. Silent refresh-token rotation is a future refinement, not
  this increment.
