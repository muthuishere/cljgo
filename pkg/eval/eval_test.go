package eval_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// Forms are hand-constructed with pkg/lang values (the reader is being
// built in parallel and is not a dependency here).

func sym(s string) *lang.Symbol { return lang.NewSymbol(s) }
func list(vals ...any) any      { return lang.NewList(vals...) }
func vec(vals ...any) any       { return lang.NewVector(vals...) }

func evalAll(t *testing.T, e *eval.Evaluator, forms ...any) any {
	t.Helper()
	var res any
	var err error
	for _, f := range forms {
		res, err = e.EvalForm(f)
		if err != nil {
			t.Fatalf("EvalForm(%s): %v", lang.PrintString(f), err)
		}
	}
	return res
}

func mustErr(t *testing.T, e *eval.Evaluator, form any) error {
	t.Helper()
	_, err := e.EvalForm(form)
	if err == nil {
		t.Fatalf("EvalForm(%s): expected error, got none", lang.PrintString(form))
	}
	return err
}

func TestSelfEvaluatingConstants(t *testing.T) {
	e := eval.New()
	for _, form := range []any{nil, true, false, int64(42), 3.5, "hello", lang.NewKeyword("k")} {
		got := evalAll(t, e, form)
		if !lang.Equiv(got, form) {
			t.Errorf("eval(%v) = %v, want identity", form, got)
		}
	}
}

func TestArithmeticBuiltins(t *testing.T) {
	e := eval.New()
	cases := []struct {
		form any
		want any
	}{
		{list(sym("+"), int64(1), int64(2), int64(3)), int64(6)},
		{list(sym("+")), int64(0)},
		{list(sym("+"), int64(7)), int64(7)},
		{list(sym("-"), int64(10), int64(3), int64(2)), int64(5)},
		{list(sym("-"), int64(4)), int64(-4)},
		{list(sym("*"), int64(2), int64(3), int64(4)), int64(24)},
		{list(sym("*")), int64(1)},
		{list(sym("/"), int64(12), int64(3), int64(2)), int64(2)},
		{list(sym("="), int64(1), int64(1), int64(1)), true},
		{list(sym("="), int64(1), int64(2)), false},
		{list(sym("<"), int64(1), int64(2), int64(3)), true},
		{list(sym("<"), int64(1), int64(3), int64(2)), false},
		{list(sym(">"), int64(3), int64(2), int64(1)), true},
		{list(sym(">"), int64(1), int64(2)), false},
	}
	for _, c := range cases {
		got := evalAll(t, e, c.form)
		if !lang.Equiv(got, c.want) {
			t.Errorf("eval(%s) = %v, want %v", lang.PrintString(c.form), got, c.want)
		}
	}
}

func TestIfTruthiness(t *testing.T) {
	e := eval.New()
	cases := []struct {
		test any
		want any
	}{
		{nil, "no"},
		{false, "no"},
		{true, "yes"},
		{int64(0), "yes"}, // 0 is truthy
		{"", "yes"},       // "" is truthy
	}
	for _, c := range cases {
		got := evalAll(t, e, list(sym("if"), list(sym("quote"), c.test), "yes", "no"))
		if got != c.want {
			t.Errorf("(if %v ...) = %v, want %v", c.test, got, c.want)
		}
	}
	// Missing else → nil.
	if got := evalAll(t, e, list(sym("if"), false, "yes")); got != nil {
		t.Errorf("(if false yes) = %v, want nil", got)
	}
}

func TestDoReturnsLastStatementsEvaluated(t *testing.T) {
	e := eval.New()
	var buf bytes.Buffer
	old := eval.Out
	eval.Out = &buf
	defer func() { eval.Out = old }()

	got := evalAll(t, e, list(sym("do"),
		list(sym("println"), "side"),
		int64(1),
		int64(2)))
	if got != int64(2) {
		t.Errorf("do = %v, want 2", got)
	}
	if !strings.Contains(buf.String(), "side") {
		t.Errorf("do statement side effect missing; out=%q", buf.String())
	}
	// Empty do → nil.
	if got := evalAll(t, e, list(sym("do"))); got != nil {
		t.Errorf("(do) = %v, want nil", got)
	}
}

