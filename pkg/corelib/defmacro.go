package corelib

// The bootstrap defmacro (design/03 §4): a hand-built macro fn — a Var
// flagged :macro whose value rewrites
//
//	(defmacro name doc? attr-map? ([params] body...)+ attr-map?)
//	                                ; or single [params] body...
//
// into
//
//	(do (def ^{:doc … :arglists …} name (fn* name ([&form &env params...] body...)+))
//	    (clojure.core/-set-macro! (var name))
//	    (var name))
//
// JVM-style: the macro fn takes &form/&env as explicit leading params on
// every arity (clojure/core.clj's defmacro does the same rewrite), and
// the expansion sets the :macro flag on the var at eval time, so a
// defmacro typed at the REPL is a macro for the very next form
// (design/03 §7a). -set-macro! is the M1 stand-in for JVM Clojure's
// (. (var name) (setMacro)) — host interop lands in v3.
//
// It lives in corelib — not pkg/eval — so BOTH boot legs intern the
// identical clojure.core/defmacro var through RegisterAll: the
// interpreter (which macroexpands with it) and a compiled binary (which
// never expands at runtime but must expose the same namespace mappings —
// ns-map parity across legs is release-blocking, ADR 0002/0007).

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// symFnStar lives in builtins.go (shared with the go/thread macros).
var (
	symDo           = lang.NewSymbol("do")
	symDef          = lang.NewSymbol("def")
	symVar          = lang.NewSymbol("var")
	symAmpForm      = lang.NewSymbol("&form")
	symAmpEnv       = lang.NewSymbol("&env")
	symSetMacroBang = lang.NewSymbol("clojure.core/-set-macro!")
)

var symQuote = lang.NewSymbol("quote")

// mergeAttrMap conj's an attr-map onto the accumulated var metadata (the
// later map wins on key conflicts, as in clojure.core/defn's `conj`), and
// unwraps one level of (quote x) from each value. See the -defn-unquote-vals
// note in core/core.clj: cljgo applies def metadata as a CONSTANT rather than
// evaluating it like the JVM compiler, so {:arglists '([x])} would otherwise
// read back as the literal (quote ([x])) form.
func mergeAttrMap(acc, attrs lang.IPersistentMap) lang.IPersistentMap {
	for s := lang.Seq(attrs); s != nil; s = s.Next() {
		e, ok := s.First().(lang.IMapEntry)
		if !ok {
			continue
		}
		val := e.Val()
		if q, isSeq := val.(lang.ISeq); isSeq {
			if sym, isSym := q.First().(*lang.Symbol); isSym && sym.Equals(symQuote) {
				if rest := q.Next(); rest != nil {
					val = rest.First()
				}
			}
		}
		acc = acc.Assoc(e.Key(), val).(lang.IPersistentMap)
	}
	return acc
}

// registerDefmacro interns the bootstrap defmacro into clojure.core and
// flags it :macro. Called from RegisterAll, before any core source loads.
func registerDefmacro() {
	v := lang.NSCore.Intern(lang.NewSymbol("defmacro"))
	v.BindRoot(NewNativeFn("defmacro", defmacroExpand))
	v.SetMacro()
}

// defmacroExpand is defmacro's expander. args = [&form &env name doc?
// fdecl...]; the two hidden args are ignored.
func defmacroExpand(args ...any) any {
	if len(args) < 4 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: defmacro", len(args)-2))
	}
	name, ok := args[2].(*lang.Symbol)
	if !ok {
		panic(fmt.Errorf("first argument to defmacro must be a symbol, got: %s", lang.PrintString(args[2])))
	}
	fdecl := args[3:]

	// (defmacro name doc-string? attr-map? ...) — same prefix as defn, and
	// like clojure.core/defmacro the accumulated map (plus a trailing
	// attr-map) lands on the var, not on a positional docstring.
	m := lang.NewMap()
	if s, isStr := fdecl[0].(string); isStr && len(fdecl) > 1 {
		m = m.Assoc(lang.KWDoc, s).(lang.IPersistentMap)
		fdecl = fdecl[1:]
	}
	if am, isMap := fdecl[0].(lang.IPersistentMap); isMap && len(fdecl) > 1 {
		m = mergeAttrMap(m, am)
		fdecl = fdecl[1:]
	}

	// Normalize the single-arity shorthand [params] body... to one
	// ([params] body...) method; otherwise every element is a method.
	// Normalizing BEFORE taking the trailing attr-map is what keeps a macro
	// whose body IS a map from being read as an attr-map.
	var methods []any
	if _, isVec := fdecl[0].(lang.IPersistentVector); isVec {
		methods = []any{lang.NewList(fdecl...)}
	} else {
		methods = fdecl
	}
	if len(methods) > 1 {
		if am, isMap := methods[len(methods)-1].(lang.IPersistentMap); isMap {
			m = mergeAttrMap(m, am)
			methods = methods[:len(methods)-1]
		}
	}

	fnParts := []any{symFnStar, name}
	arglists := []any{}
	for _, mth := range methods {
		mseq, isSeq := mth.(lang.ISeq)
		if !isSeq {
			panic(fmt.Errorf("invalid defmacro method form: %s", lang.PrintString(mth)))
		}
		parts := lang.ToSlice(mseq)
		pvec, isVec := parts[0].(lang.IPersistentVector)
		if !isVec {
			panic(fmt.Errorf("defmacro method requires a parameter vector, got: %s", lang.PrintString(parts[0])))
		}
		// :arglists records the USER-visible params (clojure.core/defmacro
		// does the same) — &form/&env are not part of them.
		arglists = append(arglists, pvec)
		// Prepend the hidden params. A trailing "& rest" pair keeps its
		// invariant (& stays second-to-last).
		params := append([]any{symAmpForm, symAmpEnv}, lang.ToSlice(pvec)...)
		method := append([]any{lang.NewVector(params...)}, parts[1:]...)
		fnParts = append(fnParts, lang.NewList(method...))
	}

	// {:arglists ...} first so a user-supplied :arglists wins, then the
	// name symbol's own metadata on top — clojure.core/defn's conj order.
	m = mergeAttrMap(lang.NewMap(lang.NewKeyword("arglists"), lang.NewList(arglists...)), m)
	if nm := name.Meta(); nm != nil {
		m = mergeAttrMap(m, nm)
	}
	name = name.WithMeta(m).(*lang.Symbol)

	defParts := []any{symDef, name}
	defParts = append(defParts, lang.NewList(fnParts...))

	theVar := lang.NewList(symVar, name)
	return lang.NewList(symDo,
		lang.NewList(defParts...),
		lang.NewList(symSetMacroBang, theVar),
		theVar)
}
