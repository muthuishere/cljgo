// secrets_test.go — white-box provider tests for bri.core.secrets (ADR 0086).
// These exercise the scalar fetch/store boundary directly (no interpreter):
// the env provider, URI parsing/dispatch, the read-only guard, and that the
// keychain scheme RESOLVES. A live keychain round-trip is NOT run — CI has no
// keychain session; the keychain path is covered by the cgo-free build + this
// scheme-resolution check. The Clojure surface (mask/reveal/chain) is tested
// through the interpreter in pkg/bri/secrets_test.go.
package secrets

import (
	"strings"
	"testing"
)

func TestFetchEnv(t *testing.T) {
	t.Setenv("BRI_SECRET_TEST", "s3cr3t")
	// env://KEY — the key is in the URI host
	if v, ok, err := fetch("env://BRI_SECRET_TEST", ""); err != nil || !ok || v != "s3cr3t" {
		t.Fatalf("env://KEY = (%q,%v,%v), want (s3cr3t,true,nil)", v, ok, err)
	}
	// the caller key names the var when the URI omits it (env://)
	if v, ok, _ := fetch("env://", "BRI_SECRET_TEST"); !ok || v != "s3cr3t" {
		t.Errorf("env:// + key = (%q,%v), want (s3cr3t,true)", v, ok)
	}
	// a miss is ok=false, not an error (drives the chain)
	if _, ok, err := fetch("env://BRI_DEFINITELY_MISSING", ""); ok || err != nil {
		t.Errorf("env miss = (ok=%v,err=%v), want (false,nil)", ok, err)
	}
}

func TestParseAndSchemeErrors(t *testing.T) {
	if _, _, err := fetch("no-scheme-here", ""); err == nil || !strings.Contains(err.Error(), "no scheme") {
		t.Errorf("missing scheme should error, got %v", err)
	}
	if _, _, err := fetch("ftp://x", ""); err == nil || !strings.Contains(err.Error(), "unknown scheme") {
		t.Errorf("unknown scheme should error, got %v", err)
	}
}

func TestKeychainSchemeResolves(t *testing.T) {
	// no live keychain call — just prove the URI parses into service/account.
	sc, host, path, err := parseURI("keychain://myapp/db-password")
	if err != nil || sc != "keychain" || host != "myapp" || path != "db-password" {
		t.Fatalf("parseURI keychain = (%q,%q,%q,%v)", sc, host, path, err)
	}
}

func TestEnvIsReadOnly(t *testing.T) {
	if err := store("env://X", "", "v"); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("env set should be read-only, got %v", err)
	}
	if err := remove("env://X", ""); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("env delete should be read-only, got %v", err)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "c"); got != "c" {
		t.Errorf("firstNonEmpty = %q, want c", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("firstNonEmpty = %q, want a", got)
	}
}
