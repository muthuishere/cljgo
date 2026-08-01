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
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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

// TestNREPLResolvesTheProjectsOwnNamespace — the SECOND instance of #185,
// found by auditing the other entry points rather than by a report.
//
// `cljgo nrepl` had the identical gap: it never called resolveRunDeps, so an
// editor connected to it saw "could not locate namespace" for the very
// project it was editing. This one matters more than the terminal REPL,
// because nREPL IS the editor path — the failure is invisible in a shell and
// lands directly on anyone using an IDE.
//
// That two of eight evaluator bootstrap sites were wrong the same way is the
// finding, not the fix: resolution lives at each call site, so every new
// entry point can forget it independently.
func TestNREPLResolvesTheProjectsOwnNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	proj := t.TempDir()
	writeCljcProject(t, proj, map[string]string{
		"src/demo/core.cljc": "(ns demo.core)\n(def marker :resolved)\n",
		"build.cljgo":        "(defn build [b])\n",
	})

	// Port 0 → the server picks one and writes it to .nrepl-port in its cwd.
	cmd := exec.Command(bin, "nrepl", "--port", "0")
	cmd.Dir = proj
	cmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting nrepl: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })

	port := waitForPortFile(t, proj)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%s", port), 5*time.Second)
	if err != nil {
		t.Fatalf("dialing nrepl: %v", err)
	}
	defer conn.Close()

	code := "(do (require 'demo.core) demo.core/marker)"
	msg := fmt.Sprintf("d2:op4:eval4:code%d:%s2:id1:1e", len(code), code)
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("writing eval: %v", err)
	}
	// Read until the "done" status: nREPL replies in several bencode frames
	// and the value can arrive after the first. A single Read caught only
	// `d2:id1:1` and reported a false failure.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var sb strings.Builder
	buf := make([]byte, 65536)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil || strings.Contains(sb.String(), "4:done") {
			break
		}
	}
	got := sb.String()

	if strings.Contains(got, "could not locate namespace") {
		t.Fatalf("nrepl could not resolve the project's own namespace:\n%s", got)
	}
	if !strings.Contains(got, ":resolved") {
		t.Fatalf("nrepl did not evaluate demo.core/marker:\n%s", got)
	}
}

// waitForPortFile polls for the .nrepl-port file the server writes on boot.
func waitForPortFile(t *testing.T, dir string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(dir + "/.nrepl-port"); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b))
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("nrepl never wrote .nrepl-port")
	return ""
}

// TestREPLResolvesADepsEdnOnlyProject — ADR 0119. A dual-host library
// declares its source roots in Clojure's deps.edn and has no reason to carry
// a cljgo build file too: it publishes to Clojars with tools.build and is
// consumed as a library. Before this, cljgo had NO declared roots for such a
// project.
//
// The failure was invisible in CI and obvious to a human: `cljgo run
// src/foo.cljc` resolves relative to the file being run, so a whole
// conformance suite passes, while opening a REPL at the project root — where
// there is no requiring file to be relative to — resolves nothing. Found
// against koine, whose 13 checks (271 assertions) pass on the release while
// its own REPL could not require koine.json from the project root.
func TestREPLResolvesADepsEdnOnlyProject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	proj := t.TempDir()
	writeCljcProject(t, proj, map[string]string{
		// deps.edn ONLY — deliberately no build.cljgo, which is koine's shape.
		"deps.edn":           "{:paths [\"src\"]\n :deps {org.clojure/clojure {:mvn/version \"1.12.5\"}}}\n",
		"src/demo/core.cljc": "(ns demo.core)\n(def marker :resolved)\n",
	})

	out := replEval(t, bin, proj, "(require 'demo.core)\n(println demo.core/marker)\n")
	if strings.Contains(out, "could not locate namespace") {
		t.Fatalf("deps.edn :paths was not honoured:\n%s", out)
	}
	if !strings.Contains(out, ":resolved") {
		t.Fatalf("REPL did not resolve through deps.edn :paths:\n%s", out)
	}
}

// TestBuildCljgoWinsOverDepsEdn pins the precedence: a project carrying BOTH
// gets its cljgo build file, which is cljgo's own and more specific
// declaration. deps.edn is the fallback, never an override — otherwise a
// project could not opt out of a stale or JVM-only :paths.
func TestBuildCljgoWinsOverDepsEdn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	proj := t.TempDir()
	writeCljcProject(t, proj, map[string]string{
		// deps.edn points ONLY at a decoy that does not hold the namespace.
		"deps.edn":           "{:paths [\"jvm-only\"]}\n",
		"jvm-only/keep.txt":  "not a source root cljgo should prefer\n",
		"build.cljgo":        "(defn build [b])\n",
		"src/demo/core.cljc": "(ns demo.core)\n(def marker :from-build-cljgo)\n",
	})

	out := replEval(t, bin, proj, "(require 'demo.core)\n(println demo.core/marker)\n")
	if !strings.Contains(out, ":from-build-cljgo") {
		t.Fatalf("build.cljgo must win over deps.edn:\n%s", out)
	}
}
