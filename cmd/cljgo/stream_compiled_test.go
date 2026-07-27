// stream_compiled_test.go — proves cljg.stream + cljg.process (ADR 0101) and
// cljg.net.http {:as :stream} LINK and behave in an AOT-compiled binary (the
// REPL↔binary parity release blocker). The interpreter behavior is covered in
// pkg/bri/stream_test.go; here a real `cljgo build` binary spawns a child
// (streaming a line through :in→:out) and streams an HTTP response body from a
// local httptest server, proving the -stream-*/-proc-* shims resolve compiled
// and the binary stays CGO_ENABLED=0 (bufio/io/os/exec/net/http are pure Go).
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

// streamApp: spawn `cat`, echo a line through :in→:out; then stream an HTTP
// body (:as :stream) line-by-line via reduce over the readable handle.
const streamApp = `(require '[cljg.process :as proc] '[cljg.stream :as st] '[cljg.net.http :as http])
(defn -main [& args]
  (let [p (proc/spawn ["cat"])]
    (st/write-line (:in p) "hello-compiled")
    (st/close (:in p))
    (println "spawn" (st/read-line (:out p)))
    (println "exit" ((:wait p))))
  (let [resp (http/request {:method :get :url (first args) :as :stream})
        v    (into [] (st/lines (:body resp)))]
    (st/close (:body resp))
    (println "status" (:status resp))
    (println "lines" (clojure.string/join "," v))))
`

func TestCljgStreamProcessCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the compiled spawn smoke test uses `cat` (Unix); interpreter tests cover behavior")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		for i := 1; i <= 3; i++ {
			fmt.Fprintf(w, "row-%d\n", i)
		}
	}))
	defer srv.Close()

	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "streamapp.clj")
	if err := os.WriteFile(src, []byte(streamApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "streamapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (cljg.stream/process app): %v\n%s", err, out)
	}

	out, err := exec.Command(app, srv.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled cljg.stream/process binary: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "spawn hello-compiled\nexit 0\nstatus 200\nlines row-1,row-2,row-3"
	if got != want {
		t.Fatalf("compiled cljg.stream/process output =\n%q\nwant\n%q", got, want)
	}
}
