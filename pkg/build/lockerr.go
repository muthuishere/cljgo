package build

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/diag"
)

// noLockError is a coded diagnostic (G5023) for the fresh-clone trap
// (issue #168): `cljgo run` on a project that declares dependencies but was
// never built gets a bare "could not locate namespace" failure from the
// require that follows, because nothing was ever resolved onto the load
// path. `cljgo run` is deliberately never allowed to write a lock (that
// decision stands — see resolveRunDeps in cmd/cljgo/main.go), so the fix is
// naming the real cause instead of letting the reader chase a phantom
// classpath or typo. Implements diag.Carrier so diag.Render/RenderError
// pick up the code and the `help:` fix in every context (REPL, `cljgo run`,
// --json).
type noLockError struct{ d diag.Diagnostic }

func (e *noLockError) Error() string { return diag.Render(e.d) }

// Diagnostic implements diag.Carrier.
func (e *noLockError) Diagnostic() (diag.Diagnostic, bool) { return e.d, true }

// Code returns the diagnostic's registered error code.
func (e *noLockError) Code() string { return e.d.ErrorCode }

// ErrNoLock builds the G5023 diagnostic for buildFile, which declares n
// dependencies but has no build.lock.edn next to it.
func ErrNoLock(buildFile string, n int) error {
	dep := "dependency"
	if n != 1 {
		dep = "dependencies"
	}
	return &noLockError{d: diag.Diagnostic{
		ErrorCode: "G5023",
		Severity:  diag.SeverityError,
		Message: fmt.Sprintf(
			"%s declares %d %s but has no build.lock.edn", buildFile, n, dep),
		Fixes: []diag.Fix{{
			Title: "run `cljgo build` once to resolve and pin them, then `cljgo run` works",
		}},
	}}
}

// noBuildFnError is a coded diagnostic (G5025) for a build file that defines
// no `build` entry point. Without it, LoadPlan evaluated its internal driver
// string anyway and surfaced the interpreter's raw failure to resolve the
// symbol:
//
//	error: evaluating build fn: compiler error at <build-driver>:1:38:
//	unable to resolve symbol: build in this context
//
// Every part of that is unusable to the person reading it. `<build-driver>`
// is a string cljgo synthesizes, so it names a file that does not exist and
// gives a column INTO that nonexistent file; the real file — the build.cljgo
// they actually wrote — is never mentioned; and "unable to resolve symbol"
// describes cljgo's internals rather than the one thing they need to know,
// which is that a `(defn build [b] …)` is missing.
//
// Found by koine while black-box bisecting issue #176: stripping the require
// from a root build.clj left this error behind, which is how it surfaced as a
// defect in its own right rather than an artifact of that bug.
type noBuildFnError struct{ d diag.Diagnostic }

func (e *noBuildFnError) Error() string { return diag.Render(e.d) }

// Diagnostic implements diag.Carrier.
func (e *noBuildFnError) Diagnostic() (diag.Diagnostic, bool) { return e.d, true }

// Code returns the diagnostic's registered error code.
func (e *noBuildFnError) Code() string { return e.d.ErrorCode }

// ErrNoBuildFn builds the G5025 diagnostic for a build file with no `build`
// entry point.
func ErrNoBuildFn(buildFile string) error {
	return &noBuildFnError{d: diag.Diagnostic{
		ErrorCode: "G5025",
		Severity:  diag.SeverityError,
		Message: fmt.Sprintf(
			"%s defines no build entry point", buildFile),
		Expected: "(defn build [b] …)",
		Found:    "no `build` var in the build file",
		Fixes: []diag.Fix{{
			Title: "add `(defn build [b] …)` — it receives the builder and declares the artifacts",
		}},
	}}
}
