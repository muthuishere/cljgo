# ADR 0103 — Bun-complete `cljg.*` wave: pure-Go (CGO_ENABLED=0) feasibility

**Synthesis of 8 spikes (s57–s64). Date: 2026-07-28.**

## 1. Headline

**YES — the whole wave runs in pure Go with `CGO_ENABLED=0`.** All 8 spikes built stripped
static binaries with cgo off and ran clean. Zero capabilities need cgo; nothing had to be
dropped. Three honest asterisks, not blockers: keychain needs an encrypted-file fallback on
headless Linux, `cljg.compress` zstd/brotli and `bri.grpc` should be opt-in-linked so they
don't bloat every binary, and a few sub-features (grpc streaming/TLS, ws permessage-deflate)
are unexercised follow-on work, not capability gaps.

## 2. Summary table

| Capability | Verdict | CGO-free | Ran clean | Binary (stripped) | Key dep(s) | Recommendation |
|---|---|---|---|---|---|---|
| `cljg.security` | MET-PUREGO-DEP | yes | yes | 3,405,826 B (3.4 MB) | x/crypto (BSD-3), golang-jwt/jwt/v5 (MIT) | GO-WITH-DEP |
| `cljg.security` keychain / `cljg.secrets` | MET-PUREGO-DEP | yes | yes (macOS only executed) | 2,550,290 B (2.5 MB) | go-keyring (MIT) + age (BSD-3) fallback | GO-WITH-DEP |
| `cljg.socket` | MET-STDLIB | yes | yes | 4,761,634 B (4.8 MB) | none (stdlib `net`, `crypto/tls`) | GO |
| `cljg.net.dns` | MET-STDLIB | yes | yes | 2,340,914 B (2.3 MB) | none (stdlib `net.Resolver`) | GO |
| `cljg.compress` | PARTIAL | yes | yes | 4,900,338 B (4.9 MB) | klauspost/compress (BSD-3), andybalholm/brotli (MIT) | GO-WITH-DEP (opt-in zstd/brotli) |
| `cljg.http/serve` | MET-STDLIB | yes | yes | 6,949,634 B (6.9 MB) | none for core; x/net (BSD-3) only for h2c | GO |
| `cljg.ws` | MET-PUREGO-DEP | yes | yes | 6,123,634 B (6.1 MB) | coder/websocket (ISC), zero transitive | GO-WITH-DEP |
| `bri.grpc` | MET-PUREGO-DEP | yes | yes (unary only) | 12,166,578 B (12.2 MB) | grpc (Apache-2.0), protobuf (BSD-3), protocompile (Apache-2.0) | GO-WITH-DEP (opt-in link) |

> Sizes are **standalone spike-driver binaries**, not the marginal cost inside a real cljgo
> build. Each driver re-pays for `crypto/tls`, `net/http`, RSA, etc. Folded into one cljgo
> binary that already links those, the true per-capability delta is smaller — except grpc,
> whose dep graph is genuinely new weight.

## 3. Traffic-light triage

### Green — build now (stdlib or clean single-purpose pure-Go dep)

- **`cljg.socket`** (MET-STDLIB, 0 deps) — TCP/UDP/unix/TLS all on stdlib `net` + `crypto/tls`.
  `net.Conn` already satisfies `io.ReadWriteCloser`, so it composes with `cljg.stream`
  (ADR 0101) with no bridge code.
- **`cljg.net.dns`** (MET-STDLIB, 0 deps) — full Bun.dns surface (A/AAAA, PTR, MX, TXT, SRV,
  CNAME, NS) over `net.Resolver{PreferGo:true}`. CGO=0 removes libc `getaddrinfo`; the pure-Go
  resolver is the only one compiled in, deterministic across platforms.
- **`cljg.http/serve`** (MET-STDLIB, 0 core deps) — HTTP/1.1, HTTPS self-signed, HTTP/2 over
  TLS via ALPN, graceful `Server.Shutdown` are all stdlib. Only h2c (cleartext HTTP/2) pulls
  x/net — gate it behind a build tag if a truly zero-dep serve is wanted.
- **`cljg.security`** (MET-PUREGO-DEP) — the biggest batteries win at lowest risk: sha/hmac/
  random/uuid/base64/hex are stdlib; argon2id, bcrypt, blake2b, JWT HS256/RS256 need two
  boring vetted libs (x/crypto, golang-jwt). All AOT-compiles into a static CGO=0 binary.
- **`cljg.ws`** (MET-PUREGO-DEP) — duplex text+binary round-trip on coder/websocket, one ISC
  module with **zero transitive deps** (go.sum is 2 lines). Maps 1:1 onto the `cljg.stream`
  duplex contract.

