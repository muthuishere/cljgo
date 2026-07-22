package conformance

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

// TestClassifyParity unit-tests the ADR 0049 dec 4 three-outcome comparator
// itself — the reusable gate. Synthetic legs, no build, so the gate's
// classification is pinned deterministically and offline.
func TestClassifyParity(t *testing.T) {
	notLinked := errors.New("go module github.com/x/y is not linked into the interpreter (accessing member Z)")
	uncompiled := errors.New("namespace a.b was not compiled into this binary")
	plainErr := errors.New("divide by zero")

	cases := []struct {
		name        string
		interp, aot ParityLeg
		wantOK      bool
	}{
		// Accepted outcome 1: identical output.
		{"identical-output", ParityLeg{Output: "42\n"}, ParityLeg{Output: "42\n"}, true},
		// Accepted outcome 2: both legs refuse.
		{"identical-error", ParityLeg{Err: plainErr}, ParityLeg{Err: plainErr}, true},
		{"both-error-diff-msg", ParityLeg{Err: plainErr}, ParityLeg{Err: errors.New("boom")}, true},
		// Accepted outcome 3: interpreter capability-error, AOT success.
		{"capability-unlinked", ParityLeg{Err: notLinked}, ParityLeg{Output: "1000\n"}, true},
		{"capability-uncompiled", ParityLeg{Err: uncompiled}, ParityLeg{Output: "ok\n"}, true},
		// Forbidden: different non-error values (the silent nil-vs-value bug).
		{"silent-nil-vs-value", ParityLeg{Output: "nil\n"}, ParityLeg{Output: "1000\n"}, false},
		{"different-values", ParityLeg{Output: "false\n"}, ParityLeg{Output: "true\n"}, false},
		// Forbidden: interpreter errored for a NON-capability reason but AOT
		// succeeded — a real divergence, not an honest capability gap.
		{"noncapability-err-vs-success", ParityLeg{Err: plainErr}, ParityLeg{Output: "7\n"}, false},
		// Forbidden: AOT errored but interpreter silently succeeded.
		{"aot-err-interp-success", ParityLeg{Output: "7\n"}, ParityLeg{Err: plainErr}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, reason := ClassifyParity(c.interp, c.aot)
			if ok != c.wantOK {
				t.Fatalf("ClassifyParity ok=%v, want %v (reason: %s)", ok, c.wantOK, reason)
			}
		})
	}
}

// TestParityEntryFile is the S25 entry-*file* repro, run live through both
// legs and asserted with the parity comparator. Pre-fix (ADR 0049 dec 3) the
// binary bound *file* to NO_SOURCE_FILE while the interpreter bound the real
// path — a silent divergence. Now both bind the logical source path, so the
// property-based fixture yields identical output. No network.
func TestParityEntryFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compiled parity leg in -short mode")
	}
	path := filepath.Join("parity", "entry-file.clj")
	interp := parityEvalLeg(t, path)
	aot := parityCompiledLeg(t, path)
	if ok, reason := ClassifyParity(interp, aot); !ok {
		t.Fatalf("entry-*file* parity FAILED (%s):\n--- interp ---\n%q err=%v\n--- aot ---\n%q err=%v",
			reason, interp.Output, interp.Err, aot.Output, aot.Err)
	}
}

// TestParityUncompiledRequire seeds the uncompiled-require behavior into the
// parity gate (ADR 0049 dec 3). A top-level (require 'no.such.ns) for a
// namespace with no source file and no provider must never SILENTLY succeed
// in either leg: the interpreter errors locating it, and the AOT
// discovery/compile leg errors too (the build refuses). Both legs refuse —
// accepted outcome 2 — the forbidden outcome would be one leg silently
// no-op'ing while the other errors. (The runtime binary hard-error for a
// require deferred past discovery is TestBinaryUncompiledRequireHardErrors in
// pkg/emit.)
func TestParityUncompiledRequire(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compiled parity leg in -short mode")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "uncompiled-require.clj")
	if err := os.WriteFile(path, []byte("(require 'no.such.ns)\n:unreached\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	interp := parityEvalLeg(t, path)
	aot := parityCompiledLeg(t, path)
	if interp.Err == nil {
		t.Fatalf("interpreter leg must refuse an uncompiled require, got output %q", interp.Output)
	}
	if aot.Err == nil {
		t.Fatalf("AOT leg must refuse an uncompiled require, got output %q", aot.Output)
	}
	if ok, reason := ClassifyParity(interp, aot); !ok {
		t.Fatalf("uncompiled-require parity FAILED (%s):\n--- interp ---\n%v\n--- aot ---\n%v",
			reason, interp.Err, aot.Err)
	}
}

// parityEvalLeg runs a file through the interpreter, capturing printed
// output and the raised error (nil on success) — a ParityLeg. Unlike
// evalOutput (compiled_test.go) it does NOT fatal on error: an interpreter
// capability-error is an accepted parity outcome, not a test failure.
func parityEvalLeg(t *testing.T, path string) ParityLeg {
	t.Helper()
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	lang.RemoveNamespace(lang.NewSymbol("user"))
	var buf bytes.Buffer
	oldOut := corelib.Out
	corelib.Out = &buf
	defer func() { corelib.Out = oldOut }()

	d := repl.New(nil, io.Discard, io.Discard)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	last, evErr := d.EvalReader(f, path)
	if evErr != nil {
		return ParityLeg{Output: buf.String(), Err: evErr}
	}
	return ParityLeg{Output: buf.String() + lang.PrintString(last) + "\n"}
}

// parityCompiledLeg compiles a file, builds it, runs the binary, and
// captures stdout and any error — a ParityLeg. Compile-time side effects are
// discarded (Load() replays them in the binary), mirroring compiledOutput.
func parityCompiledLeg(t *testing.T, path string) ParityLeg {
	t.Helper()
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	prog, err := emit.CompileProgram(path)
	corelib.Out = oldOut
	if err != nil {
		return ParityLeg{Err: err}
	}
	dir := t.TempDir()
	if err := emit.WriteProgram(dir, prog, emit.Options{PrintLastValue: true}); err != nil {
		return ParityLeg{Err: err}
	}
	bin := filepath.Join(dir, "prog"+emit.ExeSuffix)
	if err := emit.GoBuild(dir, bin); err != nil {
		return ParityLeg{Err: err}
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		return ParityLeg{Output: string(out), Err: err}
	}
	return ParityLeg{Output: string(out)}
}
