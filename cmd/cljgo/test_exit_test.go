// test_exit_test.go — the "CI must not go green on red" gate (ADR 0105
// tasks 2.1/2.2/2.3). A compiled test binary used to exit 0 with failing
// tests: the hand-written -main called run-tests, threw the summary away,
// and the process reported success. These tests pin the properties that
// close that hole in the only way that cannot drift — by building real
// binaries and reading their real exit codes.
package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

// failingSuite is the smallest program that reproduces the QA-sweep bug: a
// deftest that fails, plus the -main a user writes by hand.
const failingSuite = `(ns suite
  (:require [clojure.test :refer [deftest is run-tests]]))

(deftest deliberate
  (is (= 1 2)))

(defn -main [& _] (run-tests 'suite))
`

const passingSuite = `(ns suite
  (:require [clojure.test :refer [deftest is run-tests]]))

(deftest fine
  (is (= 1 1)))

(defn -main [& _] (run-tests 'suite))
`

// exitCode runs cmd and returns its exit code plus combined output.
func exitCode(t *testing.T, cmd *exec.Cmd) (int, string) {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("run %v: %v\n%s", cmd.Args, err, out)
	}
	return ee.ExitCode(), string(out)
}

// TestCompiledSuiteExitCode is the headline regression: a binary whose -main
// runs a FAILING suite must fail the process, and one whose suite passes must
// not (so the check cannot be satisfied by always exiting 1).
func TestCompiledSuiteExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	cljgo := buildCljgo(t)
	for _, c := range []struct {
		name string
		src  string
		want int
	}{
		{"failing", failingSuite, 1},
		{"passing", passingSuite, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "suite.cljg")
			if err := os.WriteFile(src, []byte(c.src), 0o644); err != nil {
				t.Fatal(err)
			}
			app := filepath.Join(dir, "suite"+emit.ExeSuffix)
			build := exec.Command(cljgo, "build", "-o", app, src)
			build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("cljgo build: %v\n%s", err, out)
			}
			if code, out := exitCode(t, exec.Command(app)); code != c.want {
				t.Fatalf("compiled suite exit = %d, want %d\n%s", code, c.want, out)
			}

			// The interpreted leg must agree. `cljgo run` does not call -main,
			// so drive the suite from a top-level form — the same assertions,
			// the same exit code (parity is about status, not just output).
			runSrc := filepath.Join(dir, "run.cljg")
			if err := os.WriteFile(runSrc, []byte(c.src+"\n(run-tests 'suite)\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			run := exec.Command(cljgo, "run", runSrc)
			run.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
			if code, out := exitCode(t, run); code != c.want {
				t.Fatalf("cljgo run of the same suite exit = %d, want %d (interpreted/compiled divergence)\n%s",
					code, c.want, out)
			}
		})
	}
}

// TestCljgoTestCompiledFlag covers `cljgo test --compiled` and `--both` on a
// generated project (ADR 0105 task 2.2), and the failure-report shape (task
// 2.3): a named test and a file:line, never the `#=(var ...)` reader form.
func TestCljgoTestCompiledFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	cljgo := buildCljgo(t)
	dir := t.TempDir()
	newCmd := exec.Command(cljgo, "new", "--template", "cli", "app")
	newCmd.Dir = dir
	if out, err := newCmd.CombinedOutput(); err != nil {
		t.Fatalf("cljgo new: %v\n%s", err, out)
	}
	proj := filepath.Join(dir, "app")
	env := append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")

	inProj := func(args ...string) *exec.Cmd {
		c := exec.Command(cljgo, args...)
		c.Dir = proj
		c.Env = env
		return c
	}

	// Green: every leg passes, and the two legs agree byte for byte.
	if code, out := exitCode(t, inProj("test", "--compiled")); code != 0 {
		t.Fatalf("cljgo test --compiled on a green project exited %d\n%s", code, out)
	}
	code, out := exitCode(t, inProj("test", "--both"))
	if code != 0 {
		t.Fatalf("cljgo test --both on a green project exited %d\n%s", code, out)
	}
	if !strings.Contains(out, "interpreted and compiled agree") {
		t.Fatalf("cljgo test --both did not report agreement\n%s", out)
	}

	// Red: append a deliberately failing test; every leg must fail.
	testFile := filepath.Join(proj, "test", "app", "core_test.cljg")
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n(deftest failing-on-purpose\n  (is (= 1 2)))\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	for _, args := range [][]string{{"test"}, {"test", "--compiled"}, {"test", "--both"}} {
		label := strings.Join(args, " ")
		code, out := exitCode(t, inProj(args...))
		if code == 0 {
			t.Fatalf("cljgo %s on a RED project exited 0 — CI would go green on red\n%s", label, out)
		}
		if strings.Contains(out, "#=(var") {
			t.Errorf("cljgo %s: failure report leaks the #=(var ...) reader form\n%s", label, out)
		}
		if !strings.Contains(out, "(failing-on-purpose)") {
			t.Errorf("cljgo %s: failure report does not name the test\n%s", label, out)
		}
		if !strings.Contains(out, "core_test.cljg:") {
			t.Errorf("cljgo %s: failure report carries no file:line\n%s", label, out)
		}
	}
}
