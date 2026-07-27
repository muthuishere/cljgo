// security_test.go — the FILE backend of the s65 unified keychain store,
// exercised deterministically (no OS keyring daemon needed, so it runs in any
// CI). The native backend is probe-gated at runtime and cannot be asserted
// portably; keychain conformance is therefore Go-level here, not a .clj file.
package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempStore points the file keystore at a per-test dir.
func withTempStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := storeDir
	storeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { storeDir = old })
	return dir
}

func TestFileBackendRoundTrip(t *testing.T) {
	withTempStore(t)
	const service, account, secret = "cljgo-test", "api-token", "s3cr3t-value"

	used, err := storeSet(service, account, secret, "file")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if used != BackendFile {
		t.Fatalf("set served by %q, want %q", used, BackendFile)
	}

	v, ok, used, err := storeGet(service, account, "file")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || v != secret || used != BackendFile {
		t.Fatalf("get = (%v, ok=%v, via %q), want the stored secret via file", v != "", ok, used)
	}

	if used, err = storeDel(service, account, "file"); err != nil || used != BackendFile {
		t.Fatalf("del: %v via %q", err, used)
	}
	if _, ok, _, err := storeGet(service, account, "file"); err != nil || ok {
		t.Fatalf("get after delete: ok=%v err=%v, want a clean miss", ok, err)
	}
}

func TestFileBackendMissIsNilNotError(t *testing.T) {
	withTempStore(t)
	v, ok, _, err := storeGet("cljgo-test", "never-stored", "file")
	if err != nil || ok || v != "" {
		t.Fatalf("miss = (%q, ok=%v, err=%v), want a clean nil miss", v, ok, err)
	}
	// deleting an absent key is a no-op, like Bun
	if _, err := storeDel("cljgo-test", "never-stored", "file"); err != nil {
		t.Fatalf("delete of absent key: %v, want no-op", err)
	}
}

func TestFileBackendEncryptsAtRest(t *testing.T) {
	dir := withTempStore(t)
	const secret = "plaintext-must-not-appear"
	if _, err := storeSet("svc", "acct", secret, "file"); err != nil {
		t.Fatalf("set: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), secret) {
			t.Fatalf("%s holds the secret in plaintext — the file backend must encrypt at rest", e.Name())
		}
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("%s has mode %v — keystore files must be 0600", e.Name(), perm)
		}
	}
}

func TestUnknownBackendIsNamedError(t *testing.T) {
	withTempStore(t)
	if _, err := storeSet("svc", "acct", "v", "cloud"); err == nil || !strings.Contains(err.Error(), "unknown keychain backend") {
		t.Fatalf("want a named unknown-backend error, got %v", err)
	}
}