func TestQuote(t *testing.T) {
	e := eval.New()
	datum := list(sym("+"), int64(1), int64(2))
	got := evalAll(t, e, list(sym("quote"), datum))
	if !lang.Equiv(got, datum) {
		t.Errorf("(quote (+ 1 2)) = %v, want the unevaluated datum", got)
	}
}

func TestCollectionLiteralsEvaluateChildren(t *testing.T) {
	e := eval.New()

	got := evalAll(t, e, vec(list(sym("+"), int64(1), int64(2)), int64(9)))
	if !lang.Equiv(got, lang.NewVector(int64(3), int64(9))) {
		t.Errorf("vector literal = %v", lang.PrintString(got))
	}

	got = evalAll(t, e, lang.NewMap(lang.NewKeyword("a"), list(sym("+"), int64(1), int64(1))))
	if !lang.Equiv(got, lang.NewMap(lang.NewKeyword("a"), int64(2))) {
		t.Errorf("map literal = %v", lang.PrintString(got))
	}

	got = evalAll(t, e, lang.NewSet(list(sym("+"), int64(2), int64(2))))
	if !lang.Equiv(got, lang.NewSet(int64(4))) {
		t.Errorf("set literal = %v", lang.PrintString(got))
	}
}

func TestDefAndVarDeref(t *testing.T) {
	e := eval.New()
	res := evalAll(t, e, list(sym("def"), sym("answer"), int64(42)))
	if _, ok := res.(*lang.Var); !ok {
		t.Fatalf("def returned %T, want *lang.Var", res)
	}
	if got := evalAll(t, e, sym("answer")); got != int64(42) {
		t.Errorf("answer = %v, want 42", got)
	}
	// Qualified reference to the same var.
	if got := evalAll(t, e, sym("user/answer")); got != int64(42) {
		t.Errorf("user/answer = %v, want 42", got)
	}
	// Re-def replaces the root.
	evalAll(t, e, list(sym("def"), sym("answer"), int64(43)))
	if got := evalAll(t, e, sym("answer")); got != int64(43) {
		t.Errorf("after re-def, answer = %v, want 43", got)
	}
}

func TestDefWithDocstring(t *testing.T) {
	e := eval.New()
	res := evalAll(t, e, list(sym("def"), sym("documented"), "the doc", int64(7)))
	v := res.(*lang.Var)
	if got := evalAll(t, e, sym("documented")); got != int64(7) {
		t.Errorf("documented = %v, want 7", got)
	}
	if v.Meta() == nil || lang.Get(v.Meta(), lang.KWDoc) != "the doc" {
		t.Errorf("var meta doc = %v, want 'the doc'", lang.Get(v.Meta(), lang.KWDoc))
	}
}

func TestLetSequentialScopingAndShadowing(t *testing.T) {
	e := eval.New()
	// (let* [x 1 y (+ x 1)] (+ x y)) → 3
	got := evalAll(t, e, list(sym("let*"),
		vec(sym("x"), int64(1), sym("y"), list(sym("+"), sym("x"), int64(1))),
		list(sym("+"), sym("x"), sym("y"))))
	if got != int64(3) {
		t.Errorf("let* sequential = %v, want 3", got)
	}
	// Later binding shadows earlier: (let* [x 1 x (+ x 1)] x) → 2
	got = evalAll(t, e, list(sym("let*"),
		vec(sym("x"), int64(1), sym("x"), list(sym("+"), sym("x"), int64(1))),
		sym("x")))
	if got != int64(2) {
		t.Errorf("let* shadow = %v, want 2", got)
	}
	// Local shadows a var of the same name.
	evalAll(t, e, list(sym("def"), sym("shade"), int64(100)))
	got = evalAll(t, e, list(sym("let*"), vec(sym("shade"), int64(1)), sym("shade")))
	if got != int64(1) {
		t.Errorf("local should shadow var: got %v, want 1", got)
	}
}

