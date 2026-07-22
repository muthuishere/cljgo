// module.go — the multi-namespace module compiler (ADR 0042, AOT-core
// piece 1). CompileProgram compiles an entry file PLUS every file-backed
// namespace it transitively requires, capturing each namespace's
// analyzed forms; WriteProgram emits one Go package per dependency
// namespace (registry-triggered Load(), design.md §2) plus the existing
// main package for the entry. A program with no file-backed requires
// takes exactly the single-file path.
package emit

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// CompiledNS is one compiled namespace: its analyzed top-level forms
// and the file-backed namespaces its own (require …) forms loaded.
type CompiledNS struct {
	Name     string      // namespace name as required ("multi.util"); "" for the entry
	Path     string      // source file path
	Forms    []*ast.Node // analyzed top-level forms, source order
	Requires []string    // file-backed requires, first-require order
}

// Program is a compiled multi-file program: the entry file plus its
// transitive file-backed requires in dependency-first order.
type Program struct {
	Entry *CompiledNS
	Deps  []*CompiledNS
}

// moduleCompiler captures namespaces as the evaluator's lib loader
// (ADR 0042 §5): each file-backed require compiles its file through the
// same analyze-and-eval pipeline as the entry (macros defined there are
// live for later forms — ADR 0002 across files) and records the forms.
// Cycle detection lives in pkg/eval's load stack.
type moduleCompiler struct {
	stack []*CompiledNS          // whose file is being compiled (top = requiring ns)
	done  map[string]*CompiledNS // by ns name
	order []*CompiledNS          // dependency-first
}

func (mc *moduleCompiler) load(e *eval.Evaluator, lib *lang.Symbol, path string) {
	name := lib.FullName()
	requiring := mc.stack[len(mc.stack)-1]
	for _, r := range requiring.Requires {
		if r == name {
			return // already an edge (and already compiled)
		}
	}
	requiring.Requires = append(requiring.Requires, name)
	if mc.done[name] != nil {
		return
	}

	cns := &CompiledNS{Name: name, Path: path}
	mc.stack = append(mc.stack, cns)
	defer func() { mc.stack = mc.stack[:len(mc.stack)-1] }()

	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Errorf("loading %s from %s: %w", name, path, err))
	}
	defer f.Close()
	forms, err := compileStream(e, f, path)
	if err != nil {
		panic(fmt.Errorf("compiling %s (%s): %w", name, path, err))
	}
	cns.Forms = forms
	mc.done[name] = cns
	mc.order = append(mc.order, cns) // after its own deps: dependency-first
}

// CompileProgram compiles srcPath and every file-backed namespace it
// transitively requires. Requires that resolve to embedded namespaces
// (clojure.string …) load as always and record nothing.
func CompileProgram(srcPath string) (p *Program, err error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// The capture loader panics on load errors (the IFn-boundary
	// convention); compileStream's evalNode recovers those raised while
	// evaluating a require form into errors, so nothing extra here.
	// ADR 0052 decision 2: a build must not resolve namespaces through
	// $CLJGO_PATH — an env-supplied root would bake foreign source into the
	// binary invisibly to the repo. Disable env-path participation for the
	// build's discovery pass; a namespace reachable only via $CLJGO_PATH then
	// fails to resolve (an error), never silently included. Restore on return so
	// a later in-process run/REPL (or the next test) is unaffected — the flag is
	// scoped to this discovery pass, not latched for the process.
	eval.SetEnvPathEnabled(false)
	defer eval.SetEnvPathEnabled(true)

	ev := eval.New()
	// ADR 0053 dec 2: the namespace-discovery pass evaluates require and
	// member-access forms through the interpreter, but the emitted binary
	// links third-party require-go modules for real — so tolerate an
	// unlinked third-party member here rather than hard-erroring (which is
	// what `cljgo run` / the REPL do, default HostUnlinkedTolerant=false).
	ev.HostUnlinkedTolerant = true
	mc := &moduleCompiler{done: map[string]*CompiledNS{}}
	ev.LibLoader = mc.load

	entry := &CompiledNS{Path: srcPath}
	mc.stack = []*CompiledNS{entry}
	if entry.Forms, err = compileStream(ev, f, srcPath); err != nil {
		return nil, err
	}
	return &Program{Entry: entry, Deps: mc.order}, nil
}

