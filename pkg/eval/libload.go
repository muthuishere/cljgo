// libload.go — the INTERPRETER's half of `require` (ADR 0042; the
// libspec surface and the provider registry moved to pkg/corelib with
// ADR 0046, because a compiled binary replays requires too).
//
// When corelib's require names a namespace that is neither present nor
// registered by a provider, it calls the hook installed here:
//
//	a source file resolved relative to the REQUIRING file: root =
//	dir(*file*) minus the requiring ns's own directory suffix when it
//	matches (src/my_app/core.clj in ns my-app.core → src/), else
//	dir(*file*); candidates <root>/<lib path>.clj then .cljg, path
//	segments munged - → _ as on the JVM. The file loads through the
//	Evaluator's LibLoader — by default read + EvalForm under a pushed
//	*ns*/*file* frame (the interpreter); pkg/emit's module compiler
//	substitutes a loader that also CAPTURES the analyzed forms.
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

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

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
	corelib.PushLibLoading(name)
	defer corelib.PopLibLoading()
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
		// Most-specific-first: cljgo-native extensions win over the portable
		// `.clj` fallback (ADR 0051), mirroring Clojure's host-extension order.
		for _, ext := range []string{".cljgo", ".cljg", ".clj"} {
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
