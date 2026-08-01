// repl_resolution_test.go — `cljgo repl` must resolve a project the same way
// `cljgo run` does. Issue #185.
//
// runREPL never called resolveRunDeps; the run path did. So the REPL booted
// with no project source roots and no dependency roots, and
// `(require 'myproj.core)` failed in a project whose own `cljgo run` and
// `cljgo build` resolved it fine — including in a freshly generated
// `cljgo new` project, which could not require its own namespace in its own
// REPL. That is a REPL-vs-run divergence, the class ADR 0007 calls
// unforgivable.
//
// Reported by the toolnexus Clojure port, which caught it while adding a
// cljgo-REPL leg to its gate. They also found why it had gone unnoticed: the
// REPL prints the error to stderr and CONTINUES to the next form, so a
// trailing `(println :OK)` still prints. Anyone grepping stdout for a marker
// gets a green REPL that resolved nothing — which is why this test asserts
// on the require's own outcome and reads stderr, never a trailing marker.
package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// replEval pipes src into `cljgo repl` in dir and returns combined output.
func replEval(t *testing.T, bin, dir, src string) string {
	t.Helper()
	cmd := exec.Command(bin, "repl")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(src)
	cmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	out, _ := cmd.CombinedOutput() // a failed require is reported, not exited on
	return string(out)
}

// TestREPLResolvesTheProjectsOwnNamespace is the reported case, on the
// layout `cljgo new` itself emits.
func TestREPLResolvesTheProjectsOwnNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	proj := t.TempDir()
	writeCljcProject(t, proj, map[string]string{
		"src/demo/core.cljc": "(ns demo.core)\n(defn add [a b] (+ a b))\n",
		"build.cljgo":        "(defn build [b])\n",
	})

	out := replEval(t, bin, proj, "(require 'demo.core)\n(demo.core/add 1 2)\n")

	// The specific failure, named so a regression is unmistakable.
	if strings.Contains(out, "could not locate namespace") {
		t.Fatalf("cljgo repl could not resolve the project's own namespace:\n%s", out)
	}
	// Assert on the CALL's result, never on a trailing marker: the REPL
	// continues past a failed require, so a marker proves nothing.
	if !strings.Contains(out, "3") {
		t.Fatalf("cljgo repl did not evaluate demo.core/add:\n%s", out)
	}
}

// TestREPLAndRunAgreeOnResolution pins the invariant rather than the symptom:
// whatever `cljgo run` can require from a directory, `cljgo repl` can too.
// A fix that made the REPL work by some other route would still have to keep
// these two agreeing.
func TestREPLAndRunAgreeOnResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	proj := t.TempDir()
	writeCljcProject(t, proj, map[string]string{
		"src/demo/core.cljc": "(ns demo.core)\n(def marker :resolved)\n",
		"build.cljgo":        "(defn build [b])\n",
		"probe.cljc":         "(require 'demo.core)\n(println demo.core/marker)\n",
	})

	runCmd := exec.Command(bin, "run", "probe.cljc")
	runCmd.Dir = proj
	runCmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	runOut, runErr := runCmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("cljgo run failed, so the invariant cannot be tested: %v\n%s", runErr, runOut)
	}
	if !strings.Contains(string(runOut), ":resolved") {
		t.Fatalf("cljgo run did not resolve demo.core:\n%s", runOut)
	}

	replOut := replEval(t, bin, proj, "(require 'demo.core)\n(println demo.core/marker)\n")
	if !strings.Contains(replOut, ":resolved") {
		t.Fatalf("cljgo run resolved demo.core but cljgo repl did not:\nrun:\n%s\nrepl:\n%s",
			runOut, replOut)
	}
}
