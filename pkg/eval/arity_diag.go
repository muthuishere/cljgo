package eval

import (
	"fmt"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/diag"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// invokeAt calls lang.Apply for an OpInvoke node, enriching an arity error
// that unwinds through it with the call-site detail the deep throw could not
// know: the qualified fn name (from the resolved Var), the source location
// (from the call form's :line/:column meta) and expected-vs-found arity
// (from the callee's params). This is the spike-s28 worked example of
// carrying a span + name into ONE runtime error; the enrichment is confined
// to the error path (a recovered arityError), so a successful call pays only
// the deferred-closure setup, and the tree-walk evaluator is not perf-gated.
//
// ADR 0048 should replace the per-call defer with a cheaper mechanism (a
// position threaded onto the analyzer's invoke node, or lang.EvalError's
// existing StackFrame slice) so the hot path pays nothing — see VERDICT.
func (e *Evaluator) invokeAt(fnVal any, args []any, callNode, fnNode *ast.Node) (res any) {
	defer func() {
		if r := recover(); r != nil {
			if ae, ok := r.(*arityError); ok && ae.diag == nil {
				ae.diag = arityDiagnostic(ae, fnVal, callNode, fnNode)
			}
			panic(r)
		}
	}()
	return lang.Apply(fnVal, args)
}

// arityDiagnostic builds the enriched Diagnostic for an arity error, or nil
// when nothing better than the bare message is knowable.
func arityDiagnostic(ae *arityError, fnVal any, callNode, fnNode *ast.Node) *diag.Diagnostic {
	name := callName(fnNode, fnVal, ae.name)
	msg := fmt.Sprintf("wrong number of args (%d) passed to: %s", ae.actual, name)

	d := diag.Diagnostic{
		Severity:   diag.SeverityError,
		Message:    msg,
		Found:      fmt.Sprintf("%d", ae.actual),
		ErrorCode:  "A2004",
		ExplainURL: diag.ExplainURL("A2004"),
	}
	if file, line, col, ok := formPosOf(callNode); ok {
		d.Location = diag.Location{File: file, Line: line, Column: col}
	}
	if ef, ok := fnVal.(*evalFn); ok {
		d.Expected = arityExpects(ef.node)
	}
	// The count is already in the message ("(N)"); drop the redundant "got N".
	d.Found = ""
	return &d
}

// callName is the JVM-accurate name for the callee: the resolved Var's
// qualified name (user/f) when the call head is a Var reference, else the
// closure's self-name, else the bare arity-error name ("fn").
func callName(fnNode *ast.Node, fnVal any, fallback string) string {
	if fnNode != nil && fnNode.Op == ast.OpVar {
		if vn, ok := fnNode.Sub.(*ast.VarNode); ok && vn.Var != nil {
			return vn.Var.ToSymbol().String()
		}
	}
	if ef, ok := fnVal.(*evalFn); ok {
		if ef.node.Local != nil {
			return ef.node.Local.Sub.(*ast.BindingNode).Name.Name()
		}
	}
	return fallback
}

// arityExpects renders the callee's accepted arities as an "expects" label,
// e.g. "1: [x]" for one method, "1: [x] or 2: [x y]" for several, with a
// trailing "& more" marker on the variadic method.
func arityExpects(fn *ast.FnNode) string {
	parts := make([]string, 0, len(fn.Methods))
	for _, mn := range fn.Methods {
		m := mn.Sub.(*ast.FnMethodNode)
		names := make([]string, 0, len(m.Params))
		for _, pn := range m.Params {
			names = append(names, pn.Sub.(*ast.BindingNode).Name.Name())
		}
		label := fmt.Sprintf("%d: [%s]", m.FixedArity, strings.Join(names, " "))
		if m.IsVariadic {
			label = fmt.Sprintf("%d+: [%s & more]", m.FixedArity, strings.Join(names, " "))
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " or ")
}

// formPosOf reads :file/:line/:column position metadata off a node's
// original form (the same convention analyzer.formPos uses). ok is false
// when the form carries no line, so the caller leaves the diagnostic
// unlocated (and Render omits the " at …" locus).
func formPosOf(n *ast.Node) (file string, line, col int, ok bool) {
	if n == nil {
		return "", 0, 0, false
	}
	im, isMeta := n.Form.(lang.IMeta)
	if !isMeta {
		return "", 0, 0, false
	}
	meta := im.Meta()
	if meta == nil {
		return "", 0, 0, false
	}
	if f, isStr := lang.Get(meta, lang.KWFile).(string); isStr {
		file = f
	}
	if l, isInt := lang.AsInt(lang.Get(meta, lang.KWLine)); isInt {
		line = l
	}
	if c, isInt := lang.AsInt(lang.Get(meta, lang.KWColumn)); isInt {
		col = c
	}
	return file, line, col, line > 0
}
