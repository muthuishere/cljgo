// jobs_compiled_test.go — proves bri.core.jobs (ADR 0094) LINKS and works in an
// AOT-compiled binary. Behavior (worker pool, drain, error capture, the Queue
// interface) is covered interpreted in pkg/bri/jobs_test.go; here a compiled app
// runs a worker pool over core.async and drains, proving the whole namespace —
// protocol, reify impl, the core.async worker loop — links + boots, CGO_ENABLED=0.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

const jobsApp = `(require '[bri.core.jobs :as jobs])
(defn -main [& _]
  (let [done (atom 0)
        q    (jobs/local {:tick (fn [_] (swap! done inc))} {:workers 3})]
    (dotimes [_ 20] (jobs/submit q :tick {}))
    (jobs/drain q)
    (jobs/stop q)
    (println "jobs" @done)))
`

func TestBriCoreJobsCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "jobsapp.clj")
	if err := os.WriteFile(src, []byte(jobsApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "jobsapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (bri.core.jobs app): %v\n%s", err, out)
	}
	out, err := exec.Command(app).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled bri.core.jobs binary: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); !strings.Contains(got, "jobs 20") {
		t.Fatalf("compiled bri.core.jobs output =\n%q\nwant it to contain %q", got, "jobs 20")
	}
}
