// io_compiled_test.go — proves cljg.io (ADR 0089) LINKS and its filesystem
// shims work in an AOT-compiled binary (dual-harness parity with the
// interpreted suite in pkg/bri/io_test.go). The compiled app makes a temp dir,
// writes + copies a file, lists/globs, then deletes the tree — exercising the
// -fs-* / -path-* shims compiled, and proving CGO_ENABLED=0 (os + path/filepath
// are pure Go).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

const ioApp = `(require '[cljg.io :as io])
(defn -main [& _]
  (let [d (io/temp-dir "cljgio-compiled-")
        f (io/path d "hello.txt")]
    (spit f "hi")
    (io/copy! f (io/path d "copy.txt"))
    (println "size" (io/size f))
    (println "file?" (io/file? f))
    (println "count" (count (io/list-files d)))
    (println "glob" (count (io/glob (io/path d "*.txt"))))
    (println "ext" (io/extension f))
    (io/delete-tree! d)
    (println "gone" (not (io/exists? d)))
    ;; process exec (ADR 0089 §3): git --version is on every CI runner + dev box
    (let [r (io/sh "git" "--version")]
      (println "gitexit" (:exit r))
      (println "gitout" (> (count (:out r)) 0)))))
`

func TestCljgIOCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "ioapp.clj")
	if err := os.WriteFile(src, []byte(ioApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "ioapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (cljg.io app): %v\n%s", err, out)
	}
	out, err := exec.Command(app).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled cljg.io binary: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := strings.Join([]string{
		"size 2", "file? true", "count 2", "glob 2", "ext .txt", "gone true",
		"gitexit 0", "gitout true",
	}, "\n")
	if got != want {
		t.Fatalf("compiled cljg.io output =\n%q\nwant\n%q", got, want)
	}
}
