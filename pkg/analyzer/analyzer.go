// Package analyzer turns read forms into pkg/ast nodes (design/03 §2–§5).
//
// The analyzer is pure and dependency-injected: it never imports the
// evaluator or touches global namespace state itself. Macro expansion and
// var resolution/interning are hooks supplied by the runtime that wires
// analyze ↔ eval (design/03 §4, §9.2). Analysis errors carry source
// position taken from the offending form's metadata (design/00 §4.5).
//
// M1 scope: literals, collection literals, symbol resolution
// (locals → vars), the specials quote / if / do / def / let* / fn* /
// loop* / recur / var / set! / binding, plus macroexpansion (v2, §4)
// and invoke. letfn*, try/throw and host interop are later phases.
// (`binding` is a special here until it can move to core as a macro
// over push/popThreadBindings; the semantics match.)
package analyzer

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// Ctx is the analysis-time expression context (design/03 §3a). The v0
// evaluator ignores it; the emitter cares.
type Ctx uint8

const (
	CtxExpr Ctx = iota
	CtxStatement
	CtxReturn
)

// RecurFrame marks the innermost recur target (fn method or loop*).
// Blocked, when non-empty, names a construct (e.g. "try") that sits
// between the recur and its target — the target is visible but crossing
// it is an error ("Cannot recur across try", design/03 §2 Phase 4). The
// `binding` form sets it because in Clojure binding expands to
// try/finally around push/popThreadBindings.
type RecurFrame struct {
	LoopID  string
	Arity   int
	Blocked string
}

// Env is the immutable analysis-time environment, threaded through
// analysis with copy-on-extend of Locals (design/03 §3a).
type Env struct {
	Locals     map[string]*ast.BindingNode
	Context    Ctx
	RecurFrame *RecurFrame
	IsTopLevel bool
}

func (e Env) withLocal(b *ast.BindingNode) Env {
	locals := make(map[string]*ast.BindingNode, len(e.Locals)+1)
	for k, v := range e.Locals {
		locals[k] = v
	}
	locals[b.Name.Name()] = b
	e.Locals = locals
	return e
}

func (e Env) withContext(c Ctx) Env {
	e.Context = c
	return e
}

// Analyzer analyzes forms. All fields are injection points.
type Analyzer struct {
	// Macroexpand1 expands a form by one macro step, returning the form
	// unchanged (identical value) when it is not a macro call. locals is
	// the analysis-time lexical environment at the call site: a local
	// shadowing a macro name suppresses expansion (Compiler.isMacro),
	// and the expander derives the macro's hidden &env argument from it
	// (design/03 §4). nil means no macro support (v0): every seq is a
	// special form or an invoke.
	Macroexpand1 func(form any, locals map[string]*ast.BindingNode) (any, error)

	// ResolveVar resolves a non-local symbol to a Var. The returned error
	// is the resolution failure message ("Unable to resolve symbol...",
	// "No such namespace...", ...); the analyzer wraps it with source
	// position. Required.
	ResolveVar func(sym *lang.Symbol) (*lang.Var, error)

	// InternVar interns (create-or-find) a Var for def in the current
	// namespace at analysis time (design/03 §2 — load-bearing for forward
	// references and self-recursion). Required.
	InternVar func(sym *lang.Symbol) (*lang.Var, error)

	gensymCounter atomic.Int64
}

func (a *Analyzer) gensym(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, a.gensymCounter.Add(1))
}

// Analyze analyzes a top-level form.
func (a *Analyzer) Analyze(form any) (*ast.Node, error) {
	return a.analyzeForm(form, Env{Context: CtxExpr, IsTopLevel: true})
}

