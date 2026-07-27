// Spike s58-keychain: prove OS keychain write+read (save/retrieve) works under
// CGO_ENABLED=0, plus a pure-Go encrypted-file fallback for headless Linux.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"filippo.io/age"
	"github.com/zalando/go-keyring"
)

func main() {
	fmt.Println("=== s58-keychain round-trip ===")
	fmt.Printf("runtime: %s/%s  CGO note: this binary built with CGO_ENABLED=0\n",
		runtime.GOOS, runtime.GOARCH)

	// ---------------------------------------------------------------
	// PART A: OS keychain via github.com/zalando/go-keyring
	// On macOS this shells out to /usr/bin/security (NO cgo).
	// ---------------------------------------------------------------
	const service = "cljgo-spike-s58"
	const user = "keychain-test-key"
	secret := fmt.Sprintf("s3cr3t-value-%d", time.Now().Unix())

	fmt.Println("\n[A] OS keychain (go-keyring)")
	if err := keyring.Set(service, user, secret); err != nil {
		fmt.Printf("  SET FAILED: %v\n", err)
	} else {
		fmt.Printf("  saveinkeychain  service=%q user=%q -> OK\n", service, user)
		got, err := keyring.Get(service, user)
		if err != nil {
			fmt.Printf("  GET FAILED: %v\n", err)
		} else {
			fmt.Printf("  retrivefromkeychain             -> %q\n", got)
			fmt.Printf("  round-trip match: %v\n", got == secret)
		}
		// cleanup so we don't litter the login keychain
		if err := keyring.Delete(service, user); err != nil {
			fmt.Printf("  delete: %v\n", err)
		} else {
			fmt.Println("  cleanup: deleted keychain entry")
		}
	}

	// ---------------------------------------------------------------
	// PART B: pure-Go encrypted-file fallback (filippo.io/age, scrypt)
	// This is the headless-Linux / no-daemon path. No OS service needed.
	// ---------------------------------------------------------------
	fmt.Println("\n[B] encrypted-file fallback (age scrypt, pure Go)")
	passphrase := "correct-horse-battery-staple"
	plaintext := []byte(secret)

	rcpt, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		fmt.Printf("  recipient FAILED: %v\n", err)
		os.Exit(1)
	}
	rcpt.SetWorkFactor(15) // keep the spike fast

	dir, _ := os.MkdirTemp("", "s58age")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "secret.age")

	f, _ := os.Create(path)
	w, err := age.Encrypt(f, rcpt)
	if err != nil {
		fmt.Printf("  encrypt FAILED: %v\n", err)
		os.Exit(1)
	}
	if _, err := w.Write(plaintext); err != nil {
		fmt.Printf("  write FAILED: %v\n", err)
		os.Exit(1)
	}
	w.Close()
	f.Close()
	fi, _ := os.Stat(path)
	fmt.Printf("  encrypted -> %s (%d bytes on disk)\n", filepath.Base(path), fi.Size())

	id, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		fmt.Printf("  identity FAILED: %v\n", err)
		os.Exit(1)
	}
	rf, _ := os.Open(path)
	r, err := age.Decrypt(rf, id)
	if err != nil {
		fmt.Printf("  decrypt FAILED: %v\n", err)
		os.Exit(1)
	}
	out, _ := io.ReadAll(r)
	rf.Close()
	fmt.Printf("  decrypted                       -> %q\n", string(out))
	fmt.Printf("  round-trip match: %v\n", bytes.Equal(out, plaintext))

	fmt.Println("\n=== done ===")
}
