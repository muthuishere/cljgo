// Package eval is the tree-walk evaluator (design/03 §6, design/00 §4.2).
// It is the REPL engine: symbol references evaluate to per-use Var derefs
// (never inlined values) so re-def stays live for every existing caller.
//
// Calling convention (the M0 seam, design/00 §4.2): internal evaluation
// returns (any, error); lang.IFn.Invoke returns any only. The single
// error→panic conversion lives at the IFn boundary (evalFn.Invoke /
// nativeFn), matching emitted code where exceptions are panics. The
// top-level EvalForm recovers panics back into errors for the REPL.
package eval

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/analyzer"
	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// Evaluator wires analyzer ↔ eval (design/03 §4, §9.2) and holds the
// current namespace. v0: one `user` namespace, no macros (Macroexpand1
// stays nil — every seq is a special or an invoke).
type Evaluator struct {
	CurrentNS *lang.Namespace
	analyzer  *analyzer.Analyzer
}

// New returns an evaluator with the `user` namespace (created if absent)
// and the v0 builtins interned.
func New() *Evaluator {
	e := &Evaluator{
		CurrentNS: lang.FindOrCreateNamespace(lang.NewSymbol("user")),
	}
	e.analyzer = &analyzer.Analyzer{
		Macroexpand1: nil, // v0: no macro support
		ResolveVar:   e.resolveVar,
		InternVar:    e.internVar,
	}
	e.internBuiltins()
	return e
}

// Analyzer exposes the wired analyzer (for tests and the REPL driver).
func (e *Evaluator) Analyzer() *analyzer.Analyzer { return e.analyzer }

// resolveVar is the analyzer's var-resolution hook (design/03 §3a).
func (e *Evaluator) resolveVar(sym *lang.Symbol) (*lang.Var, error) {
	if sym.HasNamespace() {
		nsSym := lang.NewSymbol(sym.Namespace())
		ns := e.CurrentNS.LookupAlias(nsSym)
		if ns == nil {
			ns = lang.FindNamespace(nsSym)
		}
		if ns == nil {
			return nil, fmt.Errorf("no such namespace: %s", sym.Namespace())
		}
		v := ns.FindInternedVar(lang.NewSymbol(sym.Name()))
		if v == nil {
			return nil, fmt.Errorf("no such var: %s", sym.FullName())
		}
		return v, nil
	}
	if m := e.CurrentNS.Mappings().ValAt(sym); m != nil {
		if v, ok := m.(*lang.Var); ok {
			return v, nil
		}
	}
	return nil, fmt.Errorf("unable to resolve symbol: %s in this context", sym.Name())
}

// internVar is the analyzer's def hook: intern (create-or-find) the Var in
// the current namespace at analysis time (design/03 §2). The name must be
// unqualified or qualified into the current ns.
func (e *Evaluator) internVar(sym *lang.Symbol) (*lang.Var, error) {
	if sym.HasNamespace() {
		if sym.Namespace() != e.CurrentNS.Name().Name() {
			return nil, fmt.Errorf("can't create defs outside of current ns: %s", sym.FullName())
		}
		sym = lang.NewSymbol(sym.Name())
	}
	return e.CurrentNS.Intern(sym), nil
}

// EvalForm analyzes and evaluates one top-level form, converting runtime
// panics back into errors. A top-level (do ...) is split and evaluated
// form-by-form (design/03 §6) so earlier defs are visible to later
// siblings in one file.
func (e *Evaluator) EvalForm(form any) (any, error) {
	if seq := asTopLevelDo(form); seq != nil {
		var res any
		var err error
		res = nil
		for s := seq; s != nil; s = s.Next() {
			res, err = e.EvalForm(s.First())
			if err != nil {
				return nil, err
			}
		}
		return res, nil
	}
	n, err := e.analyzer.Analyze(form)
	if err != nil {
		return nil, err
	}
	return e.evalTop(n)
}

// asTopLevelDo returns the body seq of a (do ...) form, or nil.
func asTopLevelDo(form any) lang.ISeq {
	seq, ok := form.(lang.ISeq)
	if !ok || lang.Seq(seq) == nil {
		return nil
	}
	sym, ok := seq.First().(*lang.Symbol)
	if !ok || sym.HasNamespace() || sym.Name() != "do" {
		return nil
	}
	return seq.Next()
}

// evalTop evaluates an analyzed node in a fresh scope, recovering panics
// (the IFn-boundary convention) into errors.
func (e *Evaluator) evalTop(n *ast.Node) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if rerr, ok := r.(error); ok {
				err = rerr
				return
			}
			err = fmt.Errorf("%v", r)
		}
	}()
	return e.Eval(n, NewScope())
}

