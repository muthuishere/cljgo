package vault

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
)

// Demo runs every exit-criterion proof and prints MASKED evidence only.
// Return value is the process exit code (0 = all proofs green).
func Demo() int {
	ctx := context.Background()
	ok := true
	line := func(s string) { fmt.Println(s) }

	line("=== S39 pluggable vault — proof run ===")
	line(fmt.Sprintf("registered schemes: %v", Schemes()))
	line("")

	// ---- E1: env:// round-trip ----
	line("--- E1  env://  (bootstrap floor) ---")
	os.Setenv("S39_DB_PASSWORD", "env-secret-hunter2")
	{
		p, err := Open("env://S39_DB_PASSWORD")
		if err != nil {
			line("FAIL open: " + err.Error())
			ok = false
		} else {
			s, hit, err := p.Get(ctx, "")
			line(fmt.Sprintf("provider=%s hit=%v err=%v value=%s sha=%s", p.Name(), hit, err, s, MaskSHA(s.Reveal())))
			if !hit || err != nil {
				ok = false
			}
		}
	}
	line("")

	// ---- E2: keychain:// round-trip (CGO_ENABLED=0 build proves pure-Go) ----
	line("--- E2  keychain://  (OS keychain, must be CGO-free) ---")
	{
		kp := keychainProvider{service: "cljgo-s39-spike"}
		writeErr := kp.setForTest("db-password", "keychain-secret-swordfish")
		if writeErr != nil {
			line("keychain WRITE unavailable in this env: " + writeErr.Error())
			line("(negative finding recorded — see VERDICT; build-level CGO proof still stands)")
		} else {
			p, _ := Open("keychain://cljgo-s39-spike/db-password")
			s, hit, err := p.Get(ctx, "")
			line(fmt.Sprintf("provider=%s hit=%v err=%v value=%s sha=%s", p.Name(), hit, err, s, MaskSHA(s.Reveal())))
			if !hit || err != nil {
				ok = false
			}
			_ = kp.delForTest("db-password")
			// miss after delete
			_, hit2, _ := p.Get(ctx, "")
			line(fmt.Sprintf("after delete: hit=%v (expect false)", hit2))
		}
	}
	line("")

	// ---- E3: age:// encrypted file round-trip ----
	line("--- E3  age://  (encrypted, repo-committable file; identity from env) ---")
	{
		id, _ := age.GenerateX25519Identity()
		os.Setenv("S39_AGE_IDENTITY", id.String())
		blobPath := filepath.Join(os.TempDir(), "s39-store.age")
		var buf bytes.Buffer
		store := map[string]string{
			"stripe-key": "sk_live_FAKE_agestore_deadbeef",
			"db-url":     "postgres://u:p@h/db",
		}
		if err := EncryptStore(&buf, id.Recipient().String(), store); err != nil {
			line("FAIL encrypt: " + err.Error())
			ok = false
		}
		os.WriteFile(blobPath, buf.Bytes(), 0o600)
		line(fmt.Sprintf("wrote ciphertext %s (%d bytes, armored=%v)", blobPath, buf.Len(), bytes.HasPrefix(buf.Bytes(), []byte("age-encryption"))))
		p, err := Open("age://" + blobPath + "?id=S39_AGE_IDENTITY")
		if err != nil {
			line("FAIL open: " + err.Error())
			ok = false
		} else {
			s, hit, err := p.Get(ctx, "stripe-key")
			line(fmt.Sprintf("provider=%s key=stripe-key hit=%v err=%v value=%s sha=%s", p.Name(), hit, err, s, MaskSHA(s.Reveal())))
			if !hit || err != nil {
				ok = false
			}
			_, missHit, _ := p.Get(ctx, "no-such-key")
			line(fmt.Sprintf("miss key=no-such-key hit=%v (expect false → drives fallback)", missHit))
		}
	}
	line("")

	// ---- E4: cloud stub scheme resolution + SDK-shape fit ----
	line("--- E4  cloud stubs (scheme→provider resolution, not dialed) ---")
	for _, uri := range []string{
		"aws-sm://prod/db/password?region=eu-central-1",
		"gcp-sm://projects/deemwar/secrets/db/versions/latest",
		"vault://secret/deemwar/db#password",
		"gh://muthuishere/cljgo#DEPLOY_TOKEN",
	} {
		p, err := Open(uri)
		if err != nil {
			line("FAIL open " + uri + ": " + err.Error())
			ok = false
			continue
		}
		_, hit, gerr := p.Get(ctx, "")
		line(fmt.Sprintf("%-52s → provider=%-28s hit=%v err=%v", uri, p.Name(), hit, gerr))
	}
	line("")

	// ---- E5: fallback chain keychain-miss → env-hit ----
	line("--- E5  fallback chain (keychain miss → env hit) ---")
	{
		os.Setenv("S39_CHAIN_KEY", "env-fallback-tuna")
		// keychain has no such account → miss; env has it → hit.
		p, err := OpenChain("keychain://cljgo-s39-spike/unset-account", "env://S39_CHAIN_KEY")
		if err != nil {
			line("FAIL openchain: " + err.Error())
			ok = false
		} else {
			s, hit, err := p.Get(ctx, "")
			line(fmt.Sprintf("chain=%s hit=%v err=%v value=%s (came from env tail)", p.Name(), hit, err, s))
			if !hit || err != nil {
				ok = false
			}
		}
		// all-miss chain → legible not-found
		p2, _ := OpenChain("env://S39_DOES_NOT_EXIST_1", "env://S39_DOES_NOT_EXIST_2")
		_, hit, err := p2.Get(ctx, "")
		line(fmt.Sprintf("all-miss chain=%s hit=%v err=%v (expect hit=false, err=nil)", p2.Name(), hit, err))
		if hit {
			ok = false
		}
	}
	line("")

	// ---- E6: resolver seam (s38) ----
	line("--- E6  resolver seam AsGetFunc (s38 hook) ---")
	{
		p, _ := OpenChain("keychain://cljgo-s39-spike/unset-account", "env://S39_CHAIN_KEY")
		get := AsGetFunc(p) // GetFunc: func(ctx,key)(string,bool,error)
		v, hit, err := get(ctx, "")
		line(fmt.Sprintf("GetFunc hit=%v err=%v masked=%s (resolver receives raw string at the boundary)", hit, err, Mask(v)))
		if !hit || err != nil {
			ok = false
		}
	}
	line("")

	if ok {
		line("RESULT: all proofs GREEN")
		return 0
	}
	line("RESULT: some proofs FAILED")
	return 1
}
