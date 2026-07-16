//go:build (amd64 || arm64) && go1.26 && !go1.27

package goid

// Fast goroutine-ID lookup (ADR 0034). Technique (petermattis/goid's
// approach, written fresh for this repo — cljgo keeps zero external
// deps): a NOSPLIT assembly getg() returns the current goroutine's *g
// (the dedicated g register on arm64, the (TLS) slot on amd64), and Go
// code reads the g struct's goid field at an offset the COMPILER
// computes from gPrefix, a field-for-field mirror of the leading
// fields of runtime.g. The mirror below is transcribed from Go 1.26's
// $GOROOT/src/runtime/runtime2.go (verified against go1.26.3) and the
// build tag pins this file to exactly that toolchain window: a future
// Go 1.27 falls back to the stack-parse until the mirror is re-vetted.
//
// Why this is safe:
//   - The *g of a live goroutine is stable: g structs are never moved
//     (stack copying moves the stack, not the g), so a preemption or
//     M-migration between getg() and the field read cannot change
//     either the pointer or the goid stored behind it.
//   - goid is written once at goroutine creation and never mutated
//     while the goroutine runs; reading it without synchronization
//     from the goroutine itself is race-free.
//   - A wrong offset can never fail silently: init() below compares
//     the fast read against the stack-parse oracle on the running
//     toolchain and panics at process start on any mismatch — before
//     a single dynamic binding can be keyed by a bad ID.

import "unsafe"

// getg returns the current goroutine's g pointer. Implemented in
// getg_amd64.s / getg_arm64.s.
func getg() unsafe.Pointer

// gPrefix mirrors the leading fields of runtime.g up to and including
// goid, from Go 1.26 $GOROOT/src/runtime/runtime2.go. Field types are
// size-equivalent stand-ins (uintptr for runtime-internal pointers,
// uint32 for atomic.Uint32); only unsafe.Offsetof(gPrefix{}.goid) is
// ever used — no value of this type is ever created or dereferenced as
// a whole.
type gPrefix struct {
	stack       stackMirror // runtime.g.stack (type stack: lo, hi uintptr)
	stackguard0 uintptr
	stackguard1 uintptr

	_panic    uintptr     // *_panic
	_defer    uintptr     // *_defer
	m         uintptr     // *m
	sched     gobufMirror // gobuf
	syscallsp uintptr
	syscallpc uintptr
	syscallbp uintptr
	stktopsp  uintptr

	param        unsafe.Pointer
	atomicstatus uint32 // atomic.Uint32
	stackLock    uint32
	goid         uint64
}

// stackMirror mirrors runtime.stack.
type stackMirror struct {
	lo uintptr
	hi uintptr
}

// gobufMirror mirrors runtime.gobuf.
type gobufMirror struct {
	sp   uintptr
	pc   uintptr
	g    uintptr // guintptr
	ctxt unsafe.Pointer
	lr   uintptr
	bp   uintptr
}

const goidOffset = unsafe.Offsetof(gPrefix{}.goid)

// Get returns the current goroutine's ID by reading it directly off
// the g struct. No allocation, no stack capture. runtime.g.goid is a
// uint64 in Go 1.26; IDs are assigned monotonically from 1 and cannot
// realistically overflow int64, so the package's int64 contract holds.
func Get() int64 {
	return int64(*(*uint64)(unsafe.Add(getg(), goidOffset)))
}

func init() {
	// One-shot offset verification against the stack-parse oracle.
	// Costs one runtime.Stack call at package load; guarantees a wrong
	// gPrefix mirror is a loud boot-time panic, never silent
	// cross-goroutine binding corruption.
	if fast, slow := Get(), getSlow(); fast != slow {
		panic("goid: fast goroutine-ID lookup disagrees with runtime.Stack parse " +
			"(runtime.g layout changed?); rebuild with a vetted toolchain")
	}
}
