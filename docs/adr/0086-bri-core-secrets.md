# ADR 0086 — `bri.core.secrets`: an opt-in, cgo-free secret store (env + OS keychain), masked by default

Date: 2026-07-25 · Status: accepted (owner-directed: *"keystore save and get login
password from web page or through api key but saved in secret store"*). Applies
ADR 0060 (pluggable secrets & vault, spike S39) under the ADR 0085 taxonomy.

## Context

ADR 0060 accepted the S39 vault design — one `Provider` interface + a URI-scheme
registry + a fallback `Chain`, all proven `CGO_ENABLED=0` — under the pre-taxonomy
name `bri.vault`. ADR 0085 then re-homed secrets under the **`bri.core.security`**
concern (*"bri.core.security — auth/JWT/guards + secrets/keystore/login/vault"*).
Two constraints shape where it lands:

- **The OS keychain + its transports are a real dependency** (`zalando/go-keyring`
  pulls `godbus/dbus` + `danieljoos/wincred` + `x/sys`). It must be **opt-in
  linked** (ADR 0074/0076) so a binary that never touches a secret store carries
  none of it — exactly like `bri.core.data`/`bri.core.telemetry`.
- **`bri.core.security` (auth) is NON-opt-in** (JWT is cheap, always linked). A
  namespace is opt-in or not as a whole; folding a heavy opt-in vault into the
  always-linked auth namespace is structurally wrong.

## Decision

Ship the vault as **`bri.core.secrets`** — a distinct **OptIn leaf** under the
`bri.core` umbrella, isolated in `pkg/bri/secrets` (its own package, linked only
when an app requires the namespace), realizing ADR 0060's design. `bri.core.security`
(auth) stays non-opt-in and untouched.

### Scope of this increment: `env://` + `keychain://`

- **`env://KEY`** — the dependency-free floor (CI/containers inject secrets as env
  vars); the natural tail of a fallback chain.
- **`keychain://service/account`** — the OS secret store via `go-keyring`, **pure
  Go on every platform** (macOS execs `/usr/bin/security`; Linux speaks the D-Bus
  Secret Service protocol; Windows calls `wincred` via `x/sys`) — S39-proven
  `CGO_ENABLED=0` + cross-compiles. It is the only writable backend here.

`age://` (committed ciphertext) and the cloud stubs (S39) are deferred to a later
increment; the scheme registry makes them additive.

### The masking boundary (the load-bearing hygiene decision)

A secret's **value must never reach a log, the REPL echo, or the model context**
(project + global CLAUDE.md, absolute rule). So `secrets/get` does NOT return a
bare string:

- `get` returns a **masked secret object** — a map `{:bri.core.secrets/secret true
  :masked "len=25 ***…sh" :source "env"}` whose printed form shows only the mask
  (`len=N ***…xy`, last-2 tail; `***(empty)` / `len=N ***` for short values). The
  **raw value lives in the object's metadata**, which `pr`/`println` do NOT print
  (`*print-meta*` is false by default) — so an accidental `(println secret)` is
  safe.
- `reveal` is the **one explicit, auditable seam** that returns the plaintext
  (from the metadata). Grep for `reveal` to find every unmask site — the same
  discipline as S39's `Secret.Reveal()`.
- `set`/`delete` (keychain only) take the raw value as an argument and return nil;
  a read-only scheme (`env`) errors on write.

### Chain (fallback)

`get` accepts a single URI or a **vector of URIs** tried left→right: first hit
wins; a real backend **failure aborts** the chain (never silently falls through a
broken keychain to a weaker source) — a plain miss rolls on. This is the
laptop-keychain-then-CI-env pattern.

## Consequences

- cljgo gains the owner's "save/get from a secret store" primitive as a static,
  cross-compilable, **opt-in** battery — a non-secrets binary links zero keychain
  code, and the whole thing stays `CGO_ENABLED=0` (a `go list -deps` cgo gate, as
  for every opt-in leaf).
- Secrets are **masked by default at every surface**; plaintext requires an
  explicit, greppable `reveal`. The value never enters an interpreter print path.
- The scheme registry keeps `age://` + cloud providers purely additive.
- The `keychain://` write path (`set`) composes with the bri.cli secret prompt
  (increment 2): a login flow prompts for an API key with echo off, then stores it
  — the owner's "login … saved in secret store", wired in a later increment.
- Interpreter-vs-binary parity holds: the Clojure surface (mask, wrap, chain
  policy) is portable; only the fetch/store are host shims, and a compiled
  `bri.core.secrets` app is dual-mode tested. Live keychain round-trips are NOT
  run in CI (no keychain session there) — CI covers env + chain + masking + the
  cgo-free build; the keychain path is covered by build + scheme resolution.

## Not chosen

- **Folding the vault into `bri.core.security`** — mixes an opt-in heavy dep into
  the always-linked auth namespace; the Spec model is per-namespace opt-in.
- **Returning a bare string from `get`** — one careless `println` leaks the secret
  into logs/context; rejected on the owner's absolute hygiene rule.
- **A Go-side handle table holding raw values** — safe from printing but a global
  mutable secret store with an unbounded lifetime; the metadata approach ties the
  plaintext's lifetime to the object's GC and stays portable.
- **`age://` + cloud now** — additive via the registry; deferred to keep this
  increment tight and its dependency surface to `go-keyring` only.
