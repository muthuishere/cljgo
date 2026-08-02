// net_http_test.go — the cljg.net.http behavior suite (ADR 0087). No JVM
// oracle; these drive the client through the interpreter against a local
// httptest server (no external network — CI-safe), covering the verbs, query
// strings, body encoding + content-type, header passthrough, response
// decoding, status/:ok?, and retry-on-5xx.
package bri_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newHTTPServer(t *testing.T, failN *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Method", r.Method)
		w.Header().Set("X-Content-Type", r.Header.Get("Content-Type"))
		w.Header().Set("X-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(200)
		fmt.Fprint(w, string(b))
	})
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.RawQuery)
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"hello":"world","n":42}`)
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, "nope")
	})
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		// fail with 500 the first *failN times, then 200
		if atomic.AddInt32(failN, -1) >= 0 {
			w.WriteHeader(500)
			fmt.Fprint(w, "boom")
			return
		}
		w.WriteHeader(200)
		fmt.Fprint(w, "recovered")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCljgNetHTTP(t *testing.T) {
	d := newDriver(t)
	var fail int32 = 2 // /flaky returns 500 twice, then 200
	srv := newHTTPServer(t, &fail)
	eval(t, d, `(require '[cljg.net.http :as hc])`)
	u := func(p string) string { return srv.URL + p }

	// GET → status/body/:ok?
	if got := eval(t, d, `(:status (hc/get "`+u("/echo")+`"))`); got != int64(200) {
		t.Errorf("GET status = %v, want 200", got)
	}
	if got := eval(t, d, `(:ok? (hc/get "`+u("/echo")+`"))`); got != true {
		t.Errorf("GET :ok? = %v, want true", got)
	}

	// query map → the server's raw query
	if got := evalString(t, d, `(:body (hc/get "`+u("/query")+`" {:query {"q" "a b" "n" 2}}))`); !strings.Contains(got, "q=a+b") || !strings.Contains(got, "n=2") {
		t.Errorf("query = %q, want q=a+b & n=2", got)
	}

	// POST :json → body echoed + content-type set
	if got := evalString(t, d, `(:body (hc/post "`+u("/echo")+`" {:json {:name "x" :n 1}}))`); !strings.Contains(got, `"name":"x"`) {
		t.Errorf("POST :json body = %q, want the JSON", got)
	}
	if got := evalString(t, d, `(clojure.core/get (:headers (hc/post "`+u("/echo")+`" {:json {:a 1}})) "X-Content-Type")`); got != "application/json" {
		t.Errorf("POST :json content-type = %q, want application/json", got)
	}
	// method reaches the server
	if got := evalString(t, d, `(clojure.core/get (:headers (hc/post "`+u("/echo")+`" {:body "hi"})) "X-Method")`); got != "POST" {
		t.Errorf("method = %q, want POST", got)
	}

	// header passthrough (the auth use case)
	if got := evalString(t, d, `(clojure.core/get (:headers (hc/get "`+u("/echo")+`" {:headers {"Authorization" "Bearer sk-123"}})) "X-Auth")`); got != "Bearer sk-123" {
		t.Errorf("auth header = %q, want 'Bearer sk-123'", got)
	}

	// json-body decodes an application/json response
	if got := evalString(t, d, `(:hello (hc/json-body (hc/get "`+u("/json")+`")))`); got != "world" {
		t.Errorf("json-body :hello = %q, want world", got)
	}
	if got := eval(t, d, `(:n (hc/json-body (hc/get "`+u("/json")+`")))`); got != int64(42) {
		t.Errorf("json-body :n = %v, want 42", got)
	}

	// 404 → :ok? false
	if got := eval(t, d, `(:status (hc/get "`+u("/missing")+`"))`); got != int64(404) {
		t.Errorf("404 status = %v, want 404", got)
	}
	if got := eval(t, d, `(:ok? (hc/get "`+u("/missing")+`"))`); got != false {
		t.Errorf("404 :ok? = %v, want false", got)
	}

	// retry: /flaky 500s twice then 200 — with :retry 3 the client recovers
	if got := evalString(t, d, `(:body (hc/get "`+u("/flaky")+`" {:retry 3}))`); got != "recovered" {
		t.Errorf("retry body = %q, want recovered", got)
	}
}

// TestCljgNetHTTPTimeoutKey pins the timeout OPTION to the wire, both
// spellings. Issue #192: cljg.net.http read `:timeout` while cljg.socket,
// cljg.io and cljg.process all read `:timeout-ms`, and an unrecognised opts
// key is silently ignored — so a caller who used the majority spelling got
// the 30 s default with no error and no warning. The koine port hit exactly
// that, and every one of their tests passed, because a fast server returns
// long before any timeout matters.
//
// The assertion is therefore NOT "the option is accepted". It is "the option
// BOUNDS a call that would otherwise outlast it" — the only form that can
// fail when the key is ignored.
func TestCljgNetHTTPTimeoutKey(t *testing.T) {
	d := newDriver(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
			fmt.Fprint(w, "too late")
		case <-r.Context().Done(): // client hung up: the timeout fired
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	eval(t, d, `(require '[cljg.net.http :as hc])`)

	for _, k := range []string{":timeout-ms", ":timeout"} {
		start := time.Now()
		errText := evalErr(t, d, `(hc/get "`+srv.URL+`/slow" {`+k+` 200})`)
		elapsed := time.Since(start)
		if errText == "" {
			t.Errorf("%s 200: request returned normally, want a timeout error "+
				"(the key was ignored and the 30s default applied)", k)
		}
		// Generous bound: the point is 200ms-ish, not 10s. Anything under a
		// second proves the option reached net/http rather than the default.
		if elapsed > 5*time.Second {
			t.Errorf("%s 200: took %v, want the call bounded near 200ms", k, elapsed)
		}
	}
}
