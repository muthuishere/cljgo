package corelib

// The bootstrap defmacro (design/03 §4): a hand-built macro fn — a Var
// flagged :macro whose value rewrites
//
//	(defmacro name doc? ([params] body...)+)   ; or single [params] body...
//
// into
//
//	(do (def name doc? (fn* name ([&form &env params...] body...)+))
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

	var doc any
	if s, isStr := fdecl[0].(string); isStr && len(fdecl) > 1 {
		doc = s
		fdecl = fdecl[1:]
	}

	// Normalize the single-arity shorthand [params] body... to one
	// ([params] body...) method; otherwise every element is a method.
	var methods []any
	if _, isVec := fdecl[0].(lang.IPersistentVector); isVec {
		methods = []any{lang.NewList(fdecl...)}
	} else {
		methods = fdecl
	}

	fnParts := []any{symFnStar, name}
	for _, m := range methods {
		mseq, isSeq := m.(lang.ISeq)
		if !isSeq {
			panic(fmt.Errorf("invalid defmacro method form: %s", lang.PrintString(m)))
		}
		parts := lang.ToSlice(mseq)
		pvec, isVec := parts[0].(lang.IPersistentVector)
		if !isVec {
			panic(fmt.Errorf("defmacro method requires a parameter vector, got: %s", lang.PrintString(parts[0])))
		}
		// Prepend the hidden params. A trailing "& rest" pair keeps its
		// invariant (& stays second-to-last).
		params := append([]any{symAmpForm, symAmpEnv}, lang.ToSlice(pvec)...)
		method := append([]any{lang.NewVector(params...)}, parts[1:]...)
		fnParts = append(fnParts, lang.NewList(method...))
	}

	defParts := []any{symDef, name}
	if doc != nil {
		defParts = append(defParts, doc)
	}
	defParts = append(defParts, lang.NewList(fnParts...))

	theVar := lang.NewList(symVar, name)
	return lang.NewList(symDo,
		lang.NewList(defParts...),
		lang.NewList(symSetMacroBang, theVar),
		theVar)
}
