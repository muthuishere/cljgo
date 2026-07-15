package conformance

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

// exeSuffix is ".exe" on Windows, "" elsewhere. `go build -o <name>` writes
// exactly <name>, so without the suffix the harness produces a file Windows
// refuses to exec ("executable file not found in %PATH%") — the emitted
// program is fine, only the harness's naming is wrong.
var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// TestConformanceCompiled is the M2 half of the dual harness (ADR 0007,
// design/03 §7d): every tests/*.clj also compiles through pkg/emit and
// runs as a native binary, and the binary's stdout must be
// BYTE-IDENTICAL to the eval harness's output for the same file.
// Canonical output of a run = everything printed during evaluation +
// pr-str of the last top-level value + "\n".
//
// Waivers: `;; harness: eval — reason` skips the compiled run; files
// expecting an error are implicitly eval-only in v0 (an error fails
// `cljgo build` at compile/eval time — there is no compiled
// error-output contract yet) but still carry the marker for
// greppability. Divergence here is THE release blocker.
func TestConformanceCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compiled harness in -short mode")
	}
	files, err := filepath.Glob(filepath.Join("tests", "*.clj"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no conformance test files found under tests/")
	}
	compiled := 0
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".clj")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			exp, err := parseExpectation(path, string(src))
			if err != nil {
				t.Fatal(err)
			}
			d := parseDirectives(string(src))
			if d.evalOnly != "" {
				t.Skipf("eval-only: %s", d.evalOnly)
			}
			if exp.isError {
				t.Skipf("expect-error file without ;; harness: eval marker — add one with a reason")
			}

			evalOut := evalOutput(t, path)
			binOut := compiledOutput(t, path)
			if evalOut != binOut {
				t.Fatalf("REPL/binary divergence (release blocker, ADR 0002/0007):\n--- eval ---\n%q\n--- compiled ---\n%q", evalOut, binOut)
			}
			// The frozen expectation must hold in the binary too: its
			// last stdout line is pr-str of the last top-level value.
			lines := strings.Split(strings.TrimRight(binOut, "\n"), "\n")
			if got := lines[len(lines)-1]; got != exp.value {
				t.Fatalf("compiled last value pr-str = %q, want %q", got, exp.value)
			}
			compiled++
		})
	}
	t.Logf("dual-harness coverage: %d/%d files compiled and compared", compiled, len(files))
}

// evalOutput runs the file through the eval harness capturing printed
// side effects (eval.Out) and appending pr-str of the last value.
func evalOutput(t *testing.T, path string) string {
	t.Helper()
	lang.RemoveNamespace(lang.NewSymbol("user"))
	var buf bytes.Buffer
	oldOut := eval.Out
	eval.Out = &buf
	defer func() { eval.Out = oldOut }()

	d := repl.New(nil, io.Discard, io.Discard)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	last, err := d.EvalReader(f, path)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return buf.String() + lang.PrintString(last) + "\n"
}

// compiledOutput compiles the file (discarding compile-time side
// effects — Load() replays them in the binary), builds the generated
// module, runs the binary, and returns its stdout.
func compiledOutput(t *testing.T, path string) string {
	t.Helper()
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := eval.Out
	eval.Out = io.Discard
	forms, err := emit.CompileFile(path)
	eval.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dir := t.TempDir()
	if err := emit.WriteModule(dir, forms, emit.Options{PrintLastValue: true}); err != nil {
		t.Fatalf("write module: %v", err)
	}
	bin := filepath.Join(dir, "prog"+exeSuffix)
	if err := emit.GoBuild(dir, bin); err != nil {
		t.Fatalf("go build: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return string(out)
}