func TestClosureCapturesBindingAtCreation(t *testing.T) {
	e := eval.New()
	// (let* [x 1 f (fn* [] x) x 2] [(f) x]) → [1 2]
	got := evalAll(t, e, list(sym("let*"),
		vec(
			sym("x"), int64(1),
			sym("f"), list(sym("fn*"), vec(), sym("x")),
			sym("x"), int64(2)),
		vec(list(sym("f")), sym("x"))))
	if !lang.Equiv(got, lang.NewVector(int64(1), int64(2))) {
		t.Errorf("closure capture = %v, want [1 2]", lang.PrintString(got))
	}
}

func TestFnClosureOverLet(t *testing.T) {
	e := eval.New()
	// (def adder (let* [n 10] (fn* [x] (+ x n)))) ; (adder 5) → 15
	evalAll(t, e, list(sym("def"), sym("adder"),
		list(sym("let*"), vec(sym("n"), int64(10)),
			list(sym("fn*"), vec(sym("x")), list(sym("+"), sym("x"), sym("n"))))))
	if got := evalAll(t, e, list(sym("adder"), int64(5))); got != int64(15) {
		t.Errorf("(adder 5) = %v, want 15", got)
	}
}

func TestFnMultiArityDispatch(t *testing.T) {
	e := eval.New()
	// (def m (fn* ([] 0) ([x] 1) ([x y] 2)))
	evalAll(t, e, list(sym("def"), sym("m"), list(sym("fn*"),
		list(vec(), int64(0)),
		list(vec(sym("x")), int64(1)),
		list(vec(sym("x"), sym("y")), int64(2)))))
	for n, want := range map[int]int64{0: 0, 1: 1, 2: 2} {
		args := []any{sym("m")}
		for i := 0; i < n; i++ {
			args = append(args, int64(i))
		}
		if got := evalAll(t, e, list(args...)); got != want {
			t.Errorf("arity %d dispatched to %v, want %v", n, got, want)
		}
	}
	// No matching arity → error, not crash.
	err := mustErr(t, e, list(sym("m"), int64(1), int64(2), int64(3)))
	if !strings.Contains(err.Error(), "wrong number of args (3)") {
		t.Errorf("arity error = %v", err)
	}
}

func TestFnVariadic(t *testing.T) {
	e := eval.New()
	// (def v (fn* [x & rest] [x rest]))
	evalAll(t, e, list(sym("def"), sym("v"),
		list(sym("fn*"), vec(sym("x"), sym("&"), sym("rest")),
			vec(sym("x"), sym("rest")))))

	got := evalAll(t, e, list(sym("v"), int64(1), int64(2), int64(3)))
	want := lang.NewVector(int64(1), lang.NewList(int64(2), int64(3)))
	if !lang.Equiv(got, want) {
		t.Errorf("(v 1 2 3) = %v, want %v", lang.PrintString(got), lang.PrintString(want))
	}
	// Zero rest args → nil binding.
	got = evalAll(t, e, list(sym("v"), int64(1)))
	if !lang.Equiv(got, lang.NewVector(int64(1), nil)) {
		t.Errorf("(v 1) = %v, want [1 nil]", lang.PrintString(got))
	}
	// Fewer than fixed arity → error.
	mustErr(t, e, list(sym("v")))

	// Exact fixed arity beats variadic:
	// (def w (fn* ([x] :fixed) ([x & r] :variadic)))
	evalAll(t, e, list(sym("def"), sym("w"), list(sym("fn*"),
		list(vec(sym("x")), lang.NewKeyword("fixed")),
		list(vec(sym("x"), sym("&"), sym("r")), lang.NewKeyword("variadic")))))
	if got := evalAll(t, e, list(sym("w"), int64(1))); !lang.Equiv(got, lang.NewKeyword("fixed")) {
		t.Errorf("(w 1) = %v, want :fixed", got)
	}
	if got := evalAll(t, e, list(sym("w"), int64(1), int64(2))); !lang.Equiv(got, lang.NewKeyword("variadic")) {
		t.Errorf("(w 1 2) = %v, want :variadic", got)
	}
}

func TestFnSelfNameRecursion(t *testing.T) {
	e := eval.New()
	// Anonymous self-recursion via the fn* self-name, no var involved:
	// ((fn* down [n] (if (< n 1) 0 (down (- n 1)))) 5) → 0
	got := evalAll(t, e, list(
		list(sym("fn*"), sym("down"), vec(sym("n")),
			list(sym("if"), list(sym("<"), sym("n"), int64(1)),
				int64(0),
				list(sym("down"), list(sym("-"), sym("n"), int64(1))))),
		int64(5)))
	if got != int64(0) {
		t.Errorf("self-recursive fn = %v, want 0", got)
	}
}

