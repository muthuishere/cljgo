// clojars.go — the `publish clojars` producer (ADR 0050 dec 1, 3).
//
// cljgo compiles to Go, not JVM bytecode, so a cljgo library reaches the
// Clojure ecosystem only as pure Clojure SOURCE the JVM's own Clojure compiles.
// PublishClojars walks the whole transitive required surface (emit.CompileProgram
// → emit.ClassifyGoInterop → emit.WholeLibPure) and REFUSES, naming the offending
// file:line, if any reachable namespace uses Go interop — that is precisely what
// cannot run on the JVM. Java interop is ALLOWED (it runs on the JVM); it is not
// a gate (decision 4). On success it copies every namespace's source into a
// JVM-consumable src/ tree, writes a git-coordinate deps.edn stub, and emits the
// cljgo.manifest.edn (pure) the resolve side (pkg/deps) reads.
package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muthuishere/cljgo/pkg/emit"
)

// settings is the resolved publish configuration (functional options).
type settings struct {
	module     string // library module / coordinate (github.com/you/lib)
	runtimeDir string // cljgo runtime tree for the go emit path (publish go)
}

// Opt configures a publish call.
type Opt func(*settings)

// WithModule sets the published library's module path / coordinate.
func WithModule(m string) Opt { return func(s *settings) { s.module = m } }

// WithRuntimeDir sets the cljgo runtime source tree for `publish go`'s emit.
func WithRuntimeDir(d string) Opt { return func(s *settings) { s.runtimeDir = d } }

func resolve(opts []Opt) settings {
	var s settings
	for _, o := range opts {
		o(&s)
	}
	return s
}

// PublishClojars compiles entrySrc's transitive surface, gates it on Go-interop
// purity, and writes a JVM-consumable source distribution under outDir. It fails
// (naming file:line) if any reachable namespace uses Go interop.
func PublishClojars(entrySrc, outDir string, opts ...Opt) error {
	s := resolve(opts)

	prog, err := emit.CompileProgram(entrySrc)
	if err != nil {
		return fmt.Errorf("publish clojars: compiling %s: %w", entrySrc, err)
	}

	// The whole-library gate: Go interop cannot run on the JVM.
	m := emit.ClassifyGoInterop(prog)
	if ok, off := emit.WholeLibPure(m); !ok {
		return fmt.Errorf(
			"publish clojars: cannot publish to the JVM — namespace %s (%s:%d) %s: uses Go interop, cannot run on the JVM (ADR 0050 decision 3)",
			off.NS, off.Path, off.Line, off.Detail)
	}

	// Fresh output tree.
	srcDir := filepath.Join(outDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return err
	}

	// Copy every namespace's source into src/<ns-path>.clj (JVM reads .clj).
	type nsFile struct{ name, path string }
	var files []nsFile
	for _, d := range prog.Deps {
		files = append(files, nsFile{name: nsName(d.Name, d.Path), path: d.Path})
	}
	files = append(files, nsFile{name: nsName(prog.Entry.Name, prog.Entry.Path), path: prog.Entry.Path})

	for _, f := range files {
		dst := filepath.Join(srcDir, nsToRelPath(f.name))
		if err := copyFile(f.path, dst); err != nil {
			return fmt.Errorf("publish clojars: copying %s: %w", f.name, err)
		}
	}

	if err := writeClojarsDepsEDN(outDir, s.module); err != nil {
		return err
	}
	if err := writePureManifest(outDir); err != nil {
		return err
	}
	return nil
}

// nsName returns a compiled namespace's name: the required name when set, else
// the entry's declared (ns …) name read textually (CompileProgram leaves the
// entry's Name "").
func nsName(name, path string) string {
	if name != "" {
		return name
	}
	if n := readNSName(path); n != "" {
		return n
	}
	// Fall back to the file's base name (still a valid, if plain, placement).
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// nsToRelPath maps a namespace to its Clojure source-tree relative path:
// dots → path separators, hyphens → underscores (the JVM Clojure file rule),
// with a .clj extension so JVM Clojure loads it.
func nsToRelPath(ns string) string {
	segs := strings.Split(ns, ".")
	for i, s := range segs {
		segs[i] = strings.ReplaceAll(s, "-", "_")
	}
	return filepath.Join(segs...) + ".clj"
}

// readNSName reads the declared namespace from a source file's leading (ns X …)
// form, textually.
func readNSName(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	str := string(b)
	i := strings.Index(str, "(ns ")
	if i < 0 {
		return ""
	}
	rest := str[i+4:]
	j := strings.IndexAny(rest, " \t\r\n()")
	if j < 0 {
		j = len(rest)
	}
	return strings.TrimSpace(rest[:j])
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// writeClojarsDepsEDN writes the git-coordinate deps.edn stub (ADR 0050: git
// coord first; a Clojars coordinate is deferred). module may be empty.
func writeClojarsDepsEDN(outDir, module string) error {
	coord := module
	if coord == "" {
		coord = "io.github.you/lib"
	}
	body := ";; Published by `cljgo publish clojars` (ADR 0050): pure Clojure source.\n" +
		";; Consume from a JVM Clojure project's deps.edn via a git coordinate:\n" +
		";;   " + coord + " {:git/url \"https://" + coord + "\" :git/sha \"<sha>\"}\n" +
		"{:paths [\"src\"]}\n"
	return os.WriteFile(filepath.Join(outDir, "deps.edn"), []byte(body), 0o644)
}

// writePureManifest emits cljgo.manifest.edn declaring the library pure — the
// impurity manifest pkg/deps reads at resolve time (a missing impure section =
// pure). It is emitted here so a downstream cljgo consumer resolves this source
// tree as a pure dependency.
func writePureManifest(outDir string) error {
	body := ";; Emitted by `cljgo publish clojars` (ADR 0050). Read as DATA by pkg/deps.\n" +
		"{:paths [\"src\"]\n" +
		" :pure? true}\n"
	return os.WriteFile(filepath.Join(outDir, "cljgo.manifest.edn"), []byte(body), 0o644)
}