// Eval is the flat per-op dispatch (design/03 §6), mirrored later by the
// emitter. Unhandled ops panic loudly (design/03 §7d).
func (e *Evaluator) Eval(n *ast.Node, s *Scope) (any, error) {
	switch n.Op {
	case ast.OpConst:
		return n.Sub.(*ast.ConstNode).Value, nil

	case ast.OpQuote:
		return n.Sub.(*ast.QuoteNode).Value, nil

	case ast.OpVector:
		sub := n.Sub.(*ast.VectorNode)
		items := make([]any, 0, len(sub.Items))
		for _, item := range sub.Items {
			v, err := e.Eval(item, s)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		return lang.NewVector(items...), nil

	case ast.OpMap:
		sub := n.Sub.(*ast.MapNode)
		kvs := make([]any, 0, 2*len(sub.Keys))
		for i := range sub.Keys {
			k, err := e.Eval(sub.Keys[i], s)
			if err != nil {
				return nil, err
			}
			v, err := e.Eval(sub.Vals[i], s)
			if err != nil {
				return nil, err
			}
			kvs = append(kvs, k, v)
		}
		return lang.NewMap(kvs...), nil

	case ast.OpSet:
		sub := n.Sub.(*ast.SetNode)
		items := make([]any, 0, len(sub.Items))
		for _, item := range sub.Items {
			v, err := e.Eval(item, s)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		return lang.NewSet(items...), nil

	case ast.OpVar:
		// Deref per use — never inlined (design/00 §4.2: REPL re-def
		// must stay live; direct linking is forbidden in the evaluator).
		return n.Sub.(*ast.VarNode).Var.Deref(), nil

	case ast.OpLocal:
		sub := n.Sub.(*ast.LocalNode)
		v, ok := s.Lookup(sub.Name.Name())
		if !ok {
			// Analyzer guarantees locals resolve; a miss is an evaluator bug.
			return nil, fmt.Errorf("internal error: unbound local: %s", sub.Name.Name())
		}
		return v, nil

	case ast.OpDo:
		sub := n.Sub.(*ast.DoNode)
		for _, stmt := range sub.Statements {
			if _, err := e.Eval(stmt, s); err != nil {
				return nil, err
			}
		}
		return e.Eval(sub.Ret, s)

	case ast.OpIf:
		sub := n.Sub.(*ast.IfNode)
		t, err := e.Eval(sub.Test, s)
		if err != nil {
			return nil, err
		}
		if lang.IsTruthy(t) {
			return e.Eval(sub.Then, s)
		}
		return e.Eval(sub.Else, s)

	case ast.OpDef:
		sub := n.Sub.(*ast.DefNode)
		if sub.Init != nil {
			v, err := e.Eval(sub.Init, s)
			if err != nil {
				return nil, err
			}
			// Re-def replaces the root, never the Var identity — existing
			// references see the new value (design/03 §3b).
			sub.Var.BindRoot(v)
		}
		return sub.Var, nil

	case ast.OpLet:
		sub := n.Sub.(*ast.LetNode)
		// One child scope per binding: a closure made between two
		// bindings of the same name must keep seeing the earlier frame.
		cur := s
		for _, bn := range sub.Bindings {
			b := bn.Sub.(*ast.BindingNode)
			v, err := e.Eval(b.Init, cur)
			if err != nil {
				return nil, err
			}
			cur = cur.Push()
			cur.Define(b.Name.Name(), v)
		}
		return e.Eval(sub.Body, cur)

	case ast.OpFn:
		return &evalFn{node: n.Sub.(*ast.FnNode), form: n.Form, env: s, eval: e}, nil

	case ast.OpInvoke:
		sub := n.Sub.(*ast.InvokeNode)
		fnVal, err := e.Eval(sub.Fn, s)
		if err != nil {
			return nil, err
		}
		args := make([]any, 0, len(sub.Args))
		for _, an := range sub.Args {
			v, err := e.Eval(an, s)
			if err != nil {
				return nil, err
			}
			args = append(args, v)
		}
		// lang.Apply dispatches IFn / keywords / colls; errors surface as
		// panics per the IFn-boundary convention and are recovered at the
		// top level.
		return lang.Apply(fnVal, args), nil

	case ast.OpBinding, ast.OpFnMethod:
		// Structural children of OpLet / OpFn — never evaluated directly.
		panic(fmt.Sprintf("eval: op %v is not directly evaluable", n.Op))

	default:
		panic(fmt.Sprintf("eval: unhandled op %v", n.Op))
	}
}
