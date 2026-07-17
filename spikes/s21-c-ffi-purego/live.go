package main

import "fmt"

// demoRedeclareLive is the headline claim from the task brief: purego dlopen
// works AT THE REPL, live, no rebuild. This spike cannot embed pkg/eval (that
// is the apply-time integration, tracked by the OpenSpec change), so it
// simulates the REPL by doing exactly what the interpreter's `(ffi/deflib ...)`
// special form would do on each evaluation: call Declare() again, in the same
// running process, and immediately use the new binding — proving there is no
// process-restart, no cache-invalidation, no link-time step in the way.
func demoRedeclareLive() {
	// "REPL turn 1": (ffi/deflib m1 "libm.dylib" (cos [:double] :double))
	m1, err := Declare("m1", darwinLibPath("libm.dylib", "libm.so.6"), []FnDecl{
		{CljName: "cos", CSymbol: "cos", Args: []Kind{KFloat64}, Ret: KFloat64},
	})
	must(err)
	v1, err := m1.Fns["cos"].Call(0.0)
	must(err)
	fmt.Printf("  turn 1: (m1/cos 0.0) => %v\n", v1)

	// "REPL turn 2": the user re-evaluates the SAME form with a DIFFERENT
	// C symbol bound to the same Clojure name — e.g. they typo'd and fix it,
	// or they're swapping libm's cos for libm's sin to see the difference.
	// No process restart between turn 1 and turn 2.
	m1redefined, err := Declare("m1", darwinLibPath("libm.dylib", "libm.so.6"), []FnDecl{
		{CljName: "cos", CSymbol: "sin", Args: []Kind{KFloat64}, Ret: KFloat64}, // rebound to sin
	})
	must(err)
	v2, err := m1redefined.Fns["cos"].Call(0.0)
	must(err)
	fmt.Printf("  turn 2 (re-declared, cos symbol now bound to C sin): (m1/cos 0.0) => %v\n", v2)
	if v1 == v2 {
		panic("expected turn 2 to answer differently after re-declaration")
	}
	fmt.Println("  same process, same var name, different answer after re-declare: REPL-live confirmed")
}
