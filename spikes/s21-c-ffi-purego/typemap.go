package main

import "reflect"

// Kind is the ffi/deflib type keyword vocabulary this spike prototypes.
// This is a strict subset of design/05 §1.2's table, enough to prove the
// dynamic-registration mechanism; the full table (:ptr!out, :cstr!out,
// :callback, :rc) is documented in VERDICT.md but not all re-implemented
// here since S7 already proved those marshaling patterns concretely.
type Kind int

const (
	KString Kind = iota
	KInt32
	KInt64
	KUintptr // :ptr — opaque handle / void*
	KFloat64
	KBool
	KVoid
)

// goType is the ONLY place a deflib type keyword becomes a concrete Go
// reflect.Type. Everything downstream (dynamic func construction, purego
// registration) is generic over this mapping.
func goType(k Kind) reflect.Type {
	switch k {
	case KString:
		return reflect.TypeOf("")
	case KInt32:
		return reflect.TypeOf(int32(0))
	case KInt64:
		return reflect.TypeOf(int64(0))
	case KUintptr:
		return reflect.TypeOf(uintptr(0))
	case KFloat64:
		return reflect.TypeOf(float64(0))
	case KBool:
		return reflect.TypeOf(false)
	default:
		panic("goType: unmapped kind")
	}
}
