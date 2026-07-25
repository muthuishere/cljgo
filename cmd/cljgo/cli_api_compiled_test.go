// cli_api_compiled_test.go — proves bri.cli.api (ADR 0091) LINKS and its
// spec-driven surface works in an AOT-compiled binary. The auto-login flow itself
// goes through the OS keychain, which CI has no session for (it is covered
// interpreted with a stubbed keychain in pkg/bri/cli_api_test.go); here the
// compiled app loads the spec by URL and lists operations, proving the whole
// composition (bri.cli.api → bri.web.openapi → cljg.net.http → bri.cli →
// bri.cli.auth → bri.core.secrets) links + boots, and CGO_ENABLED=0 holds even
// with the keychain client linked in.
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

const cliApiApp = `(require '[bri.cli.api :as api])
(defn -main [& args]
  ;; building the client + listing ops is keychain-free (auth is lazy in :auth-fn),
  ;; so this runs on a CI box with no keychain session; the auth flow is covered
  ;; interpreted with a stubbed keychain. A :device client (ADR 0092) is built too,
  ;; proving the RFC-8628 device-flow code path LINKS + loads AOT (also keychain-free
  ;; until a call fires the poll loop).
  (let [a   (api/api (str (first args) "/openapi.json") {:service "svc" :auth :token})
        dev (api/api (str (first args) "/openapi.json")
                     {:service "svc" :auth :device
                      :device {:device-url (str (first args) "/device")
                               :token-url  (str (first args) "/token")
                               :client-id  "cli-abc"}})]
    (println "ops" (count (api/operations a)) "dev" (count (api/operations dev)))))
`

func TestBriCliApiCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"openapi":"3.0.0","servers":[{"url":%q}],
          "paths":{"/me":{"get":{"operationId":"getMe"}},
                   "/login":{"post":{"operationId":"login"}}}}`, base)
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "cliapiapp.clj")
	if err := os.WriteFile(src, []byte(cliApiApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "cliapiapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (bri.cli.api app): %v\n%s", err, out)
	}
	out, err := exec.Command(app, srv.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled bri.cli.api binary: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); !strings.Contains(got, "ops 2 dev 2") {
		t.Fatalf("compiled bri.cli.api output =\n%q\nwant it to contain %q", got, "ops 2 dev 2")
	}
}