func (a *Analyzer) analyzeForm(form any, env Env) (*ast.Node, error) {
	switch f := form.(type) {
	case *lang.Symbol:
		return a.analyzeSymbol(f, env)
	case lang.IPersistentVector:
		return a.analyzeVector(f, env)
	case lang.IPersistentMap:
		return a.analyzeMap(f, env)
	case lang.IPersistentSet:
		return a.analyzeSet(f, env)
	case lang.ISeq:
		return a.analyzeSeq(f, env)
	default:
		// Everything else is self-evaluating: nil, bool, numbers,
		// strings, chars, keywords, regexes, host values.
		return &ast.Node{Op: ast.OpConst, Form: form, Sub: &ast.ConstNode{Value: form}, IsLiteral: true}, nil
	}
}

// analyzeSymbol implements resolution per design/03 §3a: locals always
// shadow vars; unresolved symbols are position-carrying errors.
func (a *Analyzer) analyzeSymbol(sym *lang.Symbol, env Env) (*ast.Node, error) {
	if !sym.HasNamespace() {
		if b, ok := env.Locals[sym.Name()]; ok {
			return &ast.Node{Op: ast.OpLocal, Form: sym, Sub: &ast.LocalNode{Name: sym, Binding: b}}, nil
		}
	}
	v, err := a.ResolveVar(sym)
	if err != nil {
		return nil, a.errPos(sym, err)
	}
	// Vars are set! targets; whether the var is dynamic and thread-bound
	// is the evaluator's runtime check, as in Clojure (design/03 §2).
	return &ast.Node{Op: ast.OpVar, Form: sym, Sub: &ast.VarNode{Var: v}, IsAssignable: true}, nil
}

