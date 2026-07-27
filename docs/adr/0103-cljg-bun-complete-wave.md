# ADR 0103 — `cljg.*` Bun-complete: transport, security & util primitives

Date: 2026-07-28 · Status: **proposed** (owner-directed, 2026-07-28: *"like Bun …
move auth as cljg.security and add password hash / save-in-keychain /
retrieve-from-keychain … grpc can go to bri, socket to cljg.socket"*). Extends
the taxonomy of **ADR 0085** and the lazy + opt-in doctrine of **ADR 0096/0101**.
Umbrella for the wave; a namespace may split to its own ADR at implementation
(block **0103–0110** reserved).

## Context

cljgo already ships the Bun "batteries" mechanism tier — HTTP client, files,
streams, SQLite+Postgres, shell, env, test-runner, native-binary build. The gaps
vs Bun are the **transport server side** and a few **util primitives**, plus one
mislabel: the raw HTTP server and the crypto/auth primitives live under `bri.*`
(framework) when they are `cljg.*` mechanism.

## Decision — the line, then the namespaces

**The line (settles every future primitive):** a **transport/connection
primitive or a crypto/util primitive** any program builds on is `cljg.*`
mechanism; an **opinionated service framework** (its own conventions, codegen,
service definitions) is `bri.*` policy.

| namespace | Bun analog | decision |
|---|---|---|
| **`cljg.http`** | `Bun.serve` | **Extract** the raw HTTP server (bind port + handler fn, TLS/HTTP2 from Go `net/http`) out of `bri.web.http` into `cljg.http/serve`. `bri.web` (routing-as-data, hiccup, middleware) is **rebuilt on top** — the framework keeps its opinion, the server becomes a primitive. |
| **`cljg.socket`** | `Bun.listen` | New: TCP + UDP `listen`/`dial`/`accept`, connections as `cljg.stream` handles (ADR 0101). |
| **`cljg.ws`** | Bun WebSocket | New: the raw websocket **connection** primitive (upgrade + bidirectional frames), a `cljg.stream`-shaped duplex. `bri.web` may add ws-route sugar on top; the connection itself is mechanism. |
| **`cljg.security`** | `Bun.password`/`Bun.hash`/crypto | **Rename** `bri.auth` → `cljg.security` and complete it: password `hash`/`verify` (argon2/bcrypt), `hmac`, `sha*`/`blake`, secure `random`/`token`, `uuid`, base64/hex — plus **`save-to-keychain`/`get-from-keychain`** (OS keychain write+read, the primitive `cljg.secrets`' `keychain://` resolver builds on). JWT sign/verify stays here. This is the same "primitive mislabeled as framework" fix as the HTTP server. |
| **`cljg.net.dns`** | `Bun.dns` | New: `resolve`/`lookup`/`reverse` over Go `net`. |
| **`cljg.compress`** | zlib | New: gzip/deflate/zstd encode+decode, streaming over `cljg.stream`. |
| **`bri.grpc`** | — | New, in **`bri`** (owner call): gRPC services carry protobuf + codegen + a service opinion — framework, not mechanism. |

Everything `cljg.*` here is **lazy + opt-in linked** (ADR 0096 mandate B): a
binary that never requires it pays zero bytes. Hot paths are **Go host
primitives** (mandate A) — `net/http`, `crypto/*`, `compress/*`, `net` — under a
thin Clojure API.

## Consequences

- **cljgo becomes Bun-complete on the mechanism tier**: serve, sockets, ws, dns,
  compression, and a full security primitive set, all built-in and available by
  default, nothing unused shipped.
- **`bri.*` gets cleaner still**: the raw server and the crypto primitives leave
  for `cljg.*`; `bri` keeps only opinion (web routing, grpc services, auth
  *policy* that composes `cljg.security`).
- **Keychain has one owner**: `cljg.security` owns the read/write primitive;
  `cljg.secrets` (ADR 0102) composes it for its `keychain://` store. No
  duplicate keychain code.
- **`cljg.security` rename touches `bri.auth` callers** (like the ADR 0102
  migration) — mechanical, grep-gated.
- **Deliberately deferred / not mirrored**: `Bun.s3`, `Bun.redis` (bring-your-own
  backend behind `cljg.cache`), TS/JSX, HTMLRewriter, node-compat.

## Feasibility — confirmed pure-Go, CGO=0 (2026-07-28, spikes s57–s64)

Before building, all 8 capabilities were spiked as self-contained Go modules,
**each compiled AND run under `CGO_ENABLED=0`** — every one green, zero cgo,
nothing dropped. Full triage + go.mod impact + wave sequencing:
`spikes/RESULTS-adr-0103-feasibility.md`.

- **Stdlib freebies (0 deps):** `cljg.socket`, `cljg.net.dns`, `cljg.http/serve`
  (core; h2c needs x/net).
- **Pure-Go dep (permissive):** `cljg.security` (x/crypto, golang-jwt),
  keychain (go-keyring **+ mandatory age encrypted-file fallback** — native
  keychain is NEEDS-DAEMON on headless Linux), `cljg.ws` (coder/websocket, zero
  transitive), `cljg.compress` zstd/brotli (klauspost, andybalholm — opt-in).
- **Killer de-risk:** `bri.grpc` runs a real unary RPC with **no `protoc`, no
  codegen** — `.proto` compiled in-process (bufbuild/protocompile) + served via
  dynamicpb. Heavy (~12 MB) → **opt-in per-namespace link only** (ADR 0074).
- All deps BSD/MIT/ISC/Apache-2.0, verified cgo-free (no `import "C"`, no C src).

## Process (staged — not one mega-change)

1. **Wave 1 (now):** `cljg.security` (rename + password + keychain), `cljg.socket`,
   `cljg.net.dns`, `cljg.compress` — additive + one rename, tractable in parallel.
2. **Wave 2:** `cljg.http/serve` extraction (delicate `bri.web` rewire — its own
   ADR + care) and `cljg.ws`.
3. **Wave 3:** `bri.grpc` (protobuf codegen — the heaviest, own ADR).

Each namespace ships lazy + opt-in with oracle-or-cljgo-frozen conformance and
dual-harness parity, gates green. **Feasibility spikes are DONE (s57–s64, all
green — see the section above);** refined build order is Wave A (stdlib
freebies) → B (security) → C (keychain, reuses B's x/crypto) → D (ws) → E
(compress + grpc, opt-in-linked, quarantined from default binary size).
