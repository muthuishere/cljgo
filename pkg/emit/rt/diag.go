package rt

import (
	"github.com/muthuishere/cljgo/pkg/diag"
)

// RenderRecovered turns a value recovered by the emitted func main()'s
// top-level defer into the same detailed error line the REPL and `cljgo
// run` print (spike s28 P0 boundary). Without this a runtime error in a
// compiled binary surfaces as a raw Go panic + goroutine stack trace — a
// completely different artifact from the interpreter's `error:` line, the
// single worst REPL-vs-binary divergence in the error surface.
//
// It normalizes the recovered value through corelib.Recover (the same
// normalization the interpreter's boundary uses, so a thrown non-error
// value renders identically), then diag.FromError + diag.Render. A Carrier
// error (e.g. a span-carrying arity error, once the emitter threads one)
// keeps its detail; a bare error renders its message plus, when the message
// classifies, an explain pointer.
func RenderRecovered(r any) string {
	return diag.Render(diag.FromError(Recover(r)))
}
