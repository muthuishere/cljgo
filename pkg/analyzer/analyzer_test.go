package analyzer_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/analyzer"
	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/lang"
)

func sym(s string) *lang.Symbol { return lang.NewSymbol(s) }
func list(vals ...any) any      { return lang.NewList(vals...) }
func vec(vals ...any) any       { return lang.NewVector(vals...) }

// newAnalyzer wires the hooks against a private test namespace — the
// analyzer itself is pure and only sees the injected functions.
func newAnalyzer(t *testing.T) (*analyzer.Analyzer, *lang.Namespace) {
	t.Helper()
	ns := lang.NewNamespace(sym("analyzer-test"))
	a := &analyzer.Analyzer{
		ResolveVar: func(s *lang.Symbol) (*lang.Var, error) {
			if !s.HasNamespace() {
				if v := ns.FindInternedVar(s); v != nil {
					return v, nil
				}
			}
			return nil, fmt.Errorf("unable to resolve symbol: %s in this context", s.Name())
		},
		InternVar: func(s *lang.Symbol) (*lang.Var, error) {
			if s.HasNamespace() {
				return nil, fmt.Errorf("can't create defs outside of current ns: %s", s.FullName())
			}
			return ns.Intern(s), nil
		},
	}
	return a, ns
}

func analyze(t *testing.T, a *analyzer.Analyzer, form any) *ast.Node {
	t.Helper()
	n, err := a.Analyze(form)
	if err != nil {
		t.Fatalf("Analyze(%s): %v", lang.PrintString(form), err)
	}
	return n
}

func analyzeErr(t *testing.T, a *analyzer.Analyzer, form any) error {
	t.Helper()
	_, err := a.Analyze(form)
	if err == nil {
		t.Fatalf("Analyze(%s): expected error", lang.PrintString(form))
	}
	return err
}

func TestConstLiterals(t *testing.T) {
	a, _ := newAnalyzer(t)
	for _, form := range []any{nil, true, int64(1), 2.5, "s", lang.NewKeyword("k")} {
		n := analyze(t, a, form)
		if n.Op != ast.OpConst || !n.IsLiteral {
			t.Errorf("analyze(%v): op=%v literal=%v, want const literal", form, n.Op, n.IsLiteral)
		}
		if got := n.Sub.(*ast.ConstNode).Value; !lang.Equiv(got, form) {
			t.Errorf("analyze(%v): value %v", form, got)
		}
	}
	// Empty list is self-evaluating.
	n := analyze(t, a, lang.NewList())
	if n.Op != ast.OpConst {
		t.Errorf("analyze(()): op=%v, want const", n.Op)
	}
}

func TestCollectionLiteralNodes(t *testing.T) {
	a, _ := newAnalyzer(t)

	n := analyze(t, a, vec(int64(1), int64(2)))
	if n.Op != ast.OpVector || len(n.Sub.(*ast.VectorNode).Items) != 2 {
		t.Errorf("vector node wrong: %v", n.Op)
	}

	n = analyze(t, a, lang.NewMap(lang.NewKeyword("a"), int64(1)))
	m := n.Sub.(*ast.MapNode)
	if n.Op != ast.OpMap || len(m.Keys) != 1 || len(m.Vals) != 1 {
		t.Errorf("map node wrong: %v", n.Op)
	}

	n = analyze(t, a, lang.NewSet(int64(1), int64(2)))
	if n.Op != ast.OpSet || len(n.Sub.(*ast.SetNode).Items) != 2 {
		t.Errorf("set node wrong: %v", n.Op)
	}
}

func TestQuoteIsUnanalyzed(t *testing.T) {
	a, _ := newAnalyzer(t)
	// (quote (undefined-sym 1)) must not resolve the symbol.
	datum := list(sym("undefined-sym"), int64(1))
	n := analyze(t, a, list(sym("quote"), datum))
	if n.Op != ast.OpQuote || !n.IsLiteral {
		t.Fatalf("quote node: op=%v literal=%v", n.Op, n.IsLiteral)
	}
	if !lang.Equiv(n.Sub.(*ast.QuoteNode).Value, datum) {
		t.Errorf("quote datum changed")
	}
	if err := analyzeErr(t, a, list(sym("quote"), int64(1), int64(2))); !strings.Contains(err.Error(), "quote") {
		t.Errorf("quote arity error = %v", err)
	}
}

