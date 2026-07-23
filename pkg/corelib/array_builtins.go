package corelib

import (
	"fmt"
	"reflect"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internArrayBuiltins interns the array constructors + accessors of ADR
// 0022 Batch 4 (cheap breadth): to-array/object-array/int-array/long-array/
// float-array/double-array/boolean-array/char-array/into-array, plus
// aget/aset/alength/aclone. Wired into internBuiltins by ONE line
// (e.internArrayBuiltins(def)), per the merge-friendly discipline.
//
// ADR 0025: a cljgo "array" is a plain native Go slice ([]any for
// object/to-array, []int64 for int/long-array, []float32/[]float64 for
// float/double-array, []bool for boolean-array, []lang.Char for
// char-array). No new interop glue is needed to make these useful — Seq,
// Nth, Get, ToString/Print, and Go-interop argument coercion
// (coerceGoValue, pkg/lang/apply.go) already special-case
// reflect.Slice/reflect.Array generically, so a cljgo array participates
// in seq/count/get and marshals across the Go interop boundary exactly
// like any other Go slice. aget/aset/alength/aclone require their
// argument to actually BE a reflect.Slice/reflect.Array (matching the
// oracle: `(aget [1 2 3] 0)` throws on real Clojure 1.12.5 too, since
// aget only expands onto real array classes, never persistent vectors).
func internArrayBuiltins(def func(string, func(...any) any) *lang.Var) {
	// to-array: (to-array coll) => []any. lang.ToSlice already handles
	// nil, vectors, maps, sets, strings, ISeqs (incl. lazy/range), and the
	// reflect.Slice/Array fallback (so `(to-array (to-array [1 2]))` also
	// works, matching the oracle: to-array on an array is a no-op copy).
	def("to-array", func(args ...any) any {
		return lang.ToSlice(oneArg("to-array", args))
	})

	// object-array: (object-array n) => n nils; (object-array coll) =>
	// []any of coll's elements. Unlike the typed ctors below, real Clojure
	// has NO (object-array n init) 2-arg form (verified: ArityException).
	def("object-array", func(args ...any) any {
		x := oneArg("object-array", args)
		if n, ok := lang.AsInt(x); ok {
			return make([]any, n)
		}
		return lang.ToSlice(x)
	})

	// int-array / long-array: cljgo's numeric tower has one fixnum
	// representation (int64 — design/08 §5 Batch 2), so both ctors
	// produce the same Go type ([]int64). Documented divergence from the
	// JVM (ADR 0025): there is no narrower `int` value to make a
	// genuinely distinct int-array.
	intArrayCtor := func(op string) func(args ...any) any {
		return func(args ...any) any {
			return typedArray(op, args, lang.LongCast, int64(0))
		}
	}
	def("int-array", intArrayCtor("int-array"))
	def("long-array", intArrayCtor("long-array"))

	def("float-array", func(args ...any) any {
		return typedArray("float-array", args, lang.FloatCast, float32(0))
	})
	def("double-array", func(args ...any) any {
		return typedArray("double-array", args, lang.AsFloat64, float64(0))
	})
	def("boolean-array", func(args ...any) any {
		return typedArray("boolean-array", args, lang.BooleanCast, false)
	})
	def("char-array", func(args ...any) any {
		return typedArray("char-array", args, lang.CharCast, lang.Char(0))
	})

	// into-array: 1-arg is a cheap, type-inferring approximation of the
	// JVM's RT.into-array (which picks the array's component class from
	// the first element's class, defaulting to Object[] when empty).
	// 2-arg (ADR 0036 follow-on, cljgo-test-suite reduce.cljc): the first
	// arg is an explicit type hint — a ClassRef, interned by ADR 0036's
	// well-known-class table (`Long`, `Integer`, `Double`, `Float`,
	// `Boolean`, `Object`, …), never a real Class. Maps to the matching
	// Go slice kind (ADR 0025); a ClassRef outside that small mapping —
	// or any non-ClassRef value — is a clear, fail-closed error rather
	// than a silent guess.
	def("into-array", func(args ...any) any {
		switch len(args) {
		case 1:
			return intoArray(args[0])
		case 2:
			return intoArrayTyped(args[0], args[1])
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: into-array", len(args)))
		}
	})

	// alength: (alength arr) => its length. Arrays only (oracle: throws on
	// a persistent vector).
	def("alength", func(args ...any) any {
		return int64(arrayReflectValue("alength", oneArg("alength", args)).Len())
	})

	// aget: (aget arr idx) => element at idx.
	def("aget", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: aget", len(args)))
		}
		v := arrayReflectValue("aget", args[0])
		idx, ok := lang.AsInt(args[1])
		if !ok {
			panic(fmt.Errorf("aget: index must be an integer, got: %s", lang.PrintString(args[1])))
		}
		if idx < 0 || idx >= v.Len() {
			panic(fmt.Errorf("aget: index %d out of bounds for length %d", idx, v.Len()))
		}
		return v.Index(idx).Interface()
	})

	// aset: (aset arr idx val) => val, mutating arr in place (a real Go
	// slice header over the same backing array — ADR 0025).
	def("aset", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: aset", len(args)))
		}
		v := arrayReflectValue("aset", args[0])
		idx, ok := lang.AsInt(args[1])
		if !ok {
			panic(fmt.Errorf("aset: index must be an integer, got: %s", lang.PrintString(args[1])))
		}
		if idx < 0 || idx >= v.Len() {
			panic(fmt.Errorf("aset: index %d out of bounds for length %d", idx, v.Len()))
		}
		lang.SliceSet(args[0], idx, args[2])
		return args[2]
	})

	// aclone: (aclone arr) => a shallow copy (independent backing array).
	def("aclone", func(args ...any) any {
		v := arrayReflectValue("aclone", oneArg("aclone", args))
		cp := reflect.MakeSlice(reflect.SliceOf(v.Type().Elem()), v.Len(), v.Len())
		reflect.Copy(cp, v)
		return cp.Interface()
	})

	// byte-array / short-array (batch A3, completing the ADR 0025 family):
	// []int8 / []int16 — int8 matches the JVM's SIGNED byte exactly
	// (oracle 1.12.5: (vec (byte-array [1 -1])) => [1 -1]; Go's unsigned
	// `byte` would report 255). Elements coerce with the UNCHECKED casts
	// because the JVM fills these arrays via Number.byteValue()/
	// .shortValue(), which WRAP rather than throw (oracle: (vec
	// (byte-array [200])) => [-56], even though (byte 200) itself
	// throws). Documented divergence (same shape as int-array ==
	// long-array): the JVM's 2-arity demands a Byte/Short-typed
	// init-val and mis-treats a Long init as a seq (throws "Don't know
	// how to create ISeq from: java.lang.Long"); cljgo's tower has ONE
	// fixnum (int64), so (byte-array 3 7) fills with 7.
	def("byte-array", func(args ...any) any {
		return typedArray("byte-array", args, lang.UncheckedByteCast, int8(0))
	})
	def("short-array", func(args ...any) any {
		return typedArray("short-array", args, lang.UncheckedShortCast, int16(0))
	})

	// bytes?: true only for a byte array — []int8 (what byte-array
	// builds) or Go-native []byte (what Go interop hands back): both are
	// honest byte arrays on this host; every other array kind is false,
	// as on the JVM (bytes? is exactly byte[]).
	// oracle 1.12.5: [(bytes? (byte-array 1)) (bytes? (int-array 1))
	// (bytes? "s") (bytes? nil)] => [true false false false]
	def("bytes?", func(args ...any) any {
		switch oneArg("bytes?", args).(type) {
		case []int8, []byte:
			return true
		}
		return false
	})

	// make-array: (make-array class dim & more-dims) => a nil-filled
	// []any per dimension (nested for multi-dim), whatever the
	// well-known class (ADR 0036 ClassRef): the JVM's make-array over a
	// CLASS builds an OBJECT array of nulls — (vec (make-array Long 3))
	// => [nil nil nil], (vec (map vec (make-array Object 2 3))) =>
	// [[nil nil nil] [nil nil nil]], oracle 1.12.5. Primitive component
	// types (Integer/TYPE) do not resolve in cljgo (fail-closed, ADR
	// 0036) — the typed ctors (int-array & co) are the typed path.
	def("make-array", func(args ...any) any {
		if len(args) < 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: make-array", len(args)))
		}
		if _, ok := args[0].(*ClassRef); !ok {
			panic(fmt.Errorf("make-array: not a class: %s", lang.PrintString(args[0])))
		}
		dims := make([]int, len(args)-1)
		for i, a := range args[1:] {
			n, ok := lang.AsInt(a)
			if !ok || n < 0 {
				panic(fmt.Errorf("make-array: dimension must be a non-negative integer, got: %s", lang.PrintString(a)))
			}
			dims[i] = n
		}
		var build func(ds []int) []any
		build = func(ds []int) []any {
			out := make([]any, ds[0])
			if len(ds) > 1 {
				for i := range out {
					out[i] = build(ds[1:])
				}
			}
			return out
		}
		return build(dims)
	})
}