### Yellow — ships, but with a specific caveat

- **`cljg.security` keychain (NEEDS-DAEMON on headless Linux).** Native OS keychain works on
  macOS (shells out to `/usr/bin/security`) and — by source inspection, not executed here —
  Windows (wincred pure-Go syscalls) and Linux-with-daemon (Secret Service over D-Bus). On a
  bare server / CI with **no secret-service daemon**, go-keyring's Linux `Set`/`Get` fail with
  `org.freedesktop.DBus.Error.ServiceUnknown`. **The pure-Go age-scrypt encrypted-file
  fallback is therefore MANDATORY, not optional.** Also: go-keyring has a ~3 KB secret cap on
  some backends (fine for tokens/keys, not blobs); macOS needs an unlocked login keychain
  (locked keychain / non-login SSH can prompt or fail).
- **`cljg.compress` (PARTIAL).** gzip/flate/zlib are free stdlib (effectively MET-STDLIB).
  **zstd and brotli each need a pure-Go dep** (klauspost/compress, andybalholm/brotli) — no C,
  no cgo, permissive, ecosystem-standard, but ~4.9 MB of decode tables that a gzip-only program
  would still pay for if all five ship always-on. **Gate zstd/brotli behind opt-in linking
  (ADR 0074 style);** ship the core three in base. A shim must normalize the per-codec API
  quirks (zstd `.IOReadCloser()`, brotli `Reader` has no `Close`) into one uniform protocol.
- **`bri.grpc` (opt-in weight).** Works fully CGO=0, but **12.2 MB** stripped for hello-world
  gRPC vs ~5.3 MB plain cljgo. **MUST be opt-in per-namespace linking (ADR 0074), never in
  default core**, or it inflates every binary. Only unary is proven; streaming + TLS + the
  Clojure-map↔dynamicpb ergonomic wrapper are follow-on (same pure-Go machinery, no new
  external deps).

### Red — hard / drop (NEEDS-CGO or NOT-AVAILABLE)

**None.** No spike required cgo and nothing is unavailable in pure Go. The only strictly
"missing" sub-features are additive polish, not blockers: `cljg.ws` permessage-deflate
compression (dropped by newer coder/websocket — gorilla/websocket, pure Go BSD-3, still has it
if ever needed), and `bri.grpc` streaming/TLS/wrapper.

## 4. go.mod impact across the whole wave

Deduplicated third-party modules the full wave would add. **Every one is pure Go, cgo-free,
permissively licensed** (verified in-spike: no `import "C"`, no `.c`/`.h` in module sources;
the `*_amd64.s` files in argon2/blake2b are Go plan9 asm, not cgo):

| Module | License | Pure Go | Used by |
|---|---|---|---|
| `golang.org/x/crypto` | BSD-3-Clause | yes | security, keychain |
| `github.com/golang-jwt/jwt/v5` | MIT | yes | security (JWT) |
| `github.com/zalando/go-keyring` | MIT | yes | keychain |
| `github.com/godbus/dbus/v5` | BSD-2-Clause | yes | keychain (Linux Secret Service) |
| `github.com/danieljoos/wincred` | MIT | yes | keychain (Windows) |
| `al.essio.dev/pkg/shellescape` | MIT | yes | keychain (indirect) |
| `filippo.io/age` | BSD-3-Clause | yes | keychain encrypted-file fallback |
| `github.com/klauspost/compress` | BSD-3-Clause | yes | compress (zstd) |
| `github.com/andybalholm/brotli` | MIT | yes | compress (brotli) |
| `golang.org/x/net` | BSD-3-Clause | yes | http/serve (h2c), grpc |
| `golang.org/x/text` | BSD-3-Clause | yes | indirect (http, grpc) |
| `golang.org/x/sys` | BSD-3-Clause | yes | indirect (security, grpc) |
| `golang.org/x/sync` | BSD-3-Clause | yes | indirect (grpc) |
| `github.com/coder/websocket` | ISC | yes | ws |
| `google.golang.org/grpc` | Apache-2.0 | yes | grpc |
| `google.golang.org/protobuf` | BSD-3-Clause | yes | grpc |
| `github.com/bufbuild/protocompile` | Apache-2.0 | yes | grpc (in-process .proto) |
| `google.golang.org/genproto/googleapis/rpc` | Apache-2.0 | yes | indirect (grpc) |

Licenses across the wave: BSD-2/BSD-3, MIT, ISC, Apache-2.0 — all permissive, no copyleft.

