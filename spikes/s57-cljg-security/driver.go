package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/blake2b"
)

func check(name string, cond bool) {
	status := "FAIL"
	if cond {
		status = "OK"
	}
	fmt.Printf("[%s] %s\n", status, name)
	if !cond {
		panic("subfeature failed: " + name)
	}
}

// ---- argon2id password hash (PHC string format, like Bun.password default) ----

type argonParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

func argonHash(password string, p argonParams) (string, error) {
	salt := make([]byte, p.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)
	b64s := base64.RawStdEncoding.EncodeToString(salt)
	b64k := base64.RawStdEncoding.EncodeToString(key)
	// $argon2id$v=19$m=...,t=...,p=...$salt$hash
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism, b64s, b64k), nil
}

func argonVerify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("bad argon2id hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, err
	}
	var mem, iter uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iter, &par); err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, iter, mem, par, uint32(len(want)))
	return hmac.Equal(got, want), nil // constant-time compare
}

// ---- uuid v4 from crypto/rand ----

func uuidV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func main() {
	fmt.Println("=== cljg.security spike (pure Go, CGO=0) ===")

	// 1. argon2id
	ap := argonParams{memory: 19 * 1024, iterations: 2, parallelism: 1, saltLength: 16, keyLength: 32}
	ah, err := argonHash("s3cr3t-pass", ap)
	if err != nil {
		panic(err)
	}
	fmt.Println("argon2id:", ah)
	okA, _ := argonVerify("s3cr3t-pass", ah)
	badA, _ := argonVerify("wrong-pass", ah)
	check("argon2id verify correct", okA)
	check("argon2id reject wrong", !badA)

	// 2. bcrypt
	bh, err := bcrypt.GenerateFromPassword([]byte("s3cr3t-pass"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	fmt.Println("bcrypt:", string(bh))
	check("bcrypt verify correct", bcrypt.CompareHashAndPassword(bh, []byte("s3cr3t-pass")) == nil)
	check("bcrypt reject wrong", bcrypt.CompareHashAndPassword(bh, []byte("wrong-pass")) != nil)

	// 3. hashes
	msg := []byte("the quick brown fox")
	s256 := sha256.Sum256(msg)
	s512 := sha512.Sum512(msg)
	fmt.Println("sha256:", hex.EncodeToString(s256[:]))
	fmt.Println("sha512:", hex.EncodeToString(s512[:]))
	check("sha256 len", len(s256) == 32)
	check("sha512 len", len(s512) == 64)

	// blake2b
	b2, _ := blake2b.New256(nil)
	b2.Write(msg)
	b2sum := b2.Sum(nil)
	fmt.Println("blake2b-256:", hex.EncodeToString(b2sum))
	check("blake2b-256 len", len(b2sum) == 32)

	// hmac-sha256
	mac := hmac.New(sha256.New, []byte("key"))
	mac.Write(msg)
	macSum := mac.Sum(nil)
	fmt.Println("hmac-sha256:", hex.EncodeToString(macSum))
	// verify hmac
	mac2 := hmac.New(sha256.New, []byte("key"))
	mac2.Write(msg)
	check("hmac-sha256 verify", hmac.Equal(macSum, mac2.Sum(nil)))
	mac3 := hmac.New(sha256.New, []byte("wrongkey"))
	mac3.Write(msg)
	check("hmac-sha256 reject wrong key", !hmac.Equal(macSum, mac3.Sum(nil)))

	// 4. secure random bytes + token
	rb := make([]byte, 32)
	if _, err := rand.Read(rb); err != nil {
		panic(err)
	}
	fmt.Println("random 32 bytes (hex):", hex.EncodeToString(rb))
	check("random bytes nonzero", func() bool { var z byte; for _, x := range rb { z |= x }; return z != 0 }())
	tok, _ := randomToken(24)
	fmt.Println("random token:", tok)
	check("random token nonempty", len(tok) > 0)

	// 5. uuid v4
	u, _ := uuidV4()
	fmt.Println("uuid v4:", u)
	check("uuid v4 shape", len(u) == 36 && u[14] == '4')

	// 6. base64 + hex round-trip
	orig := []byte("cljgo binary=1010")
	b64 := base64.StdEncoding.EncodeToString(orig)
	dec64, _ := base64.StdEncoding.DecodeString(b64)
	fmt.Println("base64:", b64)
	check("base64 round-trip", string(dec64) == string(orig))
	hx := hex.EncodeToString(orig)
	dechx, _ := hex.DecodeString(hx)
	fmt.Println("hex:", hx)
	check("hex round-trip", string(dechx) == string(orig))

	// 7. JWT HS256
	claims := jwt.MapClaims{"sub": "user-42", "role": "admin"}
	hsKey := []byte("supersecretkey")
	hsTok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	hsSigned, err := hsTok.SignedString(hsKey)
	if err != nil {
		panic(err)
	}
	fmt.Println("JWT HS256:", hsSigned)
	parsedHS, err := jwt.Parse(hsSigned, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected alg")
		}
		return hsKey, nil
	})
	check("JWT HS256 verify", err == nil && parsedHS.Valid)
	// tamper
	tampered := hsSigned[:len(hsSigned)-4] + "AAAA"
	_, errT := jwt.Parse(tampered, func(t *jwt.Token) (interface{}, error) { return hsKey, nil })
	check("JWT HS256 reject tampered", errT != nil)

	// 8. JWT RS256
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	rsTok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	rsSigned, err := rsTok.SignedString(rsaKey)
	if err != nil {
		panic(err)
	}
	fmt.Println("JWT RS256:", rsSigned[:60]+"...")
	parsedRS, err := jwt.Parse(rsSigned, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected alg")
		}
		return &rsaKey.PublicKey, nil
	})
	check("JWT RS256 verify", err == nil && parsedRS.Valid)
	tamperedRS := rsSigned[:len(rsSigned)-4] + "BBBB"
	_, errRS := jwt.Parse(tamperedRS, func(t *jwt.Token) (interface{}, error) { return &rsaKey.PublicKey, nil })
	check("JWT RS256 reject tampered", errRS != nil)

	fmt.Println("=== ALL SUBFEATURES PASSED ===")
}
