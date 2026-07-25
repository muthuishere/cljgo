// openapi_compiled_test.go — proves bri.web.openapi (ADR 0090) LINKS and drives a
// real request in an AOT-compiled binary (dual-harness parity with the interpreted
// suite in pkg/bri/openapi_test.go). The test starts an httptest server that serves
// a small OpenAPI spec + one echo endpoint; the compiled app loads the spec by URL,
// calls the operation, and prints what came back — exercising the whole
// bri.web.openapi → cljg.net.http path compiled, and proving CGO_ENABLED=0.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

const openapiApp = `(require '[bri.web.openapi :as oa])
(defn -main [& args]
  (let [base (first args)
        c (oa/client (str base "/openapi.json") {:token "abc"})
        r (oa/result (oa/call c :getUser {:id 7 :verbose "on"}))]
    (println "id" (:id r))
    (println "auth" (:auth r))
    (println "ops" (count (oa/operations c)))))
`

func TestBriWebOpenAPICompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	// server: spec at /openapi.json (servers[0] = its own base) + echo endpoint
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"openapi":"3.0.0","servers":[{"url":%q}],
          "paths":{"/users/{id}":{"get":{"operationId":"getUser","parameters":[
            {"name":"id","in":"path"},{"name":"verbose","in":"query"}]}},
                   "/notes":{"post":{"operationId":"createNote"}}}}`, base)
	})
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":%q,"auth":%q}`, r.PathValue("id"), r.Header.Get("Authorization"))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "openapiapp.clj")
	if err := os.WriteFile(src, []byte(openapiApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "openapiapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (bri.web.openapi app): %v\n%s", err, out)
	}
	out, err := exec.Command(app, srv.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled bri.web.openapi binary: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "id 7\nauth Bearer abc\nops 2"
	if got != want {
		t.Fatalf("compiled bri.web.openapi output =\n%q\nwant\n%q", got, want)
	}
}