func (a *Analyzer) analyzeVector(v lang.IPersistentVector, env Env) (*ast.Node, error) {
	itemEnv := env.withContext(CtxExpr)
	items := make([]*ast.Node, 0, v.Count())
	for i := 0; i < v.Count(); i++ {
		n, err := a.analyzeForm(v.Nth(i), itemEnv)
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return &ast.Node{Op: ast.OpVector, Form: v, Sub: &ast.VectorNode{Items: items}}, nil
}

func (a *Analyzer) analyzeMap(m lang.IPersistentMap, env Env) (*ast.Node, error) {
	itemEnv := env.withContext(CtxExpr)
	var keys, vals []*ast.Node
	for s := lang.Seq(m); s != nil; s = s.Next() {
		entry, ok := s.First().(lang.IMapEntry)
		if !ok {
			return nil, a.errf(m, "map literal: unexpected entry %v", s.First())
		}
		k, err := a.analyzeForm(entry.Key(), itemEnv)
		if err != nil {
			return nil, err
		}
		v, err := a.analyzeForm(entry.Val(), itemEnv)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		vals = append(vals, v)
	}
	return &ast.Node{Op: ast.OpMap, Form: m, Sub: &ast.MapNode{Keys: keys, Vals: vals}}, nil
}

func (a *Analyzer) analyzeSet(set lang.IPersistentSet, env Env) (*ast.Node, error) {
	itemEnv := env.withContext(CtxExpr)
	var items []*ast.Node
	for s := lang.Seq(set); s != nil; s = s.Next() {
		n, err := a.analyzeForm(s.First(), itemEnv)
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return &ast.Node{Op: ast.OpSet, Form: set, Sub: &ast.SetNode{Items: items}}, nil
}

// maxMacroExpansions bounds the macroexpansion loop in analyzeSeq so a
// self-expanding macro ((defmacro m [] '(m))) is a positioned error, not
// a stack overflow. JVM Clojure has no limit; the bound is deliberate.
const maxMacroExpansions = 1000

// analyzeSeq is Compiler.java's analyzeSeq: specials first, then
// macroexpand-1 (re-analyzing if the form changed), else invoke. The
// fixed-point loop of design/03 §4 is explicit here: while an expansion
// yields another non-empty seq the loop continues (checking specials
// again each round, so intermediate expansions that produce specials
// stop expanding); a non-seq expansion re-enters analyzeForm.
func (a *Analyzer) analyzeSeq(seq lang.ISeq, env Env) (*ast.Node, error) {
	if lang.Seq(seq) == nil {
		// The empty list is self-evaluating.
		return &ast.Node{Op: ast.OpConst, Form: seq, Sub: &ast.ConstNode{Value: seq}, IsLiteral: true}, nil
	}

	form := seq
	for i := 0; i < maxMacroExpansions; i++ {
		if sym, ok := form.First().(*lang.Symbol); ok && !sym.HasNamespace() {
			// Specials are checked before locals and macros: they cannot
			// be shadowed (Compiler.java analyzeSeq).
			if parse, isSpecial := a.specialParser(sym.Name()); isSpecial {
				return parse(form, env)
			}
		}

		if a.Macroexpand1 == nil {
			return a.parseInvoke(form, env)
		}
		expanded, err := a.Macroexpand1(form, env.Locals)
		if err != nil {
			return nil, a.errPos(form, err)
		}
		if expanded == any(form) {
			return a.parseInvoke(form, env)
		}
		if eseq, ok := expanded.(lang.ISeq); ok && lang.Seq(eseq) != nil {
			form = eseq // expanded to another call form: keep expanding
			continue
		}
		// Expanded to a non-seq (or empty seq): re-enter analyze.
		return a.analyzeForm(expanded, env)
	}
	return nil, a.errf(seq, "too many macroexpansions (limit %d) expanding: %s", maxMacroExpansions, lang.PrintString(seq.First()))
}

type specialParserFn func(seq lang.ISeq, env Env) (*ast.Node, error)

// IsSpecial reports whether name names a special form (Compiler.java's
// specials map). Specials are never macros: the runtime's macroexpand1
// consults this before resolving the operator to a var (design/03 §4).
func IsSpecial(name string) bool {
	var probe Analyzer
	_, ok := probe.specialParser(name)
	return ok
}

// specialParser returns the parser for a v0 special form name.
func (a *Analyzer) specialParser(name string) (specialParserFn, bool) {
	switch name {
	case "quote":
		return a.parseQuote, true
	case "if":
		return a.parseIf, true
	case "do":
		return a.parseDo, true
	case "def":
		return a.parseDef, true
	case "let*":
		return a.parseLet, true
	case "loop*":
		return a.parseLoop, true
	case "recur":
		return a.parseRecur, true
	case "var":
		return a.parseVar, true
	case "set!":
		return a.parseSetBang, true
	case "binding":
		return a.parseBinding, true
	case "fn*":
		return a.parseFnStar, true
	}
	return nil, false
}

func (a *Analyzer) parseQuote(seq lang.ISeq, env Env) (*ast.Node, error) {
	args := seqToSlice(seq.Next())
	if len(args) != 1 {
		return nil, a.errf(seq, "wrong number of args (%d) passed to quote", len(args))
	}
	return &ast.Node{Op: ast.OpQuote, Form: seq, Sub: &ast.QuoteNode{Value: args[0]}, IsLiteral: true}, nil
}

func (a *Analyzer) parseIf(seq lang.ISeq, env Env) (*ast.Node, error) {
	args := seqToSlice(seq.Next())
	switch len(args) {
	case 2, 3:
	default:
		if len(args) > 3 {
			return nil, a.errf(seq, "too many arguments to if")
		}
		return nil, a.errf(seq, "too few arguments to if")
	}
	test, err := a.analyzeForm(args[0], env.withContext(CtxExpr))
	if err != nil {
		return nil, err
	}
	then, err := a.analyzeForm(args[1], env)
	if err != nil {
		return nil, err
	}
	var els *ast.Node
	if len(args) == 3 {
		els, err = a.analyzeForm(args[2], env)
		if err != nil {
			return nil, err
		}
	} else {
		els = constNil()
	}
	return &ast.Node{Op: ast.OpIf, Form: seq, Sub: &ast.IfNode{Test: test, Then: then, Else: els}}, nil
}

func (a *Analyzer) parseDo(seq lang.ISeq, env Env) (*ast.Node, error) {
	return a.analyzeBody(seq, seqToSlice(seq.Next()), env)
}

// analyzeBody analyzes an implicit-do body into an OpDo node. An empty
// body yields Ret = const nil.
func (a *Analyzer) analyzeBody(form any, body []any, env Env) (*ast.Node, error) {
	if len(body) == 0 {
		return &ast.Node{Op: ast.OpDo, Form: form, Sub: &ast.DoNode{Ret: constNil()}}, nil
	}
	stmtEnv := env.withContext(CtxStatement)
	stmtEnv.IsTopLevel = false
	stmts := make([]*ast.Node, 0, len(body)-1)
	for _, f := range body[:len(body)-1] {
		n, err := a.analyzeForm(f, stmtEnv)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, n)
	}
	retEnv := env
	retEnv.IsTopLevel = false
	ret, err := a.analyzeForm(body[len(body)-1], retEnv)
	if err != nil {
		return nil, err
	}
	return &ast.Node{Op: ast.OpDo, Form: form, Sub: &ast.DoNode{Statements: stmts, Ret: ret}}, nil
}

// parseDef handles (def sym), (def sym init), (def sym doc init)
// (Compiler.java DefExpr.Parser). The Var is interned at analysis time.
func (a *Analyzer) parseDef(seq lang.ISeq, env Env) (*ast.Node, error) {
	args := seqToSlice(seq.Next())
	var docstring any
	switch len(args) {
	case 1, 2:
	case 3:
		doc, ok := args[1].(string)
		if !ok {
			return nil, a.errf(seq, "too many arguments to def")
		}
		docstring = doc
		args = []any{args[0], args[2]}
	default:
		if len(args) > 3 {
			return nil, a.errf(seq, "too many arguments to def")
		}
		return nil, a.errf(seq, "too few arguments to def")
	}

	sym, ok := args[0].(*lang.Symbol)
	if !ok {
		return nil, a.errf(seq, "first argument to def must be a symbol")
	}
	v, err := a.InternVar(sym)
	if err != nil {
		return nil, a.errPos(seq, err)
	}

	// Symbol metadata (+ docstring) goes onto the var. v0: metadata is
	// constant (from the reader / hand-built forms), so it is applied
	// here rather than analyzed as an expression; DefNode.Meta stays nil.
	meta := sym.Meta()
	if docstring != nil {
		if meta == nil {
			meta = lang.NewMap(lang.KWDoc, docstring)
		} else {
			meta = meta.Assoc(lang.KWDoc, docstring).(lang.IPersistentMap)
		}
	}
	if meta != nil {
		v.SetMeta(meta)
		// ^:dynamic marks the var dynamically rebindable (binding/set!).
		if lang.IsTruthy(lang.Get(meta, lang.KWDynamic)) {
			v.SetDynamic()
		}
	}

	var init *ast.Node
	if len(args) == 2 {
		initEnv := env.withContext(CtxExpr)
		initEnv.IsTopLevel = false
		init, err = a.analyzeForm(args[1], initEnv)
		if err != nil {
			return nil, err
		}
	}
	return &ast.Node{Op: ast.OpDef, Form: seq, Sub: &ast.DefNode{Name: sym, Var: v, Init: init}}, nil
}

// parseLet handles let*: an even-count binding vector of simple symbols,
// sequentially scoped; body is an implicit do (design/03 §2).
func (a *Analyzer) parseLet(seq lang.ISeq, env Env) (*ast.Node, error) {
	return a.parseLetOrLoop(seq, env, false)
}

// parseLoop handles loop*: same parser as let* (Compiler.java uses
// LetExpr.Parser for both), but the body is a recur target — analyzed in
// return context with a fresh RecurFrame (design/03 §2 Phase 2).
func (a *Analyzer) parseLoop(seq lang.ISeq, env Env) (*ast.Node, error) {
	return a.parseLetOrLoop(seq, env, true)
}

func (a *Analyzer) parseLetOrLoop(seq lang.ISeq, env Env, isLoop bool) (*ast.Node, error) {
	formName, kind, op := "let*", ast.BindLet, ast.OpLet
	if isLoop {
		formName, kind, op = "loop*", ast.BindLoop, ast.OpLoop
	}
	args := seqToSlice(seq.Next())
	if len(args) < 1 {
		return nil, a.errf(seq, "%s requires a binding vector", formName)
	}
	bvec, ok := args[0].(lang.IPersistentVector)
	if !ok {
		return nil, a.errf(seq, "%s requires a vector for its bindings", formName)
	}
	if bvec.Count()%2 != 0 {
		return nil, a.errf(seq, "%s requires an even number of forms in binding vector", formName)
	}

	bodyEnv := env
	bindings := make([]*ast.Node, 0, bvec.Count()/2)
	for i := 0; i < bvec.Count(); i += 2 {
		nameForm := bvec.Nth(i)
		initForm := bvec.Nth(i + 1)
		sym, err := a.simpleBindingSym(seq, nameForm)
		if err != nil {
			return nil, err
		}
		initEnv := bodyEnv.withContext(CtxExpr)
		initEnv.IsTopLevel = false
		init, err := a.analyzeForm(initForm, initEnv)
		if err != nil {
			return nil, err
		}
		b := &ast.BindingNode{Name: sym, Init: init, Kind: kind}
		bindings = append(bindings, &ast.Node{Op: ast.OpBinding, Form: sym, Sub: b})
		bodyEnv = bodyEnv.withLocal(b)
	}

	loopID := ""
	if isLoop {
		loopID = a.gensym("loop_")
		bodyEnv.Context = CtxReturn
		bodyEnv.RecurFrame = &RecurFrame{LoopID: loopID, Arity: len(bindings)}
	}
	body, err := a.analyzeBody(seq, args[1:], bodyEnv)
	if err != nil {
		return nil, err
	}
	return &ast.Node{Op: op, Form: seq, Sub: &ast.LetNode{Bindings: bindings, Body: body, LoopID: loopID}}, nil
}

// parseRecur checks — at analysis time, as Clojure does — that recur sits
// in tail position of the innermost loop*/fn-method frame and that its
// arg count matches that frame's arity (design/03 §2 Phase 2). Args are
// analyzed with the frame cleared: no recur inside recur args.
func (a *Analyzer) parseRecur(seq lang.ISeq, env Env) (*ast.Node, error) {
	frame := env.RecurFrame
	if frame == nil || env.Context != CtxReturn {
		return nil, a.errf(seq, "can only recur from tail position")
	}
	if frame.Blocked != "" {
		return nil, a.errf(seq, "cannot recur across %s", frame.Blocked)
	}
	args := seqToSlice(seq.Next())
	if len(args) != frame.Arity {
		return nil, a.errf(seq, "mismatched argument count to recur, expected: %d args, got: %d", frame.Arity, len(args))
	}
	argEnv := env.withContext(CtxExpr)
	argEnv.RecurFrame = nil
	argEnv.IsTopLevel = false
	exprs := make([]*ast.Node, 0, len(args))
	for _, f := range args {
		n, err := a.analyzeForm(f, argEnv)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, n)
	}
	return &ast.Node{Op: ast.OpRecur, Form: seq, Sub: &ast.RecurNode{Exprs: exprs, LoopID: frame.LoopID}}, nil
}

