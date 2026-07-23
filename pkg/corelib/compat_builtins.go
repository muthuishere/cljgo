package corelib

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// compat_builtins.go — the tail-wave compat surface (fundamentals "all
// complete", 2026-07-23): the typed-array micro-API (aset-<type> +
// the array casts booleans/bytes/chars/doubles/floats/ints/longs/shorts +
// to-array-2d), `cast`, `iterator-seq`/`enumeration-seq` over the Go
// host's iterator shapes, and `bean` over Go struct reflection.
// Registered into internBuiltins by ONE line (internCompatBuiltins(def)),
// per the merge-friendly discipline.
//
// Oracle (JVM Clojure 1.12.5, 2026-07-23, scratch tailwave/o2.clj+o3.clj):
//
//	(let [a (int-array 3)] [(aset-int a 0 7) (vec a)]) => [7 [7 0 0]]
//	(let [a (int-array [0 0])] (aset-int a 0 1.5) (vec a)) => [1 0]
//	  — aset-<type> RETURNS the original val and STORES the (type val)
//	    checked cast, exactly the JVM's def-aset expansion.
//	(vec (ints (int-array [1 2]))) => [1 2]; (let [a (int-array [1])]
//	  (identical? a (ints a))) => true — the cast returns the SAME array.
//	(booleans nil) => nil (a null reference casts to any array type).
//	(ints (long-array [1])) THROWS ClassCastException on the JVM.
//	  DEVIATION (documented, ADR 0025): cljgo's int-array and long-array
//	  are both []int64 (one fixnum representation), so ints/longs accept
//	  either; likewise bytes accepts []int8 AND Go-native []byte.
//	(vec (map vec (to-array-2d [[1 2] [3]]))) => [[1 2] [3]]
//	(to-array-2d [1 2]) throws "Unable to convert: ... to Object[]"
//	(cast Long 5) => 5; (cast Long nil) => nil;
//	(cast String 5) throws ClassCastException
//	  "Cannot cast java.lang.Long to java.lang.String"
//	(iterator-seq (.iterator [1 2 3])) => (1 2 3); empty iterator => nil
//	(enumeration-seq (java.util.Collections/enumeration [1 2])) => (1 2)
//	(bean o) on the JVM reflects a Java bean's getters into a map.
//
// DEVIATIONS (documented — the cljgo-truthful receiver sets):
//   - iterator-seq accepts (a) any Go value with a HasNext() bool /
//     Next() any method pair (the java-shaped Go iterator), (b) a
//     *lang.Channel or raw Go channel (the host's natural iterator:
//     the seq takes until the channel closes). There is no
//     java.util.Iterator on a Go host.
//   - enumeration-seq accepts HasMoreElements()/NextElement() pairs and
//     falls back to the same receiver set as iterator-seq — Go has no
//     java.util.Enumeration.
//   - bean reflects a GO STRUCT (or pointer to one): exported fields
//     become kebab-cased keyword keys (RawQuery -> :raw-query),
//     read-only snapshot, no :class entry, unexported fields skipped —
//     there are no JavaBean getters on a Go host.
func internCompatBuiltins(def func(string, func(...any) any) *lang.Var) {
	// ---- aset-<type>: aset + the checked (type val) element cast. Each
	// returns the ORIGINAL val (the JVM def-aset contract) and stores the
	// coerced element; lang.SliceSet converts the coerced value to the
	// target slice's element type.
	asetTyped := func(op string, coerce func(any) any) func(args ...any) any {
		return func(args ...any) any {
			if len(args) != 3 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
			}
			v := arrayReflectValue(op, args[0])
			idx, ok := lang.AsInt(args[1])
			if !ok {
				panic(fmt.Errorf("%s: index must be an integer, got: %s", op, lang.PrintString(args[1])))
			}
			if idx < 0 || idx >= v.Len() {
				panic(fmt.Errorf("%s: index %d out of bounds for length %d", op, idx, v.Len()))
			}
			lang.SliceSet(args[0], idx, coerce(args[2]))
			return args[2]
		}
	}
	def("aset-boolean", asetTyped("aset-boolean", func(x any) any { return lang.BooleanCast(x) }))
	def("aset-byte", asetTyped("aset-byte", func(x any) any { return lang.ByteCast(x) }))
	def("aset-char", asetTyped("aset-char", func(x any) any { return lang.CharCast(x) }))
	def("aset-double", asetTyped("aset-double", func(x any) any { return lang.AsFloat64(x) }))
	def("aset-float", asetTyped("aset-float", func(x any) any { return lang.FloatCast(x) }))
	def("aset-int", asetTyped("aset-int", func(x any) any { return int64(lang.IntCast(x)) }))
	def("aset-long", asetTyped("aset-long", func(x any) any { return lang.LongCast(x) }))
	def("aset-short", asetTyped("aset-short", func(x any) any { return lang.ShortCast(x) }))

	// ---- array casts: return the argument unchanged when it already IS
	// the matching array kind (JVM: the cast is a no-op on the same
	// array), nil for nil, ClassCastException otherwise.
	arrayCast := func(op string, want string, matches func(any) bool) func(args ...any) any {
		return func(args ...any) any {
			x := oneArg(op, args)
			if x == nil {
				return nil
			}
			if matches(x) {
				return x
			}
			panic(lang.NewClassCastError("", fmt.Sprintf("Cannot cast %T to %s", x, want)))
		}
	}
	def("booleans", arrayCast("booleans", "[]bool", func(x any) bool { _, ok := x.([]bool); return ok }))
	def("bytes", arrayCast("bytes", "[]int8", func(x any) bool {
		switch x.(type) {
		case []int8, []byte:
			return true
		}
		return false
	}))
	def("chars", arrayCast("chars", "[]char", func(x any) bool { _, ok := x.([]lang.Char); return ok }))
	def("doubles", arrayCast("doubles", "[]float64", func(x any) bool { _, ok := x.([]float64); return ok }))
	def("floats", arrayCast("floats", "[]float32", func(x any) bool { _, ok := x.([]float32); return ok }))
	longsCast := func(op string) func(args ...any) any {
		return arrayCast(op, "[]int64", func(x any) bool { _, ok := x.([]int64); return ok })
	}
	def("ints", longsCast("ints"))
	def("longs", longsCast("longs"))
	def("shorts", arrayCast("shorts", "[]int16", func(x any) bool { _, ok := x.([]int16); return ok }))

	// to-array-2d: a collection of collections -> an array of []any rows
	// (the JVM's Object[][]). Each row must itself be seqable; a non-coll
	// row throws, message shaped like RT.toArray's.
	def("to-array-2d", func(args ...any) any {
		coll := oneArg("to-array-2d", args)
		var out []any
		for s := lang.Seq(coll); s != nil; s = s.Next() {
			row := s.First()
			switch row.(type) {
			case nil, string, int64, int, float64, bool, lang.Char, lang.Keyword, *lang.Symbol:
				// The RT.toArray message, class-named like the JVM's
				// ("Unable to convert: class java.lang.Long to Object[]").
				panic(fmt.Errorf("Unable to convert: class %s to Object[]", castClassName(row)))
			}
			out = append(out, lang.ToSlice(row))
		}
		if out == nil {
			out = []any{}
		}
		return out
	})

	// cast: x when x is an instance of class c (nil casts to anything),
	// else ClassCastException — message byte-shaped like the JVM's
	// Class.cast ("Cannot cast <found> to <wanted>"). c is a well-known
	// ClassRef (ADR 0036) or a deftype/defrecord's *TypeMarker; instance
	// checks ride the SAME machinery as instance? (classNameMatchesValue /
	// dispatchKey), so cast and instance? can never disagree.
	def("cast", func(args ...any) any {
		c, x := twoArgs("cast", args)
		if x == nil {
			return nil
		}
		var wanted string
		ok := false
		switch cl := c.(type) {
		case *ClassRef:
			wanted = cl.Name()
			ok = classNameMatchesValue(cl.Name(), x)
		case *TypeMarker:
			wanted = cl.name
			ok = dispatchKey(x) == cl.name
		default:
			panic(fmt.Errorf("cast: not a class: %s", lang.PrintString(c)))
		}
		if !ok {
			panic(lang.NewClassCastError("", fmt.Sprintf("Cannot cast %s to %s", castClassName(x), wanted)))
		}
		return x
	})

	// iterator-seq / enumeration-seq: a lazy seq over the host's iterator
	// shapes (receiver sets in the header DEVIATIONS note). nil -> nil,
	// like the JVM's RT.chunkIteratorSeq on an empty iterator.
	def("iterator-seq", func(args ...any) any {
		return hostIteratorSeq("iterator-seq", oneArg("iterator-seq", args),
			"HasNext", "Next")
	})
	def("enumeration-seq", func(args ...any) any {
		return hostIteratorSeq("enumeration-seq", oneArg("enumeration-seq", args),
			"HasMoreElements", "NextElement")
	})

	// bean: a read-only map view of a Go struct's exported fields,
	// kebab-cased keywords for keys (the cljgo-truthful analogue of the
	// JVM's JavaBean getter reflection — DEVIATIONS note above).
	def("bean", func(args ...any) any {
		x := oneArg("bean", args)
		v := reflect.ValueOf(x)
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				panic(fmt.Errorf("bean: nil pointer"))
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			panic(fmt.Errorf("bean: not a Go struct: %s", lang.PrintString(x)))
		}
		t := v.Type()
		var kvs []any
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			kvs = append(kvs, lang.InternKeywordString(kebabCase(f.Name)), v.Field(i).Interface())
		}
		return lang.NewMap(kvs...)
	})
}

