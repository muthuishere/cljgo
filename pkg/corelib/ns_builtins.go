package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// ns_builtins.go — clojure.core's namespace-introspection surface
// (fundamentals audit 2026-07, A-list "introspection needed for tooling"):
// ns-name, the-ns, all-ns, ns-publics, ns-interns, ns-map, ns-refers,
// ns-aliases, ns-imports. All read the live pkg/lang namespace registry —
// the same world the audit had to dump through a throwaway Go test because
// exactly these fns were missing. Registered into internBuiltins by ONE
// line (internNamespaceBuiltins(def)), per the merge-friendly discipline.
//
// Every behavior is oracle-verified against JVM Clojure 1.12.5 (`clojure`
// CLI, 2026-07-21); frozen evidence in conformance/tests/ns-introspection*.clj.
//
// cljgo note: ns-unmap/ns-unalias/ns-resolve stay out of this batch (not
// A-list), and ns-imports is honest-but-empty — cljgo has no JVM class
// imports, so no mapping is ever a class (see below).

// coerceNS coerces a *lang.Namespace or a symbol naming one, exactly like
// clojure.core/the-ns — distinct from var_builtins.go's theNS only in its
// JVM-shaped miss message. oracle: (the-ns 'nope.nope) => "No namespace:
// nope.nope found"; (the-ns (the-ns 'user)) is identity.
func coerceNS(ctx string, v any) *lang.Namespace {
	switch x := v.(type) {
	case *lang.Namespace:
		return x
	case *lang.Symbol:
		if ns := lang.FindNamespace(x); ns != nil {
			return ns
		}
		panic(fmt.Errorf("No namespace: %s found", x.FullName()))
	default:
		panic(fmt.Errorf("%s: not a namespace or symbol: %s", ctx, lang.PrintString(v)))
	}
}

// nsMappingsWhere builds a {symbol -> mapping} map of ns's mappings
// satisfying pred — the shared body of ns-map/ns-publics/ns-interns/
// ns-refers/ns-imports.
func nsMappingsWhere(ns *lang.Namespace, pred func(sym *lang.Symbol, v any) bool) lang.IPersistentMap {
	res := lang.NewMap()
	for seq := lang.Seq(ns.Mappings()); seq != nil; seq = seq.Next() {
		entry := seq.First().(lang.IMapEntry)
		sym, ok := entry.Key().(*lang.Symbol)
		if !ok {
			continue
		}
		if pred(sym, entry.Val()) {
			res = res.Assoc(sym, entry.Val()).(lang.IPersistentMap)
		}
	}
	return res
}

// internedHere reports whether mapping v is a Var interned in ns itself
// (vs referred from another namespace).
func internedHere(ns *lang.Namespace, v any) bool {
	vr, ok := v.(*lang.Var)
	return ok && vr.Namespace() == ns
}

func internNamespaceBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// (ns-name ns-or-sym) -> the namespace's name symbol.
	// oracle: (ns-name 'clojure.core) => clojure.core; (ns-name *ns*) => user.
	def("ns-name", func(args ...any) any {
		return coerceNS("ns-name", oneArg("ns-name", args)).Name()
	})

	// (the-ns ns-or-sym) -> the Namespace, or throws.
	def("the-ns", func(args ...any) any {
		return coerceNS("the-ns", oneArg("the-ns", args))
	})

	// (all-ns) -> seq of all live namespaces (unordered, as on the JVM).
	// oracle: (some #(= 'clojure.core (ns-name %)) (all-ns)) => true.
	def("all-ns", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: all-ns", len(args)))
		}
		return lang.AllNamespaces()
	})

	// (ns-map ns-or-sym) -> {sym -> var} of ALL mappings (interned +
	// referred; cljgo has no class imports to include).
	// oracle: (contains? (ns-map 'user) 'map) => true.
	def("ns-map", func(args ...any) any {
		ns := coerceNS("ns-map", oneArg("ns-map", args))
		return nsMappingsWhere(ns, func(*lang.Symbol, any) bool { return true })
	})

	// (ns-interns ns-or-sym) -> {sym -> var} of vars interned in ns,
	// public AND private. oracle: after (def ^:private priv-var 1),
	// (contains? (ns-interns 'user) 'priv-var) => true.
	def("ns-interns", func(args ...any) any {
		ns := coerceNS("ns-interns", oneArg("ns-interns", args))
		return nsMappingsWhere(ns, func(_ *lang.Symbol, v any) bool {
			return internedHere(ns, v)
		})
	})

	// (ns-publics ns-or-sym) -> {sym -> var} of PUBLIC vars interned in
	// ns. oracle: (contains? (ns-publics 'clojure.core) 'map) => true;
	// a ^:private def is excluded.
	def("ns-publics", func(args ...any) any {
		ns := coerceNS("ns-publics", oneArg("ns-publics", args))
		return nsMappingsWhere(ns, func(_ *lang.Symbol, v any) bool {
			return internedHere(ns, v) && v.(*lang.Var).IsPublic()
		})
	})

	// (ns-refers ns-or-sym) -> {sym -> var} of vars REFERRED from other
	// namespaces. oracle: (contains? (ns-refers 'user) 'map) => true;
	// a locally interned var is excluded.
	def("ns-refers", func(args ...any) any {
		ns := coerceNS("ns-refers", oneArg("ns-refers", args))
		return nsMappingsWhere(ns, func(_ *lang.Symbol, v any) bool {
			vr, ok := v.(*lang.Var)
			return ok && vr.Namespace() != ns
		})
	})

	// (ns-aliases ns-or-sym) -> {alias-sym -> Namespace}.
	// oracle: after (require '[clojure.set :as sss]),
	// (contains? (ns-aliases 'user) 'sss) => true and the value's ns-name
	// is clojure.set.
	def("ns-aliases", func(args ...any) any {
		return coerceNS("ns-aliases", oneArg("ns-aliases", args)).Aliases()
	})

	// (ns-imports ns-or-sym) -> {sym -> class} of imported classes.
	// cljgo DEVIATION (documented): the Go host has no JVM class imports —
	// namespace mappings are only ever Vars (well-known class names
	// resolve through the fixed ClassRef table, ADR 0036, never through
	// per-namespace import mappings) — so this is always {} today, where
	// the JVM preloads java.lang. Kept for API-shape compatibility; the
	// filter is honest (any non-Var mapping WOULD appear).
	def("ns-imports", func(args ...any) any {
		ns := coerceNS("ns-imports", oneArg("ns-imports", args))
		return nsMappingsWhere(ns, func(_ *lang.Symbol, v any) bool {
			_, isVar := v.(*lang.Var)
			return !isVar
		})
	})

	// --- namespace mutation (fundamentals batch A4, 2026-07-23) ----------
	// ns-unmap / ns-unalias / remove-ns over the new pkg/lang primitives
	// (Namespace.Unmap / Namespace.RemoveAlias / RemoveNamespace-returning,
	// see pkg/lang/PROVENANCE.md). Each oracle-verified vs JVM 1.12.5;
	// frozen evidence in conformance/tests/ns-unmap-*.clj,
	// ns-unalias-*.clj, remove-ns-*.clj.

	// (ns-unmap ns sym) -> nil; removes sym's mapping from ns (interned or
	// referred alike). oracle: (ns-unmap *ns* 'map) => nil, then
	// (resolve 'map) => nil; a qualified symbol throws "Can't unintern
	// namespace-qualified symbol"; unmapping an absent name is a no-op.
	def("ns-unmap", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: ns-unmap", len(args)))
		}
		ns := coerceNS("ns-unmap", args[0])
		sym, ok := args[1].(*lang.Symbol)
		if !ok {
			panic(fmt.Errorf("ns-unmap expects a symbol, got: %s", lang.PrintString(args[1])))
		}
		ns.Unmap(sym)
		return nil
	})

	// (ns-unalias ns sym) -> nil; removes the alias, absent alias = no-op.
	// oracle: after (alias 'so 'scratch.one), (ns-unalias *ns* 'so) => nil
	// and (contains? (ns-aliases *ns*) 'so) => false;
	// (ns-unalias *ns* 'never-existed) => nil.
	def("ns-unalias", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: ns-unalias", len(args)))
		}
		ns := coerceNS("ns-unalias", args[0])
		sym, ok := args[1].(*lang.Symbol)
		if !ok {
			panic(fmt.Errorf("ns-unalias expects a symbol, got: %s", lang.PrintString(args[1])))
		}
		ns.RemoveAlias(sym)
		return nil
	})

	// (remove-ns sym) -> the removed Namespace, or nil when no such
	// namespace exists. oracle: (remove-ns 'scratch.one) => the namespace
	// object; (remove-ns 'scratch.never) => nil; (remove-ns 'clojure.core)
	// throws "Cannot remove clojure namespace".
	def("remove-ns", func(args ...any) any {
		sym := symbolArg("remove-ns", args)
		ns := lang.RemoveNamespace(sym)
		if ns == nil {
			return nil
		}
		return ns
	})
}