// parseVar handles (var sym): resolve to an EXISTING var or error; the
// node evaluates to the Var object itself (design/03 §2 Phase 2).
func (a *Analyzer) parseVar(seq lang.ISeq, env Env) (*ast.Node, error) {
	args := seqToSlice(seq.Next())
	if len(args) != 1 {
		return nil, a.errf(seq, "wrong number of args (%d) passed to var", len(args))
	}
	sym, ok := args[0].(*lang.Symbol)
	if !ok {
		return nil, a.errf(seq, "var requires a symbol, got: %s", lang.PrintString(args[0]))
	}
	v, err := a.ResolveVar(sym)
	if err != nil {
		return nil, a.errf(seq, "unable to resolve var: %s in this context", sym.FullName())
	}
	return &ast.Node{Op: ast.OpTheVar, Form: seq, Sub: &ast.TheVarNode{Var: v}}, nil
}

// parseSetBang handles (set! target val). v1 targets: an OpVar only —
// per Clojure, whether the var is dynamic AND thread-bound is enforced
// by the evaluator at runtime, not here (design/03 §2 Phase 2). Locals
// get Clojure's "cannot assign to non-mutable" error.
func (a *Analyzer) parseSetBang(seq lang.ISeq, env Env) (*ast.Node, error) {
	args := seqToSlice(seq.Next())
	if len(args) != 2 {
		return nil, a.errf(seq, "malformed assignment, expecting (set! target val)")
	}
	exprEnv := env.withContext(CtxExpr)
	exprEnv.IsTopLevel = false
	target, err := a.analyzeForm(args[0], exprEnv)
	if err != nil {
		return nil, err
	}
	if !target.IsAssignable {
		if target.Op == ast.OpLocal {
			return nil, a.errf(seq, "cannot assign to non-mutable: %s", target.Sub.(*ast.LocalNode).Name.Name())
		}
		return nil, a.errf(seq, "invalid assignment target")
	}
	val, err := a.analyzeForm(args[1], exprEnv)
	if err != nil {
		return nil, err
	}
	return &ast.Node{Op: ast.OpSetBang, Form: seq, Sub: &ast.SetBangNode{Target: target, Val: val}}, nil
}