func TestIfShapeAndArity(t *testing.T) {
	a, _ := newAnalyzer(t)
	n := analyze(t, a, list(sym("if"), true, int64(1)))
	sub := n.Sub.(*ast.IfNode)
	if n.Op != ast.OpIf || sub.Else == nil {
		t.Fatalf("if node: %+v", sub)
	}
	// Missing else analyzes to const nil.
	if sub.Else.Op != ast.OpConst || sub.Else.Sub.(*ast.ConstNode).Value != nil {
		t.Errorf("missing else should be const nil")
	}
	if err := analyzeErr(t, a, list(sym("if"), true)); !strings.Contains(err.Error(), "too few arguments to if") {
		t.Errorf("if too-few error = %v", err)
	}
	if err := analyzeErr(t, a, list(sym("if"), true, int64(1), int64(2), int64(3))); !strings.Contains(err.Error(), "too many arguments to if") {
		t.Errorf("if too-many error = %v", err)
	}
}

func TestDoShape(t *testing.T) {
	a, _ := newAnalyzer(t)
	n := analyze(t, a, list(sym("do"), int64(1), int64(2), int64(3)))
	sub := n.Sub.(*ast.DoNode)
	if n.Op != ast.OpDo || len(sub.Statements) != 2 || sub.Ret == nil {
		t.Errorf("do node: %d statements, ret=%v", len(sub.Statements), sub.Ret)
	}
	// (do) → Ret const nil.
	n = analyze(t, a, list(sym("do")))
	if n.Sub.(*ast.DoNode).Ret.Sub.(*ast.ConstNode).Value != nil {
		t.Errorf("(do) ret should be const nil")
	}
}

func TestDefInternsAtAnalysisTime(t *testing.T) {
	a, ns := newAnalyzer(t)
	n := analyze(t, a, list(sym("def"), sym("x"), int64(1)))
	sub := n.Sub.(*ast.DefNode)
	if n.Op != ast.OpDef || sub.Var == nil || sub.Init == nil {
		t.Fatalf("def node: %+v", sub)
	}
	if ns.FindInternedVar(sym("x")) != sub.Var {
		t.Errorf("def did not intern the var at analysis time")
	}
	// (def x) with no init: analyzes, Init nil.
	n = analyze(t, a, list(sym("def"), sym("noinit")))
	if n.Sub.(*ast.DefNode).Init != nil {
		t.Errorf("(def noinit) should have nil Init")
	}
	// Forward reference now resolves (interning is load-bearing).
	analyze(t, a, sym("x"))

	if err := analyzeErr(t, a, list(sym("def"), int64(1), int64(2))); !strings.Contains(err.Error(), "must be a symbol") {
		t.Errorf("def non-symbol error = %v", err)
	}
	if err := analyzeErr(t, a, list(sym("def"), sym("y"), int64(1), int64(2))); !strings.Contains(err.Error(), "too many arguments to def") {
		t.Errorf("def arity error = %v", err)
	}
	analyzeErr(t, a, list(sym("def")))
}

func TestDefSelfRecursionResolves(t *testing.T) {
	a, _ := newAnalyzer(t)
	// (def f (fn* [] (f))) — f resolves inside its own init because the
	// var is interned before the init is analyzed.
	analyze(t, a, list(sym("def"), sym("f"),
		list(sym("fn*"), vec(), list(sym("f")))))
}