// typedArray builds a typed cljgo array from one of the three ctor shapes
// Clojure supports for a typed array (int-array/float-array/etc, all but
// object-array): (ctor n) => n zero-valued elements; (ctor coll) => coll's
// elements coerced via `coerce`; (ctor n init) => n copies of init
// (coerced). T is the array's Go element type; zero is its zero value.
func typedArray[T any](op string, args []any, coerce func(any) T, zero T) []T {
	switch len(args) {
	case 1:
		if n, ok := lang.AsInt(args[0]); ok {
			return make([]T, n)
		}
		src := lang.ToSlice(args[0])
		out := make([]T, len(src))
		for i, x := range src {
			out[i] = coerce(x)
		}
		return out
	case 2:
		n, ok := lang.AsInt(args[0])
		if !ok {
			panic(fmt.Errorf("%s: size must be an integer, got: %s", op, lang.PrintString(args[0])))
		}
		init := coerce(args[1])
		out := make([]T, n)
		for i := range out {
			out[i] = init
		}
		return out
	default:
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
}

// intoArray infers an element type from coll's first element (a cheap
// approximation of RT.into-array's per-element-class inference — ADR
// 0025), falling back to []any for an empty coll or a mixed/unrecognized
// element type.
func intoArray(coll any) any {
	src := lang.ToSlice(coll)
	if len(src) == 0 {
		return []any{}
	}
	switch src[0].(type) {
	case int64:
		return typedArray("into-array", []any{coll}, lang.LongCast, int64(0))
	case float64:
		return typedArray("into-array", []any{coll}, lang.AsFloat64, float64(0))
	case bool:
		return typedArray("into-array", []any{coll}, lang.BooleanCast, false)
	case lang.Char:
		return typedArray("into-array", []any{coll}, lang.CharCast, lang.Char(0))
	case string:
		out := make([]string, len(src))
		for i, x := range src {
			s, ok := x.(string)
			if !ok {
				return append([]any{}, src...)
			}
			out[i] = s
		}
		return out
	default:
		return append([]any{}, src...)
	}
}

// intoArrayTyped backs 2-arg into-array: typeHint must be a ClassRef (ADR
// 0036) naming one of the well-known primitive-wrapper/Object classes;
// the coll's elements are coerced to the matching Go slice kind (ADR
// 0025), mirroring the 1-arity typed ctors (int-array/long-array/etc)
// rather than guessing from the first element.
func intoArrayTyped(typeHint, coll any) any {
	cr, ok := typeHint.(*ClassRef)
	if !ok {
		panic(fmt.Errorf("into-array: not a class: %s", lang.PrintString(typeHint)))
	}
	switch cr.Name() {
	case "java.lang.Object":
		return lang.ToSlice(coll)
	case "java.lang.Long", "java.lang.Integer", "java.lang.Short", "java.lang.Byte":
		return typedArray("into-array", []any{coll}, lang.LongCast, int64(0))
	case "java.lang.Float":
		return typedArray("into-array", []any{coll}, lang.FloatCast, float32(0))
	case "java.lang.Double":
		return typedArray("into-array", []any{coll}, lang.AsFloat64, float64(0))
	case "java.lang.Boolean":
		return typedArray("into-array", []any{coll}, lang.BooleanCast, false)
	case "java.lang.Character":
		return typedArray("into-array", []any{coll}, lang.CharCast, lang.Char(0))
	default:
		panic(fmt.Errorf("into-array: unsupported type: %s", cr.Name()))
	}
}

// arrayReflectValue asserts x is a real Go slice/array (a cljgo array,
// ADR 0025) and returns its reflect.Value — aget/aset/alength/aclone all
// reject persistent vectors and other collections, matching the oracle
// (real Clojure's aget/aset/alength/aclone only accept actual arrays).
func arrayReflectValue(op string, x any) reflect.Value {
	if x == nil {
		panic(fmt.Errorf("%s: not an array: nil", op))
	}
	v := reflect.ValueOf(x)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		panic(fmt.Errorf("%s: not an array: %s", op, lang.PrintString(x)))
	}
	return v
}