// parseBinding handles (binding [sym val ...] body...). Each sym must
// resolve to a Var (locals are ignored — Clojure's binding var-izes its
// names); vals are analyzed in expression context and evaluated before
// any binding is pushed. The body's recur frame is marked Blocked:
// Clojure's binding expands to try/finally, so recur out of a binding
// body is "cannot recur across try".
func (a *Analyzer) parseBinding(seq lang.ISeq, env Env) (*ast.Node, error) {
	args := seqToSlice(seq.Next())
	if len(args) < 1 {
		return nil, a.errf(seq, "binding requires a binding vector")
	}
	bvec, ok := args[0].(lang.IPersistentVector)
	if !ok {
		return nil, a.errf(seq, "binding requires a vector for its bindings")
	}
	if bvec.Count()%2 != 0 {
		return nil, a.errf(seq, "binding requires an even number of forms in binding vector")
	}

	exprEnv := env.withContext(CtxExpr)
	exprEnv.IsTopLevel = false
	var vars, vals []*ast.Node
	for i := 0; i < bvec.Count(); i += 2 {
		sym, ok := bvec.Nth(i).(*lang.Symbol)
		if !ok {
			return nil, a.errf(seq, "bad binding form, expected symbol, got: %s", lang.PrintString(bvec.Nth(i)))
		}
		v, err := a.ResolveVar(sym)
		if err != nil {
			return nil, a.errPos(sym, err)
		}
		vars = append(vars, &ast.Node{Op: ast.OpVar, Form: sym, Sub: &ast.VarNode{Var: v}, IsAssignable: true})
		val, err := a.analyzeForm(bvec.Nth(i+1), exprEnv)
		if err != nil {
			return nil, err
		}
		vals = append(vals, val)
	}

	bodyEnv := env
	bodyEnv.IsTopLevel = false
	if bodyEnv.RecurFrame != nil {
		blocked := *bodyEnv.RecurFrame
		blocked.Blocked = "try"
		bodyEnv.RecurFrame = &blocked
	}
	body, err := a.analyzeBody(seq, args[1:], bodyEnv)
	if err != nil {
		return nil, err
	}
	return &ast.Node{Op: ast.OpDynBind, Form: seq, Sub: &ast.DynBindNode{Vars: vars, Vals: vals, Body: body}}, nil
}

