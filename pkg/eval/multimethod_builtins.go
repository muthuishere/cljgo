package eval

import (
	"fmt"
	"sync"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// This file implements cljgo's multimethod runtime — the other Clojure
// polymorphism mechanism (dispatch on an arbitrary value produced by a
// dispatch-fn, not on type). It mirrors the protocols layer
// (pkg/eval/protocols.go): a runtime registry lives INSIDE a value stored
// in a Var (no global Go state), the surface is macros-over-builtins with
// NO new AST op, and the private `-`-prefixed builtins are the substrate
// the core.clj `defmulti`/`defmethod` macros expand onto. Because the
// AOT-compiled binary boots the SAME evaluator (rt.Boot -> eval.New) that
// interns these builtins and loads core.clj, a multimethod dispatches
// byte-identically interpreted and compiled (design/00 §2, §6 M5).
//
// v0 is a FLAT, `=`-based dispatch table: the method for a dispatch value
// is the entry whose key is lang.Equiv to it, with an optional :default
// fallback. No isa?/hierarchy/prefer-method (Clojure's inheritance-based
// resolution) — those are a later increment; the vendored pkg/lang/MultiFn
// carries that machinery but depends on isa?/parents/hierarchy vars that
// this runtime does not yet intern, so it stays unused.

// multiEntry is one (dispatch-value -> impl) pair in the flat table.
type multiEntry struct {
	val any
	fn  lang.IFn
}

// MultiFn is a multimethod value: a dispatch fn, an =-keyed method table,
// and the dispatch value used as the fallback (:default). It implements
// lang.IFn (Invoke/ApplyTo) so a var bound to a MultiFn is directly
// callable — `(area x)` routes through Invoke, exactly like an ordinary fn.
type MultiFn struct {
	name       string
	dispatchFn lang.IFn
	defaultVal any
	mu         sync.RWMutex
	entries    []multiEntry
}

func (m *MultiFn) String() string { return "#multifn[" + m.name + "]" }

// addMethod installs (or replaces, by =) the impl for a dispatch value.
func (m *MultiFn) addMethod(val any, fn lang.IFn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.entries {
		if lang.Equiv(m.entries[i].val, val) {
			m.entries[i].fn = fn
			return
		}
	}
	m.entries = append(m.entries, multiEntry{val: val, fn: fn})
}

// removeMethod drops the impl for a dispatch value (by =), if present.
func (m *MultiFn) removeMethod(val any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.entries {
		if lang.Equiv(m.entries[i].val, val) {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return
		}
	}
}

// getMethod finds the impl registered EXACTLY for a dispatch value (by =),
// without falling back to :default.
func (m *MultiFn) getMethod(val any) (lang.IFn, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.entries {
		if lang.Equiv(m.entries[i].val, val) {
			return m.entries[i].fn, true
		}
	}
	return nil, false
}

// methodFor resolves the impl for a dispatch value, falling back to the
// :default method when there is no exact match.
func (m *MultiFn) methodFor(dv any) (lang.IFn, bool) {
	if fn, ok := m.getMethod(dv); ok {
		return fn, true
	}
	return m.getMethod(m.defaultVal)
}

// methodTable snapshots the table as a Clojure map dispatch-val -> fn.
func (m *MultiFn) methodTable() lang.IPersistentMap {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := lang.NewMap()
	for _, e := range m.entries {
		res = res.Assoc(e.val, e.fn).(lang.IPersistentMap)
	}
	return res
}

// Invoke applies the dispatch fn to the args, looks up the matching impl
// (or :default), and applies it — else a Clojure-shaped "No method ..."
// error. This is what makes `(a-multifn args...)` work.
func (m *MultiFn) Invoke(args ...any) any {
	dv := m.dispatchFn.Invoke(args...)
	fn, ok := m.methodFor(dv)
	if !ok {
		panic(fmt.Errorf("No method in multimethod '%s' for dispatch value: %s",
			m.name, lang.PrintString(dv)))
	}
	return fn.Invoke(args...)
}

func (m *MultiFn) ApplyTo(args lang.ISeq) any {
	return m.Invoke(lang.ToSlice(args)...)
}

func asMultiFn(v any, ctx string) *MultiFn {
	m, ok := v.(*MultiFn)
	if !ok {
		panic(fmt.Errorf("%s: not a multimethod: %s", ctx, lang.PrintString(v)))
	}
	return m
}

// internMultimethodBuiltins registers the multimethod substrate into
// clojure.core (design/00 §6 M5). The `-`-prefixed helpers are the private
// substrate the core.clj defmulti/defmethod macros expand onto; the public
// spellings (methods / get-method / remove-method) are user-facing
// clojure.core fns that operate on a MultiFn value.
func (e *Evaluator) internMultimethodBuiltins(def func(string, func(...any) any) *lang.Var) {
	// (-defmulti name-string dispatch-fn) -> a fresh MultiFn whose default
	// dispatch value is :default (Clojure's fixed v0 default).
	def("-defmulti", func(args ...any) any {
		name := lang.ToString(args[0])
		fn, ok := args[1].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("defmulti: dispatch fn is not a function: %s", lang.PrintString(args[1])))
		}
		return &MultiFn{
			name:       name,
			dispatchFn: fn,
			defaultVal: lang.InternKeywordString("default"),
		}
	})

	// (-defmethod multifn dispatch-val fn) -> the multifn (registers impl).
	def("-defmethod", func(args ...any) any {
		m := asMultiFn(args[0], "defmethod")
		fn, ok := args[2].(lang.IFn)
		if !ok {
			panic(fmt.Errorf("defmethod: method impl is not a function: %s", lang.PrintString(args[2])))
		}
		m.addMethod(args[1], fn)
		return m
	})

	// (methods multifn) -> map of dispatch-val -> impl fn.
	def("methods", func(args ...any) any {
		return asMultiFn(args[0], "methods").methodTable()
	})

	// (get-method multifn dispatch-val) -> the impl for that value
	// (including a :default fallback), or nil.
	def("get-method", func(args ...any) any {
		m := asMultiFn(args[0], "get-method")
		if fn, ok := m.methodFor(args[1]); ok {
			return fn
		}
		return nil
	})

	// (remove-method multifn dispatch-val) -> the multifn (drops the impl).
	def("remove-method", func(args ...any) any {
		m := asMultiFn(args[0], "remove-method")
		m.removeMethod(args[1])
		return m
	})
}
