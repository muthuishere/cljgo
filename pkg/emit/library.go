// library.go — go-gettable library emission (ADR 0050 dec 1, `publish go`).
//
// WriteProgram (module.go) emits an EXECUTABLE: the entry namespace becomes
// `package main` with main()+rt.Boot(). A published Go library is the same
// per-namespace layout, LIBRARY-shaped: EVERY namespace — the entry included —
// emits as a registered RegisterLib/Load package (the exact shape module.go
// already gives dependency namespaces), with NO main(). The entry package
// additionally gets a hand-generated wrappers.go exposing each exported defn as
// an idiomatic exported Go function, so a Go developer `go get`s the module and
// calls `pkg.Greet(x)` directly.
//
// Scope (honest, ADR 0050 risk note): exported wrappers use the `any`-typed
// calling convention (`func Name(args ...any) any` for defns, `func Name() any`
// for value defs). Resolved-from-type-hint Go signatures (defn ^long → func(int64)
// int64) are DEFERRED — the module still compiles and is go-gettable; the
// signatures are just uniformly `any`. Go-interop namespaces are supported for
// stdlib host packages (go/packages resolves stdlib with no module deps);
// third-party go-require resolution into the emitted module is deferred.

package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// LibExport is one exported member of the entry namespace, surfaced as an
// exported Go function in wrappers.go.
type LibExport struct {
	CljName string // clojure name ("greet", "parse-int")
	GoName  string // exported Go identifier ("Greet", "ParseInt")
	IsFn    bool   // true → variadic apply wrapper; false → value getter
}

// WriteLibrary emits a go-gettable library module for p under dir: every
// namespace as a registered Load package (entry included, no main()), a go.mod
// with the library module path, and a wrappers.go in the entry package exposing
// each exported defn as an exported Go function. It returns the entry namespace
// name and the exported members (for reporting).
func WriteLibrary(dir string, p *Program, opts Options) (entryNS string, exports []LibExport, err error) {
	if p == nil || p.Entry == nil {
		return "", nil, fmt.Errorf("emit: WriteLibrary: nil program")
	}
	moduleName := opts.ModuleName
	if moduleName == "" {
		moduleName = "cljgo.gen/lib"
	}
	entryNS = nsRealName(p.Entry)

	// Create the output dir up front: host-facts loading (below) drives
	// go/packages with HostFactsDir == dir, which chdirs into it — and for a
	// go-interop library that runs BEFORE any writePkg/SynthGoMod creates dir,
	// so without this a real go-interop lib dies with "chdir: no such file or
	// directory" (a test using t.TempDir never hits it — the dir pre-exists).
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, fmt.Errorf("emit: WriteLibrary: create out dir: %w", err)
	}

	// Interop facts load once for the whole module (union pre-scan), exactly as
	// WriteProgram — a pure library references none and pays no go/packages cost.
	var all []*ast.Node
	for _, d := range p.Deps {
		all = append(all, d.Forms...)
	}
	all = append(all, p.Entry.Forms...)
	var host *hostFacts
	if hostPaths := collectHostPaths(all); len(hostPaths) > 0 {
		factsDir, ferr := hostFactsDir(opts)
		if ferr != nil {
			return "", nil, ferr
		}
		if host, err = loadHostFacts(factsDir, hostPaths); err != nil {
			return "", nil, err
		}
	}

	importPath := func(ns string) string { return moduleName + "/" + nsDir(ns) }
	imports := func(requires []string) []string {
		paths := make([]string, 0, len(requires))
		for _, r := range requires {
			paths = append(paths, importPath(r))
		}
		return paths
	}

	// Collision check across dependency package directories (WriteProgram rule).
	dirs := map[string]string{}
	nsList := append([]*CompiledNS{}, p.Deps...)
	nsList = append(nsList, p.Entry)
	for _, d := range nsList {
		nd := nsDir(nsRealName(d))
		if prev, ok := dirs[nd]; ok {
			return "", nil, fmt.Errorf("emit: namespaces %s and %s both emit to package directory %s (munging is lossy — rename one)", prev, nsRealName(d), nd)
		}
		dirs[nd] = nsRealName(d)
	}

	writePkg := func(forms []*ast.Node, spec pkgSpec, outPath string) error {
		formatted, raw, perr := emitPackage(forms, opts, spec)
		if perr != nil {
			if len(raw) > 0 {
				return fmt.Errorf("emit: %w\n--- unformatted source ---\n%s", perr, raw)
			}
			return perr
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(outPath, formatted, 0o644)
	}

	// Every namespace — deps AND entry — is a registered library package.
	for _, d := range nsList {
		name := nsRealName(d)
		nd := nsDir(name)
		pkg := pkgName(name)
		spec := pkgSpec{
			pkgName:    pkg,
			nsName:     name,
			srcFile:    d.Path,
			depImports: imports(d.Requires),
			host:       host,
		}
		if err := writePkg(d.Forms, spec, filepath.Join(dir, filepath.FromSlash(nd), pkg+".go")); err != nil {
			return "", nil, fmt.Errorf("namespace %s: %w", name, err)
		}
	}

	// Exported wrappers for the entry namespace's public defns.
	exports = collectExports(p.Entry.Forms)
	entryDir := filepath.Join(dir, filepath.FromSlash(nsDir(entryNS)))
	if err := writeWrappers(filepath.Join(entryDir, "wrappers.go"), pkgName(entryNS), entryNS, exports); err != nil {
		return "", nil, err
	}

	// A library must not carry the release-pin/tidy path go.mod branch that
	// needs network; keep the runtime replace when a runtime dir is resolvable
	// (dev/in-repo). SynthGoMod applies the same ADR 0028 precedence as the exe
	// path, so publish go behaves like build for module resolution.
	if err := SynthGoMod(dir, moduleName, opts.RuntimeDir, nil); err != nil {
		return "", nil, err
	}
	return entryNS, exports, nil
}