// WriteProgram writes the generated module for a compiled program: the
// single-file layout when there are no file-backed requires, else one
// package per dependency namespace plus main.go (ADR 0042 §1).
func WriteProgram(dir string, p *Program, opts Options) error {
	// ADR 0053 dec 3: the entry namespace's *file* binds to its logical
	// source path so a binary matches the interpreter (not NO_SOURCE_FILE).
	opts.EntrySrcFile = p.Entry.Path
	if len(p.Deps) == 0 {
		return WriteModule(dir, p.Entry.Forms, opts)
	}

	moduleName := opts.ModuleName
	if moduleName == "" {
		moduleName = "cljgo.gen/main"
	}

	// Interop facts load once for the whole module (union pre-scan).
	var all []*ast.Node
	for _, d := range p.Deps {
		all = append(all, d.Forms...)
	}
	all = append(all, p.Entry.Forms...)
	var host *hostFacts
	if hostPaths := collectHostPaths(all); len(hostPaths) > 0 {
		factsDir, err := hostFactsDir(opts)
		if err != nil {
			return err
		}
		if host, err = loadHostFacts(factsDir, hostPaths); err != nil {
			return err
		}
	}

	// Namespace → package layout, with a lossy-munge collision check.
	dirs := map[string]string{} // ns dir → ns name
	importPath := func(ns string) string { return moduleName + "/" + nsDir(ns) }
	for _, d := range p.Deps {
		nd := nsDir(d.Name)
		if prev, ok := dirs[nd]; ok {
			return fmt.Errorf("emit: namespaces %s and %s both emit to package directory %s (munging is lossy — rename one)", prev, d.Name, nd)
		}
		dirs[nd] = d.Name
	}

	writePkg := func(forms []*ast.Node, spec pkgSpec, outPath string) error {
		formatted, raw, err := emitPackage(forms, opts, spec)
		if err != nil {
			if len(raw) > 0 {
				return fmt.Errorf("emit: %w\n--- unformatted source ---\n%s", err, raw)
			}
			return err
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(outPath, formatted, 0o644)
	}

	imports := func(requires []string) []string {
		paths := make([]string, 0, len(requires))
		for _, r := range requires {
			paths = append(paths, importPath(r))
		}
		return paths
	}

	for _, d := range p.Deps {
		nd := nsDir(d.Name)
		pkg := pkgName(d.Name)
		spec := pkgSpec{
			pkgName:    pkg,
			nsName:     d.Name,
			srcFile:    d.Path,
			depImports: imports(d.Requires),
			host:       host,
		}
		if err := writePkg(d.Forms, spec, filepath.Join(dir, filepath.FromSlash(nd), pkg+".go")); err != nil {
			return fmt.Errorf("namespace %s: %w", d.Name, err)
		}
	}

	mainSpec := pkgSpec{
		pkgName:    "main",
		isMain:     true,
		srcFile:    p.Entry.Path, // ADR 0053 dec 3: entry *file* = logical source path
		depImports: imports(p.Entry.Requires),
		host:       host,
	}
	if err := writePkg(p.Entry.Forms, mainSpec, filepath.Join(dir, "main.go")); err != nil {
		return err
	}
	return SynthGoMod(dir, opts.ModuleName, opts.RuntimeDir, nil)
}

// hostFactsDir resolves the directory go/packages loads host facts from
// — the EmitMain precedence (ADR 0033).
func hostFactsDir(opts Options) (string, error) {
	if opts.HostFactsDir != "" {
		return opts.HostFactsDir, nil
	}
	if opts.RuntimeDir != "" {
		return opts.RuntimeDir, nil
	}
	return FindRuntimeDir()
}

// nsDir is a namespace's package directory relative to the module root:
// segments munged (JVM rule and then Go-identifier munging), joined
// with "/" ("my-app.util" → "my_app/util").
func nsDir(ns string) string {
	segs := strings.Split(ns, ".")
	for i, s := range segs {
		segs[i] = munge(s)
	}
	return strings.Join(segs, "/")
}

// pkgName is the Go package name for a namespace: its munged last
// segment, kept clear of Go keywords (and "main", which the entry owns).
func pkgName(ns string) string {
	segs := strings.Split(ns, ".")
	name := munge(segs[len(segs)-1])
	if token.IsKeyword(name) || name == "main" {
		name += "_pkg"
	}
	return name
}
