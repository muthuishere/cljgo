package eval

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// internMiscBuiltins registers a small grab-bag of clojure.core vars the
// jank clojure-test-suite harness needs (ADR 0022 batch/harness-misc):
// delay/force/delay?, the instance? substrate, add-watch/remove-watch, and
// a Thread/sleep seam for the portability shim. Wired into internBuiltins
// by ONE line (e.internMiscBuiltins(def, defPrivate)), per the
// merge-friendly discipline (three other agents editing builtins.go
// concurrently).
func (e *Evaluator) internMiscBuiltins(def func(name string, fn func(args ...any) any) *lang.Var, defPrivate func(name string, fn func(args ...any) any)) {
	// --- delay / force / delay? ------------------------------------------
	//
	// cljgo's pkg/lang already vendors a Delay type (IDeref + IPending) —
	// only the clojure.core surface was missing. `delay` itself is a MACRO
	// (core.clj: (defmacro delay [& body] (-make-delay (fn* [] body...)))
	// so the body isn't evaluated until forced; -make-delay is the private
	// substrate that macro expands onto (mirrors real clojure.core/delay
	// => (new clojure.lang.Delay (fn* [] body))).
	defPrivate("-make-delay", func(args ...any) any {
		fn, ok := oneArg("-make-delay", args).(lang.IFn)
		if !ok {
			panic(fmt.Errorf("delay: not a thunk: %s", lang.PrintString(args[0])))
		}
		return lang.NewDelay(fn)
	})
	// force: forces a Delay (realizing + memoizing it), or returns any
	// other value unchanged — exactly clojure.core/force.
	def("force", func(args ...any) any {
		return lang.ForceDelay(oneArg("force", args))
	})
	// delay?: is x a Delay.
	def("delay?", func(args ...any) any {
		_, ok := oneArg("delay?", args).(*lang.Delay)
		return ok
	})

	// --- instance? substrate ----------------------------------------------
	//
	// cljgo has no java.lang.Class objects (design/05: host interop is Go
	// structs, not a JVM class hierarchy), so `instance?`'s class position
	// cannot be an ordinary value the way JVM Clojure's is. `instance?` is
	// therefore a MACRO (core.clj) that treats a literal class symbol as
	// SYNTAX — matched by NAME, exactly like `catch`'s class symbol already
	// is (CatchMatches, ex_builtins.go) — never resolved as a var. This is
	// documented as a deliberate v0 deviation in ADR 0026: `instance?` with
	// a literal symbol class only works in direct call position, not as a
	// first-class function value (so (partial instance? String) does not
	// work; (instance? String x) does).
	//
	// -instance-of-name?: (class-name-string value) -> bool. First tries a
	// deftype/defrecord type var (a real value bound to a *TypeMarker,
	// exactly what -instance? already checks for extend-type's dispatch
	// key); then a fixed table of designator names covering both cljgo's
	// dispatchKey vocabulary (String/Long/Double/...) and its host wrapper
	// types (Atom/Delay/Var/Namespace/UUID/BigInt/BigDecimal) that have no
	// clojure.core designator of their own. A qualified name's LAST dotted
	// segment is what the table matches (clojure.lang.Atom ~ Atom,
	// java.util.UUID ~ UUID), mirroring how Clojure programmers read them.
	defPrivate("-instance-of-name?", func(args ...any) any {
		name := lang.ToString(args[0])
		v := args[1]

		if vr, err := e.resolveVar(lang.NewSymbol(name)); err == nil {
			if m, ok := vr.Deref().(*TypeMarker); ok {
				return dispatchKey(v) == m.name
			}
		}

		simple := name
		if i := strings.LastIndex(name, "."); i >= 0 {
			simple = name[i+1:]
		}
		switch simple {
		case "Object":
			return v != nil
		case "String":
			_, ok := v.(string)
			return ok
		case "Long", "Integer", "Short", "Byte":
			switch v.(type) {
			case int64, int, int32, int16, int8:
				return true
			}
			return false
		case "Double", "Float":
			switch v.(type) {
			case float64, float32:
				return true
			}
			return false
		case "Character":
			_, ok := v.(lang.Char)
			return ok
		case "Boolean":
			_, ok := v.(bool)
			return ok
		case "Keyword":
			_, ok := v.(lang.Keyword)
			return ok
		case "Symbol":
			_, ok := v.(*lang.Symbol)
			return ok
		case "Atom":
			_, ok := v.(*lang.Atom)
			return ok
		case "Delay":
			_, ok := v.(*lang.Delay)
			return ok
		case "Var":
			_, ok := v.(*lang.Var)
			return ok
		case "Namespace":
			_, ok := v.(*lang.Namespace)
			return ok
		case "BigInt":
			_, ok := v.(*lang.BigInt)
			return ok
		case "BigDecimal", "BigDec":
			_, ok := v.(*lang.BigDecimal)
			return ok
		case "UUID", "Guid":
			_, ok := v.(reader.UUID)
			return ok
		case "PersistentVector":
			_, ok := v.(lang.IPersistentVector)
			return ok
		case "PersistentArrayMap", "PersistentHashMap":
			_, ok := v.(lang.IPersistentMap)
			return ok
		case "PersistentHashSet":
			_, ok := v.(lang.IPersistentSet)
			return ok
		case "ISeq":
			_, ok := v.(lang.ISeq)
			return ok
		case "IPending":
			_, ok := v.(lang.IPending)
			return ok
		case "IFn":
			_, ok := v.(lang.IFn)
			return ok
		default:
			return dispatchKey(v) == simple
		}
	})

	// --- add-watch / remove-watch ------------------------------------------
	//
	// Generic over any lang.IRef (Atom, Var — Delay/Agent don't carry
	// watches in cljgo yet). (key ref old new) callback shape matches real
	// clojure.core exactly (pkg/lang's Atom/Var already notify in that
	// order; see atom.go/var.go notifyWatches).
	def("add-watch", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: add-watch", len(args)))
		}
		ref, ok := args[0].(lang.IRef)
		if !ok {
			panic(fmt.Errorf("add-watch: not watchable: %s", lang.PrintString(args[0])))
		}
		fn, ok := args[2].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("add-watch: not a function: %s", lang.PrintString(args[2])))
		}
		return ref.AddWatch(args[1], fn)
	})
	def("remove-watch", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: remove-watch", len(args)))
		}
		ref, ok := args[0].(lang.IRef)
		if !ok {
			panic(fmt.Errorf("remove-watch: not watchable: %s", lang.PrintString(args[0])))
		}
		ref.RemoveWatch(args[1])
		return args[0]
	})

	// --- clojure.edn substrate ----------------------------------------------
	//
	// -edn-read-string backs clojure.edn/read-string (core/edn.cljg): read
	// ONE form from a string with cljgo's own reader. Not evaluated — read
	// only. An empty/whitespace-only string reads as nil (oracle: JVM
	// (clojure.edn/read-string "") => nil); a syntactically invalid string
	// throws, as JVM edn does. DEVIATION (documented in core/edn.cljg):
	// this is the full cljgo reader, not a restricted EDN-only reader, so
	// reader macros EDN forbids (#(…) fn literals, `quasiquote`) do not
	// throw here. The suite's own #=(…) eval-reader probe still throws —
	// cljgo's reader has no #= at all.
	defPrivate("-edn-read-string", func(args ...any) any {
		s, ok := oneArg("-edn-read-string", args).(string)
		if !ok {
			if args[0] == nil {
				panic(fmt.Errorf("edn/read-string: nil input"))
			}
			panic(fmt.Errorf("edn/read-string expects a string, got: %s", lang.PrintString(args[0])))
		}
		form, err := reader.ReadString(s, reader.WithResolver(e.ReaderResolver()))
		if err != nil {
			if errors.Is(err, reader.ErrEOF) {
				return nil
			}
			panic(err)
		}
		return form
	})

	// --- type --------------------------------------------------------------
	//
	// cljgo has no java.lang.Class objects (design/05), so `type` cannot
	// return a real Class the way JVM Clojure's does. What the suite
	// actually needs (num.cljc, transient.cljc) is a value that's stable
	// and comparable via `=`: (= (type n) (type (num n))) checks that a
	// no-op numeric coercion doesn't change representation, and
	// (= (type coll) (type persisted)) checks a transient round-trips to
	// the same collection shape. reflect.Type satisfies exactly that: it's
	// a comparable Go interface value (== is true iff same concrete type),
	// which Equiv's `a == b` fast path already handles — no Equalser
	// needed. Real Clojure's :type metadata override (deftype/defrecord's
	// ::type key) is NOT implemented; this is a v0 stand-in, not a full
	// `type`/`class` substrate (that's its own future ADR).
	def("type", func(args ...any) any {
		x := oneArg("type", args)
		if x == nil {
			return nil
		}
		if m, ok := x.(lang.IMeta); ok {
			if meta := m.Meta(); meta != nil {
				if t := meta.ValAt(lang.KWType); t != nil {
					return t
				}
			}
		}
		return reflect.TypeOf(x)
	})

	// --- sleep --------------------------------------------------------------
	//
	// Backs the cljgo portability shim's `sleep` (core/clojure_test_portability.cljg)
	// — the suite's own test/clojure/core_test/portability.cljc defines
	// sleep as a #?(:clj (Thread/sleep ms) ...) host call; cljgo has no
	// Thread class, so this is a direct time.Sleep seam.
	defPrivate("-sleep-ms", func(args ...any) any {
		ms := lang.AsInt64(oneArg("-sleep-ms", args))
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return nil
	})
}