func TestLetScopingAndShadowing(t *testing.T) {
	a, ns := newAnalyzer(t)
	ns.Intern(sym("shadowed"))

	// Sequential: second init sees the first binding as a local.
	n := analyze(t, a, list(sym("let*"),
		vec(sym("p"), int64(1), sym("q"), sym("p")),
		sym("q")))
	sub := n.Sub.(*ast.LetNode)
	if n.Op != ast.OpLet || len(sub.Bindings) != 2 || sub.LoopID != "" {
		t.Fatalf("let node: %+v", sub)
	}
	qInit := sub.Bindings[1].Sub.(*ast.BindingNode).Init
	if qInit.Op != ast.OpLocal {
		t.Errorf("q's init should resolve p as a local, got %v", qInit.Op)
	}

	// A local shadows a var of the same name.
	n = analyze(t, a, list(sym("let*"), vec(sym("shadowed"), int64(1)), sym("shadowed")))
	if body := n.Sub.(*ast.LetNode).Body.Sub.(*ast.DoNode).Ret; body.Op != ast.OpLocal {
		t.Errorf("local should shadow var, got %v", body.Op)
	}

	// Outside the let, the binding is gone.
	if _, err := a.Analyze(sym("p")); err == nil {
		t.Errorf("let-bound local leaked out of scope")
	}

	// Errors.
	if err := analyzeErr(t, a, list(sym("let*"), vec(sym("x")), sym("x"))); !strings.Contains(err.Error(), "even number of forms") {
		t.Errorf("odd bindings error = %v", err)
	}
	if err := analyzeErr(t, a, list(sym("let*"), vec(sym("a/b"), int64(1)), int64(1))); !strings.Contains(err.Error(), "can't let qualified name") {
		t.Errorf("qualified binding error = %v", err)
	}
	if err := analyzeErr(t, a, list(sym("let*"), vec(int64(1), int64(2)), int64(1))); !strings.Contains(err.Error(), "bad binding form") {
		t.Errorf("non-symbol binding error = %v", err)
	}
	analyzeErr(t, a, list(sym("let*"), int64(1)))
	analyzeErr(t, a, list(sym("let*")))
}

func TestFnStarShapes(t *testing.T) {
	a, _ := newAnalyzer(t)

	// Single-method shorthand normalizes.
	n := analyze(t, a, list(sym("fn*"), vec(sym("x")), sym("x")))
	fn := n.Sub.(*ast.FnNode)
	if n.Op != ast.OpFn || len(fn.Methods) != 1 || fn.IsVariadic || fn.MaxFixedArity != 1 || fn.Local != nil {
		t.Fatalf("fn node: %+v", fn)
	}
	m := fn.Methods[0].Sub.(*ast.FnMethodNode)
	if m.FixedArity != 1 || m.IsVariadic || m.LoopID == "" {
		t.Errorf("method node: %+v", m)
	}

	// Self-name binds inside the body only.
	n = analyze(t, a, list(sym("fn*"), sym("me"), vec(), sym("me")))
	fn = n.Sub.(*ast.FnNode)
	if fn.Local == nil || fn.Local.Sub.(*ast.BindingNode).Kind != ast.BindFn {
		t.Fatalf("self-name missing: %+v", fn)
	}
	if _, err := a.Analyze(sym("me")); err == nil {
		t.Errorf("self-name leaked out of fn scope")
	}

	// Multi-arity + variadic.
	n = analyze(t, a, list(sym("fn*"),
		list(vec(sym("x")), sym("x")),
		list(vec(sym("x"), sym("&"), sym("r")), sym("r"))))
	fn = n.Sub.(*ast.FnNode)
	if !fn.IsVariadic || fn.MaxFixedArity != 1 || len(fn.Methods) != 2 {
		t.Fatalf("multi-arity fn: %+v", fn)
	}
	vm := fn.Methods[1].Sub.(*ast.FnMethodNode)
	if !vm.IsVariadic || vm.FixedArity != 1 || len(vm.Params) != 2 {
		t.Errorf("variadic method: %+v", vm)
	}
	if b := vm.Params[1].Sub.(*ast.BindingNode); !b.IsVariadic {
		t.Errorf("rest param not marked variadic: %+v", b)
	}
}

