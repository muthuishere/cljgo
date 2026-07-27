// security.go — cljg.security's Go half (ADR 0103, renamed from auth.go /
// bri.core.security): the S44-blessed security primitives plus the
// Bun.hash/crypto tier.
//
//   - JWT HS256, HAND-ROLLED on stdlib crypto/sha256 (S44 VERDICT: 2–3×
//     faster than golang-jwt/v5, ⅓ the allocs, zero external JWT dep).
//     The algorithm is PINNED server-side — the token's own "alg" header
//     is never consulted to choose verification, so alg-confusion /
//     alg:none forgeries are structurally impossible.
//   - Password hashing: argon2id (OWASP m=19 MiB, t=2, p=1) as the
//     blessed default, bcrypt-verify for importing legacy hashes. Both
//     pure Go (golang.org/x/crypto) — ADR 0056 CGO_ENABLED=0 holds.
//     These are DELIBERATELY slow (~15–45 ms); never SIMD-fast.
//   - Crypto primitives (ADR 0103): sha256, hmac-sha256, secure-random,
//     uuid v4, base64/hex codecs — all stdlib crypto/encoding, no deps.
//
// The KEYCHAIN trio (-keychain-set/-get/-del) is NOT here: it pulls
// go-keyring + age, so it lives in the ISOLATED opt-in pkg/bri/security
// (RegisterInstaller), keeping always-linked packages keyring-free.
//
// Interned as :private vars into cljg.security on first (require 'cljg.security),
// same lazy lib-provider path as bri.web.http (see bri.go). exp/iat live in
// the Clojure half (cljg/security.cljg) so tests can freeze the clock.
package bri

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// jwtFixedHeader is the ONLY header bri ever emits or accepts:
// base64url({"alg":"HS256","typ":"JWT"}). Precomputed so signing does no
// header work, and verification is a constant-string compare (the alg
// pin).
const jwtFixedHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

var jwtB64 = base64.RawURLEncoding

// installSecurityShims interns cljg.security's private Go primitives (the
// stdlib/x-crypto half — the opt-in keychain trio arrives via the registry).
func installSecurityShims(def func(name string, fn func(args ...any) any)) {
	def("-jwt-sign", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -jwt-sign", len(args)))
		}
		return jwtSign(asString(args[0]), args[1])
	})
	def("-jwt-verify", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -jwt-verify", len(args)))
		}
		return jwtVerify(asString(args[0]), asString(args[1]))
	})
	def("-argon2-hash", func(args ...any) any {
		return argon2Hash(asString(one("-argon2-hash", args)))
	})
	def("-argon2-verify", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -argon2-verify", len(args)))
		}
		return argon2Verify(asString(args[0]), asString(args[1]))
	})
	def("-bcrypt-verify", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -bcrypt-verify", len(args)))
		}
		return bcrypt.CompareHashAndPassword([]byte(asString(args[1])), []byte(asString(args[0]))) == nil
	})
	def("-rand-token", func(args ...any) any { return randToken() })
	def("-now-millis", func(args ...any) any { return nowMillis() })
	def("-getenv", getenvShim)

	// --- crypto primitives (ADR 0103) ---------------------------------------
	def("-sha256", func(args ...any) any {
		sum := sha256.Sum256([]byte(asString(one("-sha256", args))))
		return hex.EncodeToString(sum[:])
	})
	def("-hmac-sha256", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -hmac-sha256 (expects 2: [key message])", len(args)))
		}
		mac := hmac.New(sha256.New, []byte(asString(args[0])))
		mac.Write([]byte(asString(args[1])))
		return hex.EncodeToString(mac.Sum(nil))
	})
	def("-secure-random", func(args ...any) any {
		n := asInt(one("-secure-random", args))
		if n <= 0 {
			panic(fmt.Errorf("cljg.security/random: expected a positive byte count, got %d", n))
		}
		b := make([]byte, n)
		if _, err := rand.Read(b); err != nil {
			panic(fmt.Errorf("-secure-random: %w", err))
		}
		return hex.EncodeToString(b)
	})
	def("-uuid", func(args ...any) any { return uuidV4() })
	def("-b64-encode", func(args ...any) any {
		return base64.StdEncoding.EncodeToString([]byte(asString(one("-b64-encode", args))))
	})
	def("-b64-decode", func(args ...any) any {
		b, err := base64.StdEncoding.DecodeString(asString(one("-b64-decode", args)))
		if err != nil {
			return nil
		}
		return string(b)
	})
	def("-hex-encode", func(args ...any) any {
		return hex.EncodeToString([]byte(asString(one("-hex-encode", args))))
	})
	def("-hex-decode", func(args ...any) any {
		b, err := hex.DecodeString(asString(one("-hex-decode", args)))
		if err != nil {
			return nil
		}
		return string(b)
	})
}

// uuidV4 is a random (version 4, variant 10) UUID from crypto/rand —
// xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx.
func uuidV4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("-uuid: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// --- JWT HS256 (hand-rolled, alg-pinned) ------------------------------------

// jwtSign builds header.payload.signature. claims is a cljgo map,
// marshaled through the same JSON shaping bri.web.http uses (keyword keys →
// names, int64 stays integral).
func jwtSign(secret string, claims any) string {
	claimsJSON := jsonEncode(claims)
	payload := jwtB64.EncodeToString([]byte(claimsJSON))
	signing := jwtFixedHeader + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + jwtB64.EncodeToString(mac.Sum(nil))
}

// jwtVerify checks the signature (constant-time) with the PINNED
// algorithm and returns the decoded claims map, or nil on ANY failure
// (bad shape, wrong header/alg, bad signature). exp/iat are NOT checked
// here — the Clojure half does that against an injectable clock.
func jwtVerify(secret, token string) any {
	first := strings.IndexByte(token, '.')
	if first < 0 {
		return nil
	}
	rest := token[first+1:]
	second := strings.IndexByte(rest, '.')
	if second < 0 {
		return nil
	}
	if token[:first] != jwtFixedHeader { // alg + typ pinned
		return nil
	}
	signing := token[:first+1+second]
	sigPart := rest[second+1:]
	payloadPart := rest[:second]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	want := mac.Sum(nil)
	got, err := jwtB64.DecodeString(sigPart)
	if err != nil || subtle.ConstantTimeCompare(want, got) != 1 {
		return nil
	}
	claimsJSON, err := jwtB64.DecodeString(payloadPart)
	if err != nil {
		return nil
	}
	return jsonDecode(string(claimsJSON))
}

// --- password hashing (argon2id + bcrypt compat) ----------------------------

// OWASP argon2id parameters (2024 guidance): 19 MiB, 2 iterations, 1
// lane, 16-byte salt, 32-byte key.
const (
	argonMemory  = 19 * 1024 // KiB
	argonTime    = 2
	argonThreads = 1
	argonSaltLen = 16
	argonKeyLen  = 32
)

var argonB64 = base64.RawStdEncoding

// argon2Hash returns a self-describing PHC string:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>.
func argon2Hash(password string) string {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		panic(fmt.Errorf("-argon2-hash: %w", err))
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		argonB64.EncodeToString(salt), argonB64.EncodeToString(key))
}

// argon2Verify recomputes the key with the encoded parameters and
// constant-time compares. Returns false on any parse failure.
func argon2Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}
	var mem, tm uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &tm, &par); err != nil {
		return false
	}
	salt, err := argonB64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := argonB64.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, tm, mem, par, uint32(len(want)))
	return subtle.ConstantTimeCompare(want, got) == 1
}

// ensure lang stays imported even if the shim set slims down later.
var _ = lang.NewKeyword
