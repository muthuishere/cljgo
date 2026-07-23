// hs256.go — the hand-rolled HS256 encoder/decoder the spike benches
// against golang-jwt/jwt/v5. This is the ~20-line core bri.auth's Go
// half will grow from: base64url(header).base64url(payload) signed with
// HMAC-SHA256, the algorithm PINNED server-side (the token's own "alg"
// header is never trusted — the classic JWT confusion vuln).
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// fixedHeader is the ONE header we ever emit or accept: {"alg":"HS256",
// "typ":"JWT"}. Precomputed base64url so sign does zero header work.
const fixedHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

var b64 = base64.RawURLEncoding

// signHS256 builds a compact JWS: header.payload.signature. claims is
// pre-marshaled JSON (the caller owns exp/iat injection).
func signHS256(secret []byte, claimsJSON []byte) string {
	var sb strings.Builder
	sb.Grow(len(fixedHeader) + 1 + b64.EncodedLen(len(claimsJSON)) + 1 + 43)
	sb.WriteString(fixedHeader)
	sb.WriteByte('.')
	payload := b64.EncodeToString(claimsJSON)
	sb.WriteString(payload)
	signing := sb.String() // header.payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	sig := mac.Sum(nil)
	sb.WriteByte('.')
	sb.WriteString(b64.EncodeToString(sig))
	return sb.String()
}

var errBadToken = errors.New("invalid token")

// verifyHS256 checks the signature (constant-time) with the PINNED
// algorithm — it never reads the token's alg header to decide how to
// verify — and returns the raw claims JSON. exp/iat validation is the
// caller's (kept out of the hot HMAC path for the bench).
func verifyHS256(secret []byte, token string) ([]byte, error) {
	// Split into exactly 3 segments without allocating a slice.
	first := strings.IndexByte(token, '.')
	if first < 0 {
		return nil, errBadToken
	}
	rest := token[first+1:]
	second := strings.IndexByte(rest, '.')
	if second < 0 {
		return nil, errBadToken
	}
	signing := token[:first+1+second] // header.payload
	sigPart := rest[second+1:]
	payloadPart := rest[:second]

	if token[:first] != fixedHeader {
		return nil, errBadToken // alg/typ pinned; anything else is rejected
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	want := mac.Sum(nil)
	got, err := b64.DecodeString(sigPart)
	if err != nil || subtle.ConstantTimeCompare(want, got) != 1 {
		return nil, errBadToken
	}
	return b64.DecodeString(payloadPart)
}

// claims is the minimal shape both implementations round-trip in the
// cross-verification test.
type claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Iat  int64  `json:"iat,omitempty"`
	Exp  int64  `json:"exp,omitempty"`
}

func marshalClaims(c claims) []byte {
	b, _ := json.Marshal(c)
	return b
}