// simpleBindingSym validates a binding name: a simple (non-namespaced,
// non-dotted) symbol.
func (a *Analyzer) simpleBindingSym(ctx any, form any) (*lang.Symbol, error) {
	sym, ok := form.(*lang.Symbol)
	if !ok {
		return nil, a.errf(ctx, "bad binding form, expected symbol, got: %s", lang.PrintString(form))
	}
	if sym.HasNamespace() {
		return nil, a.errf(ctx, "can't let qualified name: %s", sym.FullName())
	}
	if strings.Contains(sym.Name(), ".") {
		return nil, a.errf(ctx, "can't bind name containing a period: %s", sym.Name())
	}
	return sym, nil
}

// parseFnStar handles fn* per design/03 §5:
// (fn* name? [params] body...) | (fn* name? ([params] body...)+)
func (a *Analyzer) parseFnStar(seq lang.ISeq, env Env) (*ast.Node, error) {
	rest := seqToSlice(seq.Next())

	var selfBinding *ast.BindingNode
	var selfNode *ast.Node
	if len(rest) > 0 {
		if sym, ok := rest[0].(*lang.Symbol); ok {
			if sym.HasNamespace() {
				return nil, a.errf(seq, "can't use qualified name as fn name: %s", sym.FullName())
			}
			selfBinding = &ast.BindingNode{Name: sym, Kind: ast.BindFn}
			selfNode = &ast.Node{Op: ast.OpBinding, Form: sym, Sub: selfBinding}
			rest = rest[1:]
		}
	}

	// Normalize the single-method shorthand (fn* [params] body...).
	if len(rest) > 0 {
		if _, ok := rest[0].(lang.IPersistentVector); ok {
			rest = []any{lang.NewList(rest...)}
		}
	}
	if len(rest) == 0 {
		return nil, a.errf(seq, "fn* requires at least one method body")
	}

	// The self-name is visible only inside the fn's own bodies.
	methodEnv := env
	if selfBinding != nil {
		methodEnv = methodEnv.withLocal(selfBinding)
	}

	methods := make([]*ast.Node, 0, len(rest))
	variadicCount := 0
	maxFixed := 0
	arities := map[int]bool{}
	var variadicFixed int
	for _, m := range rest {
		mseq, ok := m.(lang.ISeq)
		if !ok {
			return nil, a.errf(seq, "invalid fn* method form: %s", lang.PrintString(m))
		}
		mn, err := a.parseFnMethod(mseq, methodEnv)
		if err != nil {
			return nil, err
		}
		sub := mn.Sub.(*ast.FnMethodNode)
		if sub.IsVariadic {
			variadicCount++
			variadicFixed = sub.FixedArity
			if variadicCount > 1 {
				return nil, a.errf(seq, "can't have more than 1 variadic overload")
			}
		} else {
			if arities[sub.FixedArity] {
				return nil, a.errf(seq, "can't have 2 overloads with same arity")
			}
			arities[sub.FixedArity] = true
			if sub.FixedArity > maxFixed {
				maxFixed = sub.FixedArity
			}
		}
		methods = append(methods, mn)
	}
	if variadicCount > 0 && maxFixed > variadicFixed {
		return nil, a.errf(seq, "can't have fixed arity function with more params than variadic function")
	}

	return &ast.Node{Op: ast.OpFn, Form: seq, Sub: &ast.FnNode{
		Methods:       methods,
		IsVariadic:    variadicCount > 0,
		MaxFixedArity: maxFixed,
		Local:         selfNode,
	}}, nil
}