// factForm builds the M0 exit-test definition:
// (def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
func factForm() any {
	return list(sym("def"), sym("fact"),
		list(sym("fn*"), sym("fact"), vec(sym("n")),
			list(sym("if"), list(sym("<"), sym("n"), int64(2)),
				int64(1),
				list(sym("*"), sym("n"),
					list(sym("fact"), list(sym("-"), sym("n"), int64(1)))))))
}

func TestM0ExitFactorial(t *testing.T) {
	e := eval.New()
	evalAll(t, e, factForm())
	got := evalAll(t, e, list(sym("fact"), int64(10)))
	if got != int64(3628800) {
		t.Fatalf("(fact 10) = %v, want 3628800", got)
	}
}

func TestM0ExitRedefVisibleToExistingCallers(t *testing.T) {
	e := eval.New()
	evalAll(t, e, factForm())

	// A second fn captures a reference to fact BEFORE the re-def. The
	// symbol compiles to an OpVar deref-per-call, never an inlined value
	// (design/00 §4.2), so the re-def must be visible through it.
	evalAll(t, e, list(sym("def"), sym("call-fact"),
		list(sym("fn*"), vec(), list(sym("fact"), int64(10)))))

	if got := evalAll(t, e, list(sym("call-fact"))); got != int64(3628800) {
		t.Fatalf("(call-fact) before re-def = %v, want 3628800", got)
	}

	// Re-def fact to something observably different.
	evalAll(t, e, list(sym("def"), sym("fact"),
		list(sym("fn*"), vec(sym("n")), list(sym("+"), sym("n"), int64(1)))))

	if got := evalAll(t, e, list(sym("call-fact"))); got != int64(11) {
		t.Fatalf("(call-fact) after re-def = %v, want 11 (new fact)", got)
	}
}

func TestTopLevelDoSplitsForms(t *testing.T) {
	e := eval.New()
	got := evalAll(t, e, list(sym("do"),
		list(sym("def"), sym("split-a"), int64(5)),
		list(sym("+"), sym("split-a"), int64(1))))
	if got != int64(6) {
		t.Errorf("top-level do = %v, want 6", got)
	}
}

func TestPrStrAndPrintln(t *testing.T) {
	e := eval.New()
	got := evalAll(t, e, list(sym("pr-str"), "hi", int64(1), vec(int64(2))))
	if s, ok := got.(string); !ok || !strings.Contains(s, "\"hi\"") {
		t.Errorf("pr-str = %v, want readable string with \"hi\"", got)
	}

	var buf bytes.Buffer
	old := eval.Out
	eval.Out = &buf
	defer func() { eval.Out = old }()
	got = evalAll(t, e, list(sym("println"), "hello", int64(42)))
	if got != nil {
		t.Errorf("println returned %v, want nil", got)
	}
	if buf.String() != "hello 42\n" {
		t.Errorf("println wrote %q, want \"hello 42\\n\"", buf.String())
	}
}

func TestRuntimeErrorsSurfaceAsErrors(t *testing.T) {
	e := eval.New()
	// Unresolved symbol (analysis error).
	err := mustErr(t, e, sym("no-such-thing"))
	if !strings.Contains(err.Error(), "unable to resolve symbol: no-such-thing") {
		t.Errorf("unresolved error = %v", err)
	}
	// Invoking a non-fn (runtime panic → error at top level).
	mustErr(t, e, list(list(sym("quote"), int64(1)), int64(2)))
	// Division by zero style arithmetic panic → error.
	mustErr(t, e, list(sym("+"), int64(1), "not-a-number"))
}

func TestEvaluatorsShareUserNamespace(t *testing.T) {
	e1 := eval.New()
	evalAll(t, e1, list(sym("def"), sym("shared-var"), int64(99)))
	e2 := eval.New()
	if got := evalAll(t, e2, sym("shared-var")); got != int64(99) {
		t.Errorf("second evaluator sees %v, want 99 (one global user ns)", got)
	}
}