// castClassName names a value's class for the cast error message: the
// printer's dispatch class (printClassOf) gives the JVM spelling for
// well-known scalars ("java.lang.Long"), the qualified type name for
// deftype/defrecord instances, and the honest Go type string otherwise.
func castClassName(x any) string {
	switch c := printClassOf(x).(type) {
	case *ClassRef:
		return c.Name()
	case *TypeMarker:
		return c.name
	case reflect.Type:
		return c.String()
	}
	return fmt.Sprintf("%T", x)
}

// hostIteratorSeq builds the lazy seq behind iterator-seq /
// enumeration-seq. Receiver resolution order: the op's own method pair
// (hasName/nextName), iterator-seq's HasNext/Next pair as a fallback
// (so enumeration-seq accepts either shape), then a channel
// (*lang.Channel or a raw Go chan) drained until it closes.
func hostIteratorSeq(op string, x any, hasName, nextName string) any {
	if x == nil {
		return nil
	}
	if ch, ok := x.(*lang.Channel); ok {
		// A core.async channel takes via ChanRecv (drains buffered values
		// and parked puts, nil once closed — nil is not a legal channel
		// value, so nil IS the end of the iteration).
		var step func() any
		step = func() any {
			v := lang.ChanRecv(ch)
			if v == nil {
				return nil
			}
			return lang.NewCons(v, lang.NewLazySeq(step))
		}
		return lang.Seq(lang.NewLazySeq(step))
	}
	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Chan {
		return chanSeq(v)
	}
	has, next := v.MethodByName(hasName), v.MethodByName(nextName)
	if !has.IsValid() || !next.IsValid() {
		has, next = v.MethodByName("HasNext"), v.MethodByName("Next")
	}
	if !has.IsValid() || !next.IsValid() {
		panic(fmt.Errorf("%s: no %s/%s (or HasNext/Next) method pair on: %s",
			op, hasName, nextName, lang.PrintString(x)))
	}
	var step func() any
	step = func() any {
		r := has.Call(nil)
		if len(r) != 1 || r[0].Kind() != reflect.Bool {
			panic(fmt.Errorf("%s: %s must return a bool", op, hasName))
		}
		if !r[0].Bool() {
			return nil
		}
		n := next.Call(nil)
		if len(n) == 0 {
			panic(fmt.Errorf("%s: %s must return a value", op, nextName))
		}
		return lang.NewCons(n[0].Interface(), lang.NewLazySeq(step))
	}
	return lang.Seq(lang.NewLazySeq(step))
}

// chanSeq lazily receives from a channel until it closes.
func chanSeq(ch reflect.Value) any {
	var step func() any
	step = func() any {
		val, ok := ch.Recv()
		if !ok {
			return nil
		}
		return lang.NewCons(val.Interface(), lang.NewLazySeq(step))
	}
	return lang.Seq(lang.NewLazySeq(step))
}

// kebabCase turns an exported Go field name into its keyword spelling:
// RawQuery -> raw-query, OmitHost -> omit-host, URL -> url (an acronym
// run only breaks before a trailing lowercase tail: RawURLPart ->
// raw-url-part).
func kebabCase(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		isUpper := r >= 'A' && r <= 'Z'
		if isUpper && i > 0 {
			prevLower := runes[i-1] >= 'a' && runes[i-1] <= 'z'
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if prevLower || nextLower {
				b.WriteRune('-')
			}
		}
		if isUpper {
			r = r + ('a' - 'A')
		}
		b.WriteRune(r)
	}
	return b.String()
}
