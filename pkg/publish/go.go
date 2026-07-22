// go.go — the `publish go` producer (ADR 0050 dec 1, target table).
//
// A cljgo library reaches the Go ecosystem as a go-gettable module: the same
// per-namespace Go packages `cljgo build` emits, LIBRARY-shaped (registered
// Load packages, no main()), plus an entry-package wrappers.go exposing each
// exported defn as an exported Go function. Go interop is ALLOWED here (unlike
// clojars) — Go is the host.
//
// Scope (honest, ADR 0050 risk note): exported wrappers use the `any`-typed
// calling convention. Resolved-from-type-hint Go signatures are DEFERRED — the
// emitted module compiles and is go-gettable, the signatures are uniformly
// `any`. See emit.WriteLibrary for the full scope statement.
package publish

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/emit"
)

// GoResult reports what a `publish go` produced, for the CLI to surface.
type GoResult struct {
	Module  string
	EntryNS string
	Exports []emit.LibExport
}

// PublishGo compiles entrySrc and emits a go-gettable library module under
// outDir. It returns a GoResult describing the emitted surface. The exported
// surface is validated to be Go-expressible by the emitter (the per-namespace
// Go emission fails with file:line on an inexpressible form).
func PublishGo(entrySrc, outDir string, opts ...Opt) (*GoResult, error) {
	s := resolve(opts)

	prog, err := emit.CompileProgram(entrySrc)
	if err != nil {
		return nil, fmt.Errorf("publish go: compiling %s: %w", entrySrc, err)
	}

	emitOpts := emit.Options{
		ModuleName:   s.module,
		RuntimeDir:   s.runtimeDir,
		HostFactsDir: outDir, // resolve stdlib host facts against the emitted module
	}
	entryNS, exports, err := emit.WriteLibrary(outDir, prog, emitOpts)
	if err != nil {
		return nil, fmt.Errorf("publish go: emitting library: %w", err)
	}

	module := s.module
	if module == "" {
		module = "cljgo.gen/lib"
	}
	return &GoResult{Module: module, EntryNS: entryNS, Exports: exports}, nil
}
