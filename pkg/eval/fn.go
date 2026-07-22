package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/diag"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// evalFn is a fn* closure (design/03 §5). env is the lexical scope
// captured at fn* evaluation time — that IS the closure. It satisfies
// lang.IFn; Invoke is the single internal-error→panic conversion point
// (design/00 §4.2).
type evalFn struct {
	node *ast.FnNode
	form any // original fn* form, for error messages
	env  *Scope
	eval *Evaluator
}

var _ lang.IFn = (*evalFn)(nil)

// arityError is an evalFn arity mismatch — Clojure's ArityException.
// Typed (not a bare fmt.Errorf) so macroexpand1 can recognize it and
// hide the two hidden macro args (&form/&env), as Compiler.macroexpand1
// does when it rethrows ArityException(e.actual - 2, e.name).
type arityError struct {
	actual int
	name   string

	// diag is the enriched, positioned diagnostic (spike s28). It is nil at
	// the throw site inside Invoke (which has no call-site position or Var
	// name) and set at the OpInvoke call site, which does. Error() is left
	// UNCHANGED whether or not it is set — conformance freezes that string
	// via strings.Contains, so all new detail lives on the diagnostic and
	// surfaces only through diag.Render.
	diag *diag.Diagnostic
}

func (e *arityError) Error() string {
	return fmt.Sprintf("wrong number of args (%d) passed to: %s", e.actual, e.name)
}

// Diagnostic implements diag.Carrier: FromError picks up the enriched
// (named + located + expected/found) diagnostic when the call site set one.
func (e *arityError) Diagnostic() (diag.Diagnostic, bool) {
	if e.diag == nil {
		return diag.Diagnostic{}, false
	}
	return *e.diag, true
}

// Invoke picks the method (exact fixed arity wins, else the variadic
// method if enough args), binds self-name and params on a scope pushed on
// the CAPTURED env (not the caller's — lexical scoping), and evaluates the
// body. Each method is its own recur target (design/03 §5): a recurSignal
// carrying this method's LoopID rebinds the params and loops — a plain Go
// loop, constant stack — never re-dispatching arities. On recur to a
// variadic method the rest param is rebound to the last recur value
// as-is (no re-packing), as in Clojure.
func (f *evalFn) Invoke(args ...any) any {
	m := f.pickMethod(len(args))
	if m == nil {
		panic(&arityError{actual: len(args), name: f.name()})
	}

	// One value per param: fixed args, then the packed rest for variadics.
	vals := make([]any, 0, len(m.Params))
	for i := 0; i < m.FixedArity; i++ {
		vals = append(vals, args[i])
	}
	if m.IsVariadic {
		rest := args[m.FixedArity:]
		if len(rest) == 0 {
			vals = append(vals, nil) // zero rest args → nil binding
		} else {
			vals = append(vals, lang.NewList(rest...))
		}
	}

	for {
		scope := f.env.Push()
		if f.node.Local != nil {
			self := f.node.Local.Sub.(*ast.BindingNode)
			scope.Define(self.Name.Name(), f)
		}
		for i, pn := range m.Params {
			b := pn.Sub.(*ast.BindingNode)
			scope.Define(b.Name.Name(), vals[i])
		}

		v, err := f.eval.Eval(m.Body, scope)
		if err == nil {
			return v
		}
		if rs, ok := err.(*recurSignal); ok && rs.loopID == m.LoopID {
			vals = rs.vals // analysis guarantees len == len(m.Params)
			continue
		}
		panic(err) // the IFn-boundary conversion (design/00 §4.2)
	}
}

func (f *evalFn) ApplyTo(args lang.ISeq) any {
	return f.Invoke(lang.ToSlice(args)...)
}

// pickMethod: exact FixedArity match wins; else the variadic method when
// len(args) >= its fixed prefix; else nil (arity error).
func (f *evalFn) pickMethod(nargs int) *ast.FnMethodNode {
	var variadic *ast.FnMethodNode
	for _, mn := range f.node.Methods {
		m := mn.Sub.(*ast.FnMethodNode)
		if m.IsVariadic {
			variadic = m
			continue
		}
		if m.FixedArity == nargs {
			return m
		}
	}
	if variadic != nil && nargs >= variadic.FixedArity {
		return variadic
	}
	return nil
}

func (f *evalFn) name() string {
	if f.node.Local != nil {
		return f.node.Local.Sub.(*ast.BindingNode).Name.Name()
	}
	return "fn"
}

func (f *evalFn) String() string {
	return fmt.Sprintf("#object[%s]", f.name())
}
