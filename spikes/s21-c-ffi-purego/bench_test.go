package main

import (
	"math"
	"testing"

	"github.com/ebitengine/purego"
)

// Three ways to compute cos(x) via libm, plus the pure-Go stdlib baseline:
//
//   BenchmarkPureGo        — math.Cos, no FFI at all (the floor)
//   BenchmarkPuregoStatic  — S7-style: purego.RegisterLibFunc into a
//                            compile-time-typed `func(float64) float64` var
//                            (what AOT emission does, see emit-sketch.go.txt)
//   BenchmarkPuregoDynamic — this spike's deflib.go: reflect.FuncOf +
//                            reflect.New + purego.RegisterFunc, called
//                            through BoundFn.Call([]any) with arity/type
//                            checks (what the interpreter does)
//
// cgo's baseline lives in cgobench/ (separate module — see its README) since
// it needs CGO_ENABLED=1 and this module intentionally builds CGO_ENABLED=0
// to prove the purego path needs no C toolchain (design/05 §1.2 table).

var staticCos func(float64) float64

func setupStatic() {
	if staticCos != nil {
		return
	}
	lib, err := purego.Dlopen(darwinLibPath("libm.dylib", "libm.so.6"), purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&staticCos, lib, "cos")
}

var dynamicLib *Lib

func setupDynamic() {
	if dynamicLib != nil {
		return
	}
	lib, err := Declare("libm_bench", darwinLibPath("libm.dylib", "libm.so.6"), []FnDecl{
		{CljName: "cos", CSymbol: "cos", Args: []Kind{KFloat64}, Ret: KFloat64},
	})
	if err != nil {
		panic(err)
	}
	dynamicLib = lib
}

func BenchmarkPureGo(b *testing.B) {
	x := 0.5
	for i := 0; i < b.N; i++ {
		x = math.Cos(x)
	}
	sink = x
}

func BenchmarkPuregoStatic(b *testing.B) {
	setupStatic()
	x := 0.5
	for i := 0; i < b.N; i++ {
		x = staticCos(x)
	}
	sink = x
}

func BenchmarkPuregoDynamic(b *testing.B) {
	setupDynamic()
	fn := dynamicLib.Fns["cos"]
	x := 0.5
	for i := 0; i < b.N; i++ {
		v, err := fn.Call(x)
		if err != nil {
			b.Fatal(err)
		}
		x = v.(float64)
	}
	sink = x
}

var sink float64
