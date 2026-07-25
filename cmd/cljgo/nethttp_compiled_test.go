// nethttp_compiled_test.go — proves cljg.net.http (ADR 0087) LINKS and behaves
// in an AOT-compiled binary (the first cljg.* namespace). The interpreter
// behavior is covered in pkg/bri/net_http_test.go; here a real `cljgo build`
// binary makes a request to a local httptest server (passed as argv) and its
// output must match, proving the shims resolve compiled and the binary is
// CGO_ENABLED=0 (net/http is pure Go).
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

const netHTTPApp = `(require '[cljg.net.http :as http])
(defn -main [& args]
  (let [resp (http/get (first args) {:headers {"Authorization" "Bearer sk-9"}})]
    (println "status" (:status resp))
    (println "ok" (:ok? resp))
    (println "hello" (:hello (http/json-body resp)))))
`

func TestCljgNetHTTPCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"hello":"world","auth":%q}`, r.Header.Get("Authorization"))
	}))
	defer srv.Close()

	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "netapp.clj")
	if err := os.WriteFile(src, []byte(netHTTPApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "netapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (cljg.net.http app): %v\n%s", err, out)
	}

	out, err := exec.Command(app, srv.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled cljg.net.http binary: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "status 200\nok true\nhello world"
	if got != want {
		t.Fatalf("compiled cljg.net.http output =\n%q\nwant\n%q", got, want)
	}
}
