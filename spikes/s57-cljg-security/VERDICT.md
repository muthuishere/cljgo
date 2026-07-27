# Spike s57 — cljg.security

**Verdict: MET-PUREGO-DEP** — the full crypto primitive set works in pure Go with
`CGO_ENABLED=0` and no external build toolchain. Stdlib covers hashing, HMAC,
secure random, UUID v4, base64/hex. Two widely-used pure-Go deps cover password
hashing (argon2id/bcrypt), blake2b, and JWT.

- **cgoFree:** true — `CGO_ENABLED=0 go build -ldflags='-s -w'` succeeded.
- **ranClean:** true — every subfeature printed `[OK]`, ended `ALL SUBFEATURES PASSED`.
- **Stripped binary:** 3,405,826 bytes (~3.4 MB), `/tmp/s57-cljg-security.bin`.

## Dependencies

| module | version | license | pure Go? |
|---|---|---|---|
| golang.org/x/crypto | v0.54.0 | BSD-3-Clause | yes — no `import "C"`; the `*_amd64.s` are Go plan9 asm (SIMD), not cgo |
| github.com/golang-jwt/jwt/v5 | v5.3.1 | MIT | yes — no C |
| golang.org/x/sys | v0.47.0 (indirect) | BSD-3-Clause | yes |

`crypto/hmac`, `crypto/rand`, `crypto/rsa`, `crypto/sha256`, `crypto/sha512`,
`encoding/base64`, `encoding/hex` are all Go **stdlib** — zero deps for those.

## Per-subfeature status

| subfeature | status | source |
|---|---|---|
| argon2id hash + verify (PHC string, Bun.password default) | works | x/crypto/argon2 |
| bcrypt hash + verify | works | x/crypto/bcrypt |
| sha256 / sha512 | works | stdlib |
| hmac-sha256 (verify + reject wrong key) | works | stdlib crypto/hmac |
| blake2b-256 | works | x/crypto/blake2b |
| crypto-secure random bytes + token | works | stdlib crypto/rand |
| uuid v4 (hand-rolled from crypto/rand) | works | stdlib — no google/uuid needed |
| base64 + hex encode/decode | works | stdlib |
| JWT HS256 sign/verify/reject-tampered | works | golang-jwt/jwt/v5 |
| JWT RS256 sign/verify/reject-tampered | works | golang-jwt/jwt/v5 + stdlib crypto/rsa |

UUID v4 was implemented directly from `crypto/rand` (16 bytes, set version nibble
`0x40` + variant `0x80`) — **no third-party uuid lib required**. If callers want v1/v5/v7
or parsing helpers, `github.com/google/uuid` (BSD-3, pure Go) is the drop-in, but v4 alone
needs nothing.

## Captured run output

```
=== cljg.security spike (pure Go, CGO=0) ===
argon2id: $argon2id$v=19$m=19456,t=2,p=1$reHbNj3bZS5nnstBDl5djw$8naw0t8KdOA/DoyPxMlhP1OVJbWMDkC6jVd4uhXYqOY
[OK] argon2id verify correct
[OK] argon2id reject wrong
bcrypt: $2a$10$udK/kokNLHT6Uslbkzzt9e3vUXzvBP7JS3KGmKctbjP1v7BIxFL4K
[OK] bcrypt verify correct
[OK] bcrypt reject wrong
sha256: 9ecb36561341d18eb65484e833efea61edc74b84cf5e6ae1b81c63533e25fc8f
sha512: d9d380f29b97ad6a1d92e987d83fa5a02653301e1006dd2bcd51afa59a9147e9caedaf89521abc0f0b682adcd47fb512b8343c834a32f326fe9bef00542ce887
[OK] sha256 len
[OK] sha512 len
blake2b-256: 9ad992086a8d14adf5d8516c38785029661251102a64ebcff25310731fe1857d
[OK] blake2b-256 len
hmac-sha256: 9119dc3209b2cc822340e7ff18d47c796736f1af694ffba590d094b4d182e7e1
[OK] hmac-sha256 verify
[OK] hmac-sha256 reject wrong key
random 32 bytes (hex): aaf43c2b99973cb252acf183d2fa8741eed34108b4bcc609b74a3b5d33e8c2c8
[OK] random bytes nonzero
random token: ZNhFmwiSMNcADQZ9R6gctaTW2lPRXt9F
[OK] random token nonempty
uuid v4: 8e704f4f-0d4a-457e-ba1a-d1368260d6bc
[OK] uuid v4 shape
base64: Y2xqZ28gYmluYXJ5PTEwMTA=
[OK] base64 round-trip
hex: 636c6a676f2062696e6172793d31303130
[OK] hex round-trip
JWT HS256: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyb2xlIjoiYWRtaW4iLCJzdWIiOiJ1c2VyLTQyIn0.fvWgYXCNTiWYlWAM5ujYZ6Fm5mpNVCnsCYs-0wsRsAc
[OK] JWT HS256 verify
[OK] JWT HS256 reject tampered
JWT RS256: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJyb2xlIjoiYWRtaW4iLCJ...
[OK] JWT RS256 verify
[OK] JWT RS256 reject tampered
=== ALL SUBFEATURES PASSED ===
```

## Risks / caveats

- **Two deps added to go.mod** — `golang.org/x/crypto` (already a de-facto standard,
  BSD-3, Go-team maintained) and `golang-jwt/jwt/v5` (MIT, the canonical Go JWT lib).
  Both are pure Go, no cgo, actively maintained. Low supply-chain risk, but they are
  third-party — this is MET-PUREGO-DEP, not MET-STDLIB.
- **argon2 cost params** are the caller's responsibility. The spike used the OWASP-ish
  m=19MiB, t=2, p=1. Bun's default is argon2id — matched. These must be tunable and the
  PHC-encoded string carries them so verify is self-describing.
- **argon2 memory cost** — each hash allocates ~19 MiB; under load this is a DoS surface.
  Expose params but keep sane defaults; document.
- **JWT alg-confusion** — the verify keyfuncs in the driver pin the expected method
  (`SigningMethodHMAC` / `SigningMethodRSA`) and reject others. Any cljg.security JWT
  wrapper MUST do the same — never trust the token header's `alg`. This is the classic
  RS256→HS256 downgrade bug; the wrapper API should not let a caller forget it.
- **Binary size:** ~3.4 MB stripped for this driver alone (includes RSA + x/crypto).
  Folded into a real cljgo binary the marginal cost is smaller (shared runtime).
- **SIMD asm** in argon2/blake2b is amd64/arm64 Go assembly with pure-Go fallbacks —
  builds and runs fine under CGO=0 on this arm64 mac; portable.

## What it means for cljgo

cljg.security is a clean GO. It is arguably the biggest single batteries win with the
least risk: most of it is stdlib, the two deps are the most boring, most-vetted crypto
libs in the Go ecosystem, and everything AOT-compiles into a static CGO=0 binary. Ship
argon2id as the password default (Bun parity) with bcrypt as the compat option, and pin
JWT alg on verify at the API layer so callers can't create an alg-confusion hole.
