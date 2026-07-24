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