func TestFnStarErrors(t *testing.T) {
	a, _ := newAnalyzer(t)
	cases := []struct {
		form any
		want string
	}{
		{list(sym("fn*"),
			list(vec(sym("x")), sym("x")),
			list(vec(sym("y")), sym("y"))), "same arity"},
		{list(sym("fn*"),
			list(vec(sym("&"), sym("a")), sym("a")),
			list(vec(sym("x"), sym("&"), sym("b")), sym("b"))), "more than 1 variadic"},
		{list(sym("fn*"),
			list(vec(sym("x"), sym("y")), sym("x")),
			list(vec(sym("&"), sym("r")), sym("r"))), "more params than variadic"},
		{list(sym("fn*"), vec(sym("&"), sym("a"), sym("b")), int64(1)), "exactly one rest param"},
		{list(sym("fn*"), vec(sym("&")), int64(1)), "exactly one rest param"},
		{list(sym("fn*"), vec(sym("a/b")), int64(1)), "can't let qualified name"},
		{list(sym("fn*"), sym("ns/nm"), vec(), int64(1)), "qualified name as fn name"},
		{list(sym("fn*")), "at least one method"},
	}
	for _, c := range cases {
		err := analyzeErr(t, a, c.form)
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("Analyze(%s) error = %q, want contains %q", lang.PrintString(c.form), err, c.want)
		}
	}
}

func TestInvokeShape(t *testing.T) {
	a, ns := newAnalyzer(t)
	ns.Intern(sym("f"))
	n := analyze(t, a, list(sym("f"), int64(1), int64(2)))
	sub := n.Sub.(*ast.InvokeNode)
	if n.Op != ast.OpInvoke || sub.Fn.Op != ast.OpVar || len(sub.Args) != 2 {
		t.Fatalf("invoke node: %+v", sub)
	}
	if err := analyzeErr(t, a, list(nil, int64(1))); !strings.Contains(err.Error(), "can't call nil") {
		t.Errorf("call-nil error = %v", err)
	}
}

func TestSymbolResolution(t *testing.T) {
	a, ns := newAnalyzer(t)
	v := ns.Intern(sym("known"))
	n := analyze(t, a, sym("known"))
	if n.Op != ast.OpVar || n.Sub.(*ast.VarNode).Var != v {
		t.Errorf("var resolution: %v", n.Op)
	}
	err := analyzeErr(t, a, sym("unknown"))
	if !strings.Contains(err.Error(), "unable to resolve symbol: unknown") {
		t.Errorf("unresolved error = %v", err)
	}
}

func TestErrorsCarryPositionFromFormMeta(t *testing.T) {
	a, _ := newAnalyzer(t)
	pos := lang.NewMap(lang.KWFile, "test.clj", lang.KWLine, int64(3), lang.KWColumn, int64(7))
	positioned := sym("unknown-here").WithMeta(pos).(*lang.Symbol)
	err := analyzeErr(t, a, positioned)
	if !strings.Contains(err.Error(), "test.clj:3:7") {
		t.Errorf("error should carry test.clj:3:7, got: %v", err)
	}
}

func TestLoopStarShape(t *testing.T) {
	a, _ := newAnalyzer(t)
	n := analyze(t, a, list(sym("loop*"), vec(sym("x"), int64(1)), list(sym("recur"), sym("x"))))
	sub := n.Sub.(*ast.LetNode)
	if n.Op != ast.OpLoop || sub.LoopID == "" || len(sub.Bindings) != 1 {
		t.Fatalf("loop node: %+v", sub)
	}
	if k := sub.Bindings[0].Sub.(*ast.BindingNode).Kind; k != ast.BindLoop {
		t.Errorf("loop binding kind = %v, want loop", k)
	}
	// The recur in tail position carries the loop's LoopID.
	rec := sub.Body.Sub.(*ast.DoNode).Ret
	if rec.Op != ast.OpRecur || rec.Sub.(*ast.RecurNode).LoopID != sub.LoopID {
		t.Errorf("recur should target the enclosing loop: %+v", rec.Sub)
	}
	// Sequential bindings, like let*.
	n = analyze(t, a, list(sym("loop*"), vec(sym("p"), int64(1), sym("q"), sym("p")), sym("q")))
	qInit := n.Sub.(*ast.LetNode).Bindings[1].Sub.(*ast.BindingNode).Init
	if qInit.Op != ast.OpLocal {
		t.Errorf("loop* bindings should be sequential, q's init = %v", qInit.Op)
	}
}

