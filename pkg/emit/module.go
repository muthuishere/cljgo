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
	"sync"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/bri"
	"github.com/muthuishere/cljgo/pkg/briloader"
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/deps"
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
	// UsesBri is set when the program required a bri.* namespace during
	// discovery (ADR 0071). bri namespaces are provider-backed (not
	// file-backed), so they never enter Deps — instead the emitted main
	// blank-imports the AOT-compiled pkg/briaot. WriteProgram passes this
	// to Options.UsesBri.
	UsesBri bool
	// OptInBriPkgs are the pkg/briaot sub-packages of any OPT-IN bri
	// namespaces required during discovery (ADR 0074, e.g. "briotel" for
	// bri.core.telemetry). Excluded from the umbrella pkg/briaot, blank-imported
	// additively by the emitted main. WriteProgram passes this to
	// Options.OptInBriPkgs.
	OptInBriPkgs []string
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
		// A Maven-origin namespace that passed the read-time interop-free
		// classification and then failed to compile gets the G5020 context:
		// that is a gap in cljgo, not evidence the library is JVM-only.
		if werr := deps.MavenCompileFailure(name, path, err); werr != nil {
			panic(werr)
		}
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
	// lang's namespace registry is PROCESS-GLOBAL, so a second `exe` in the
	// same build.cljgo inherits every namespace the first build interned, and
	// would silently omit them from its own program (unbound vars at runtime).
	//
	// The predicate is deliberately NARROW: it answers "not in this program"
	// only for a namespace a PREVIOUS build in this process compiled from a
	// file. Everything else — providers, embedded namespaces, namespaces
	// created by in-ns during evaluation — answers true and is left alone.
	// A broader predicate (anything not in mc.done) sends namespaces that
	// have no file down the file loader, which fails.
	//
	// Cleared on return, so `cljgo run` and the REPL never see it.
	corelib.SetLibInProgram(func(name string) bool {
		if mc.done[name] != nil {
			return true // compiled into THIS program already
		}
		return !createdByAnEarlierBuild(name)
	})
	defer corelib.SetLibInProgram(nil)
	// Snapshot the namespace table now and diff it on the way out: whatever is
	// new was brought into existence BY THIS BUILD, which is precisely the set
	// a later build must not mistake for "already loaded". Diffing beats
	// tracking mc.done plus the entry namespace, because *ns* is restored by
	// the time the entry finishes compiling and the entry's own name is not
	// otherwise recoverable here.
	before := namespaceNameSet()
	defer func() { rememberCreated(before) }()

	// ADR 0071: register the bri lib providers on this discovery evaluator
	// so (require '[bri.web.http]) resolves at build time. bri is provider-backed
	// (not file-backed), so it NEVER enters mc.load / Program.Deps — the
	// namespaces are AOT-compiled in pkg/briaot, blank-imported when a bri
	// provider fires here. Track that so the main package imports briaot only
	// when the app actually uses bri; guard so a repeated require is a no-op.
	usesBri := false
	briDone := map[string]bool{}
	var optInBriPkgs []string
	optInSeen := map[string]bool{}
	for _, s := range bri.Specs() {
		s := s
		corelib.RegisterLibProvider(s.Name, func() {
			usesBri = true
			// ADR 0074: an OPT-IN namespace links a separate sub-package the
			// emitter must blank-import; record it (deduped) when its provider
			// fires during discovery — including transitively (a required
			// namespace whose source requires an opt-in one triggers this too).
			if s.OptIn && !optInSeen[s.Pkg] {
				optInSeen[s.Pkg] = true
				optInBriPkgs = append(optInBriPkgs, s.Pkg)
			}
			if briDone[s.Name] {
				return
			}
			briDone[s.Name] = true
			briloader.LoadSpec(ev, s)
		})
	}

	entry := &CompiledNS{Path: srcPath}
	mc.stack = []*CompiledNS{entry}
	// The entry file's own namespace has to be remembered too, and it does not
	// pass through mc.load — it is compiled directly here. Without it, an
	// `exe` whose main IS the shared namespace leaves that namespace
	// unrecorded, and the NEXT exe skips loading it: exactly the second-`exe`
	// defect, just with the artifacts the other way round.
	if entry.Forms, err = compileStream(ev, f, srcPath); err != nil {
		// Mark it as a SOURCE error so the CLI renders it through diag.Render
		// (issue 8): everything raised in here came from the user's Clojure,
		// including the requires the capture loader compiled recursively.
		return nil, &CompileError{Err: err}
	}
	return &Program{Entry: entry, Deps: mc.order, UsesBri: usesBri, OptInBriPkgs: optInBriPkgs}, nil
}

// WriteProgram writes the generated module for a compiled program: the
// single-file layout when there are no file-backed requires, else one
// package per dependency namespace plus main.go (ADR 0042 §1).
func WriteProgram(dir string, p *Program, opts Options) error {
	// ADR 0053 dec 3: the entry namespace's *file* binds to its logical
	// source path so a binary matches the interpreter (not NO_SOURCE_FILE).
	opts.EntrySrcFile = p.Entry.Path
	// ADR 0071: carry bri usage into emission so the main package
	// blank-imports the AOT-compiled framework (pkg/briaot).
	opts.UsesBri = p.UsesBri
	opts.OptInBriPkgs = p.OptInBriPkgs
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
		if err := writePkg(d.Forms, spec, filepath.Join(dir, filepath.FromSlash(nd), pkgFileName(pkg))); err != nil {
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

// pkgFileName is the generated source file for a namespace's package.
// Normally <pkg>.go — EXCEPT when that would end in `_test.go`, which the
// Go toolchain treats as a test file and excludes from the build ("no
// non-test Go files"). Every Clojure test namespace is called <thing>-test,
// so without this NO test namespace could ever be compiled (found compiling
// `cljgo test --compiled`, ADR 0105 task 2.2).
func pkgFileName(pkg string) string {
	if strings.HasSuffix(pkg, "_test") {
		return pkg + "_ns.go"
	}
	return pkg + ".go"
}

// Cross-build namespace leakage (the second-`exe` defect).
//
// lang's namespace registry is process-global and nothing clears it between
// two CompileProgram calls, so the second build sees the first build's
// namespaces as already present and skips loading them — emitting a program
// with those namespaces missing and their vars unbound. The fix needs to know
// exactly one thing: which namespaces a previous build in this process
// compiled FROM A FILE. Those, and only those, must be re-loaded rather than
// assumed present.
var (
	createdMu      sync.Mutex
	createdEarlier = map[string]bool{}
)

func createdByAnEarlierBuild(name string) bool {
	createdMu.Lock()
	defer createdMu.Unlock()
	return createdEarlier[name]
}

// namespaceNameSet is the set of namespace names that exist right now.
func namespaceNameSet() map[string]bool {
	out := map[string]bool{}
	for s := lang.AllNamespaces(); s != nil; s = s.Next() {
		if ns, ok := s.First().(*lang.Namespace); ok && ns != nil {
			out[ns.Name().FullName()] = true
		}
	}
	return out
}

// rememberCreated records every namespace that came into existence during this
// build, so a later build in the same process re-loads it rather than trusting
// the process-global registry.
func rememberCreated(before map[string]bool) {
	createdMu.Lock()
	defer createdMu.Unlock()
	for name := range namespaceNameSet() {
		if !before[name] {
			createdEarlier[name] = true
		}
	}
}
