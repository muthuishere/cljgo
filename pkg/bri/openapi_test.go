// openapi_test.go — the bri.web.openapi behavior suite (ADR 0090). No JVM oracle;
// the client is driven through the interpreter against a local httptest server
// that BOTH serves a small OpenAPI spec (so the URL-load path is exercised) and
// echoes back what each endpoint received — so path/query/header/body routing and
// auth attachment are asserted end to end. Alias `oa` (vars are process-global).
package bri_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newOpenAPIServer serves an OpenAPI spec at /openapi.json (its servers[0].url is
// the server's own base) plus two operations that echo what they received as JSON.
func newOpenAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
          "openapi":"3.0.0",
          "servers":[{"url":%q}],
          "paths":{
            "/users/{id}":{"get":{"operationId":"getUser","parameters":[
               {"name":"id","in":"path"},{"name":"verbose","in":"query"},
               {"name":"X-Trace","in":"header"}]}},
            "/notes":{"post":{"operationId":"createNote","requestBody":{}}}
          }
        }`, base)
	})
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"verbose":%q,"trace":%q,"auth":%q}`,
			r.PathValue("id"), r.URL.Query().Get("verbose"),
			r.Header.Get("X-Trace"), r.Header.Get("Authorization"))
	})
	mux.HandleFunc("POST /notes", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"got":%s,"auth":%q,"ctype":%q}`,
			string(b), r.Header.Get("Authorization"), r.Header.Get("Content-Type"))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func TestBriWebOpenAPI(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.web.openapi :as oa])`)
	srv := newOpenAPIServer(t)

	// load from a URL; base-url comes from the spec's servers[0]
	eval(t, d, `(def c (oa/client "`+srv.URL+`/openapi.json" {:token "abc"}))`)

	// operations are indexed by operationId
	if got := evalString(t, d, `(pr-str (oa/operations c))`); got != `[:createNote :getUser]` {
		t.Errorf("operations = %s, want [:createNote :getUser]", got)
	}

	// path param substituted, query + header routed by the spec, auth attached
	res := evalString(t, d, `(let [r (oa/result (oa/call c :getUser {:id 42 :verbose "yes" :X-Trace "t1"}))]
                               (str (:id r) "|" (:verbose r) "|" (:trace r) "|" (:auth r)))`)
	if res != "42|yes|t1|Bearer abc" {
		t.Errorf("getUser echo = %q, want %q", res, "42|yes|t1|Bearer abc")
	}

	// POST body is sent as JSON with the right content-type
	body := evalString(t, d, `(let [r (oa/result (oa/call c :createNote {:body {:title "hi"}}))]
                                (str (get-in r [:got :title]) "|" (:ctype r)))`)
	if body != "hi|application/json" {
		t.Errorf("createNote echo = %q, want %q", body, "hi|application/json")
	}

	// :ok? is set on the response
	if got := eval(t, d, `(:ok? (oa/call c :getUser {:id 1}))`); got != true {
		t.Errorf(":ok? = %v, want true", got)
	}
}

func TestBriWebOpenAPIAuthFnAndErrors(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.web.openapi :as oa])`)
	srv := newOpenAPIServer(t)

	// :auth-fn is evaluated PER request (the ADR 0080 login seam) — a counter atom
	// proves it is called, and its value lands in the Authorization header
	eval(t, d, `(def calls (atom 0))`)
	eval(t, d, `(def c (oa/client "`+srv.URL+`/openapi.json"
                                  {:auth-fn (fn [] (swap! calls inc) (str "Bearer tok-" @calls))}))`)
	if got := evalString(t, d, `(:auth (oa/result (oa/call c :getUser {:id 1})))`); got != "Bearer tok-1" {
		t.Errorf("auth-fn header (1st call) = %q, want %q", got, "Bearer tok-1")
	}
	if got := evalString(t, d, `(:auth (oa/result (oa/call c :getUser {:id 2})))`); got != "Bearer tok-2" {
		t.Errorf("auth-fn header (2nd call) = %q, want it re-evaluated to Bearer tok-2", got)
	}

	// unknown operation throws, naming what is available
	if msg := evalErr(t, d, `(oa/call c :nope {})`); !strings.Contains(msg, "no operation") {
		t.Errorf("unknown op error = %q", msg)
	}
	// a missing path parameter throws, naming the parameter
	if msg := evalErr(t, d, `(oa/call c :getUser {})`); !strings.Contains(msg, "missing path parameter") {
		t.Errorf("missing path param error = %q", msg)
	}
}
