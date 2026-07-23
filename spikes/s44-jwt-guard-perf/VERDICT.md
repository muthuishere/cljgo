# S44 VERDICT — JWT + guard + password-hash performance

**Status: CLOSED. Blessed path: hand-rolled HS256 on Go stdlib
`crypto/sha256`, ZERO external JWT/crypto dependencies for signing;
argon2id (OWASP params) for passwords via `golang.org/x/crypto`
(pure-Go), bcrypt-verify for compat. Guard composition proven
nanoseconds-class. Feeds ADR 0069.**

Hardware: Apple M5 Pro (arm64, SHA-2 in ARMv8 crypto instructions),
`CGO_ENABLED=0`, `go test -bench -benchtime=200ms`. Numbers are
representative, not absolute — the RATIOS and alloc counts are the
decision, and they reproduce across runs.

## Criterion 1 — HMAC-SHA256: stdlib vs minio/sha256-simd

| bench | ns/op | allocs/op |
|---|---|---|
| stdlib, JWT-sized (~64 B) | **219** | 6 |
| minio/sha256-simd, JWT-sized | 241 | 6 |
| stdlib, 1 KiB | **503** | 6 |
| minio/sha256-simd, 1 KiB | 528 | 6 |

**stdlib WINS on this hardware.** Go's `crypto/sha256` already
dispatches to the ARMv8 SHA-2 instructions (and SHA-NI on x86); the
minio package's edge is AVX-512 on big server payloads, and it is
*slower* on JWT-sized inputs on Apple silicon. The "use SIMD-based
stuff to make it fast" directive is ALREADY SATISFIED by the stdlib —
adding the dependency would cost binary size and buy nothing (often a
regression) for token-sized HMACs. **Decision: stdlib `crypto/sha256`,
no minio dependency.**

## Criterion 2 — JWT HS256 sign/verify: golang-jwt/jwt/v5 vs hand-rolled

| op | impl | ns/op | allocs/op |
|---|---|---|---|
| sign | **hand-rolled** | **348** | **12** |
| sign | golang-jwt/v5 | 987 | 32 |
| verify | **hand-rolled** | **763** | **16** |
| verify | golang-jwt/v5 | 1581 | 53 |

Hand-rolled is **2.8× faster to sign, 2.1× faster to verify**, with
~⅓ the allocations (golang-jwt's reflection-driven MapClaims marshaling
dominates its cost). The hand-rolled encoder is the ~40-line
`hs256.go`: precomputed `{"alg":"HS256","typ":"JWT"}` header,
base64url(payload), HMAC-SHA256, constant-time compare, **algorithm
PINNED server-side** (the token's own `alg` header is never consulted
to choose verification — the classic alg-confusion / `alg:none` vuln is
structurally impossible).

**Correctness is proven, not assumed** (`verify_test.go`, all green):
- hand-rolled ACCEPTS the canonical RFC 7519 / jwt.io example token;
- a hand-rolled token verifies under golang-jwt (two independent impls agree);
- a golang-jwt token verifies under hand-rolled;
- `alg:none` forged token REJECTED;
- tampered-payload-with-valid-old-signature REJECTED.

**Decision: hand-rolled HS256. Zero external JWT dependency** — the
Go half of `bri.auth` carries ~40 lines of stdlib crypto, keeping the
`CGO_ENABLED=0` static binary lean.

## Criterion 3 — guard composition overhead

| bench | ns/op | allocs/op |
|---|---|---|
| bare handler | 10.5 | 0 |
| 3 composed guards (logged-in → role → handler) | **20.4** | **0** |
| auth-guard incl. HMAC verify + JSON claims parse | 900 | 20 |

The **composition itself is ~10 ns and zero-alloc** — three stacked
Ring closures doing map lookups, exactly nanoseconds-class as required.
The only real per-request cost is the ONE token verification (~900 ns,
the HMAC + JSON decode), paid once by the outermost auth guard;
stacking more role checks on top is free. `userOnly`/`adminOnly`/
`loggedInOnly` as plain `handler→handler` middleware composed with `->`
is the right, cheap model.

## Criterion 4 — password hashing (deliberately slow, and correct)

| algo | ms/op |
|---|---|
| bcrypt cost 10 | 44 |
| argon2id (m=19 MiB, t=2, p=1) | 16 |

Both are in the ~10–100 ms band that is **correct** for password
hashing — the slowness IS the anti-brute-force feature. This is the one
place where "make it fast" is a vulnerability; SIMD/SHA-NI must never
touch a password. **Decision: argon2id blessed default (OWASP
recommendation, memory-hard), bcrypt-verify kept for importing legacy
hashes.** `golang.org/x/crypto` is pure Go (no cgo) → passes the ADR
0056 filter.

## Net decisions for ADR 0069 / bri.auth

1. **JWT:** hand-rolled HS256, stdlib `crypto/sha256`, no external dep.
   Algorithm pinned server-side. exp/iat handled; secret from
   `APP_AUTH__SECRET` / config.
2. **HMAC:** stdlib, not minio/sha256-simd (already SIMD via the CPU).
3. **Guards:** plain Ring middleware, `->`-composable, ~10 ns compose +
   one ~900 ns verify per request. Claims land under `:auth/claims`.
4. **Passwords:** argon2id default, bcrypt compat, both via pure-Go
   `golang.org/x/crypto`. Never fast-hashed.
