# adr-0103-cljg-bun-wave1 — Bun-complete cljg.* Wave 1

## Why

ADR 0103 (feasibility proven by spikes s57–s65, all pure-Go CGO=0) makes cljgo
Bun-complete on the mechanism tier. Owner-final Wave-1 scope (2026-07-28):
**`cljg.http/serve` and `cljg.socket` are core** and land now, alongside
`cljg.security`, `cljg.net.dns`, `cljg.compress`. WebSocket and gRPC are
framework-tier (`bri.ws` / `bri.grpc`) and land later with bri.

## What Changes

Five namespaces, all lazy + registered in `bri.Specs()` (never
`core.BootSources()`), all dual-harness conformance-gated:

1. **`cljg.security`** — RENAME `bri.core.security` (file `core/bri/auth.cljg`,
   spec row bri.go:95) → `cljg.security`, rename-in-place at row position
   (ADR 0102 style), then COMPLETE it: password `hash-password`/`check-password`
   (argon2id — do NOT shadow the existing JWT `verify`), `sha256`, `hmac`,
   secure `random`/`token`, `uuid`, base64/hex — and `save-to-keychain`/
   `get-from-keychain`/`delete-from-keychain` with the s65 unified store:
   native go-keyring when reachable, age-encrypted-file fallback otherwise
   (`:backend :auto|:native|:file`). Keychain Go code goes in an ISOLATED
   opt-in package (`pkg/bri/security`) because `TestSecretsIsOptIn` forbids
   go-keyring in always-linked pkg/bri; crypto shims stay in pkg/bri.
   Namespace becomes OptIn with ShimImport. Secret values never printed.
2. **`cljg.http/serve`** — EXTRACT the raw HTTP server primitive out of
   `bri.web.http`: `serve` (port + handler fn, request map in / response map
   out), TLS, graceful `stop`. New namespace `cljg.http` (file
   `core/cljg/http.cljg`, Go shim carved from pkg/bri/http.go's `-serve`).
   `bri.web.http` REBUILDS on top (requires cljg.http, keeps routing-as-data,
   middleware, ops endpoints, var-deref live-reload). ALL existing bri.web
   conformance/templates must stay byte-identical — the framework's observable
   behavior may not change.
3. **`cljg.socket`** — new: TCP/UDP/unix `listen`/`accept`/`dial`,
   connections as `cljg.stream`-composable duplex handles (stdlib net;
   net.Conn is io.ReadWriteCloser). TLS dial/listen via crypto/tls.
4. **`cljg.net.dns`** — new: `lookup`/`reverse`/`mx`/`txt`/`srv`/`cname`/`ns`
   over `net.Resolver{PreferGo:true}` (stdlib, 0 deps).
5. **`cljg.compress`** — new: gzip/deflate/zlib compress+decompress
   (stdlib), string and byte surfaces + streaming over cljg.stream.
   zstd/brotli DEFERRED (opt-in deps, later).

## Impact

- New/renamed rows in `pkg/bri/bri.go` Specs(); new embeds in `core/cljg.go`;
  regenerated briaot twins (`go generate ./pkg/briaot`); briloader/genbri blank
  imports for the isolated security package.
- `bri.web.http` internals rewire onto `cljg.http` (behavior frozen).
- Callers of `bri.core.security` renamed (grep-gated: zero stale refs).
- go.mod adds: zalando/go-keyring, filippo.io/age (isolated opt-in package
  only). x/crypto already present.
- CI guards extended: keychain deps must NOT appear in always-linked packages.
