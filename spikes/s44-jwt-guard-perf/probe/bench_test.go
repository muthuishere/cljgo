// bench_test.go — the S44 evidence, run via `go test -bench`. Four
// criteria: (1) HMAC-SHA256 stdlib vs minio/sha256-simd, (2) JWT HS256
// sign/verify golang-jwt vs hand-rolled, (3) guard composition +
// per-request verify overhead, (4) password hashing bcrypt vs argon2id.
package main

import (
	stdsha "crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	simd "github.com/minio/sha256-simd"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

var (
	secret     = []byte("s44-benchmark-secret-key-32bytes!")
	jwtPayload = []byte(`{"sub":"user-42","role":"admin","iat":1690000000,"exp":1690003600}`)
	kib        = make([]byte, 1024)
)

// --- criterion 1: HMAC-SHA256 primitive -------------------------------------

func BenchmarkHMAC_Stdlib_JWTsize(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := stdsha.New(sha256.New, secret)
		m.Write(jwtPayload)
		_ = m.Sum(nil)
	}
}

func BenchmarkHMAC_SIMD_JWTsize(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := stdsha.New(simd.New, secret)
		m.Write(jwtPayload)
		_ = m.Sum(nil)
	}
}

func BenchmarkHMAC_Stdlib_1KiB(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := stdsha.New(sha256.New, secret)
		m.Write(kib)
		_ = m.Sum(nil)
	}
}

func BenchmarkHMAC_SIMD_1KiB(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := stdsha.New(simd.New, secret)
		m.Write(kib)
		_ = m.Sum(nil)
	}
}

// --- criterion 2: JWT HS256 sign/verify -------------------------------------

func BenchmarkJWT_Sign_Handrolled(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = signHS256(secret, jwtPayload)
	}
}

func BenchmarkJWT_Sign_GolangJWT(b *testing.B) {
	b.ReportAllocs()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-42", "role": "admin", "iat": 1690000000, "exp": 1690003600,
	})
	for i := 0; i < b.N; i++ {
		if _, err := tok.SignedString(secret); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJWT_Verify_Handrolled(b *testing.B) {
	token := signHS256(secret, jwtPayload)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := verifyHS256(secret, token)
		if err != nil {
			b.Fatal(err)
		}
		var c claims
		_ = json.Unmarshal(raw, &c)
	}
}

func BenchmarkJWT_Verify_GolangJWT(b *testing.B) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-42", "role": "admin", "iat": 1690000000, "exp": 1690003600,
	})
	token, _ := tok.SignedString(secret)
	keyfn := func(t *jwt.Token) (interface{}, error) { return secret, nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := jwt.Parse(token, keyfn,
			jwt.WithValidMethods([]string{"HS256"}),
			jwt.WithoutClaimsValidation())
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- criterion 3: guard composition + per-request verify --------------------

// A minimal Ring model for the composition micro-bench: a handler is
// func(map)->map; a guard wraps one. This mirrors what the tree-walk
// evaluator does at the Clojure layer, isolated to measure the SHAPE
// (map lookups + closure calls) in native Go.
type req = map[string]any
type handler func(req) req

func loggedInOnly(h handler) handler {
	return func(r req) req {
		if _, ok := r["auth/claims"]; !ok {
			return req{"status": 401}
		}
		return h(r)
	}
}

func roleOnly(role string, h handler) handler {
	return func(r req) req {
		cl, _ := r["auth/claims"].(map[string]any)
		if cl == nil {
			return req{"status": 401}
		}
		if cl["role"] != role {
			return req{"status": 403}
		}
		return h(r)
	}
}

func BenchmarkGuard_BareHandler(b *testing.B) {
	h := func(r req) req { return req{"status": 200} }
	r := req{"auth/claims": map[string]any{"role": "admin"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = h(r)
	}
}

func BenchmarkGuard_ThreeComposed(b *testing.B) {
	base := func(r req) req { return req{"status": 200} }
	// logged-in -> role-check -> handler
	h := loggedInOnly(roleOnly("admin", base))
	r := req{"auth/claims": map[string]any{"role": "admin"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = h(r)
	}
}

// The realistic per-request cost: a guard that must verify the Bearer
// token (HMAC) before populating :auth/claims. This is what the FIRST
// guard in the chain actually pays.
func BenchmarkGuard_VerifyThenCompose(b *testing.B) {
	token := signHS256(secret, jwtPayload)
	base := func(r req) req { return req{"status": 200} }
	inner := loggedInOnly(roleOnly("admin", base))
	authGuard := func(h handler) handler {
		return func(r req) req {
			raw, err := verifyHS256(secret, token)
			if err != nil {
				return req{"status": 401}
			}
			var c claims
			_ = json.Unmarshal(raw, &c)
			r["auth/claims"] = map[string]any{"role": c.Role, "sub": c.Sub}
			return h(r)
		}
	}
	h := authGuard(inner)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h(req{})
	}
}

// --- criterion 4: password hashing (deliberately slow) ----------------------

func BenchmarkPassword_Bcrypt10(b *testing.B) {
	pw := []byte("correct horse battery staple")
	for i := 0; i < b.N; i++ {
		if _, err := bcrypt.GenerateFromPassword(pw, 10); err != nil {
			b.Fatal(err)
		}
	}
}

// OWASP argon2id params: m=19456 KiB (19 MiB), t=2, p=1, 16-byte salt,
// 32-byte key.
func BenchmarkPassword_Argon2id(b *testing.B) {
	pw := []byte("correct horse battery staple")
	salt := []byte("0123456789abcdef")
	for i := 0; i < b.N; i++ {
		_ = argon2.IDKey(pw, salt, 2, 19456, 1, 32)
	}
}

// keep time import referenced for potential wall-clock assertions
var _ = time.Now