func TestRecurAnalysisChecks(t *testing.T) {
	a, ns := newAnalyzer(t)
	ns.Intern(sym("+"))

	// Tail position through nested if/do is fine.
	analyze(t, a, list(sym("loop*"), vec(sym("n"), int64(3)),
		list(sym("do"), int64(1),
			list(sym("if"), true, list(sym("recur"), int64(1)), int64(0)))))

	// fn methods are their own recur targets.
	fn := analyze(t, a, list(sym("fn*"), vec(sym("x")), list(sym("recur"), sym("x"))))
	m := fn.Sub.(*ast.FnNode).Methods[0].Sub.(*ast.FnMethodNode)
	rec := m.Body.Sub.(*ast.DoNode).Ret
	if rec.Op != ast.OpRecur || rec.Sub.(*ast.RecurNode).LoopID != m.LoopID {
		t.Errorf("recur should target the fn method: %+v", rec.Sub)
	}

	// Non-tail position (oracle: "Can only recur from tail position").
	err := analyzeErr(t, a, list(sym("loop*"), vec(sym("x"), int64(1)),
		list(sym("+"), list(sym("recur"), int64(2)), int64(1))))
	if !strings.Contains(err.Error(), "recur from tail position") {
		t.Errorf("non-tail recur error = %v", err)
	}
	// No enclosing frame at all.
	if err := analyzeErr(t, a, list(sym("recur"), int64(1))); !strings.Contains(err.Error(), "recur from tail position") {
		t.Errorf("top-level recur error = %v", err)
	}
	// Statement position inside the loop body is not tail.
	if err := analyzeErr(t, a, list(sym("loop*"), vec(), list(sym("recur")), int64(1))); !strings.Contains(err.Error(), "recur from tail position") {
		t.Errorf("statement-position recur error = %v", err)
	}
	// Arity mismatch (oracle: "Mismatched argument count to recur,
	// expected: 2 args, got: 1").
	err = analyzeErr(t, a, list(sym("loop*"), vec(sym("x"), int64(1), sym("y"), int64(2)),
		list(sym("recur"), int64(5))))
	if !strings.Contains(err.Error(), "argument count to recur, expected: 2 args, got: 1") {
		t.Errorf("recur arity error = %v", err)
	}
	// No recur inside recur args (frame cleared).
	err = analyzeErr(t, a, list(sym("loop*"), vec(sym("x"), int64(1)),
		list(sym("recur"), list(sym("recur"), int64(1)))))
	if !strings.Contains(err.Error(), "recur from tail position") {
		t.Errorf("recur-in-recur-args error = %v", err)
	}
	// Variadic method arity counts the rest param.
	analyze(t, a, list(sym("fn*"), vec(sym("x"), sym("&"), sym("r")),
		list(sym("recur"), int64(1), sym("r"))))
}

func TestVarSpecial(t *testing.T) {
	a, ns := newAnalyzer(t)
	v := ns.Intern(sym("known-var"))
	n := analyze(t, a, list(sym("var"), sym("known-var")))
	if n.Op != ast.OpTheVar || n.Sub.(*ast.TheVarNode).Var != v {
		t.Fatalf("var node: %+v", n)
	}
	// Oracle: "Unable to resolve var: no-such-var in this context".
	err := analyzeErr(t, a, list(sym("var"), sym("no-such-var")))
	if !strings.Contains(err.Error(), "unable to resolve var: no-such-var in this context") {
		t.Errorf("var resolve error = %v", err)
	}
	analyzeErr(t, a, list(sym("var")))
	analyzeErr(t, a, list(sym("var"), int64(1)))
}

func TestSetBangAnalysis(t *testing.T) {
	a, ns := newAnalyzer(t)
	ns.Intern(sym("target-var"))

	n := analyze(t, a, list(sym("set!"), sym("target-var"), int64(2)))
	sub := n.Sub.(*ast.SetBangNode)
	if n.Op != ast.OpSetBang || sub.Target.Op != ast.OpVar || !sub.Target.IsAssignable {
		t.Fatalf("set! node: %+v", sub)
	}

	// Locals are not assignable (oracle: "Cannot assign to non-mutable: q").
	err := analyzeErr(t, a, list(sym("let*"), vec(sym("q"), int64(1)), list(sym("set!"), sym("q"), int64(2))))
	if !strings.Contains(err.Error(), "assign to non-mutable: q") {
		t.Errorf("set! local error = %v", err)
	}
	// Arbitrary expressions neither (oracle: "Invalid assignment target").
	err = analyzeErr(t, a, list(sym("set!"), "notatarget", int64(3)))
	if !strings.Contains(err.Error(), "invalid assignment target") {
		t.Errorf("set! target error = %v", err)
	}
	analyzeErr(t, a, list(sym("set!"), sym("target-var")))
}

