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

	"github.com/muthuishere/cljgo/pkg/build"
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

// TestJVMBuildCljDoesNotBreakTheProject is the regression test for issue #176,
// reported by koine with a clean-clone bisect: v0.8.2 works, v0.8.3 and v0.8.4
// fail, and renaming build.clj away is the whole delta.
//
// A dual-host library has a root `build.clj` — that is the tools.build
// convention and the only way it publishes to Clojars. cljgo accepted that
// name as one of ITS build files (ADR 0055), which was harmless while nothing
// read the build file on the `run` path. v0.8.3 started reading it (for deps
// and source roots), so cljgo began EVALUATING the JVM tool's build script;
// its `(:require [clojure.tools.build.api])` is unresolvable here, and every
// `cljgo run` in the project died with "could not locate namespace
// clojure.tools.build.api" — a total block on exactly the dual-host projects
// cljgo exists to serve.
//
// Note koine's proposed mechanism (v0.8.3's source-roots work sweeping in the
// project root) was NOT the cause: DefaultSourceRoots is ["src","test"] and
// the root is never added. The cause is the build-file NAME collision, which
// is why this test pins FindBuildFile rather than the source roots.
func TestJVMBuildCljDoesNotBreakTheProject(t *testing.T) {
	bin := cljgoBin(t)
	proj := t.TempDir()
	writeTree(t, proj, map[string]string{
		// The tools.build file, verbatim in shape: a ns requiring a namespace
		// that exists only on the JVM.
		"build.clj":         "(ns build\n  (:require [clojure.tools.build.api :as b]))\n(defn jar [_] nil)\n",
		"src/demo/app.cljc": "(ns demo.app)\n(println \"ok\")\n",
	})

	out, code := runIn(t, bin, proj, "run", "src/demo/app.cljc")
	// Name the specific failure first, so a regression is unmistakable rather
	// than just a non-zero exit.
	if strings.Contains(out, "clojure.tools.build.api") {
		t.Fatalf("cljgo evaluated the JVM build.clj as its own build file:\n%s", out)
	}
	if code != 0 {
		t.Fatalf("cljgo run alongside a JVM build.clj: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("cljgo run output = %q, want it to contain \"ok\"", out)
	}
}

// TestMissingBuildFnNamesTheBuildFile — a build file with no `build` entry
// point must name the user's file and the missing form, not cljgo's
// internals. Before G5025 it surfaced from inside the driver string:
//
//	error: evaluating build fn: compiler error at <build-driver>:1:38:
//	unable to resolve symbol: build in this context
//
// `<build-driver>` is a string cljgo synthesizes, so that named a file the
// user never wrote and gave a column INTO that nonexistent file, while never
// mentioning their build.cljgo or the missing (defn build [b] …).
//
// Found by koine while black-box bisecting #176: stripping the require from a
// root build.clj left this behind, which is how it surfaced as a defect in
// its own right rather than an artifact of that bug.
func TestMissingBuildFnNamesTheBuildFile(t *testing.T) {
	bin := cljgoBin(t)
	proj := t.TempDir()
	writeTree(t, proj, map[string]string{
		// A plausible near-miss: the fn exists but is not called `build`.
		build.BuildFileName: "(defn jar [_] nil)\n",
		"src/demo/app.cljc": "(ns demo.app)\n(println \"ok\")\n",
	})

	out, code := runIn(t, bin, proj, "build")
	if code == 0 {
		t.Fatalf("cljgo build with no build fn succeeded:\n%s", out)
	}
	// The internals must not leak.
	if strings.Contains(out, "<build-driver>") {
		t.Fatalf("error names cljgo's synthesized driver:\n%s", out)
	}
	if strings.Contains(out, "unable to resolve symbol") {
		t.Fatalf("error describes cljgo's internals:\n%s", out)
	}
	// It must name the user's file, the missing form, and carry the code.
	for _, want := range []string{build.BuildFileName, "defn build", "G5025"} {
		if !strings.Contains(out, want) {
			t.Fatalf("error does not mention %q:\n%s", want, out)
		}
	}
}
