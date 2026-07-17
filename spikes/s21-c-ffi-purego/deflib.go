package main

import (
	"fmt"
	"reflect"

	"github.com/ebitengine/purego"
)

// This file is the actual risk this spike exists to retire: S7 (docs/adr/0011,
// design/07-spikes.md) proved purego marshaling patterns from a Go program
// where every bound func was a STATIC `var f func(...)T` known at compile
// time (`purego.RegisterLibFunc(&f, lib, name)`). A cljgo interpreter does
// not have that — `(ffi/deflib sqlite "libsqlite3" (version [] :string) ...)`
// is read, analyzed and evaluated at runtime; the Go func TYPE for `version`
// does not exist as source until (if ever) AOT emission writes it out.
//
// So the REPL/interpreter path needs a fully dynamic construction:
//   1. build a reflect.Type for the C signature from the declared keywords
//   2. reflect.New a func of that type (addressable, gives a *T we can hand
//      to purego)
//   3. purego.RegisterFunc(ptr.Interface(), sym) fills in that dynamic func
//   4. call it later via reflect.Value.Call([]reflect.Value) — no Go source
//      naming the signature ever existed.
// This file proves steps 1-4 work identically to S7's static registration.

// FnDecl mirrors one clause of `(name "c_symbol" [arg-kinds...] ret-kind)`
// after deflib macro expansion has parsed it into data the evaluator can act
// on. Variadic C functions have no representation here — see Declare's check.
type FnDecl struct {
	CljName  string
	CSymbol  string
	Args     []Kind
	Ret      Kind
	Variadic bool // set true only to exercise the rejection path in main.go
}

// BoundFn is what evaluator-land holds after registration: a callable value
// (dynamically typed) plus enough metadata for arity/type errors to name the
// declaration, not just "reflect: panic".
type BoundFn struct {
	Decl FnDecl
	fn   reflect.Value // the *dynamically constructed* func value, non-addressable copy
}

// Call marshals []any (imagine: Clojure argument values already coerced to
// the right Go type by the analyzer per Decl.Args) into a reflect.Call.
// Arity is checked BEFORE reflect.Call so a wrong-arity ffi call fails with
// a cljgo-shaped error, never a runtime panic surfacing raw reflect text.
func (b *BoundFn) Call(args ...any) (result any, err error) {
	if len(args) != len(b.Decl.Args) {
		return nil, fmt.Errorf("ffi: %s/%s expects %d arg(s), got %d",
			"lib", b.Decl.CljName, len(b.Decl.Args), len(args))
	}
	in := make([]reflect.Value, len(args))
	for i, a := range args {
		want := goType(b.Decl.Args[i])
		v := reflect.ValueOf(a)
		if !v.Type().AssignableTo(want) {
			return nil, fmt.Errorf("ffi: %s/%s arg %d: expected %s, got %s",
				"lib", b.Decl.CljName, i, want, v.Type())
		}
		in[i] = v
	}
	// A bad C signature (declared type doesn't match the library's real
	// ABI) is NOT caught here — it is caught at the OS/hardware level as a
	// crash or garbage result. See VERDICT.md §3 "wrong signature".
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ffi: %s/%s call panicked (signature likely wrong for the C symbol): %v",
				"lib", b.Decl.CljName, r)
		}
	}()
	out := b.fn.Call(in)
	if b.Decl.Ret == KVoid {
		return nil, nil
	}
	return out[0].Interface(), nil
}

// Lib is the runtime value `ffi/deflib` binds a namespace to: a dlopen
// handle plus every declared function, dynamically registered.
type Lib struct {
	Name   string
	handle uintptr
	Fns    map[string]*BoundFn
}

// Declare dlopens libPath and registers every decl. This is the function an
// `ffi/deflib` macro expansion would call at eval time (interpreted) or that
// generated code would call once at package-init time (AOT) — see
// emit-sketch.md for the latter. Both paths funnel through this ONE function,
// which is the "identical in interpreted and AOT modes" claim from design/05.
func Declare(libName, libPath string, decls []FnDecl) (*Lib, error) {
	handle, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("ffi/deflib %s: dlopen %q failed: %w (declaration-time failure, no functions bound)", libName, libPath, err)
	}
	lib := &Lib{Name: libName, handle: handle, Fns: map[string]*BoundFn{}}
	for _, d := range decls {
		if d.Variadic {
			return nil, fmt.Errorf(
				"ffi/deflib %s: %s (%s) is declared variadic — rejected at expansion time per ADR 0011/S7: "+
					"purego's ...any splices Go args, it is NOT C varargs (confirmed broken on darwin/arm64, "+
					"silent wrong answers). Wrap this symbol in a small cgo or Go package instead.",
				libName, d.CljName, d.CSymbol)
		}
		sym, err := purego.Dlsym(handle, d.CSymbol)
		if err != nil {
			return nil, fmt.Errorf("ffi/deflib %s: symbol %q (fn %s) not found: %w (declaration-time failure)",
				libName, d.CSymbol, d.CljName, err)
		}
		in := make([]reflect.Type, len(d.Args))
		for i, k := range d.Args {
			in[i] = goType(k)
		}
		var out []reflect.Type
		if d.Ret != KVoid {
			out = []reflect.Type{goType(d.Ret)}
		}
		funcType := reflect.FuncOf(in, out, false)
		fnPtr := reflect.New(funcType) // addressable *func(...)...
		purego.RegisterFunc(fnPtr.Interface(), sym)
		lib.Fns[d.CljName] = &BoundFn{Decl: d, fn: fnPtr.Elem()}
	}
	return lib, nil
}