func TestDefDynamicMetaMarksVar(t *testing.T) {
	a, ns := newAnalyzer(t)
	dyn := sym("*dyn*").WithMeta(lang.NewMap(lang.KWDynamic, true)).(*lang.Symbol)
	analyze(t, a, list(sym("def"), dyn, int64(1)))
	v := ns.FindInternedVar(sym("*dyn*"))
	// No exported dynamic-flag getter on lang.Var: prove it behaviorally —
	// PushThreadBindings panics on non-dynamic vars.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("^:dynamic var not marked dynamic: %v", r)
			}
		}()
		lang.PushThreadBindings(lang.NewMap(v, int64(2)))
		lang.PopThreadBindings()
	}()
}

func TestBindingFormAnalysis(t *testing.T) {
	a, ns := newAnalyzer(t)
	ns.Intern(sym("*b*"))

	n := analyze(t, a, list(sym("binding"), vec(sym("*b*"), int64(2)), sym("*b*")))
	sub := n.Sub.(*ast.DynBindNode)
	if n.Op != ast.OpDynBind || len(sub.Vars) != 1 || len(sub.Vals) != 1 || sub.Body == nil {
		t.Fatalf("binding node: %+v", sub)
	}
	if sub.Vars[0].Op != ast.OpVar {
		t.Errorf("binding var node = %v, want var", sub.Vars[0].Op)
	}

	// The binding name must resolve to a var — locals are ignored
	// (Clojure's binding var-izes its names).
	err := analyzeErr(t, a, list(sym("let*"), vec(sym("loc"), int64(1)),
		list(sym("binding"), vec(sym("loc"), int64(2)), sym("loc"))))
	if !strings.Contains(err.Error(), "unable to resolve symbol: loc") {
		t.Errorf("binding local error = %v", err)
	}

	// recur cannot cross a binding form (oracle: "Cannot recur across
	// try" — Clojure's binding expands to try/finally).
	err = analyzeErr(t, a, list(sym("loop*"), vec(sym("x"), int64(1)),
		list(sym("binding"), vec(sym("*b*"), int64(2)), list(sym("recur"), int64(2)))))
	if !strings.Contains(err.Error(), "recur across try") {
		t.Errorf("recur-across-binding error = %v", err)
	}

	// Shape errors.
	analyzeErr(t, a, list(sym("binding")))
	analyzeErr(t, a, list(sym("binding"), int64(1)))
	analyzeErr(t, a, list(sym("binding"), vec(sym("*b*"))))
	analyzeErr(t, a, list(sym("binding"), vec(int64(1), int64(2)), int64(3)))
}

func TestMacroexpand1HookIsUsed(t *testing.T) {
	a, ns := newAnalyzer(t)
	ns.Intern(sym("g"))
	// A hook that rewrites (my-macro x) → (g x); everything else unchanged.
	a.Macroexpand1 = func(form any, locals map[string]*ast.BindingNode) (any, error) {
		seq, ok := form.(lang.ISeq)
		if !ok {
			return form, nil
		}
		if s, ok := seq.First().(*lang.Symbol); ok && s.Name() == "my-macro" {
			return lang.NewCons(sym("g"), seq.Next()), nil
		}
		return form, nil
	}
	n := analyze(t, a, list(sym("my-macro"), int64(1)))
	if n.Op != ast.OpInvoke {
		t.Fatalf("expanded form should analyze as invoke, got %v", n.Op)
	}
	if fnNode := n.Sub.(*ast.InvokeNode).Fn; fnNode.Op != ast.OpVar {
		t.Errorf("expansion target should resolve to var g, got %v", fnNode.Op)
	}
	// Specials are not macros: quote must reach the special parser even
	// with a hook installed.
	n = analyze(t, a, list(sym("quote"), sym("whatever")))
	if n.Op != ast.OpQuote {
		t.Errorf("special bypassed: %v", n.Op)
	}
}
