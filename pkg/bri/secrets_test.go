// secrets_test.go — the bri.core.secrets behavior suite through the real
// interpreter (ADR 0086). No JVM oracle (bri.core.secrets is bri-specific), so
// these drive the Clojure surface: masking hygiene (the plaintext never
// appears in a printed form), the fallback chain, and the read-only guard. The
// scalar providers are covered white-box in pkg/bri/secrets. The opt-in
// namespace loads because briloader blank-imports pkg/bri/secrets.
package bri_test

import (
	"strings"
	"testing"
)

// TestSecretsMaskingHygiene: get returns a MASKED secret — the plaintext is
// reachable only via reveal, never in pr-str/println (the CLAUDE.md rule).
func TestSecretsMaskingHygiene(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.core.secrets :as secrets])`)

	// reveal round-trips the plaintext
	if got := evalString(t, d, `(secrets/reveal (secrets/secret "hunter2"))`); got != "hunter2" {
		t.Errorf("reveal = %q, want hunter2", got)
	}
	// the masked form names the length + last 2 chars, never the value
	if got := evalString(t, d, `(:masked (secrets/secret "hunter2"))`); got != "len=7 ***…r2" {
		t.Errorf("mask = %q, want len=7 ***…r2", got)
	}
	// CRITICAL: printing a secret must NOT leak the plaintext (raw lives in meta)
	printed := evalString(t, d, `(pr-str (secrets/secret "hunter2"))`)
	if strings.Contains(printed, "hunter2") {
		t.Fatalf("SECRET LEAKED in pr-str: %q", printed)
	}
	if !strings.Contains(printed, "***") {
		t.Errorf("printed secret should show the mask, got %q", printed)
	}
	// tiny + empty masks don't leak length-1 values
	if got := evalString(t, d, `(:masked (secrets/secret ""))`); got != "***(empty)" {
		t.Errorf("empty mask = %q", got)
	}
	if got := evalString(t, d, `(:masked (secrets/secret "ab"))`); got != "len=2 ***" {
		t.Errorf("short mask = %q, want len=2 ***", got)
	}
}

// TestSecretsGetChain: env resolution, the left→right fallback chain, and a
// clean nil on a total miss.
func TestSecretsGetChain(t *testing.T) {
	d := newDriver(t)
	t.Setenv("BRI_SECRET_ONE", "alpha")
	eval(t, d, `(require '[bri.core.secrets :as secrets])`)

	// single env URI → a secret revealing the value
	if got := evalString(t, d, `(secrets/reveal (secrets/get "env://BRI_SECRET_ONE"))`); got != "alpha" {
		t.Errorf("get env = %q, want alpha", got)
	}
	// chain: first (missing) rolls to the second (present)
	if got := evalString(t, d, `(secrets/reveal (secrets/get ["env://BRI_MISSING_X" "env://BRI_SECRET_ONE"]))`); got != "alpha" {
		t.Errorf("chain fallback = %q, want alpha", got)
	}
	// a total miss is nil (not an error)
	if got := evalString(t, d, `(pr-str (secrets/get "env://BRI_DEFINITELY_MISSING"))`); got != "nil" {
		t.Errorf("miss = %q, want nil", got)
	}
	// the returned value is a secret object, and reveal passes a bare string through
	if got := evalString(t, d, `(str (secrets/secret? (secrets/get "env://BRI_SECRET_ONE")))`); got != "true" {
		t.Errorf("get returns a secret? = %q, want true", got)
	}
	if got := evalString(t, d, `(secrets/reveal "plain")`); got != "plain" {
		t.Errorf("reveal of a bare string = %q, want plain", got)
	}
}

// TestSecretsReadOnlyGuard: env:// is read-only; set/delete are named errors,
// and an unknown scheme is reported with the known ones.
func TestSecretsReadOnlyGuard(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.core.secrets :as secrets])`)
	if msg := evalErr(t, d, `(secrets/set "env://X" "v")`); !strings.Contains(msg, "read-only") {
		t.Errorf("env set error = %q, want read-only", msg)
	}
	if msg := evalErr(t, d, `(secrets/get "ftp://nope")`); !strings.Contains(msg, "unknown scheme") {
		t.Errorf("unknown scheme error = %q", msg)
	}
}

// TestSecretsBunStyleKeychain: the Bun.secrets front door — (set/get/delete)
// by (service, name) or a {:service :name :value} map routes to the OS
// keychain (no URI). CI has no keychain, so the -keychain-* shims are stubbed
// with an in-memory map to test the ROUTING deterministically; a plain service
// name must NOT be mistaken for a URI.
func TestSecretsBunStyleKeychain(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
	  (require '[bri.core.secrets :as secrets])
	  (clojure.core/in-ns 'bri.core.secrets)
	  (def store (atom {}))
	  (defn -keychain-set [svc acct v] (swap! store assoc [svc acct] v) nil)
	  (defn -keychain-get [svc acct] (@store [svc acct]))
	  (defn -keychain-del [svc acct] (swap! store dissoc [svc acct]) nil)
	  (clojure.core/in-ns 'user)
	  (require '[bri.core.secrets :as secrets])`)

	// (set service name value) → (get service name) round-trips via keychain
	if got := evalString(t, d, `(do (secrets/set "my-app" "api-key" "sk-123")
	                                 (secrets/reveal (secrets/get "my-app" "api-key")))`); got != "sk-123" {
		t.Errorf("Bun-style set/get = %q, want sk-123", got)
	}
	// the get is a MASKED secret, not a bare string
	if got := evalString(t, d, `(str (secrets/secret? (secrets/get "my-app" "api-key")))`); got != "true" {
		t.Errorf("keychain get returns a secret? = %q, want true", got)
	}
	// map form mirrors Bun.secrets.set({service, name, value})
	if got := evalString(t, d, `(do (secrets/set {:service "svc2" :name "tok" :value "v2"})
	                                 (secrets/reveal (secrets/get {:service "svc2" :name "tok"})))`); got != "v2" {
		t.Errorf("Bun map form = %q, want v2", got)
	}
	// delete removes it → get is nil (and deleting again is a no-op)
	if got := evalString(t, d, `(do (secrets/delete "my-app" "api-key")
	                                 (secrets/delete "my-app" "api-key")
	                                 (pr-str (secrets/get "my-app" "api-key")))`); got != "nil" {
		t.Errorf("after delete get = %q, want nil", got)
	}
	// a URI-shaped arg still routes to the scheme path (env), proving the split
	t.Setenv("BRI_BUN_ENV", "fromenv")
	if got := evalString(t, d, `(secrets/reveal (secrets/get "env://BRI_BUN_ENV"))`); got != "fromenv" {
		t.Errorf("URI arg should use the scheme path = %q, want fromenv", got)
	}
}
