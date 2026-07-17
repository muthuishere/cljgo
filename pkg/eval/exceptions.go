package eval

import (
	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/corelib"
)

// The tree-walk OpTry evaluator. The throw/catch NORMALIZATION it uses
// (Throw / Recover / CatchMatches) moved to pkg/corelib with ADR 0046 —
// a compiled binary shapes exceptions with no interpreter linked — and
// the AOT emitter reaches the same functions through pkg/emit/rt, so both
// modes are byte-identical by construction (design/03 §7d).

// evalTry runs an OpTry (design/03 §6): the protected body, catch matching
// in order (first matching class binds the caught exception and runs its
// body), and a finally that always runs for side effect with its value
// discarded. A finally runs on the normal path, on a caught throw, and
// while an uncaught throw unwinds (Go's defer). recur never crosses a try
// (analysis-blocked), so a recurSignal returned by the body is propagated,
// never caught.
func (e *Evaluator) evalTry(n *ast.Node, s *Scope) (result any, rerr error) {
	sub := n.Sub.(*ast.TryNode)

	if sub.Finally != nil {
		defer func() {
			// finally's value is discarded; if finally itself errors it
			// replaces the try's outcome (as a finally that throws does).
			if _, ferr := e.Eval(sub.Finally, s); ferr != nil {
				result, rerr = nil, ferr
			}
		}()
	}

	val, thrown := e.evalProtected(sub.Body, s)
	if thrown == nil {
		return val, nil
	}
	if rs, ok := thrown.(*recurSignal); ok {
		return nil, rs // recur is analysis-blocked across try; be safe
	}
	for _, cn := range sub.Catches {
		c := cn.Sub.(*ast.CatchNode)
		if corelib.CatchMatches(c.ClassName, thrown) {
			cs := s.Push()
			b := c.Binding.Sub.(*ast.BindingNode)
			cs.Define(b.Name.Name(), thrown)
			// The catch body runs UNPROTECTED: a throw inside it is not
			// caught by this try, but the deferred finally still runs.
			return e.Eval(c.Body, cs)
		}
	}
	return nil, thrown // no catch matched: propagate (finally still runs)
}

// evalProtected evaluates the try body, turning a panic (a throw, or any
// builtin panicking) into a returned thrown error and passing a returned
// error (e.g. a failed dynamic binding) through unchanged.
func (e *Evaluator) evalProtected(body *ast.Node, s *Scope) (val any, thrown error) {
	defer func() {
		if r := recover(); r != nil {
			thrown = corelib.Recover(r)
		}
	}()
	v, err := e.Eval(body, s)
	if err != nil {
		return nil, err
	}
	return v, nil
}
