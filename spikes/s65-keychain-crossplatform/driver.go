// Spike s65-keychain-crossplatform: prove save-to-keychain / get-from-keychain
// works on ALL THREE platforms (macOS, Windows, Linux) "like Bun" — under
// CGO_ENABLED=0 — via ONE unified store that always succeeds:
//
//	native OS keychain  (macOS Keychain / Windows Credential Manager / Linux
//	                     Secret Service) when a credential store is reachable,
//	transparent fallback to a machine-local age-encrypted file when it is not
//	                     (headless servers, CI — the case a bare Linux box hits).
//
// This models the shipping cljg.security keychain contract: the caller gets the
// same save/get/delete API on every platform and it never fails just because a
// desktop keyring daemon is absent. The secret VALUE is never printed except
// the round-trip echo of a throwaway test secret.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/zalando/go-keyring"
)

// Backend names the store that actually served an operation.
type Backend string

const (
	BackendNative Backend = "native-os-keychain"
	BackendFile   Backend = "encrypted-file-fallback"
)

// Store is the unified secure credential store. Auto mode tries the native OS
// keychain first and falls back to the encrypted file when native is
// unavailable — so the SAME code path works on macOS, Windows and Linux.
type Store struct {
	dir   string // where the fallback identity + blobs live (0700)
	force Backend
}

func NewStore(dir string, force Backend) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, force: force}, nil
}

// nativeUsable reports whether the OS credential store answered a probe. A
// failed probe (no Secret Service daemon on a headless Linux box, locked
// keychain, etc.) means "use the fallback" — never an error to the caller.
func nativeUsable() bool {
	const probe = "cljgo-native-probe"
	if err := keyring.Set(probe, probe, "1"); err != nil {
		return false
	}
	_, err := keyring.Get(probe, probe)
	_ = keyring.Delete(probe, probe)
	return err == nil
}

func (s *Store) pick() Backend {
	if s.force != "" {
		return s.force
	}
	if nativeUsable() {
		return BackendNative
	}
	return BackendFile
}

func (s *Store) Set(service, user, secret string) (Backend, error) {
	switch s.pick() {
	case BackendNative:
		return BackendNative, keyring.Set(service, user, secret)
	default:
		return BackendFile, s.fileSet(service, user, secret)
	}
}

func (s *Store) Get(service, user string) (string, Backend, error) {
	switch s.pick() {
	case BackendNative:
		v, err := keyring.Get(service, user)
		return v, BackendNative, err
	default:
		v, err := s.fileGet(service, user)
		return v, BackendFile, err
	}
}

func (s *Store) Delete(service, user string) (Backend, error) {
	switch s.pick() {
	case BackendNative:
		return BackendNative, keyring.Delete(service, user)
	default:
		return BackendFile, os.Remove(s.blobPath(service, user))
	}
}

// --- encrypted-file fallback (pure Go, age X25519, machine-local key) --------
//
// A per-machine age identity is generated once, stored 0600 in the store dir;
// secrets are encrypted to it. No passphrase prompt, no daemon — works on any
// headless box. (In the real cljg.security this key is itself protected by the
// OS keychain when available, or file perms + optional passphrase when not.)

func (s *Store) identity() (*age.X25519Identity, error) {
	keyPath := filepath.Join(s.dir, "machine.age-key")
	if b, err := os.ReadFile(keyPath); err == nil {
		return age.ParseX25519Identity(strings.TrimSpace(string(b)))
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(id.String()), 0o600); err != nil {
		return nil, err
	}
	return id, nil
}

func (s *Store) blobPath(service, user string) string {
	safe := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(service + "__" + user)
	return filepath.Join(s.dir, safe+".age")
}

func (s *Store) fileSet(service, user, secret string) error {
	id, err := s.identity()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, id.Recipient())
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, secret); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.WriteFile(s.blobPath(service, user), buf.Bytes(), 0o600)
}

func (s *Store) fileGet(service, user string) (string, error) {
	id, err := s.identity()
	if err != nil {
		return "", err
	}
	ct, err := os.ReadFile(s.blobPath(service, user))
	if err != nil {
		return "", err
	}
	r, err := age.Decrypt(bytes.NewReader(ct), id)
	if err != nil {
		return "", err
	}
	pt, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func main() {
	// Optional force: `go run . file` or `go run . native` (default: auto).
	force := Backend("")
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "file":
			force = BackendFile
		case "native":
			force = BackendNative
		}
	}

	fmt.Println("=== s65 unified cross-platform keychain ===")
	fmt.Printf("runtime: %s/%s   CGO_ENABLED=0 static binary   mode: %s\n",
		runtime.GOOS, runtime.GOARCH, orAuto(force))
	fmt.Printf("native OS keychain reachable here? %v\n", nativeUsable())

	dir := filepath.Join(os.TempDir(), "cljgo-s65-store")
	_ = os.RemoveAll(dir)
	st, err := NewStore(dir, force)
	must(err)

	const service = "cljgo-spike-s65"
	const user = "api-token"
	secret := fmt.Sprintf("s3cr3t-%s-%d", runtime.GOOS, time.Now().Unix())

	// saveinkeychain
	usedSet, err := st.Set(service, user, secret)
	must(err)
	fmt.Printf("\nsave-to-keychain    service=%q user=%q  -> OK via %s\n", service, user, usedSet)

	// retrivefromkeychain
	got, usedGet, err := st.Get(service, user)
	must(err)
	match := got == secret
	fmt.Printf("get-from-keychain                          -> %q via %s\n", got, usedGet)
	fmt.Printf("round-trip match: %v\n", match)

	// delete (cleanup)
	usedDel, err := st.Delete(service, user)
	must(err)
	fmt.Printf("delete-from-keychain                       -> OK via %s\n", usedDel)
	_ = os.RemoveAll(dir)

	if !match {
		fmt.Println("\nRESULT: FAIL (round-trip mismatch)")
		os.Exit(1)
	}
	fmt.Printf("\nRESULT: OK — save/get/delete round-trip on %s via %s\n", runtime.GOOS, usedGet)
}

func orAuto(b Backend) string {
	if b == "" {
		return "auto (native, else file)"
	}
	return string(b)
}

func must(err error) {
	if err != nil {
		// keyring.ErrNotFound is only meaningful on Get; surface everything else.
		if errors.Is(err, keyring.ErrNotFound) {
			return
		}
		fmt.Printf("\nERROR: %v\n", err)
		os.Exit(1)
	}
}
