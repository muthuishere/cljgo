// libload.go — filesystem + provider loading for `require` (ADR 0042).
//
// When require names a namespace that is not already present, loadLib
// falls through here, in order:
//
//  1. the lib-provider registry — emitted dependency packages register
//     their guarded Load() from package init() (via rt.RegisterLib), so
//     the replayed (require …) form in a compiled binary triggers the
//     dependency load at exactly its source position (byte-identical
//     side-effect order with the interpreter; oracle in ADR 0042);
//  2. a source file resolved relative to the REQUIRING file: root =
//     dir(*file*) minus the requiring ns's own directory suffix when it
//     matches (src/my_app/core.clj in ns my-app.core → src/), else
//     dir(*file*); candidates <root>/<lib path>.clj then .cljg, path
//     segments munged - → _ as on the JVM. The file loads through the
//     Evaluator's LibLoader — by default read + EvalForm under a pushed
//     *ns*/*file* frame (the interpreter); pkg/emit's module compiler
//     substitutes a loader that also CAPTURES the analyzed forms.
//
// Cyclic loads fail like the JVM ("Cyclic load dependency", oracled
// 2026-07-17 against Clojure 1.12.5), tracked by an in-progress stack.
package eval

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// libProviders is the runtime registry of namespace loaders. Emitted
// packages register from init() (a plain map write — safe before
// rt.Boot()); require consults it before touching the filesystem.
var (
	libProvidersMu sync.Mutex
	libProviders   = map[string]func(){}
)

// RegisterLibProvider registers a loader for a namespace, keyed by its
// full name ("my-app.util"). Called by generated code via
// rt.RegisterLib. Last registration wins (re-registration is harmless:
// providers are guarded, load-once).
func RegisterLibProvider(name string, load func()) {
	libProvidersMu.Lock()
	defer libProvidersMu.Unlock()
	libProviders[name] = load
}

func lookupLibProvider(name string) func() {
	libProvidersMu.Lock()
	defer libProvidersMu.Unlock()
	return libProviders[name]
}

// libsLoading is the in-progress load stack for cycle detection (load
// state is process-global, like the namespace registry itself).
var libsLoading []string

// checkCyclicLoad panics when name's source file is already mid-load
// (JVM parity: "Cyclic load dependency"). Namespace existence is no
// proof of loadedness — a file's (in-ns …) runs before its requires.
func checkCyclicLoad(name string) {
	for _, n := range libsLoading {
		if n == name {
			panic(fmt.Errorf("cyclic load dependency: %s -> %s",
				strings.Join(libsLoading, " -> "), name))
		}
	}
}

func pushLibLoading(name string) {
	checkCyclicLoad(name)
	libsLoading = append(libsLoading, name)
}

func popLibLoading() { libsLoading = libsLoading[:len(libsLoading)-1] }

// loadLibFile makes libSym's namespace exist by loading its source
// file. Panics (the IFn-boundary error convention) when no file
// resolves. Providers were already consulted by loadLib.
func loadLibFile(e *Evaluator, libSym *lang.Symbol) {
	name := libSym.FullName()
	path := ResolveLibPath(libSym)
	if path == "" {
		panic(fmt.Errorf("could not locate namespace %s (no registered provider, and no %s.clj/.cljg relative to the requiring file)",
			name, filepath.ToSlash(libPathStem(libSym))))
	}
	pushLibLoading(name)
	defer popLibLoading()
	loader := e.LibLoader
	if loader == nil {
		loader = evalLibFile
	}
	loader(e, libSym, path)
}

// libPathStem is the lib's relative source path without extension:
// my-app.util → my_app/util (JVM munging: - → _, . → /).
func libPathStem(libSym *lang.Symbol) string {
	segs := strings.Split(libSym.FullName(), ".")
	for i, s := range segs {
		segs[i] = strings.ReplaceAll(s, "-", "_")
	}
	return filepath.Join(segs...)
}

// ResolveLibPath resolves a namespace symbol to an existing source file
// relative to the requiring file (ADR 0042 §4), or "" when none exists.
func ResolveLibPath(libSym *lang.Symbol) string {
	file, _ := lang.VarFile.Deref().(string)
	if file == "" || file == "NO_SOURCE_FILE" || file == "NO_SOURCE_PATH" {
		return ""
	}
	dir := filepath.Dir(file)

	// Candidate roots: dir(*file*) stripped of the requiring ns's own
	// directory suffix (so sibling namespaces under one source root
	// resolve), then dir(*file*) itself.
	roots := []string{}
	if ns, ok := lang.VarCurrentNS.Deref().(*lang.Namespace); ok {
		if nsDir := filepath.Dir(libPathStem(ns.Name())); nsDir != "." {
			suffix := string(filepath.Separator) + nsDir
			if strings.HasSuffix(dir, suffix) {
				roots = append(roots, strings.TrimSuffix(dir, suffix))
			} else if dir == nsDir {
				roots = append(roots, ".")
			}
		}
	}
	roots = append(roots, dir)

	stem := libPathStem(libSym)
	for _, root := range roots {
		for _, ext := range []string{".clj", ".cljg"} {
			cand := filepath.Join(root, stem+ext)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				return cand
			}
		}
	}
	return ""
}

// evalLibFile is the interpreter's lib loader: read and evaluate the
// file form by form under a pushed *ns*/*file* frame (the same load
// frame as repl.Driver.EvalReader), so the file's in-ns is undone
// afterwards and *file* reads as the dep's path while it loads.
func evalLibFile(e *Evaluator, libSym *lang.Symbol, path string) {
	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Errorf("loading %s from %s: %w", libSym.FullName(), path, err))
	}
	defer f.Close()

	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, e.CurrentNS(),
		lang.VarFile, path,
	))
	defer lang.PopThreadBindings()

	rd := reader.New(bufio.NewReader(f), reader.WithFilename(path),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := rd.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("loading %s from %s: %w", libSym.FullName(), path, err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("loading %s from %s: %w", libSym.FullName(), path, err))
		}
	}
}
