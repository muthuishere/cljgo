package build_test

// Two defects a real consumer hit, both of which shipped a GREEN-LOOKING
// wrong answer. Neither had a test, which is why neither was caught here.
//
//  1. A test/ tree was invisible to `cljgo run` and `cljgo build`: a namespace
//     resolved only relative to the requiring file, so test/app/core_test.cljg
//     requiring app.core looked for test/app/core.cljg. The error named the
//     NAMESPACE and not the paths tried, so the cause was unguessable, and the
//     only workaround was moving the suite under src/ — which dual-host
//     projects cannot do, because they keep tests beside the code.
//
//  2. Two (exe …) artifacts in one build.cljgo corrupted the SECOND binary,
//     whichever it was. lang's namespace registry is process-global, so the
//     second build's fresh evaluator saw the first build's namespaces already
//     interned, skipped loading them, and emitted a program with those
//     namespaces MISSING — their vars interned as hollow shells and unbound at
//     runtime. The first binary was fine, so a project building an app plus a
//     test runner shipped a working app and a test suite that could not run.
//
// These run the real `cljgo` binary against real project trees, because both
// bugs live in the seam between the build driver, the load path and the
// emitter — a unit test on any one of the three would have missed both.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cljgoBin builds the CLI once per test binary and returns its path.
func cljgoBin(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cljgo")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "github.com/muthuishere/cljgo/cmd/cljgo")
	cmd.Dir = repoRootFor(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building cljgo: %v\n%s", err, out)
	}
	return bin
}

func repoRootFor(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repo root (no go.mod above cwd)")
		}
		dir = parent
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// runIn runs the cljgo binary in dir, returning combined output and exit code.
func runIn(t *testing.T, bin, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRootFor(t))
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running cljgo %v: %v", args, err)
	}
	return string(out), code
}

// TestTestTreeResolvesAgainstSrc — defect 1. A namespace under test/ must see
// the code under src/, with NO declaration, because that is how a Clojure
// project is laid out.
func TestTestTreeResolvesAgainstSrc(t *testing.T) {
	bin := cljgoBin(t)
	proj := t.TempDir()
	writeTree(t, proj, map[string]string{
		"build.cljgo": `(defn build [b]
  (install b (exe b {:name "runner" :main "test/app/core_test.cljg"})))
`,
		"src/app/core.cljg":       "(ns app.core)\n(defn add [a b] (+ a b))\n",
		"test/app/core_test.cljg": "(ns app.core-test (:require [app.core :as c]))\n(defn -main [& _] (println \"sum:\" (c/add 1 2)))\n",
	})

	// The interpreted leg: requiring across the two roots must resolve.
	if out, code := runIn(t, bin, proj, "run", "test/app/core_test.cljg"); code != 0 {
		t.Fatalf("cljgo run could not resolve app.core from test/: exit %d\n%s", code, out)
	}

	// The compiled leg, which is the one that shipped the failure.
	if out, code := runIn(t, bin, proj, "build"); code != 0 {
		t.Fatalf("cljgo build could not resolve app.core from test/: exit %d\n%s", code, out)
	}
	exe := filepath.Join(proj, "runner")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("running the built binary: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "sum: 3") {
		t.Errorf("built binary printed %q, want it to contain \"sum: 3\"", out)
	}
}

// TestPathsVerbOverridesTheDefaultRoots — the configurable half. A project
// that keeps its suite somewhere other than test/ says so, once.
func TestPathsVerbOverridesTheDefaultRoots(t *testing.T) {
	bin := cljgoBin(t)
	proj := t.TempDir()
	writeTree(t, proj, map[string]string{
		"build.cljgo": `(defn build [b]
  (paths b ["src" "spec"])
  (install b (exe b {:name "runner" :main "spec/app/core_spec.cljg"})))
`,
		"src/app/core.cljg":       "(ns app.core)\n(defn add [a b] (+ a b))\n",
		"spec/app/core_spec.cljg": "(ns app.core-spec (:require [app.core :as c]))\n(defn -main [& _] (println \"sum:\" (c/add 2 3)))\n",
	})
	if out, code := runIn(t, bin, proj, "build"); code != 0 {
		t.Fatalf("(paths …) did not make spec/ a source root: exit %d\n%s", code, out)
	}
	exe := filepath.Join(proj, "runner")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("running the built binary: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "sum: 5") {
		t.Errorf("built binary printed %q, want it to contain \"sum: 5\"", out)
	}
}

// TestTwoExeArtifactsBothWork — defect 2, in BOTH declaration orders, because
// the corruption hit whichever artifact came second. The shared namespace is
// the point: it is what the first build interns globally and the second build
// used to skip.
func TestTwoExeArtifactsBothWork(t *testing.T) {
	for _, order := range []struct {
		name  string
		build string
	}{
		{"app-then-test", `(defn build [b]
  (install b (exe b {:name "myapp"  :main "src/app/core.cljg"}))
  (install b (exe b {:name "mytest" :main "src/app/runner.cljg"})))
`},
		{"test-then-app", `(defn build [b]
  (install b (exe b {:name "mytest" :main "src/app/runner.cljg"}))
  (install b (exe b {:name "myapp"  :main "src/app/core.cljg"})))
`},
	} {
		t.Run(order.name, func(t *testing.T) {
			bin := cljgoBin(t)
			proj := t.TempDir()
			writeTree(t, proj, map[string]string{
				"build.cljgo":         order.build,
				"src/app/core.cljg":   "(ns app.core)\n(defn report [] \"APP-REPORT\")\n(defn -main [& _] (println \"app:\" (report)))\n",
				"src/app/runner.cljg": "(ns app.runner (:require [app.core :as c]))\n(defn -main [& _] (println \"runner sees:\" (c/report)))\n",
			})
			if out, code := runIn(t, bin, proj, "build"); code != 0 {
				t.Fatalf("build failed: exit %d\n%s", code, out)
			}
			// BOTH binaries must work. Before the fix the second one built
			// fine and then died on "cannot call unbound var".
			for _, want := range []struct{ exe, contains string }{
				{"myapp", "app: APP-REPORT"},
				{"mytest", "runner sees: APP-REPORT"},
			} {
				p := filepath.Join(proj, want.exe)
				if runtime.GOOS == "windows" {
					p += ".exe"
				}
				out, err := exec.Command(p).CombinedOutput()
				if err != nil {
					t.Errorf("%s failed to run: %v\n%s", want.exe, err, out)
					continue
				}
				if !strings.Contains(string(out), want.contains) {
					t.Errorf("%s printed %q, want it to contain %q", want.exe, out, want.contains)
				}
			}
		})
	}
}
