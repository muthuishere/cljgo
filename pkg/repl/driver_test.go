package repl

import (
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestCompletedFormNotSwallowedByLaterSyntaxError(t *testing.T) {
	// Line 2's first ) completes the pending form (- 23 56 \)) — its
	// result MUST print (our eval numerically coerces the char to -74;
	// JVM Clojure throws a cast error — either way, whatever eval
	// produces must surface). Only the second ) is a syntax error.
	out, errOut := run(t, "(- 23 56\\)\n))\n(+ 1 2)\n")
	lines := outLines(out)
	if len(lines) != 2 || lines[0] != "-74" || lines[1] != "3" {
		t.Fatalf("completed form swallowed: out = %q, want [-74 3]", lines)
	}
	if !strings.Contains(errOut, "Unmatched delimiter") {
		t.Fatalf("want unmatched-delimiter error, got %q", errOut)
	}
}

func TestFormsEvaluateAsEachCompletes(t *testing.T) {
	// The complete forms on line 2 evaluate immediately even though the
	// line ends mid-form; when line 3 closes the tail, the already-
	// evaluated forms must NOT run again (c would become 2 → 102).
	in := "(def c 0)\n(def c (+ c 1)) (+ c\n100)\n"
	out, errOut := run(t, in)
	lines := outLines(out)
	if got := lines[len(lines)-1]; got != "101" {
		t.Fatalf("out = %q, want last line 101 (no re-evaluation)", lines)
	}
	if errOut != "" {
		t.Fatalf("unexpected error output: %q", errOut)
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

// syncBuffer is a goroutine-safe writer: interrupt tests read the
// driver's output while Run is still producing it.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitFor(t *testing.T, what string, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// startInteractive runs a prompting driver over a pipe so tests can
// feed input and interrupt with real timing.
func startInteractive(t *testing.T) (w io.WriteCloser, d *Driver, out *syncBuffer, errOut *syncBuffer, ran chan error) {
	t.Helper()
	lang.RemoveNamespace(lang.NewSymbol("user"))
	pr, pw := io.Pipe()
	out, errOut = &syncBuffer{}, &syncBuffer{}
	d = New(pr, out, errOut)
	d.Prompts = true
	ran = make(chan error, 1)
	go func() { ran <- d.Run() }()
	waitFor(t, "first prompt", func() bool { return strings.Contains(out.String(), "user=> ") })
	return pw, d, out, errOut, ran
}

func TestInterruptDiscardsPendingContinuation(t *testing.T) {
	w, d, out, errOut, ran := startInteractive(t)
	io.WriteString(w, "(+ 1\n") // leave a form open → continuation prompt
	waitFor(t, "continuation prompt", func() bool { return strings.Contains(out.String(), "#_=> ") })

	d.Interrupt() // Ctrl-C: discard the stuck continuation, fresh prompt
	waitFor(t, "fresh prompt after interrupt", func() bool {
		return strings.Count(out.String(), "user=> ") >= 2
	})

	io.WriteString(w, "(* 2 3)\n") // next input starts clean, not appended to (+ 1
	waitFor(t, "result 6", func() bool { return strings.Contains(out.String(), "6\n") })
	w.Close()
	if err := <-ran; err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The discarded "(+ 1" must not resurface as an EOF error at exit.
	if errOut.String() != "" {
		t.Fatalf("unexpected error output: %q", errOut.String())
	}
}

func TestInterruptAtEmptyPromptKeepsSessionAlive(t *testing.T) {
	w, d, out, _, ran := startInteractive(t)
	d.Interrupt() // Ctrl-C at an empty prompt: newline + prompt, no exit
	waitFor(t, "prompt redraw", func() bool {
		return strings.Count(out.String(), "user=> ") >= 2
	})
	io.WriteString(w, "(+ 2 2)\n")
	waitFor(t, "result 4", func() bool { return strings.Contains(out.String(), "4\n") })
	w.Close()
	if err := <-ran; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestSIGINTIsHandledInsideRun(t *testing.T) {
	// Run installs its own SIGINT listener: a real signal must behave
	// exactly like Interrupt() — discard pending input, keep running.
	w, _, out, errOut, ran := startInteractive(t)
	io.WriteString(w, "(- 9\n")
	waitFor(t, "continuation prompt", func() bool { return strings.Contains(out.String(), "#_=> ") })

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "fresh prompt after SIGINT", func() bool {
		return strings.Count(out.String(), "user=> ") >= 2
	})

	io.WriteString(w, "(+ 40 2)\n")
	waitFor(t, "result 42", func() bool { return strings.Contains(out.String(), "42\n") })
	w.Close()
	if err := <-ran; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errOut.String() != "" {
		t.Fatalf("unexpected error output: %q", errOut.String())
	}
}

// panicOnPrint blows up when the printer stringifies it: results with
// broken print paths must land in *e, not kill the loop.
type panicOnPrint struct{}

func (panicOnPrint) String() string { panic("boom: print exploded") }

func TestPrintPanicRecoveredIntoStarE(t *testing.T) {
	in := "boom-val\n(if *e :err-bound :no-err)\n(+ 1 1)\n"
	d, out, errOut := newSession(in)
	d.Evaluator().CurrentNS().InternWithValue(
		lang.NewSymbol("boom-val"), panicOnPrint{}, true)
	if err := d.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "boom: print exploded") {
		t.Fatalf("print panic not reported: %q", errOut.String())
	}
	if !strings.Contains(out.String(), ":err-bound") {
		t.Fatalf("*e not bound after print panic: %q", out.String())
	}
	lines := outLines(out.String())
	if got := lines[len(lines)-1]; got != "2" {
		t.Fatalf("loop must survive a print panic, out = %q", out.String())
	}
}
