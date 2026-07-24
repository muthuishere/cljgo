package conformance

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
// WHY THE BATCHING (perf/parallel-compiled-conformance). Profiling showed
// the ~237 s wall-clock was NOT `go build`/link bound (building all 422
// modules in parallel is ~11 s) and NOT run bound (re-running all 422
// binaries warm is <1 s). It was dominated by macOS's *first-exec*
// penalty: the kernel/amfid registers every never-before-seen executable
// inode's code signature on its first exec — a ~0.4–0.7 s, effectively
// serial, size-independent cost paid once per DISTINCT binary file (422
// of them ⇒ ~160 s, and parallelism does not help because the check
// serializes). The structural fix is to run fewer distinct binaries:
//
//   - Single-file programs (the vast majority; e.g. 418/422) are emitted
//     as ordinary `cljgo build` output, then that output is mechanically
//     wrapped into a per-program Go PACKAGE (package clause + main→Run
//     rename only — the compiled Clojure body is byte-identical to what
//     ships) and G such packages are linked into ONE group binary with a
//     dispatcher. Each program still runs in its OWN fresh process
//     (selected by the CLJGO_BATCH_PROG env var, argv left pristine so
//     *command-line-args* matches standalone), preserving full per-program
//     isolation — the global namespace registry is never shared across
//     programs. G distinct binaries ⇒ G first-exec penalties instead of
//     hundreds. Nothing about WHAT is proven changes: every file is still
//     compiled and BOTH assertions (eval==binary divergence check, and
//     binary last line == frozen expectation) run per file.
//   - Multi-dep / bri programs keep the original one-module-per-file path
//     (few in number; their extra first-execs are negligible).
//
// The suite is split into two phases:
//   - Phase A (serial, in-process): drives the ONE global cljgo
//     interpreter — computes each file's eval output and emits its Go
//     source. It MUST stay serial: it mutates the process-global namespace
//     registry and corelib.Out, bracketed by namespaceSnapshot /
//     removeNewNamespaces. It then writes the group modules.
//   - Phase B (parallel, subprocess-only): builds the group + standalone
//     binaries in parallel, then runs each program and applies the SAME
//     two assertions verbatim. No shared Go state ⇒ t.Parallel().
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

	base := t.TempDir()

	// Group count: enough distinct binaries to keep the single `go build`
	// links parallel across workers, few enough that first-exec penalties
	// stay small. Tunable via CLJGO_CONF_GROUPS (0/unset ⇒ default).
	groups := defaultGroups()
	if v := os.Getenv("CLJGO_CONF_GROUPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			groups = n
		}
	}

	// --- Phase A: serial, touches global interpreter/emitter state. ---
	preps := make([]prepared, 0, len(files))
	batches := make([]*batchModule, groups)
	next := 0 // round-robin cursor over groups
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
		if p.evalOut, err = evalOutput(path); err != nil {
			p.err = fmt.Errorf("eval: %w", err)
			preps = append(preps, p)
			continue
		}
		// Try the batch path (single-file, no deps, no bri, not marked
		// `;; harness: standalone`). Any file that does not fit falls back
		// to a standalone one-module build — never silently dropped.
		pkg, ok, err := compileBatchPackage(path)
		if err != nil {
			p.err = err
			preps = append(preps, p)
			continue
		}
		if d.noBatch != "" {
			ok = false // registry-introspection: must run as its own binary
		}
		if ok {
			g := next % groups
			next++
			if batches[g] == nil {
				batches[g] = &batchModule{dir: filepath.Join(base, "batch"+strconv.Itoa(g))}
			}
			p.batch = batches[g]
			p.batchKey = fmt.Sprintf("p%d", len(batches[g].progs))
			batches[g].progs = append(batches[g].progs, batchProg{key: p.batchKey, name: name, src: pkg})
			preps = append(preps, p)
			continue
		}
		// Standalone fallback (multi-dep / bri).
		p.dir = filepath.Join(base, name)
		if err := os.Mkdir(p.dir, 0o755); err != nil {
			p.err = err
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

	// Resolve the runtime dir once for every generated module's go.mod.
	runtimeDir, err := emit.FindRuntimeDir()
	if err != nil {
		t.Fatal(err)
	}
	// Materialize each group module on disk (packages + dispatcher + go.mod).
	for _, b := range batches {
		if b == nil || len(b.progs) == 0 {
			continue
		}
		if err := b.write(runtimeDir); err != nil {
			t.Fatalf("writing batch module %s: %v", b.dir, err)
		}
	}

	// --- Phase B: build binaries in parallel, then run + assert. ---
	// Build every group binary and every standalone module up front, in
	// parallel; the group binary is shared by many subtests so it cannot be
	// built lazily-then-removed per subtest.
	buildErrs := &sync.Map{}
	{
		type buildJob struct {
			dir, bin string
			mark     func(error)
		}
		var jobs []buildJob
		for _, b := range batches {
			if b == nil || len(b.progs) == 0 {
				continue
			}
			b := b
			b.bin = filepath.Join(b.dir, "batch"+emit.ExeSuffix)
			jobs = append(jobs, buildJob{b.dir, b.bin, func(e error) { b.buildErr = e }})
		}
		for i := range preps {
			p := &preps[i]
			if p.batch != nil || p.skip != "" || p.err != nil || p.dir == "" {
				continue
			}
			bin := filepath.Join(p.dir, "prog"+emit.ExeSuffix)
			key := p.name
			jobs = append(jobs, buildJob{p.dir, bin, func(e error) {
				if e != nil {
					buildErrs.Store(key, e)
				}
			}})
		}
		sem := make(chan struct{}, runtime.GOMAXPROCS(0))
		var wg sync.WaitGroup
		for _, j := range jobs {
			wg.Add(1)
			go func(j buildJob) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				j.mark(emit.GoBuild(j.dir, j.bin))
			}(j)
		}
		wg.Wait()
	}

	var compiled int64
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
			var binOut string
			if p.batch != nil {
				if p.batch.buildErr != nil {
					t.Fatalf("go build (batch): %v", p.batch.buildErr)
				}
				out, err := runBatch(p.batch.bin, p.batchKey)
				if err != nil {
					t.Fatal(err)
				}
				binOut = out
			} else {
				if e, ok := buildErrs.Load(p.name); ok {
					t.Fatalf("go build: %v", e.(error))
				}
				out, err := runStandalone(p.dir)
				if err != nil {
					t.Fatal(err)
				}
				binOut = out
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

// defaultGroups picks how many group binaries to link. One per build worker
// keeps the single-threaded-tail `go build` links running concurrently while
// holding the distinct-binary count (⇒ first-exec penalties) low.
func defaultGroups() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		n = 1
	}
	return n
}

