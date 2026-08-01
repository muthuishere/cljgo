// depsedn.go — read `:paths` from a Clojure `deps.edn` (ADR 0119).
//
// A dual-host library targets JVM Clojure AND cljgo from one .cljc tree, and
// on the JVM its source roots are declared in deps.edn. Such a project has no
// reason to also carry a build.cljgo — it publishes to Clojars with
// tools.build and is consumed as a library — so before this, cljgo had no
// declared source roots for it at all. Its tests still passed, because
// `cljgo run src/foo.cljc` resolves relative to the file being run, and that
// is how the failure stayed invisible: everything worked except opening a
// REPL at the project root, where there is no requiring file to be relative
// to (issue #185's remaining half, found against koine).
//
// This is deliberately the NARROWEST possible reading:
//
//   - `:paths` only. Not `:deps`, not `:aliases`, not `:extra-paths`, not
//     `:replace-paths`. Those belong to tools.deps, which resolves them with
//     alias semantics cljgo does not implement; reading them halfway would
//     produce a load path that agrees with neither host.
//   - Only a vector of strings. tools.deps also allows a map (path aliases);
//     that form is skipped rather than guessed at.
//   - Only when the directory has NO cljgo build file. A build.cljgo is
//     cljgo's own, more specific declaration and always wins (ADR 0055's
//     most-specific-first, applied to the project description rather than the
//     file extension).
//
// The precedent is let-go, which folds deps.edn `:paths` into its search path
// as a fallback when no explicit flag or env var is given
// (references/let-go `pkg/resolver/resolver.go:65-99`). The precedence
// principle points the same way: deps.edn is Clojure's file, not ours, so the
// right move is to honour the part we can honour exactly, and to keep quiet
// about the rest rather than inventing a cljgo dialect of it.
package deps

import (
	"os"
	"path/filepath"
)

// DepsEDNFileName is the Clojure project description cljgo reads `:paths`
// from when a project declares no cljgo build file of its own.
const DepsEDNFileName = "deps.edn"

// DepsEDNPaths returns the source roots declared by `:paths` in dir's
// deps.edn, resolved against dir. It returns nil when there is no deps.edn,
// when it cannot be read, or when `:paths` is absent or not a vector of
// strings.
//
// Errors are deliberately swallowed into nil: a malformed or exotic deps.edn
// belongs to the JVM toolchain, and cljgo failing to start because of a form
// it was never going to use would be worse than cljgo simply not learning
// anything from it. Nothing here can make resolution WRONG — it can only add
// roots the project itself declared.
func DepsEDNPaths(dir string) []string {
	form, err := readEDNFile(filepath.Join(dir, DepsEDNFileName))
	if err != nil {
		return nil
	}
	var roots []string
	for _, p := range ednStrs(ednGet(form, "paths")) {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(dir, p)
		}
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			roots = append(roots, abs)
		}
	}
	return roots
}
