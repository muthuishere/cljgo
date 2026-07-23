# S44 — JWT + guard + password-hash performance (bri's API-first security tier)

Owner directive (2026-07-23): *"http should be so fast; middleware
security and all should be simple enough; … authentication JWT in a
single way; middleware like userOnly adminOnly loggedInOnly composable;
JWT creation should be simple; use SIMD-based stuff for hash or
whatever if possible to make it fast."*

Before ADR 0069 blesses ONE JWT path for `bri.auth`, this spike proves
which implementation is actually fastest — and that the guard
middleware model (composed Ring handler→handler closures doing map
lookups) costs nanoseconds, not microseconds. It also measures the
DELIBERATELY slow counterpart: password hashing, where ~50–100ms is
correct (anti-brute-force) and SIMD-fast would be a vulnerability.

Constraint filter (ADR 0056): pure Go, `CGO_ENABLED=0`. Go assembler is
fine (it is not cgo); anything needing a C toolchain is disqualified.

## Exit criteria (written BEFORE any code)

1. **HMAC-SHA256 primitive: stdlib vs minio/sha256-simd.** Go's stdlib
   `crypto/sha256` already dispatches to SHA-NI / ARMv8 SHA2
   instructions on modern CPUs; `github.com/minio/sha256-simd` claims
   wins mainly on AVX-512 servers. Bench both hashing a JWT-sized
   (~120 B) and a 1 KiB payload, ns/op + allocs/op, and report whether
   the extra dependency buys anything on this class of hardware. Named
   winner.
2. **JWT HS256 sign/verify: `golang-jwt/jwt/v5` vs hand-rolled.** JWT
   HS256 is base64url(header).base64url(claims-JSON) + HMAC-SHA256 —
   ~20 lines. Bench sign and verify for both, ns/op + allocs/op, with
   both implementations CROSS-VERIFIED (a token signed by one must
   verify under the other, both must accept the canonical jwt.io/RFC
   7519 example token). Named winner with the alloc story.
3. **Guard overhead: 3 composed guards vs bare handler.** A request
   map through logged-in → role-check → handler (map lookups + one
   HMAC verify amortized out; the composition itself) vs the bare
   handler. Must be nanoseconds-class. Report ns/op for the closure
   composition alone and for token-verify-per-request.
4. **Password hashing: bcrypt(10) vs argon2id (OWASP m=19 MiB, t=2,
   p=1).** Report ms/op for both and state loudly that this is the ONE
   place slow is correct. Pick the blessed default (OWASP says
   argon2id) and the compat story (bcrypt verify for imported hashes).

## Non-goals

- No RS256/ES256/EdDSA — ADR 0069 pins ONE algorithm (HS256) server-side.
- No JWKS, no key rotation, no refresh tokens — later tiers if ever.
- No interpreter-side measurement here — the tree-walk overhead per
  request is already gated by `pkg/bri`'s perf seam
  (`TestInterpretedHandlerOverhead`); this spike isolates the CRYPTO
  and COMPOSITION costs the Go half will carry.

## Run

```
cd spikes/s44-jwt-guard-perf && ./run.sh
```

Throwaway module (own go.mod); touches nothing outside this directory.
Verdict: `VERDICT.md`.