**Binary-size honesty (per CLAUDE.md competitive discipline — factual, no spin):**
The three stdlib capabilities (socket, dns, http-core) add **zero** modules. The security,
keychain, ws, and compress-core deps are small and mostly shared (x/crypto is reused by
security + keychain). The one real weight is **grpc's dep graph (~12.2 MB driver)** — that is
new, genuinely heavy, and the reason grpc must be lazily linked. `go list -m all` on the grpc
spike shows ~45 modules, but ~30 are grpc test-only/optional (otel/envoy/xds/spiffe/gonum/
testify) and are NOT in the 154-package compiled graph — the actual build pulls only the deps
table above. Don't claim grpc is "free"; claim it's "opt-in and isolated."

## 5. Keychain — the honest cross-platform truth under CGO=0

- **macOS:** proven by execution. go-keyring **shells out to `/usr/bin/security`** — it does
  NOT link the Keychain C API, which is exactly why it stays CGO=0. `otool -L` on the binary
  shows only `libSystem`/`libresolv`, no Keychain framework. Needs `/usr/bin/security` on PATH
  (always present) and an **unlocked login keychain** — a locked keychain or non-login SSH
  session can prompt or fail.
- **Windows:** **not executed** — claimed from source. wincred uses pure-Go syscalls to
  Credential Manager, no cgo. Should work; treat as unverified until run on Windows.
- **Linux (desktop, daemon running):** **not executed** — claimed from source. Secret Service
  over D-Bus (godbus, pure Go). Works when gnome-keyring/KWallet is up on a session bus.
- **Linux (headless / CI, no daemon):** **native keychain does NOT work** — `Set`/`Get` fail
  with `ServiceUnknown`. This is the real trap. **The pure-Go age-scrypt encrypted-file
  fallback (proven in-spike, 205-byte round-trip) is mandatory here.**

Net: ship go-keyring for native OS keychain **plus** the age encrypted-file fallback as a
first-class, always-available backend — not a nice-to-have.

## 6. The grpc-without-protoc finding — the single most important de-risking result

`bri.grpc` served a real unary RPC (`/demo.Greeter/SayHello`, `hello, cljgo!` round-trip)
with the **full purity contract intact: CGO=0, no `protoc` binary, no codegen, no daemon.**
The `.proto` was compiled **in-process** via `bufbuild/protocompile`, and the service was
served dynamically via `dynamicpb` + a runtime `grpc.ServiceDesc` — **no generated `.pb.go`
files at all.**

Why this matters: the classic gRPC objection is the toolchain — install protoc, run codegen,
regenerate on every schema change. That entire burden is gone. For cljgo it means **"add
gRPC" = cljgo + a `.proto` file at runtime**, which fits the REPL/interpreter model natively
and needs no build step. The remaining work (streaming, TLS via stdlib `crypto/tls`, and a
Clojure-map↔dynamicpb ergonomic wrapper) is bounded data-shuffling on the same pure-Go
machinery — no new external deps, no new risk. This turns grpc from "probably too heavy /
too much toolchain" into "yes, opt-in, and the hard part is already solved."

## 7. Recommended wave sequencing

Sequence by (value × low-risk) and by dependency sharing, so the cheap stdlib wins land first
and the heavy opt-in stuff is isolated last:

1. **Wave A — stdlib freebies (zero deps, ship immediately):** `cljg.socket`, `cljg.net.dns`,
   `cljg.http/serve` (core, h2c gated off). No go.mod churn, no size story to defend.
2. **Wave B — the batteries win:** `cljg.security`. Biggest user value (passwords/JWT/hashing),
   two boring vetted deps, 3.4 MB. Enforce JWT alg-pinning on verify **at the wrapper layer**
   so callers can't create the RS256→HS256 downgrade hole; ship argon2id as the password
   default (Bun parity), bcrypt for compat.
3. **Wave C — reuses B's crypto dep:** keychain / `cljg.secrets`, riding x/crypto already
   pulled by security. Ship native keychain **and** the age fallback together; document the
   headless-Linux truth up front.
4. **Wave D — duplex I/O:** `cljg.ws` (one ISC dep, zero transitive) — pairs naturally with the
   `cljg.stream` contract and the Wave-A http server.
5. **Wave E — opt-in-linked, don't touch default binary size:** `cljg.compress` (core three in
   base; zstd/brotli behind opt-in link) and `bri.grpc` (fully lazy per-namespace link,
   ADR 0074). These carry the wave's only real weight, so they go last and stay off the
   default path.

Rationale: Waves A–D grow the default binary modestly and share deps; Wave E's heavy/optional
pieces are quarantined behind opt-in linking so a hello-world cljgo binary never pays for zstd
tables or the grpc graph. Every wave is CGO=0 and permissively licensed — the purity and
size story stays defensible at each step.
