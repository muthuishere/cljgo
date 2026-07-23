package eval

// Macroexpansion (eval v2, design/03 §4): macroexpand1 is the analyzer's
// injected hook; the bootstrap defmacro is a hand-built macro fn; the
// embedded core/core.clj defines the user-facing macros on top.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/muthuishere/cljgo/core"
	"github.com/muthuishere/cljgo/pkg/analyzer"
	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// macroexpand1 expands form by one macro step, exactly Compiler.java's
// macroexpand1 (design/03 §4): non-seqs, specials, locals-shadowed
// operators, and operators that do not resolve to a :macro var pass
// through unchanged (the identical value — the analyzer loops on
// identity). A macro var's fn is invoked NOW, at analysis time, with
// &form (the whole call form) and &env (a map keyed by the local
// symbols in scope, nil when none) prepended to the call's arguments.
func (e *Evaluator) macroexpand1(form any, locals map[string]*ast.BindingNode) (any, error) {
	seq, ok := form.(lang.ISeq)
	if !ok || lang.Seq(seq) == nil {
		return form, nil
	}
	op, ok := seq.First().(*lang.Symbol)
	if !ok {
		return form, nil
	}
	if !op.HasNamespace() {
		if analyzer.IsSpecial(op.Name()) {
			return form, nil // specials are not macros
		}
		if _, shadowed := locals[op.Name()]; shadowed {
			return form, nil // a local shadows the macro name
		}
	}
	v, err := e.resolveVar(op)
	if err != nil || !v.IsMacro() {
		// Unresolvable operators are NOT an expansion error: the form is
		// simply not a macro call (parseInvoke reports resolution errors
		// with position).
		return form, nil
	}

	// (macrofn &form &env args...) — the two hidden leading args, per
	// Compiler.java l.7583.
	rest := lang.ToSlice(seq.Next())
	margs := make([]any, 0, 2+len(rest))
	margs = append(margs, form, localsEnvMap(locals))
	margs = append(margs, rest...)

	// The macro fn runs under the IFn panic convention; expansion happens
	// during analysis (outside evalTop's recover), so recover here.
	var expanded any
	err = func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				rerr, ok := r.(error)
				if !ok {
					rerr = fmt.Errorf("%v", r)
				}
				err = rerr
			}
		}()
		expanded = lang.Apply(v.Deref(), margs)
		return nil
	}()
	if err != nil {
		// An arity error from the macro fn itself counts &form/&env; hide
		// the two hidden args, as Compiler.macroexpand1 does (it rethrows
		// ArityException(e.actual - 2, e.name) when e.name is the macro's
		// own name). Verified vs clojure 1.12.5: (defmacro mm [a b] 1)
		// (mm 1) => "Wrong number of args (1) passed to: user/mm".
		var ae *arityError
		if errors.As(err, &ae) {
			if f, isEval := v.Deref().(*evalFn); isEval && f.name() == ae.name {
				err = &arityError{actual: ae.actual - 2, name: ae.name}
			}
		}
		return nil, fmt.Errorf("macroexpanding %s: %w", op.Name(), err)
	}
	return expanded, nil
}

// localsEnvMap builds the &env value from the analysis-time locals: a
// map keyed by the local symbols (values nil — we carry no per-binding
// info yet), or nil when no locals are in scope, as on JVM Clojure.
func localsEnvMap(locals map[string]*ast.BindingNode) any {
	if len(locals) == 0 {
		return nil
	}
	kvs := make([]any, 0, 2*len(locals))
	for name := range locals {
		kvs = append(kvs, lang.NewSymbol(name), nil)
	}
	return lang.NewMap(kvs...)
}

// macroexpand is user-facing (macroexpand form): repeat macroexpand1 on
// the returned form until the operator is no longer a macro. Subforms
// are not expanded, as on JVM Clojure. Called with no locals in scope
// (&env = nil), like the clojure.core fns it backs.
func (e *Evaluator) macroexpand(form any) (any, error) {
	for i := 0; i < maxUserMacroExpansions; i++ {
		expanded, err := e.macroexpand1(form, nil)
		if err != nil {
			return nil, err
		}
		if expanded == form {
			return form, nil
		}
		form = expanded
	}
	return nil, fmt.Errorf("too many macroexpansions (limit %d)", maxUserMacroExpansions)
}

// maxUserMacroExpansions bounds the user-facing macroexpand loop (the
// analyzer's own loop has its own limit).
const maxUserMacroExpansions = 1000

// loadBootSource reads and evaluates one embedded boot source into its
// namespace, form by form, exactly as a file load (design/03 §7a): *ns*
// (the source's target namespace) and *file* are bound for the duration,
// so a file's own (in-ns …) is undone afterwards and *file* reads as its
// name while it loads. Boot is programmer-owned input — errors panic.
//
// The source list and its order live in core.BootSources() — ONE table,
// shared with the AOT core compiler (cmd/gencore, ADR 0046), so the
// interpreter and a compiled binary load exactly the same namespaces in
// exactly the same order.
func (e *Evaluator) loadBootSource(s core.BootSource) {
	ns := lang.FindOrCreateNamespace(lang.NewSymbol(s.NS))
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ns,
		lang.VarFile, s.File,
	))
	defer lang.PopThreadBindings()

	r := reader.New(strings.NewReader(*s.Source),
		reader.WithFilename(s.File),
		reader.WithResolver(e.ReaderResolver()))
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return
		}
		if err != nil {
			panic(fmt.Errorf("boot: reading %s: %w", s.File, err))
		}
		if _, err := e.EvalForm(form); err != nil {
			panic(fmt.Errorf("boot: evaluating %s: %w", s.File, err))
		}
	}
}