// prepared is one file's Phase-A result carried into Phase B. Exactly one of
// {skip, err} may be set; otherwise the file is ready to build+run either via
// its group binary (batch != nil) or a standalone module (dir != "").
type prepared struct {
	name     string
	skip     string       // non-empty => t.Skip with this exact message
	err      error        // non-nil => t.Fatal with this error (prep failure)
	evalOut  string       // eval-harness output to compare against the binary
	exp      expectation  // frozen expectation (last-value pr-str)
	dir      string       // standalone module directory (multi-dep / bri)
	batch    *batchModule // group binary this program is linked into
	batchKey string       // CLJGO_BATCH_PROG selector within the group
}

// batchProg is one single-file program wrapped as a Go package inside a group
// module: key is its package name / dispatch selector, src its package source.
type batchProg struct {
	key  string // "p<i>" — package name and CLJGO_BATCH_PROG value
	name string // conformance file base name (for readability)
	src  string // rewritten Go source (package p<i>, func Run())
}

// batchModule is one group: many program packages + a dispatcher main linked
// into a single binary, so all its programs share ONE first-exec penalty.
type batchModule struct {
	dir      string
	bin      string
	progs    []batchProg
	buildErr error
}

// write materializes the group module: prog<i>/prog<i>.go for each program, a
// dispatcher main.go that runs the CLJGO_BATCH_PROG-selected program, and a
// go.mod (replace => runtimeDir, external requires, go.sum) via SynthGoMod.
func (b *batchModule) write(runtimeDir string) error {
	const moduleName = "cljgo.gen/batch"
	for _, p := range b.progs {
		pdir := filepath.Join(b.dir, p.key)
		if err := os.MkdirAll(pdir, 0o755); err != nil {
			return err
		}
		src := strings.Replace(p.src, pkgPlaceholder, p.key, 1)
		if err := os.WriteFile(filepath.Join(pdir, p.key+".go"), []byte(src), 0o644); err != nil {
			return err
		}
	}
	var d strings.Builder
	d.WriteString("// Code generated by the cljgo conformance harness. DO NOT EDIT.\n\n")
	d.WriteString("package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n")
	for _, p := range b.progs {
		fmt.Fprintf(&d, "\t%s %q\n", p.key, moduleName+"/"+p.key)
	}
	d.WriteString(")\n\nfunc main() {\n\tswitch os.Getenv(\"CLJGO_BATCH_PROG\") {\n")
	for _, p := range b.progs {
		fmt.Fprintf(&d, "\tcase %q:\n\t\t%s.Run()\n", p.key, p.key)
	}
	d.WriteString("\tdefault:\n\t\tfmt.Fprintln(os.Stderr, \"unknown CLJGO_BATCH_PROG\")\n\t\tos.Exit(2)\n\t}\n}\n")
	if err := os.WriteFile(filepath.Join(b.dir, "main.go"), []byte(d.String()), 0o644); err != nil {
		return err
	}
	return emit.SynthGoMod(b.dir, moduleName, runtimeDir, nil)
}

