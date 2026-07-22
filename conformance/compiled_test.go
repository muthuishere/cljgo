package conformance

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

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
//
// The suite is split into two phases so the ~410 `go build` + run
// subprocesses — which share NO Go state and dominate wall-clock — can
// run in parallel without weakening the divergence gate:
//
//   - Phase A (serial, in-process): drives the ONE global cljgo
//     interpreter — computes each file's eval output and emits+writes
//     its Go module. It MUST stay serial: it mutates the process-global
//     namespace registry and corelib.Out, bracketed by the same
//     namespaceSnapshot / removeNewNamespaces restore as before.
//   - Phase B (parallel, subprocess-only): `go build` + run the binary,
//     then the SAME two assertions verbatim — eval == binary (the
//     divergence check) and the binary's last line == the frozen
//     expectation. No shared Go state, so subtests run with
//     t.Parallel() (capped at -parallel = GOMAXPROCS). Each worker
//     deletes its module dir right after the run to cap peak disk.
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

	// One base temp dir for every emitted module (Windows-safe, cleaned
	// at test end); Phase B removes each subdir after its run so peak
	// on-disk binaries stay bounded by the parallelism, not by len(files).
	base := t.TempDir()

	// --- Phase A: serial, touches global interpreter/emitter state. ---
	preps := make([]prepared, 0, len(files))
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".clj")
		p := prepared{name: name}
		src, err := os.ReadFile(path)
		if err != nil {
			p.err = err
			preps = append(preps, p)
			continue
		}
		exp, err := parseExpectation(path, string(src))
		if err != nil {
			p.err = err
			preps = append(preps, p)
			continue
		}
		d := parseDirectives(string(src))
		if d.evalOnly != "" {
			p.skip = fmt.Sprintf("eval-only: %s", d.evalOnly)
			preps = append(preps, p)
			continue
		}
		if exp.isError {
			p.skip = "expect-error file without ;; harness: eval marker — add one with a reason"
			preps = append(preps, p)
			continue
		}
		p.exp = exp
		p.dir = filepath.Join(base, name)
		if err := os.Mkdir(p.dir, 0o755); err != nil {
			p.err = err
			preps = append(preps, p)
			continue
		}
		if p.evalOut, err = evalOutput(path); err != nil {
			p.err = fmt.Errorf("eval: %w", err)
			preps = append(preps, p)
			continue
		}
		if err := emitModule(path, p.dir); err != nil {
			p.err = err
			preps = append(preps, p)
			continue
		}
		preps = append(preps, p)
	}

	// --- Phase B: parallel, subprocess-only, no shared Go state. ---
	var compiled int64
	// t.Cleanup on the parent runs after ALL subtests finish (including
	// the parallel ones), so the coverage count is complete here.
	t.Cleanup(func() {
		t.Logf("dual-harness coverage: %d/%d files compiled and compared", atomic.LoadInt64(&compiled), len(files))
	})
	for i := range preps {
		p := preps[i]
		t.Run(p.name, func(t *testing.T) {
			if p.skip != "" {
				t.Skip(p.skip)
			}
			if p.err != nil {
				t.Fatal(p.err)
			}
			t.Parallel()
			binOut, err := runBinary(p.dir)
			// Cap peak disk: drop the module + binary as soon as it has run.
			os.RemoveAll(p.dir)
			if err != nil {
				t.Fatal(err)
			}
			if p.evalOut != binOut {
				t.Fatalf("REPL/binary divergence (release blocker, ADR 0002/0007):\n--- eval ---\n%q\n--- compiled ---\n%q", p.evalOut, binOut)
			}
			// The frozen expectation must hold in the binary too: its
			// last stdout line is pr-str of the last top-level value.
			lines := strings.Split(strings.TrimRight(binOut, "\n"), "\n")
			if got := lines[len(lines)-1]; got != p.exp.value {
				t.Fatalf("compiled last value pr-str = %q, want %q", got, p.exp.value)
			}
			atomic.AddInt64(&compiled, 1)
		})
	}
}

// prepared is one file's Phase-A result carried into Phase B. Exactly
// one of {skip, err} may be set; otherwise the file is ready to build.
type prepared struct {
	name    string
	skip    string      // non-empty => t.Skip with this exact message
	err     error       // non-nil => t.Fatal with this error (prep failure)
	evalOut string      // eval-harness output to compare against the binary
	exp     expectation // frozen expectation (last-value pr-str)
	dir     string      // emitted module directory under base
}

// evalOutput runs the file through the eval harness capturing printed
// side effects (corelib.Out) and appending pr-str of the last value.
// Serial-only: it mutates the process-global namespace registry and
// corelib.Out, bracketed by namespaceSnapshot / removeNewNamespaces.
func evalOutput(path string) (string, error) {
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
		return "", err
	}
	defer f.Close()
	last, err := d.EvalReader(f, path)
	if err != nil {
		return "", err
	}
	return buf.String() + lang.PrintString(last) + "\n", nil
}

// emitModule compiles the file (discarding compile-time side effects —
// Load() replays them in the binary) and writes the generated module to
// dir. Serial-only: CompileProgram drives the global interpreter.
func emitModule(path, dir string) error {
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	prog, err := emit.CompileProgram(path)
	corelib.Out = oldOut
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}
	if err := emit.WriteProgram(dir, prog, emit.Options{PrintLastValue: true}); err != nil {
		return fmt.Errorf("write module: %w", err)
	}
	return nil
}

// runBinary builds the pre-written module in dir and runs the binary,
// returning its stdout. Subprocess-only — no shared Go state — so it is
// safe to call from parallel subtests.
func runBinary(dir string) (string, error) {
	bin := filepath.Join(dir, "prog"+emit.ExeSuffix)
	if err := emit.GoBuild(dir, bin); err != nil {
		return "", fmt.Errorf("go build: %w", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		return "", fmt.Errorf("run: %w", err)
	}
	return string(out), nil
}
