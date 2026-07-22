// The AOT shape from ADR 0044 decision 1 / S21's emit-sketch.go.txt: a
// package-level var + RegisterLibFunc in init(). Here the library is
// ABSENT, which is what a consumer on a machine lacking the dependency's
// C library gets. Measures what the user actually sees.
package main

import (
	"fmt"

	"github.com/ebitengine/purego"
)

var depFn func() int32

func init() {
	// emitted for a dependency's (ffi/deflib ...) — the consumer wrote none
	// of this and does not know this library exists.
	h, err := purego.Dlopen("libabsent_dep_s27.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		// Even the polite shape can only panic: init() has no error return.
		panic(err)
	}
	purego.RegisterLibFunc(&depFn, h, "some_fn")
}

func main() { fmt.Println("main never runs:", depFn()) }