// parseFnMethod analyzes one ([params] body...) method.
func (a *Analyzer) parseFnMethod(mseq lang.ISeq, env Env) (*ast.Node, error) {
	parts := seqToSlice(mseq)
	if len(parts) == 0 {
		return nil, a.errf(mseq, "fn* method requires a parameter vector")
	}
	pvec, ok := parts[0].(lang.IPersistentVector)
	if !ok {
		return nil, a.errf(mseq, "fn* method requires a vector for its parameters")
	}

	bodyEnv := env
	params := make([]*ast.Node, 0, pvec.Count())
	fixedArity := 0
	isVariadic := false
	argID := 0
	for i := 0; i < pvec.Count(); i++ {
		pform := pvec.Nth(i)
		if sym, isSym := pform.(*lang.Symbol); isSym && !sym.HasNamespace() && sym.Name() == "&" {
			if isVariadic {
				return nil, a.errf(mseq, "invalid parameter list: multiple &")
			}
			if i != pvec.Count()-2 {
				return nil, a.errf(mseq, "invalid parameter list: & must be followed by exactly one rest param")
			}
			isVariadic = true
			continue
		}
		sym, err := a.simpleBindingSym(mseq, pform)
		if err != nil {
			return nil, err
		}
		b := &ast.BindingNode{Name: sym, Kind: ast.BindArg, ArgID: argID, IsVariadic: isVariadic}
		argID++
		if !isVariadic {
			fixedArity++
		}
		params = append(params, &ast.Node{Op: ast.OpBinding, Form: sym, Sub: b})
		bodyEnv = bodyEnv.withLocal(b)
	}

	loopID := a.gensym("fn_method_")
	bodyEnv.Context = CtxReturn
	bodyEnv.RecurFrame = &RecurFrame{LoopID: loopID, Arity: len(params)}
	body, err := a.analyzeBody(mseq, parts[1:], bodyEnv)
	if err != nil {
		return nil, err
	}

	return &ast.Node{Op: ast.OpFnMethod, Form: mseq, Sub: &ast.FnMethodNode{
		Params:     params,
		FixedArity: fixedArity,
		IsVariadic: isVariadic,
		Body:       body,
		LoopID:     loopID,
	}}, nil
}

