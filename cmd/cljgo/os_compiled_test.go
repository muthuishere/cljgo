// os_compiled_test.go — proves cljg.os (ADR 0088) LINKS and its cron shim works
// in an AOT-compiled binary. The scheduler LOOP isn't run here (real `run`
// sleeps until the next minute — covered interpreted with a fake clock in
// pkg/bri/os_test.go); this checks the compiled -cron-next path returns a
// sensible future fire, proving the shim resolves compiled and the binary is
// CGO_ENABLED=0 (stdlib time is pure Go).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

const osApp = `(require '[cljg.os :as os])
(defn -main [& _]
  (let [now (os/now)
        nxt (os/cron-next "*/5 * * * *" now)]
    (println "future" (> nxt now))
    (println "aligned" (zero? (mod (quot nxt 60000) 5)))))
`

func TestCljgOsCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "osapp.clj")
	if err := os.WriteFile(src, []byte(osApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "osapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (cljg.os app): %v\n%s", err, out)
	}
	out, err := exec.Command(app).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled cljg.os binary: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "future true\naligned true" // */5 fire lands on a 5-minute boundary
	if got != want {
		t.Fatalf("compiled cljg.os output =\n%q\nwant\n%q", got, want)
	}
}
