package lang

import (
	"math"
	"math/big"
	"reflect"
)

// This file defines the two equality relations Clojure requires
// (clojure.lang.Util.equiv vs Util.equals):
//
//	Equiv  — Clojure `=`: nil-safe, numbers compared BY CATEGORY
//	         ((= 1 1.0) is false, (= 1 1N) is true), collections
//	         compared structurally via IPersistentCollection.Equiv
//	         (so (= [1 2] '(1 2)) is true). Map/set key lookup and
//	         contains?/get use this relation.
//	Equals — the Java Object.equals analog: type-strict for numbers
//	         (Equals(int32(1), int64(1)) is false), floats compared by
//	         bit pattern (Double.equals semantics), collections via
//	         their Java-shaped Equals methods. Used by interop and
//	         future .equals surfaces.
//
// (cljgo M0: replaces the vendored Glojure alias `Equiv = Equals`,
// design doc 02 §1.3 / S4 defect #1.)

// Equiv is Clojure `=` — a faithful port of clojure.lang.Util.equiv.
func Equiv(a, b any) bool {
	// Check functions first, because == panics on func comparison.
	aVal, bVal := reflect.ValueOf(a), reflect.ValueOf(b)
	if aVal.Kind() == reflect.Func || bVal.Kind() == reflect.Func {
		if !(aVal.Kind() == reflect.Func && bVal.Kind() == reflect.Func) {
			return false
		}
		return aVal.Pointer() == bVal.Pointer()
	}
	// Go maps/slices participate as seqable collections (the sanctioned
	// "any Go value is a Clojure value" interop path); == panics on them.
	if aVal.Kind() == reflect.Map || bVal.Kind() == reflect.Map || aVal.Kind() == reflect.Slice || bVal.Kind() == reflect.Slice {
		if aVal.Kind() != bVal.Kind() {
			return false
		}
		return Equiv(Seq(a), Seq(b))
	}

	if a == b {
		return true
	}

	aNil, bNil := IsNil(a), IsNil(b)
	if aNil && bNil {
		return true
	}
	if aNil || bNil {
		return false
	}

	// Numbers: category-based equality (Util.equiv → Numbers.equal).
	if IsNumber(a) {
		if !IsNumber(b) {
			return false
		}
		return NumbersEqual(a, b)
	}
	if IsNumber(b) {
		return false
	}

	// Collections: structural equiv (Util.equiv → pcequiv).
	if _, ok := a.(IPersistentCollection); ok {
		return pcEquiv(a, b)
	}
	if _, ok := b.(IPersistentCollection); ok {
		return pcEquiv(a, b)
	}

	// Everything else: Java k1.equals(k2).
	if a, ok := a.(Equalser); ok {
		return a.Equals(b)
	}
	if b, ok := b.(Equalser); ok {
		return b.Equals(a)
	}
	if a, ok := a.(Equiver); ok {
		return a.Equiv(b)
	}
	if b, ok := b.(Equiver); ok {
		return b.Equiv(a)
	}

	return false
}

// Equals is the Java Object.equals analog — a port of
// clojure.lang.Util.equals with Java's type-strict number semantics.
func Equals(a, b any) bool {
	// Check functions first, because == panics on func comparison.
	aVal, bVal := reflect.ValueOf(a), reflect.ValueOf(b)
	if aVal.Kind() == reflect.Func || bVal.Kind() == reflect.Func {
		if !(aVal.Kind() == reflect.Func && bVal.Kind() == reflect.Func) {
			return false
		}
		return aVal.Pointer() == bVal.Pointer()
	}
	// Go maps/slices as seqable collections; elements compared with
	// Equals (interop escape hatch, mirrors the Equiv branch above).
	if aVal.Kind() == reflect.Map || bVal.Kind() == reflect.Map || aVal.Kind() == reflect.Slice || bVal.Kind() == reflect.Slice {
		if aVal.Kind() != bVal.Kind() {
			return false
		}
		return Equals(Seq(a), Seq(b))
	}

	if a == b {
		return true
	}

	aNil, bNil := IsNil(a), IsNil(b)
	if aNil && bNil {
		return true
	}
	if aNil || bNil {
		return false
	}

	// Numbers: Java .equals is type-strict — Long(1).equals(Integer(1))
	// is false. Different dynamic types never compare equal.
	if IsNumber(a) || IsNumber(b) {
		if reflect.TypeOf(a) != reflect.TypeOf(b) {
			return false
		}
		switch x := a.(type) {
		case float64:
			// Double.equals: bit-pattern comparison (NaN equals NaN,
			// 0.0 does not equal -0.0).
			return math.Float64bits(x) == math.Float64bits(b.(float64))
		case float32:
			return math.Float32bits(x) == math.Float32bits(b.(float32))
		case *big.Int:
			return x.Cmp(b.(*big.Int)) == 0
		}
		if a, ok := a.(Equalser); ok { // *BigInt, *BigDecimal, *Ratio
			return a.Equals(b)
		}
		// Comparable primitives of identical type: equal values were
		// already caught by a == b above.
		return false
	}

	if a, ok := a.(Equalser); ok {
		return a.Equals(b)
	}
	if b, ok := b.(Equalser); ok {
		return b.Equals(a)
	}

	return false
}

func Identical(a, b any) bool {
	aVal, bVal := reflect.ValueOf(a), reflect.ValueOf(b)

	// check if comparing functions, because == panics on func comparison.
	if aVal.Kind() == reflect.Func || bVal.Kind() == reflect.Func {
		if !(aVal.Kind() == reflect.Func && bVal.Kind() == reflect.Func) {
			return false
		}
		return aVal.Pointer() == bVal.Pointer()
	}
	// slices
	if aVal.Kind() == reflect.Slice || bVal.Kind() == reflect.Slice {
		if !(aVal.Kind() == reflect.Slice && bVal.Kind() == reflect.Slice) {
			return false
		}
		return aVal.Pointer() == bVal.Pointer()
	}
	// arrays
	if aVal.Kind() == reflect.Array || bVal.Kind() == reflect.Array {
		if !(aVal.Kind() == reflect.Array && bVal.Kind() == reflect.Array) {
			return false
		}
		return aVal.Pointer() == bVal.Pointer()
	}
	// maps
	if aVal.Kind() == reflect.Map || bVal.Kind() == reflect.Map {
		if !(aVal.Kind() == reflect.Map && bVal.Kind() == reflect.Map) {
			return false
		}
		return aVal.Pointer() == bVal.Pointer()
	}

	return a == b
}

func pcEquiv(a, b any) bool {
	if a, ok := a.(IPersistentCollection); ok {
		return a.Equiv(b)
	}
	return b.(IPersistentCollection).Equiv(a)
}
