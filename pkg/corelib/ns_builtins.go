package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internNSBuiltins registers the namespace-introspection surface
// (ns-name / the-ns / all-ns / ns-publics / ns-interns / ns-map /
// ns-refers / ns-aliases / ns-imports) — the clojure.core fns the
// fundamentals audit (docs/fundamentals-audit-2026-07.md) flagged
// borderline-A because tooling cannot exist without them: clojure.repl's
// apropos/dir/find-doc and clojure.test's test-all-vars/test-ns are all
// built on these. Semantics oracle-verified against JVM Clojure 1.12.5:
//
//	(ns-name *ns*)                   => user (a symbol)
//	(the-ns 'no.such.ns)             => throws "No namespace: no.such.ns found"
//	(ns-publics 'clojure.set)        => {union #'clojure.set/union, ...}
//	(sort (map first (ns-publics 'clojure.set))) => (difference index ...)
//
// Wired into internBuiltins by ONE line (internNSBuiltins(def)), per the
// merge-friendly discipline.
func internNSBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// ns-name: the namespace's name as a symbol. Accepts a Namespace or a
	// symbol naming an existing one (the-ns coercion, as JVM).
	def("ns-name", func(args ...any) any {
		return theNamespace("ns-name", oneArg("ns-name", args)).Name()
	})

	// the-ns: coerce to an existing Namespace or throw (never creates).
	// JVM message shape: "No namespace: foo found".
	def("the-ns", func(args ...any) any {
		return theNamespace("the-ns", oneArg("the-ns", args))
	})

	// all-ns: a seq of all currently-defined Namespace objects.
	def("all-ns", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: all-ns", len(args)))
		}
		return lang.AllNamespaces()
	})

	// ns-interns: {name-symbol Var} for every var INTERNED in ns
	// (private included; refers excluded).
	def("ns-interns", func(args ...any) any {
		ns := theNamespace("ns-interns", oneArg("ns-interns", args))
		return nsMappingMap(ns, func(sym *lang.Symbol, val any) bool {
			v, ok := val.(*lang.Var)
			return ok && v.Namespace() == ns
		})
	})

	// ns-publics: {name-symbol Var} for every PUBLIC var interned in ns.
	def("ns-publics", func(args ...any) any {
		ns := theNamespace("ns-publics", oneArg("ns-publics", args))
		return nsMappingMap(ns, func(sym *lang.Symbol, val any) bool {
			v, ok := val.(*lang.Var)
			return ok && v.Namespace() == ns && v.IsPublic()
		})
	})

	// ns-refers: {name-symbol Var} for every var REFERRED into ns (a var
	// interned in some other namespace).
	def("ns-refers", func(args ...any) any {
		ns := theNamespace("ns-refers", oneArg("ns-refers", args))
		return nsMappingMap(ns, func(sym *lang.Symbol, val any) bool {
			v, ok := val.(*lang.Var)
			return ok && v.Namespace() != ns
		})
	})

	// ns-map: every mapping in ns (interns + refers + imports).
	def("ns-map", func(args ...any) any {
		ns := theNamespace("ns-map", oneArg("ns-map", args))
		return nsMappingMap(ns, func(sym *lang.Symbol, val any) bool {
			return true
		})
	})

	// ns-imports: the non-Var mappings (class imports). cljgo's class
	// refs live outside namespace mappings (class_refs.go), so this is
	// usually empty — honest: nothing is imported into cljgo namespaces.
	def("ns-imports", func(args ...any) any {
		ns := theNamespace("ns-imports", oneArg("ns-imports", args))
		return nsMappingMap(ns, func(sym *lang.Symbol, val any) bool {
			_, isVar := val.(*lang.Var)
			return !isVar
		})
	})

	// ns-aliases: {alias-symbol Namespace} for ns's require/alias table.
	def("ns-aliases", func(args ...any) any {
		ns := theNamespace("ns-aliases", oneArg("ns-aliases", args))
		return ns.Aliases()
	})
}

// theNamespace coerces v to an existing Namespace with the JVM the-ns
// error shape ("No namespace: foo found"); never creates one.
func theNamespace(op string, v any) *lang.Namespace {
	switch x := v.(type) {
	case *lang.Namespace:
		return x
	case *lang.Symbol:
		ns := lang.FindNamespace(x)
		if ns == nil {
			panic(fmt.Errorf("No namespace: %s found", x.FullName()))
		}
		return ns
	default:
		panic(fmt.Errorf("%s expects a namespace or symbol, got: %s", op, lang.PrintString(v)))
	}
}

// nsMappingMap builds a persistent {symbol mapping} map from ns's
// mappings, keeping the entries keep() approves.
func nsMappingMap(ns *lang.Namespace, keep func(sym *lang.Symbol, val any) bool) lang.IPersistentMap {
	kvs := []any{}
	for s := lang.Seq(ns.Mappings()); s != nil; s = s.Next() {
		entry := s.First().(lang.IMapEntry)
		sym, ok := entry.Key().(*lang.Symbol)
		if !ok {
			continue
		}
		if keep(sym, entry.Val()) {
			kvs = append(kvs, sym, entry.Val())
		}
	}
	return lang.NewMap(kvs...)
}
