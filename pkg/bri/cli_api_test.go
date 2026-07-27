// cli_api_test.go — the bri.cli.api auto-login behavior suite (ADR 0091). No JVM
// oracle. The credential flow goes through the OS keychain (no CI session), so the
// keychain shims are stubbed with an in-memory map (as in cli_auth_test.go), and
// prompts are scripted through the bri.cli/*prompt* seam. An httptest server serves
// the OpenAPI spec + a protected endpoint (401 for a stale token) + a login
// endpoint, so token/password strategies and 401-refresh run end to end. Alias
// `capi` (vars are process-global).
package bri_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/muthuishere/cljgo/pkg/repl"
)

// stubbedAPIDriver loads bri.cli.api (+ cli/auth/secrets) and replaces the keychain
// shims with a fresh in-memory atom so the flow runs with no real keychain.
func stubbedAPIDriver(t *testing.T) *repl.Driver {
	d := newDriver(t)
	eval(t, d, `
	  (require '[bri.cli.api :as capi] '[bri.cli :as cli]
	           '[bri.cli.auth :as cauth] '[cljg.secrets :as secrets])
	  (clojure.core/in-ns 'cljg.secrets)
	  (def kc (atom {}))
	  (defn -keychain-set [s a v] (swap! kc assoc [s a] v) nil)
	  (defn -keychain-get [s a] (@kc [s a]))
	  (defn -keychain-del [s a] (swap! kc dissoc [s a]) nil)
	  (clojure.core/in-ns 'user)
	  (require '[bri.cli.api :as capi] '[bri.cli :as cli]
	           '[bri.cli.auth :as cauth] '[cljg.secrets :as secrets])`)
	return d
}

// newAPIServer serves the spec (getMe GET /me, login POST /login), a protected
// /me that 401s on a "Bearer STALE" token and otherwise echoes the auth header,
// and a /login that mints "jwt-alice" for the right password.
func newAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"openapi":"3.0.0","servers":[{"url":%q}],
          "paths":{"/me":{"get":{"operationId":"getMe"}},
                   "/login":{"post":{"operationId":"login"}}}}`, base)
	})
	mux.HandleFunc("GET /me", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer STALE" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"expired"}`)
			return
		}
		fmt.Fprintf(w, `{"auth":%q}`, auth)
	})
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"secret"`) {
			fmt.Fprint(w, `{"token":"jwt-alice"}`)
			return
		}
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"bad creds"}`)
	})
	// RFC 8628 device flow: /device issues the codes, /token returns
	// authorization_pending for the first two polls then grants "device-jwt".
	var polls int32
	mux.HandleFunc("POST /device", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"device_code":"dev-123","user_code":"WDJB-MJHT",
          "verification_uri":%q,"verification_uri_complete":%q,
          "interval":0,"expires_in":900}`, base+"/activate", base+"/activate?code=WDJB-MJHT")
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&polls, 1) < 3 {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"authorization_pending"}`)
			return
		}
		fmt.Fprint(w, `{"access_token":"device-jwt","token_type":"Bearer"}`)
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func TestCliApiTokenStrategy(t *testing.T) {
	d := stubbedAPIDriver(t)
	srv := newAPIServer(t)
	eval(t, d, `(def a (capi/api "`+srv.URL+`/openapi.json" {:service "svc" :auth :token}))`)
	eval(t, d, `(def prompts (atom 0))`)

	// first call: no credential → prompt once (echo off) → cache → attach Bearer
	got := evalString(t, d, `(binding [cli/*prompt* (fn [_ _] (swap! prompts inc) "sk-key")]
	                            (:auth (capi/result (capi/call a :getMe {}))))`)
	if got != "Bearer sk-key" {
		t.Errorf("first call auth = %q, want %q", got, "Bearer sk-key")
	}
	// second call: credential cached → the prompt (which would return WRONG) must
	// NOT fire; the cached key is reused
	got2 := evalString(t, d, `(binding [cli/*prompt* (fn [_ _] (swap! prompts inc) "WRONG")]
	                             (:auth (capi/result (capi/call a :getMe {}))))`)
	if got2 != "Bearer sk-key" {
		t.Errorf("second call auth = %q, want the cached %q", got2, "Bearer sk-key")
	}
	if n := evalString(t, d, `(str @prompts)`); n != "1" {
		t.Errorf("prompted %s times, want exactly 1 (login is automatic + cached)", n)
	}

	// explicit lifecycle: authed? / logout
	if got := eval(t, d, `(capi/authed? a)`); got != true {
		t.Errorf("authed? = %v, want true", got)
	}
	eval(t, d, `(capi/logout a)`)
	if got := eval(t, d, `(capi/authed? a)`); got != false {
		t.Errorf("after logout authed? = %v, want false", got)
	}
}

