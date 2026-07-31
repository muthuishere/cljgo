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
