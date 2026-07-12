package repl

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// newSession gives each test an isolated `user` namespace: namespaces
// are process-global, so stale defs from a prior test would otherwise
// leak in.
func newSession(in string) (*Driver, *strings.Builder, *strings.Builder) {
	lang.RemoveNamespace(lang.NewSymbol("user"))
	var out, errOut strings.Builder
	d := New(strings.NewReader(in), &out, &errOut)
	return d, &out, &errOut
}

func run(t *testing.T, in string) (string, string) {
	t.Helper()
	d, out, errOut := newSession(in)
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.String(), errOut.String()
}

func outLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func TestEvalAndPrint(t *testing.T) {
	out, errOut := run(t, "(+ 1 2)\n")
	if got := strings.TrimSpace(out); got != "3" {
		t.Fatalf("out = %q, want 3", got)
	}
	if errOut != "" {
		t.Fatalf("unexpected error output: %q", errOut)
	}
}

func TestMultiFormLine(t *testing.T) {
	out, _ := run(t, "(+ 1 2) (* 3 4)\n")
	lines := outLines(out)
	if len(lines) != 2 || lines[0] != "3" || lines[1] != "12" {
		t.Fatalf("lines = %q, want [3 12]", lines)
	}
}

func TestIncompleteFormContinuation(t *testing.T) {
	// The form spans three lines; nothing must evaluate until it closes.
	out, errOut := run(t, "(+ 1\n   2\n   3)\n")
	if got := strings.TrimSpace(out); got != "6" {
		t.Fatalf("out = %q, want 6", got)
	}
	if errOut != "" {
		t.Fatalf("unexpected error output: %q", errOut)
	}
}

func TestIncompleteStringContinuation(t *testing.T) {
	out, _ := run(t, "\"ab\ncd\"\n")
	if got := strings.TrimSpace(out); got != `"ab\ncd"` {
		t.Fatalf("out = %q", got)
	}
}

func TestUnterminatedAtEOFIsError(t *testing.T) {
	out, errOut := run(t, "(+ 1 2")
	if out != "" {
		t.Fatalf("nothing should print, got %q", out)
	}
	if !strings.Contains(errOut, "EOF while reading") || !strings.Contains(errOut, "REPL:") {
		t.Fatalf("want positioned EOF error, got %q", errOut)
	}
}

func TestSyntaxErrorWithPositionThenRecovers(t *testing.T) {
	out, errOut := run(t, ")\n(+ 1 1)\n")
	if !strings.Contains(errOut, "REPL:1:") {
		t.Fatalf("want position in error, got %q", errOut)
	}
	if got := strings.TrimSpace(out); got != "2" {
		t.Fatalf("loop must continue after syntax error, out = %q", out)
	}
}

func TestEvalErrorBindsStarEAndContinues(t *testing.T) {
	// *e is a session-bound dynamic var: it must hold the LAST error
	// while the session runs (checked in-session via the if below) and
	// revert to its nil root once Run's session frame pops.
	in := "(undefined-sym 1)\n(f 1 2 3)\n(if *e :err-bound :no-err)\n(+ 1 1)\n"
	d, out, errOut := newSession("(def f (fn* f [x] x))\n" + in)
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "unable to resolve symbol: undefined-sym") {
		t.Fatalf("missing resolve error: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "wrong number of args (3) passed to: f") {
		t.Fatalf("missing arity error (panic must be recovered): %q", errOut.String())
	}
	if !strings.Contains(out.String(), ":err-bound") {
		t.Fatalf("*e not bound in-session: %q", out.String())
	}
	ve := lang.NSCore.FindInternedVar(lang.NewSymbol("*e"))
	if got := ve.Deref(); got != nil {
		t.Fatalf("*e after session = %v, want nil root (session binding popped)", got)
	}
}

func TestStar123Shift(t *testing.T) {
	out, _ := run(t, "1\n2\n3\n[*1 *2 *3]\n")
	lines := outLines(out)
	if got := lines[len(lines)-1]; got != "[3 2 1]" {
		t.Fatalf("[*1 *2 *3] = %q, want [3 2 1]", got)
	}
}

func TestRedefVisibleToCapturedReference(t *testing.T) {
	in := `(def f (fn* [x] (+ x 1)))
(def g (fn* [x] (f x)))
(g 1)
(def f (fn* [x] (* x 100)))
(g 1)
`
	out, _ := run(t, in)
	lines := outLines(out)
	if lines[2] != "2" || lines[4] != "100" {
		t.Fatalf("re-def not live: %q", lines)
	}
}

func TestPrompts(t *testing.T) {
	lang.RemoveNamespace(lang.NewSymbol("user"))
	var out, errOut strings.Builder
	d := New(strings.NewReader("(+ 1\n2)\n"), &out, &errOut)
	d.Prompts = true
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "user=> ") || !strings.Contains(out.String(), "#_=> ") {
		t.Fatalf("want primary and continuation prompts, got %q", out.String())
	}
}

func TestPromptFollowsInNs(t *testing.T) {
	lang.RemoveNamespace(lang.NewSymbol("user"))
	lang.RemoveNamespace(lang.NewSymbol("repl-test.moved"))
	var out, errOut strings.Builder
	d := New(strings.NewReader("(in-ns 'repl-test.moved)\n(clojure.core/refer 'clojure.core)\n(clojure.core/+ 1 2)\n"), &out, &errOut)
	d.Prompts = true
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "user=> ") || !strings.Contains(out.String(), "repl-test.moved=> ") {
		t.Fatalf("prompt should follow in-ns, got %q", out.String())
	}
	// The session's *ns* binding pops with the session: a fresh driver
	// starts back in user.
	if got := d.Evaluator().CurrentNS().Name().Name(); got != "user" {
		t.Fatalf("ns after session = %q, want user (session *ns* binding popped)", got)
	}
	if errOut.String() != "" {
		t.Fatalf("unexpected error output: %q", errOut.String())
	}
}

func TestStar123AreSessionDynamicVars(t *testing.T) {
	// *1 *2 *3 are core dynamic vars bound per session: usable via
	// binding/set! semantics in-session, nil roots after.
	out, _ := run(t, "7\n(binding [*1 :shadow] *1)\n")
	lines := outLines(out)
	if lines[len(lines)-1] != ":shadow" {
		t.Fatalf("*1 should be dynamically rebindable, got %q", lines)
	}
	v1 := lang.NSCore.FindInternedVar(lang.NewSymbol("*1"))
	if got := v1.Deref(); got != nil {
		t.Fatalf("*1 after session = %v, want nil root", got)
	}
}

func TestEvalStringLastValue(t *testing.T) {
	d, _, _ := newSession("")
	v, err := d.EvalString("(def x 20) (+ x 2)", "test.clj")
	if err != nil {
		t.Fatalf("EvalString: %v", err)
	}
	if got := lang.PrintString(v); got != "22" {
		t.Fatalf("last value = %q, want 22", got)
	}
}

func TestEvalStringErrorHasPosition(t *testing.T) {
	d, _, _ := newSession("")
	_, err := d.EvalString("(+ 1 1)\n(let* [x] x)", "test.clj")
	if err == nil || !strings.Contains(err.Error(), "test.clj:2") {
		t.Fatalf("want positioned analyzer error, got %v", err)
	}
}
