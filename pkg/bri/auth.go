// auth.go — bri.core.security's Go half: the S44-blessed security primitives.
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
//
// Interned as :private vars into bri.core.security on first (require 'bri.core.security),
// same lazy lib-provider path as bri.web.http (see bri.go). exp/iat live in
// the Clojure half (bri/auth.cljg) so tests can freeze the clock.
package bri

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
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

// installAuthShims interns bri.core.security's private Go primitives.
func installAuthShims(def func(name string, fn func(args ...any) any)) {
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
