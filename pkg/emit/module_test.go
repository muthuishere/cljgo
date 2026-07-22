package emit

// module_test.go — the ADR 0042 proof: a 3-namespace program (entry →
// multi.util → multi.data, cross-ns var refs, a macro used across the
// ns boundary) compiles to a binary whose stdout is byte-identical to
// the interpreted run. Interpreted output oracled against real Clojure
// 1.12.5 (2026-07-17, `clojure -Sdeps '{:paths ["."]}' -M entry.clj`):
//
//	loading multi.data
//	loading multi.util
//	loading entry
//	hi!
//	42
//
// with 82 as the last top-level value ((load-file "entry.clj") ⇒ 82).

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

const (
	multiData = `(ns multi.data)
(println "loading multi.data")
(def base 40)
(defmacro twice [x] (list '+ x x))
`
	multiUtil = `(ns multi.util
  (:require [multi.data :as d]))
(println "loading multi.util")
(def offset (+ d/base 2))
(defn shout [s] (str s "!"))
`
	multiEntry = `(require '[multi.util :as u]
         '[multi.data :as d])
(println "loading entry")
(println (u/shout "hi"))
(println (d/twice 21))
(+ u/offset d/base)
`
	multiExpected = "loading multi.data\nloading multi.util\nloading entry\nhi!\n42\n82\n"
)

// writeMultiNSProgram lays the 3-ns source tree into a temp dir and
// returns the entry path.
func writeMultiNSProgram(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "multi"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, src := range map[string]string{
		filepath.Join(dir, "multi", "data.clj"): multiData,
		filepath.Join(dir, "multi", "util.clj"): multiUtil,
		filepath.Join(dir, "entry.clj"):         multiEntry,
	} {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "entry.clj")
}

// namespaceSnapshot / removeNewNamespaces isolate runs: the namespace
// registry is process-global, so a namespace loaded by one run would
// make the next run's require skip loading it (no side-effect prints,
// no capture) — cross-talk, not semantics.
func namespaceSnapshot() map[string]bool {
	snap := map[string]bool{}
	for s := lang.AllNamespaces(); s != nil; s = s.Next() {
		snap[s.First().(*lang.Namespace).Name().String()] = true
	}
	return snap
}

func removeNewNamespaces(snap map[string]bool) {
	for s := lang.AllNamespaces(); s != nil; s = s.Next() {
		name := s.First().(*lang.Namespace).Name()
		if !snap[name.String()] {
			lang.RemoveNamespace(name)
		}
	}
}

// interpretFile runs a file through the eval harness (the conformance
// evalOutput shape): printed side effects + pr-str of the last value.
func interpretFile(t *testing.T, path string) string {
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
	last, err := d.EvalReader(f, path)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	return buf.String() + lang.PrintString(last) + "\n"
}

func TestMultiNamespaceProgram(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile-and-run in -short mode")
	}
	entry := writeMultiNSProgram(t)

	evalOut := interpretFile(t, entry)
	if evalOut != multiExpected {
		t.Fatalf("interpreted output diverges from the JVM oracle:\n got %q\nwant %q", evalOut, multiExpected)
	}

	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard // compile-time side effects don't belong to the run
	prog, err := CompileProgram(entry)
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("CompileProgram: %v", err)
	}

	if len(prog.Deps) != 2 {
		t.Fatalf("expected 2 dependency namespaces, got %d", len(prog.Deps))
	}
	// Dependency-first order: data (leaf) before util.
	if prog.Deps[0].Name != "multi.data" || prog.Deps[1].Name != "multi.util" {
		t.Fatalf("dependency order = [%s %s], want [multi.data multi.util]", prog.Deps[0].Name, prog.Deps[1].Name)
	}
	if got := prog.Deps[1].Requires; len(got) != 1 || got[0] != "multi.data" {
		t.Fatalf("multi.util requires = %v, want [multi.data]", got)
	}
	if got := prog.Entry.Requires; len(got) != 1 || got[0] != "multi.util" {
		// multi.data already existed when the entry required it (loaded
		// via multi.util), so only the hook-firing edge is recorded.
		t.Fatalf("entry requires = %v, want [multi.util]", got)
	}

	gen := t.TempDir()
	if err := WriteProgram(gen, prog, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("WriteProgram: %v", err)
	}
	for _, f := range []string{"main.go", "multi/util/util.go", "multi/data/data.go"} {
		if _, err := os.Stat(filepath.Join(gen, filepath.FromSlash(f))); err != nil {
			t.Fatalf("expected generated file %s: %v", f, err)
		}
	}
	bin := filepath.Join(gen, "prog"+ExeSuffix)
	if err := GoBuild(gen, bin); err != nil {
		t.Fatalf("go build: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != evalOut {
		t.Fatalf("REPL/binary divergence (release blocker, ADR 0002/0007):\n--- eval ---\n%q\n--- compiled ---\n%q", evalOut, out)
	}
}

