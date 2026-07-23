package corelib

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
// Dispatch resolution (fundamentals audit 2026-07, prefer-method): an
// EXACT lang.Equiv match wins first (the fast path, and always correct —
// the exact key isa?-dominates every other matching key); otherwise the
// table is scanned for keys the dispatch value `isa?` (clojure.core/isa?,
// the real global-hierarchy fn from core/hierarchies.cljg, resolved
// lazily by var), picking the dominant match per prefer-method
// preferences exactly as JVM MultiFn.findAndCacheBestMethod does —
// including the "Multiple methods ... and neither is preferred" error on
// genuine ambiguity; only then the :default fallback. No per-call method
// cache yet (the JVM's cache/hierarchy-epoch machinery) — correctness
// first, the exact-match fast path keeps the common case cheap.

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
	// preferTable is prefer-method's state: dispatch-val -> IPersistentSet
	// of dispatch-vals it is preferred over (JVM MultiFn.preferTable).
	preferTable lang.IPersistentMap
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

// coreFnVar lazily resolves a clojure.core var by name — the hierarchy
// fns (isa?, parents) live in core/hierarchies.cljg, loaded after these
// builtins are interned, so resolution must happen at dispatch time, not
// registration time.
func coreFnVar(name string) *lang.Var {
	ns := lang.FindNamespace(lang.NewSymbol("clojure.core"))
	if ns == nil {
		return nil
	}
	return ns.FindInternedVar(lang.NewSymbol(name))
}

// isaVal is (clojure.core/isa? child parent) against the global
// hierarchy — false when the hierarchy layer isn't loaded (bare boot).
func isaVal(child, parent any) bool {
	v := coreFnVar("isa?")
	if v == nil {
		return false
	}
	return lang.IsTruthy(v.Invoke(child, parent))
}

// prefersVal is JVM MultiFn.prefers: x is preferred over y when the
// prefer table says so directly, or transitively through either side's
// hierarchy parents. Takes no lock of its own — safe under either a held
// RLock (dispatch) or the write lock (preferMethod's conflict check).
func (m *MultiFn) prefersVal(x, y any) bool {
	if m.preferTable != nil {
		if s, ok := lang.Get(m.preferTable, x).(lang.IPersistentSet); ok && s.Contains(y) {
			return true
		}
	}
	parentsVar := coreFnVar("parents")
	if parentsVar == nil {
		return false
	}
	for ps := lang.Seq(parentsVar.Invoke(y)); ps != nil; ps = ps.Next() {
		if m.prefersVal(x, ps.First()) {
			return true
		}
	}
	for ps := lang.Seq(parentsVar.Invoke(x)); ps != nil; ps = ps.Next() {
		if m.prefersVal(ps.First(), y) {
			return true
		}
	}
	return false
}

// dominates is JVM MultiFn.dominates: preference wins, else isa?.
func (m *MultiFn) dominates(x, y any) bool {
	return m.prefersVal(x, y) || isaVal(x, y)
}

// bestIsaMethod scans the table for entries the dispatch value isa?-
// matches (exact = matches were already handled by the caller) and picks
// the dominant one, panicking on ambiguity exactly as the JVM does
// (oracle 1.12.5: "Multiple methods in multimethod 'f2' match dispatch
// value: :user/c -> :user/b and :user/a, and neither is preferred").
func (m *MultiFn) bestIsaMethod(dv any) (lang.IFn, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	best := -1
	for i := range m.entries {
		if !isaVal(dv, m.entries[i].val) {
			continue
		}
		if best < 0 || m.dominates(m.entries[i].val, m.entries[best].val) {
			best = i
		}
		if !m.dominates(m.entries[best].val, m.entries[i].val) {
			panic(fmt.Errorf("Multiple methods in multimethod '%s' match dispatch value: %s -> %s and %s, and neither is preferred",
				m.name, lang.PrintString(dv), lang.PrintString(m.entries[i].val), lang.PrintString(m.entries[best].val)))
		}
	}
	if best < 0 {
		return nil, false
	}
	return m.entries[best].fn, true
}

// preferMethod records that dispatch value x should win over y, guarding
// against a contradictory existing preference (oracle 1.12.5:
// "Preference conflict in multimethod 'f2': :user/a is already preferred
// to :user/b").
func (m *MultiFn) preferMethod(x, y any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.prefersVal(y, x) {
		panic(fmt.Errorf("Preference conflict in multimethod '%s': %s is already preferred to %s",
			m.name, lang.PrintString(y), lang.PrintString(x)))
	}
	if m.preferTable == nil {
		m.preferTable = lang.NewMap()
	}
	s, _ := lang.Get(m.preferTable, x).(lang.IPersistentSet)
	if s == nil {
		s = lang.NewSet()
	}
	m.preferTable = m.preferTable.Assoc(x, s.Cons(y).(lang.IPersistentSet)).(lang.IPersistentMap)
}

// preferTableSnapshot is `prefers`: the val -> #{vals} preference map.
func (m *MultiFn) preferTableSnapshot() lang.IPersistentMap {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.preferTable == nil {
		return lang.NewMap()
	}
	return m.preferTable
}

// removeAllMethods empties the method table (remove-all-methods).
func (m *MultiFn) removeAllMethods() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = nil
}

// methodFor resolves the impl for a dispatch value: exact = match first
// (always dominant when it exists), then the isa?/preference scan, then
// the :default fallback.
func (m *MultiFn) methodFor(dv any) (lang.IFn, bool) {
	if fn, ok := m.getMethod(dv); ok {
		return fn, true
	}
	if fn, ok := m.bestIsaMethod(dv); ok {
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
func internMultimethodBuiltins(def func(string, func(...any) any) *lang.Var) {
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
		// The first NON-:default method registered on print-method or
		// print-dup activates the printer's multimethod seam (batch A2,
		// printread_builtins.go); until then lang.Print pays one atomic
		// bool load and never dispatches. Name-keyed on purpose: the two
		// core multimethods are resolved by var when the seam fires, so a
		// same-named user defmulti can only cost a redundant lookup, never
		// hijack printing.
		if (m.name == "print-method" || m.name == "print-dup") &&
			!lang.Equiv(args[1], lang.InternKeywordString("default")) {
			lang.PrintDispatchActive.Store(true)
		}
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

	// (remove-all-methods multifn) -> the multifn (empties the method
	// table). oracle 1.12.5: after remove-all-methods, (methods h) => {}
	// and a call throws "No method in multimethod 'h' for dispatch
	// value: ..." — even the :default method is gone.
	def("remove-all-methods", func(args ...any) any {
		m := asMultiFn(args[0], "remove-all-methods")
		m.removeAllMethods()
		return m
	})

	// (prefer-method multifn dispatch-val-x dispatch-val-y) -> the multifn.
	// Causes x to win over y in otherwise-ambiguous isa? dispatch; a
	// contradictory preference throws "Preference conflict ..." (oracle
	// 1.12.5, see preferMethod).
	def("prefer-method", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: prefer-method", len(args)))
		}
		m := asMultiFn(args[0], "prefer-method")
		m.preferMethod(args[1], args[2])
		return m
	})

	// (prefers multifn) -> map of preferred dispatch-val -> #{vals it is
	// preferred over}. oracle 1.12.5: (get (prefers f2) ::a) => #{:user/b}.
	def("prefers", func(args ...any) any {
		return asMultiFn(args[0], "prefers").preferTableSnapshot()
	})
}
