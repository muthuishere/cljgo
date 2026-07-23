package corelib

import (
	"fmt"
	"sync/atomic"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// gensymCounter backs gensym's uniqueness. A single monotonically
// increasing id, matching Clojure's RT/nextID role.
var gensymCounter int64

// internSeqBuiltins interns the seq/coll + symbol/keyword primitives that
// core.clj's `destructure` (and the destructuring-aware let/loop/fn/defn
// macros) consume: nth, nthnext, nnext, count, gensym, conj, contains?,
// keys, and the symbol/keyword constructors + predicates. All are real
// clojure.core fns — precedence-safe additions, never renames (CLAUDE.md
// precedence principle). Kept in this file so builtins.go gains exactly
// one call line inside internBuiltins.
func internSeqBuiltins(def func(string, func(...any) any) *lang.Var) {
	// nth: (nth coll n) errors out of range; (nth coll n not-found) yields
	// the default. Backed by lang.Nth (Indexed/ISeq/string aware).
	def("nth", func(args ...any) any {
		if len(args) != 2 && len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: nth", len(args)))
		}
		// n is a primitive int in Clojure's nth(Object, int[, Object])
		// signature, so a non-integer n throws even when coll is nil (the
		// unboxing happens before nthFrom's null check) — oracle: (nth nil
		// nil) throws. But once n IS an integer, a nil coll is nil at ANY
		// index (nthFrom checks coll==null first, ignoring n's value) —
		// with a not-found arg it yields that default instead. Oracle:
		// (nth nil 10) => nil; (nth nil -5) => nil; (nth nil 10 :nf) => :nf.
		idx, ok := lang.AsInt(args[1])
		if !ok {
			panic(fmt.Errorf("nth: index must be an integer, got: %s", lang.PrintString(args[1])))
		}
		if args[0] == nil {
			if len(args) == 3 {
				return args[2]
			}
			return nil
		}
		v, found := lang.Nth(args[0], idx)
		if found {
			return v
		}
		if len(args) == 3 {
			return args[2]
		}
		// Typed IndexOutOfBoundsError (JVM: IndexOutOfBoundsException,
		// oracle 1.12.5); message stays byte-stable and the type's
		// DiagCode keeps G5004.
		panic(lang.NewIndexOutOfBoundsErrorMsg(fmt.Sprintf("index %d out of bounds", idx)))
	})

	// nthnext: (nthnext coll n) — the seq after n calls of next, or nil.
	def("nthnext", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: nthnext", len(args)))
		}
		// Mirrors (loop [s (seq coll) n n] (if (and s (pos? n)) (recur (next
		// s) (dec n)) s)): n is only ever inspected once s is truthy, so a
		// nil/empty coll never needs n to be an integer. Oracle: (nthnext
		// nil nil) => nil.
		s := lang.Seq(args[0])
		if s == nil {
			return nil
		}
		idx, ok := lang.AsInt(args[1])
		if !ok {
			panic(fmt.Errorf("nthnext: index must be an integer, got: %s", lang.PrintString(args[1])))
		}
		for i := 0; i < idx && s != nil; i++ {
			s = s.Next()
		}
		if s == nil {
			return nil
		}
		return s
	})

	// nnext: (nnext x) == (next (next x)).
	def("nnext", func(args ...any) any {
		return lang.Next(lang.Next(oneArg("nnext", args)))
	})

	// count: (count coll).
	def("count", func(args ...any) any {
		return int64(lang.Count(oneArg("count", args)))
	})

	// gensym: (gensym) or (gensym prefix) — a fresh, unique unqualified
	// symbol. Prefix defaults to "G__".
	def("gensym", func(args ...any) any {
		prefix := "G__"
		if len(args) == 1 {
			switch p := args[0].(type) {
			case string:
				prefix = p
			case *lang.Symbol:
				prefix = p.Name()
			default:
				panic(fmt.Errorf("gensym: prefix must be a string or symbol, got: %s", lang.PrintString(args[0])))
			}
		} else if len(args) > 1 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: gensym", len(args)))
		}
		n := atomic.AddInt64(&gensymCounter, 1)
		return lang.NewSymbol(fmt.Sprintf("%s%d", prefix, n))
	})

	// conj: (conj coll & xs) — adds each x to coll (vector: at the end,
	// seq/list: at the front). nil conjes onto an empty list.
	def("conj", func(args ...any) any {
		if len(args) == 0 {
			return lang.NewVector()
		}
		acc := args[0]
		for _, x := range args[1:] {
			if acc == nil {
				acc = lang.Conj(nil, x)
				continue
			}
			c, ok := acc.(lang.Conser)
			if !ok {
				panic(lang.NewCodedError("G5005", fmt.Sprintf("conj: cannot conj onto %s", lang.PrintString(acc))))
			}
			acc = lang.Conj(c, x)
		}
		return acc
	})

	// contains?: (contains? coll k) — associative key presence (also index
	// bounds for vectors, membership for sets).
	def("contains?", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: contains?", len(args)))
		}
		switch c := args[0].(type) {
		case nil:
			return false
		case lang.Associative:
			return c.ContainsKey(args[1])
		case lang.IPersistentSet:
			return c.Contains(args[1])
		case lang.ITransientSet:
			return c.Contains(args[1])
		case lang.ITransientAssociative:
			// Transients have no ContainsKey; probe via ValAtDefault with a
			// unique sentinel the same way get-in does. Oracle: (contains?
			// (transient {:x 1}) :x) => true; (contains? (transient [0 1])
			// 5) => false (out of bounds).
			sentinel := &struct{}{}
			return c.ValAtDefault(args[1], sentinel) != any(sentinel)
		case string:
			// contains? on a string/array checks index bounds, not value
			// membership. Oracle: (contains? "abc" 2) => true; (contains?
			// "abc" 3) => false (out of bounds, not an error) — but a
			// non-integer key still throws (real Clojure: key isn't a
			// Number to cast, ClassCastException), e.g. (contains? "abc"
			// "a") throws.
			idx, ok := lang.AsInt(args[1])
			if !ok {
				panic(fmt.Errorf("contains? not supported on: %s (key must be an integer, got: %s)",
					lang.PrintString(args[0]), lang.PrintString(args[1])))
			}
			return idx >= 0 && idx < len([]rune(c))
		default:
			panic(fmt.Errorf("contains? not supported on: %s", lang.PrintString(args[0])))
		}
	})

	// keys: (keys map) — a seq of the map's keys, or nil when empty.
	def("keys", func(args ...any) any {
		coll := oneArg("keys", args)
		if coll == nil {
			return nil
		}
		ks := lang.Keys(coll)
		if ks == nil {
			return nil
		}
		return ks
	})

	// name: (name x) — the name string of a symbol/keyword, or the string
	// itself.
	def("name", func(args ...any) any {
		switch x := oneArg("name", args).(type) {
		case string:
			return x
		case *lang.Symbol:
			return x.Name()
		case lang.Keyword:
			return x.Name()
		default:
			panic(fmt.Errorf("name: expected string, symbol or keyword, got: %s", lang.PrintString(x)))
		}
	})

	// namespace: (namespace x) — the namespace string of a symbol/keyword,
	// or nil.
	def("namespace", func(args ...any) any {
		switch x := oneArg("namespace", args).(type) {
		case *lang.Symbol:
			if x.HasNamespace() {
				return x.Namespace()
			}
			return nil
		case lang.Keyword:
			return x.Namespace()
		default:
			panic(fmt.Errorf("namespace: expected symbol or keyword, got: %s", lang.PrintString(x)))
		}
	})

	// symbol: (symbol name) | (symbol ns name).
	def("symbol", func(args ...any) any {
		switch len(args) {
		case 1:
			switch x := args[0].(type) {
			case string:
				return lang.NewSymbol(x)
			case *lang.Symbol:
				return x
			case lang.Keyword:
				return x.Sym()
			case *lang.Var:
				return x.ToSymbol()
			default:
				panic(fmt.Errorf("symbol: cannot make a symbol from: %s", lang.PrintString(args[0])))
			}
		case 2:
			var ns any
			if args[0] != nil {
				ns = args[0].(string)
			}
			return lang.InternSymbol(ns, args[1].(string))
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: symbol", len(args)))
		}
	})

	// keyword: (keyword name) | (keyword ns name).
	def("keyword", func(args ...any) any {
		switch len(args) {
		case 1:
			switch x := args[0].(type) {
			case nil:
				return nil
			case string:
				return lang.NewKeyword(x)
			case *lang.Symbol:
				return lang.InternKeywordSymbol(x)
			case lang.Keyword:
				return x
			default:
				panic(fmt.Errorf("keyword: cannot make a keyword from: %s", lang.PrintString(args[0])))
			}
		case 2:
			var ns any
			if args[0] != nil {
				ns = args[0].(string)
			}
			return lang.InternKeyword(ns, args[1].(string))
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: keyword", len(args)))
		}
	})

	// find-keyword: (find-keyword name) | (find-keyword ns name) — the
	// keyword ONLY if one with that name was already interned, else nil;
	// NEVER interns (lang.FindKeyword checks the registry without adding).
	// Accepts the same argument shapes as `keyword`; a keyword argument
	// returns itself (it exists, so it is interned by definition).
	// oracle 1.12.5: (find-keyword "no-such-kw-zzz") => nil;
	// (find-keyword :abc) => :abc; after :interned-zzq is read,
	// (find-keyword "interned-zzq") => :interned-zzq;
	// (find-keyword "user" "zzz-nope") => nil.
	def("find-keyword", func(args ...any) any {
		found := func(s string) any {
			if kw, ok := lang.FindKeyword(s); ok {
				return kw
			}
			return nil
		}
		switch len(args) {
		case 1:
			switch x := args[0].(type) {
			case lang.Keyword:
				return x
			case *lang.Symbol:
				return found(x.FullName())
			case string:
				return found(x)
			default:
				panic(fmt.Errorf("find-keyword: cannot look up a keyword from: %s", lang.PrintString(args[0])))
			}
		case 2:
			name, ok := args[1].(string)
			if !ok {
				panic(fmt.Errorf("find-keyword: name must be a string, got: %s", lang.PrintString(args[1])))
			}
			if args[0] == nil {
				return found(name)
			}
			ns, ok := args[0].(string)
			if !ok {
				panic(fmt.Errorf("find-keyword: ns must be a string, got: %s", lang.PrintString(args[0])))
			}
			return found(ns + "/" + name)
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: find-keyword", len(args)))
		}
	})

	// Predicates the destructure algorithm branches on.
	def("symbol?", func(args ...any) any {
		_, ok := oneArg("symbol?", args).(*lang.Symbol)
		return ok
	})
	def("keyword?", func(args ...any) any {
		_, ok := oneArg("keyword?", args).(lang.Keyword)
		return ok
	})
	def("vector?", func(args ...any) any {
		_, ok := oneArg("vector?", args).(lang.IPersistentVector)
		return ok
	})
	def("map?", func(args ...any) any {
		_, ok := oneArg("map?", args).(lang.IPersistentMap)
		return ok
	})
	def("ident?", func(args ...any) any {
		x := oneArg("ident?", args)
		if _, ok := x.(*lang.Symbol); ok {
			return true
		}
		_, ok := x.(lang.Keyword)
		return ok
	})
}