func TestCliApiPasswordStrategy(t *testing.T) {
	d := stubbedAPIDriver(t)
	srv := newAPIServer(t)
	eval(t, d, `(def a (capi/api "`+srv.URL+`/openapi.json"
	                             {:service "svc2" :auth :password
	                              :login {:op :login :token-path [:token]}}))`)

	// username (non-secret) then password (secret) are prompted, exchanged at the
	// login op for a token, cached, and attached as Bearer jwt-alice
	got := evalString(t, d, `(binding [cli/*prompt* (fn [_ secret?] (if secret? "secret" "alice"))]
	                            (:auth (capi/result (capi/call a :getMe {}))))`)
	if got != "Bearer jwt-alice" {
		t.Errorf("password-strategy auth = %q, want %q", got, "Bearer jwt-alice")
	}
	// the exchanged token is cached (no second login)
	if got := eval(t, d, `(capi/authed? a)`); got != true {
		t.Errorf("authed? after password login = %v, want true", got)
	}

	// a bad password → login op returns 401 → named login-failed error
	eval(t, d, `(capi/logout a)`)
	if msg := evalErr(t, d, `(binding [cli/*prompt* (fn [_ secret?] (if secret? "wrong" "alice"))]
	                           (capi/call a :getMe {}))`); !strings.Contains(msg, "login failed") {
		t.Errorf("bad-password error = %q, want it to mention login failed", msg)
	}
}

func TestCliApiDeviceStrategy(t *testing.T) {
	d := stubbedAPIDriver(t)
	srv := newAPIServer(t)
	// :device — no secret is prompted; the flow polls /token until it grants.
	// :poll-interval-ms 2 keeps the (three-poll) loop to a few ms.
	eval(t, d, `(def a (capi/api "`+srv.URL+`/openapi.json"
	                             {:service "svc4" :auth :device
	                              :device {:device-url "`+srv.URL+`/device"
	                                       :token-url  "`+srv.URL+`/token"
	                                       :client-id  "cli-abc"}
	                              :poll-interval-ms 2}))`)
	// the first call runs the device grant (browser-authorized), caches the
	// access token, and attaches it — with NO prompt bound at all
	got := evalString(t, d, `(:auth (capi/result (capi/call a :getMe {})))`)
	if got != "Bearer device-jwt" {
		t.Errorf("device-strategy auth = %q, want %q", got, "Bearer device-jwt")
	}
	// the granted token is cached (no second device dance)
	if got := eval(t, d, `(capi/authed? a)`); got != true {
		t.Errorf("authed? after device login = %v, want true", got)
	}
}

func TestCliApiRefreshesOn401(t *testing.T) {
	d := stubbedAPIDriver(t)
	srv := newAPIServer(t)
	// pre-seed a STALE credential the server rejects with 401
	eval(t, d, `(cauth/login "svc3" {:key "STALE"})`)
	eval(t, d, `(def a (capi/api "`+srv.URL+`/openapi.json" {:service "svc3" :auth :token}))`)

	// call attaches STALE → 401 → drops it → retries → re-login (FRESH) → 200
	got := evalString(t, d, `(binding [cli/*prompt* (fn [_ _] "FRESH")]
	                            (:auth (capi/result (capi/call a :getMe {}))))`)
	if got != "Bearer FRESH" {
		t.Errorf("after 401 refresh auth = %q, want %q", got, "Bearer FRESH")
	}
}
