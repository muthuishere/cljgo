// cli_auth_test.go — the bri.cli.auth behavior suite (ADR 0080, credential
// core). No JVM oracle (bri.cli.auth is bri-specific). The login/token/logout
// flow goes through the OS keychain, which CI has no session for, so the
// keychain shims are stubbed with an in-memory map (as in secrets_test.go) —
// these test the AUTH logic: prompt→store, read-back, header shape, logout,
// and the empty-credential guard. Transitive opt-in linking is proven
// separately in pkg/emit (TestBriCliAuthLinksSecretsTransitively).
package bri_test

import (
	"strings"
	"testing"
)

func TestCliAuth(t *testing.T) {
	d := newDriver(t)
	// load bri.cli.auth (pulls bri.cli + cljg.secrets), then stub the
	// keychain shims so the flow runs deterministically with no real keychain.
	eval(t, d, `
	  (require '[bri.cli.auth :as cauth] '[cljg.secrets :as secrets])
	  (clojure.core/in-ns 'cljg.secrets)
	  (def kc (atom {}))
	  (defn -keychain-set [s a v] (swap! kc assoc [s a] v) nil)
	  (defn -keychain-get [s a] (@kc [s a]))
	  (defn -keychain-del [s a] (swap! kc dissoc [s a]) nil)
	  (clojure.core/in-ns 'user)
	  (require '[bri.cli :as cli] '[bri.cli.auth :as cauth] '[cljg.secrets :as secrets])`)

	// login by PROMPTING for the key with echo off (via the *prompt* seam)
	eval(t, d, `(binding [cli/*prompt* (fn [_ secret?] (if secret? "sk-abc123" (throw (ex-info "expected a secret prompt" {}))))]
	              (cauth/login "my-api"))`)

	// token reads it back MASKED; reveal → plaintext
	if got := evalString(t, d, `(:masked (cauth/token "my-api"))`); !strings.HasPrefix(got, "len=9") {
		t.Errorf("token masked = %q, want a len=9 mask", got)
	}
	if got := evalString(t, d, `(secrets/reveal (cauth/token "my-api"))`); got != "sk-abc123" {
		t.Errorf("reveal(token) = %q, want sk-abc123", got)
	}
	// authed?
	if got := eval(t, d, `(cauth/authed? "my-api")`); got != true {
		t.Errorf("authed? = %v, want true", got)
	}
	// auth-header builds Bearer by default
	if got := evalString(t, d, `(clojure.core/get (cauth/auth-header "my-api") "Authorization")`); got != "Bearer sk-abc123" {
		t.Errorf("auth-header = %q, want 'Bearer sk-abc123'", got)
	}
	// :scheme "" → a bare token (API-key header value)
	if got := evalString(t, d, `(clojure.core/get (cauth/auth-header "my-api" {:scheme ""}) "Authorization")`); got != "sk-abc123" {
		t.Errorf("bare-scheme header = %q, want sk-abc123", got)
	}

	// login with a supplied :key skips the prompt
	eval(t, d, `(cauth/login "other" {:key "tok-xyz"})`)
	if got := evalString(t, d, `(secrets/reveal (cauth/token "other"))`); got != "tok-xyz" {
		t.Errorf("login :key = %q, want tok-xyz", got)
	}

	// logout forgets it: authed? false, token nil, header nil
	eval(t, d, `(cauth/logout "my-api")`)
	if got := eval(t, d, `(cauth/authed? "my-api")`); got != false {
		t.Errorf("after logout authed? = %v, want false", got)
	}
	if got := evalString(t, d, `(pr-str (cauth/token "my-api"))`); got != "nil" {
		t.Errorf("after logout token = %q, want nil", got)
	}
	if got := evalString(t, d, `(pr-str (cauth/auth-header "my-api"))`); got != "nil" {
		t.Errorf("after logout auth-header = %q, want nil", got)
	}

	// an empty credential is a named error, not a silent store
	if msg := evalErr(t, d, `(binding [cli/*prompt* (fn [_ _] "   ")] (cauth/login "svc"))`); !strings.Contains(msg, "no credential") {
		t.Errorf("empty credential error = %q, want it to mention no credential", msg)
	}
}