// TestWriteProgramSingleFileDelegates proves the zero-dependency path
// is exactly the existing single-file module writer.
func TestWriteProgramSingleFileDelegates(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "one.clj")
	if err := os.WriteFile(src, []byte("(def x 21)\n(+ x x)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	prog, err := CompileProgram(src)
	if err != nil {
		t.Fatalf("CompileProgram: %v", err)
	}
	if len(prog.Deps) != 0 || len(prog.Entry.Requires) != 0 {
		t.Fatalf("single-file program should have no deps, got %+v", prog)
	}
	gen := t.TempDir()
	if err := WriteProgram(gen, prog, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("WriteProgram: %v", err)
	}
	fromProgram, err := os.ReadFile(filepath.Join(gen, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	gen2 := t.TempDir()
	// WriteProgram threads the entry's logical source path into the delegated
	// WriteModule call (ADR 0049 dec 3: entry *file*), so the byte-for-byte
	// delegation comparison must pass the same EntrySrcFile.
	if err := WriteModule(gen2, prog.Entry.Forms, Options{PrintLastValue: true, EntrySrcFile: prog.Entry.Path}); err != nil {
		t.Fatalf("WriteModule: %v", err)
	}
	fromModule, err := os.ReadFile(filepath.Join(gen2, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromProgram, fromModule) {
		t.Fatalf("WriteProgram(no deps) main.go differs from WriteModule's")
	}
}

// TestRequireCycleFailsCompile proves compile-time cycle detection
// (oracle: JVM Clojure throws "Cyclic load dependency: [ /cyc/a
// ]->/cyc/b->[ /cyc/a ]" for the same shape).
func TestRequireCycleFailsCompile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cyc"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(dir, "cyc", "a.clj"): "(ns cyc.a (:require [cyc.b]))\n(def x 1)\n",
		filepath.Join(dir, "cyc", "b.clj"): "(ns cyc.b (:require [cyc.a]))\n(def y 2)\n",
		filepath.Join(dir, "entry.clj"):    "(require 'cyc.a)\n:done\n",
	}
	for path, src := range files {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	_, err := CompileProgram(filepath.Join(dir, "entry.clj"))
	if err == nil || !strings.Contains(err.Error(), "cyclic load dependency") {
		t.Fatalf("expected cyclic load dependency error, got %v", err)
	}
}

// TestThirdPartyDiscoveryTolerates is the emitter half of ADR 0049 dec 2:
// the namespace-discovery pass sets HostUnlinkedTolerant=true, so compiling
// a program that require-go's a third-party (domain-dotted) module and
// references an unlinked member SUCCEEDS (the emitted binary links it for
// real) — rather than hard-erroring as `cljgo run` now does. Deterministic
// and offline: this exercises only the compile-time discovery pass, no `go
// get` / link. (The full build+link is TestBuildWebsocketBinary /
// TestParityThirdPartyGoRequire, network-gated.)
func TestThirdPartyDiscoveryTolerates(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.clj")
	src := "(require-go '[\"example.com/foo/bar\" :as fk])\n" +
		"(def code fk/CloseNormalClosure)\n" +
		"(def frame (fk/FormatCloseMessage 1 \"x\"))\n:done\n"
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	oldOut := corelib.Out
	corelib.Out = io.Discard
	_, err := CompileProgram(entry)
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("CompileProgram of a third-party program must tolerate the unlinked "+
			"member during discovery, got: %v", err)
	}
}

// TestBinaryUncompiledRequireHardErrors is ADR 0049 dec 3: a binary that
// evaluates (require 'some.ns) for a namespace NOT compiled into it must
// hard-error naming the namespace, rather than silently no-op'ing behind the
// provider registry. The require is deferred inside -main, so the build-time
// discovery pass never resolves it (nothing to compile in) — only the binary,
// invoking -main at runtime, reaches it. Offline: no third-party module.
func TestBinaryUncompiledRequireHardErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go-build in -short mode")
	}
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.clj")
	// -main is defined but not called at top level → foo.bar is invisible to
	// discovery; the binary's main() calls -main and hits it at runtime.
	src := "(ns app)\n(defn -main [& _]\n  (require 'foo.bar)\n  (println \"unreached\"))\n"
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	prog, err := CompileProgram(entry)
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("CompileProgram (deferred require must compile): %v", err)
	}
	gen := t.TempDir()
	if err := WriteProgram(gen, prog, Options{}); err != nil {
		t.Fatalf("WriteProgram: %v", err)
	}
	bin := filepath.Join(gen, "prog"+ExeSuffix)
	if err := GoBuild(gen, bin); err != nil {
		t.Fatalf("go build: %v", err)
	}
	out, err := exec.Command(bin).CombinedOutput()
	if err == nil {
		t.Fatalf("binary must hard-error on the uncompiled require, exited 0: %q", out)
	}
	for _, want := range []string{"foo.bar", "was not compiled into this binary"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("binary error %q missing %q", out, want)
		}
	}
}

// TestCompileFileRefusesMultiNS: the single-file API must refuse (not
// silently drop) a file-backed require — ADR 0042 §5.
func TestCompileFileRefusesMultiNS(t *testing.T) {
	entry := writeMultiNSProgram(t)
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	oldOut := corelib.Out
	corelib.Out = io.Discard
	_, err := CompileFile(entry)
	corelib.Out = oldOut
	if err == nil || !strings.Contains(err.Error(), "CompileProgram") {
		t.Fatalf("expected single-file refusal naming CompileProgram, got %v", err)
	}
}