// compileBatchPackage emits path as ordinary `cljgo build` output and, if it
// is a single-file program (no file-backed deps, no bri), mechanically wraps
// that output into a Go package source: only the `package main` clause and the
// `func main()` signature are rewritten (→ package p / func Run()); the
// compiled Clojure body is byte-identical to what ships. ok=false ⇒ the
// program is multi-dep/bri and the caller must use the standalone path.
func compileBatchPackage(path string) (src string, ok bool, err error) {
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	prog, cerr := emit.CompileProgram(path)
	corelib.Out = oldOut
	if cerr != nil {
		return "", false, fmt.Errorf("compile: %w", cerr)
	}
	if prog.UsesBri || len(prog.Deps) > 0 {
		return "", false, nil // standalone path
	}
	dir, err := os.MkdirTemp("", "cljgo-batchgen-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(dir)
	if err := emit.WriteProgram(dir, prog, emit.Options{PrintLastValue: true}); err != nil {
		return "", false, fmt.Errorf("write module: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		return "", false, err
	}
	return raw2package(string(raw)), true, nil
}

// pkgPlaceholder marks the package clause in wrapped batch sources; write()
// substitutes each program's real package name (its selector key).
const pkgPlaceholder = "__CLJGO_BATCH_PKG__"

// raw2package turns emitted `package main` + `func main()` output into a named
// package (placeholder, resolved at write time) with an exported Run(). It
// rewrites ONLY the package clause and the main signature — every other line,
// including the whole compiled body, is preserved verbatim.
func raw2package(raw string) string {
	// Package clause: `package main` can't be imported; give it a name.
	raw = strings.Replace(raw, "\npackage main\n", "\npackage "+pkgPlaceholder+"\n", 1)
	// Rename the entry point; the process-per-program dispatch means Run()
	// does exactly what main() did (Boot → Load → print).
	raw = strings.Replace(raw, "\nfunc main() {", "\nfunc Run() {", 1)
	return raw
}

// evalOutput runs the file through the eval harness capturing printed side
// effects (corelib.Out) and appending pr-str of the last value. Serial-only:
// it mutates the process-global namespace registry and corelib.Out, bracketed
// by namespaceSnapshot / removeNewNamespaces.
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

// emitModule compiles the file (discarding compile-time side effects — Load()
// replays them in the binary) and writes the generated module to dir.
// Serial-only: CompileProgram drives the global interpreter. Used for the
// standalone (multi-dep / bri) path.
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

// runStandalone runs the already-built standalone binary in dir, returning its
// stdout. The binary must have been built in the Phase-B pre-build step.
func runStandalone(dir string) (string, error) {
	bin := filepath.Join(dir, "prog"+emit.ExeSuffix)
	out, err := exec.Command(bin).Output()
	if err != nil {
		return "", fmt.Errorf("run: %w", err)
	}
	return string(out), nil
}

// runBatch runs the group binary once for a single program, selected by the
// CLJGO_BATCH_PROG env var. argv is left pristine (just the binary name) so
// the program observes the same *command-line-args* as a standalone binary.
func runBatch(bin, key string) (string, error) {
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "CLJGO_BATCH_PROG="+key)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("run (batch %s): %w", key, err)
	}
	return string(out), nil
}
