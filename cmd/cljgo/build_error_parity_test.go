// build_error_parity_test.go — the build-phase diagnostic gate
// (docs/known-issues-2026-07-28.md §8). `cljgo build` used to print a bare
// err.Error() for a failure that came out of the user's Clojure, dropping the
// expected-vs-found arity, the source locus and the `help:` explain pointer
// that `cljgo run` showed for the SAME call in the SAME file. Error text must
// read the same in every context (CLAUDE.md), so this asserts the two are
// byte-identical, and that an infrastructure failure (a missing source file)
// still reads plainly in both.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// arityErrSrc is the canonical case from the doctrine: a named fn called with
// the wrong arity at a known source position. The JVM oracle (clojure 1.12.5)
// on this exact file reports:
//
//	Execution error (ArityException) at e2/eval145 (err2.clj:3).
//	Wrong number of args (3) passed to: e2/f
//
// cljgo keeps lowercase "wrong" (conformance/tests/arity-error.clj) and adds
// the expects/locus/help detail on top.
const arityErrSrc = "(ns e2)\n(defn f [x] x)\n(f 1 2 3)\n"

func TestBuildAndRunRenderTheSameDiagnostic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "err2.clj")
	if err := os.WriteFile(src, []byte(arityErrSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	runOut := stderrOf(t, exec.Command(bin, "run", src), dir)
	buildCmd := exec.Command(bin, "build", "-o", filepath.Join(dir, "out"), src)
	buildCmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	buildOut := stderrOf(t, buildCmd, dir)

	if runOut != buildOut {
		t.Fatalf("run↔build diagnostic divergence (release blocker):\n--- run ---\n%s\n--- build ---\n%s",
			runOut, buildOut)
	}
	want := "error: wrong number of args (3) passed to: e2/f (expects 1: [x]) at " + src + ":3:1\n" +
		"help: run `cljgo explain A2004`\n"
	if buildOut != want {
		t.Fatalf("build diagnostic =\n%q\nwant\n%q", buildOut, want)
	}
}

// A build failure that is NOT the user's Clojure keeps its plain text — the
// same way `cljgo run` prints its os.Open failure plainly, with no spurious
// "run `cljgo explain G5000`" pointer.
func TestBuildInfraErrorStaysPlain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.clj")

	got := stderrOf(t, exec.Command(bin, "build", missing), dir)
	if !strings.HasPrefix(got, "error: open "+missing+":") {
		t.Fatalf("missing-file build error = %q", got)
	}
	if strings.Contains(got, "cljgo explain") {
		t.Fatalf("infrastructure error grew an explain pointer: %q", got)
	}
}

// stderrOf runs cmd in dir and returns its stderr, requiring a non-zero exit
// (every case here is a failure).
func stderrOf(t *testing.T, cmd *exec.Cmd, dir string) string {
	t.Helper()
	cmd.Dir = dir
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	cmd.Stdout = nil
	if err := cmd.Run(); err == nil {
		t.Fatalf("%v: want a non-zero exit, got success (stderr %q)", cmd.Args, errBuf.String())
	}
	return errBuf.String()
}