func (a *Analyzer) parseInvoke(seq lang.ISeq, env Env) (*ast.Node, error) {
	if seq.First() == nil {
		return nil, a.errf(seq, "can't call nil")
	}
	exprEnv := env.withContext(CtxExpr)
	exprEnv.IsTopLevel = false
	fn, err := a.analyzeForm(seq.First(), exprEnv)
	if err != nil {
		return nil, err
	}
	var args []*ast.Node
	for s := seq.Next(); s != nil; s = s.Next() {
		n, err := a.analyzeForm(s.First(), exprEnv)
		if err != nil {
			return nil, err
		}
		args = append(args, n)
	}
	return &ast.Node{Op: ast.OpInvoke, Form: seq, Sub: &ast.InvokeNode{Fn: fn, Args: args}}, nil
}

func constNil() *ast.Node {
	return &ast.Node{Op: ast.OpConst, Form: nil, Sub: &ast.ConstNode{Value: nil}, IsLiteral: true}
}

func seqToSlice(s lang.ISeq) []any {
	var out []any
	for s = lang.Seq(s); s != nil; s = s.Next() {
		out = append(out, s.First())
	}
	return out
}

// errf builds a position-carrying analysis error from the form's metadata.
func (a *Analyzer) errf(form any, format string, args ...any) error {
	return a.errPos(form, fmt.Errorf(format, args...))
}

func (a *Analyzer) errPos(form any, err error) error {
	var ce *lang.CompilerError
	if errors.As(err, &ce) {
		return err // already positioned
	}
	file, line, col := formPos(form)
	return lang.NewCompilerError(file, line, col, err)
}

// formPos extracts :file/:line/:column from a form's metadata
// (design/00 §4.5). Missing metadata yields zero values.
func formPos(form any) (file string, line, col int) {
	im, ok := form.(lang.IMeta)
	if !ok {
		return "", 0, 0
	}
	meta := im.Meta()
	if meta == nil {
		return "", 0, 0
	}
	if f, ok := lang.Get(meta, lang.KWFile).(string); ok {
		file = f
	}
	if l, ok := lang.AsInt(lang.Get(meta, lang.KWLine)); ok {
		line = l
	}
	if c, ok := lang.AsInt(lang.Get(meta, lang.KWColumn)); ok {
		col = c
	}
	return file, line, col
}
