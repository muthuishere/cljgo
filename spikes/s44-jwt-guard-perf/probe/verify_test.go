// verify_test.go — correctness cross-check: the hand-rolled HS256 and
// golang-jwt/jwt/v5 are two INDEPENDENT implementations that must agree,
// and both must accept the canonical RFC 7519 / jwt.io example token.
// Proves the hand-rolled path (which ADR 0069 blesses on the perf
// evidence) is a real, interoperable JWT — not a lookalike.
package main

import (
	"encoding/json"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// The canonical jwt.io HS256 example (RFC 7519 §3.1 shape), secret
// "your-256-bit-secret".
const (
	rfcSecret = "your-256-bit-secret"
	rfcToken  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
		"eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ." +
		"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
)

func TestHandrolledAcceptsRFCVector(t *testing.T) {
	raw, err := verifyHS256([]byte(rfcSecret), rfcToken)
	if err != nil {
		t.Fatalf("hand-rolled rejected the RFC 7519 example token: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["sub"] != "1234567890" || m["name"] != "John Doe" {
		t.Fatalf("claims mismatch: %v", m)
	}
}

func TestGolangJWTAcceptsHandrolledToken(t *testing.T) {
	tok := signHS256(secret, jwtPayload)
	parsed, err := jwt.Parse(tok, func(t *jwt.Token) (interface{}, error) { return secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation())
	if err != nil {
		t.Fatalf("golang-jwt rejected a hand-rolled token: %v", err)
	}
	cl := parsed.Claims.(jwt.MapClaims)
	if cl["sub"] != "user-42" || cl["role"] != "admin" {
		t.Fatalf("round-trip claims mismatch: %v", cl)
	}
}

func TestHandrolledAcceptsGolangJWTToken(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-42", "role": "admin",
	})
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := verifyHS256(secret, s)
	if err != nil {
		t.Fatalf("hand-rolled rejected a golang-jwt token: %v", err)
	}
	var c claims
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if c.Sub != "user-42" || c.Role != "admin" {
		t.Fatalf("claims mismatch: %+v", c)
	}
}

// The security-critical negative: a token whose alg header says "none"
// (or anything but HS256) must be REJECTED even if the rest is
// well-formed — the classic alg-confusion attack.
func TestHandrolledRejectsWrongAlg(t *testing.T) {
	// {"alg":"none","typ":"JWT"} header, same payload, empty sig.
	forged := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiJ1c2VyLTQyIiwicm9sZSI6ImFkbWluIn0."
	if _, err := verifyHS256(secret, forged); err == nil {
		t.Fatal("hand-rolled ACCEPTED an alg=none token — alg-confusion vuln")
	}
}

func TestHandrolledRejectsTamperedPayload(t *testing.T) {
	tok := signHS256(secret, []byte(`{"sub":"user-42","role":"user"}`))
	// splice an admin payload with the user-role signature
	admin := b64.EncodeToString([]byte(`{"sub":"user-42","role":"admin"}`))
	parts := []byte(tok)
	_ = parts
	forged := fixedHeader + "." + admin + "." + tok[len(tok)-43:]
	if _, err := verifyHS256(secret, forged); err == nil {
		t.Fatal("hand-rolled ACCEPTED a tampered payload")
	}
}