// collectExports returns the entry namespace's public (non-private) def'd
// members, in source order, each with its exported Go name. A defn (Init is an
// fn*) becomes a variadic apply wrapper; any other def becomes a value getter.
func collectExports(forms []*ast.Node) []LibExport {
	var out []LibExport
	// Reserve the exported identifiers the entry package already generates, so a
	// public defn (e.g. `load`) cannot munge onto one and redeclare it — the
	// emitted module would then fail to compile with no cljgo-level diagnostic.
	// `Load()` is the generated AOT loader wrappers.go itself calls; bootOnce/
	// ensureLoaded are lowercase and cannot collide with an exported wrapper.
	seen := map[string]bool{"Load": true}
	for _, n := range forms {
		if n.Op != ast.OpDef {
			continue
		}
		d := n.Sub.(*ast.DefNode)
		if d.Var == nil {
			continue
		}
		if lang.IsTruthy(lang.Get(d.Var.Meta(), lang.KWPrivate)) {
			continue // ^:private is not part of the public surface
		}
		clj := d.Name.Name()
		if clj == "-main" {
			continue // an entry point, not a library export
		}
		goName := exportGoName(clj, seen)
		out = append(out, LibExport{
			CljName: clj,
			GoName:  goName,
			IsFn:    d.Init != nil && d.Init.Op == ast.OpFn,
		})
	}
	return out
}

// exportGoName turns a clojure name into an exported Go identifier: munge to a
// valid Go identifier body, drop underscores at word boundaries into camel-case,
// uppercase the first letter, and dedup against names already taken.
func exportGoName(clj string, seen map[string]bool) string {
	base := goExportIdent(munge(clj))
	name := base
	for i := 2; seen[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	seen[name] = true
	return name
}

// goExportIdent camel-cases an already-munged identifier (underscores mark word
// boundaries) and exports it. "parse_int" → "ParseInt"; "greet" → "Greet".
func goExportIdent(m string) string {
	parts := strings.Split(m, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	s := b.String()
	if s == "" || !unicode.IsLetter([]rune(s)[0]) {
		s = "X" + s
	}
	return s
}

// writeWrappers emits the entry package's wrappers.go: a booted-once guard plus
// one exported Go function per export. Wrappers apply/deref the interned var by
// its fully-qualified name (no dependence on the generated package's private
// hoist names), so this file is self-contained.
func writeWrappers(outPath, pkg, ns string, exports []LibExport) error {
	var b strings.Builder
	b.WriteString("// Code generated by cljgo publish. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	b.WriteString("import (\n")
	b.WriteString("\t\"sync\"\n\n")
	fmt.Fprintf(&b, "\trt %q\n", runtimeModule+"/pkg/emit/rt")
	fmt.Fprintf(&b, "\tlang %q\n", runtimeModule+"/pkg/lang")
	// The AOT core must be linked for rt.Boot() to find clojure.core (ADR 0046).
	fmt.Fprintf(&b, "\t_ %q\n", runtimeModule+"/pkg/coreaot")
	b.WriteString(")\n\n")

	b.WriteString("// bootOnce boots the cljgo runtime and loads this namespace exactly once,\n")
	b.WriteString("// so a Go caller need not know about rt.Boot()/Load().\n")
	b.WriteString("var bootOnce sync.Once\n\n")
	b.WriteString("func ensureLoaded() {\n")
	b.WriteString("\tbootOnce.Do(func() {\n")
	b.WriteString("\t\trt.Boot()\n")
	b.WriteString("\t\tLoad()\n")
	b.WriteString("\t})\n")
	b.WriteString("}\n\n")

	// Deterministic order (source order is already deterministic; keep it).
	for _, e := range exports {
		fmt.Fprintf(&b, "// %s calls %s/%s.\n", e.GoName, ns, e.CljName)
		varExpr := fmt.Sprintf("lang.InternVarName(lang.NewSymbol(%q), lang.NewSymbol(%q))", ns, e.CljName)
		if e.IsFn {
			fmt.Fprintf(&b, "func %s(args ...any) any {\n", e.GoName)
			b.WriteString("\tensureLoaded()\n")
			fmt.Fprintf(&b, "\treturn lang.Apply(%s.Get(), args)\n", varExpr)
			b.WriteString("}\n\n")
		} else {
			fmt.Fprintf(&b, "func %s() any {\n", e.GoName)
			b.WriteString("\tensureLoaded()\n")
			fmt.Fprintf(&b, "\treturn %s.Get()\n", varExpr)
			b.WriteString("}\n\n")
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}
