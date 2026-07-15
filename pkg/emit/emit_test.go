package emit

import (
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// exeSuffix is ".exe" on Windows, "" elsewhere. `go build -o <name>` writes
// exactly <name>, so without the suffix the harness produces a file Windows
// refuses to exec.
var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

func TestMunge(t *testing.T) {
	cases := map[string]string{
		"foo":       "foo",
		"foo-bar":   "foo_bar",
		"pos?":      "pos_QMARK_",
		"set!":      "set_BANG_",
		"*ns*":      "X_STAR_ns_STAR_",
		"+":         "X_PLUS_",
		"->":        "X__GT_",
		"-main":     "X_main",
		"1st":       "X1st",
		"map":       "map_",
		"a.b":       "a_DOT_b",
		"λ":         "X_u03bb_",
		"clj/go":    "clj_SLASH_go",
		"'quoted":   "X_SINGLEQUOTE_quoted",
		"":          "X",
		"_priv":     "X_priv",
		"user_name": "user_name",
	}
	for in, want := range cases {
		if got := munge(in); got != want {
			t.Errorf("munge(%q) = %q, want %q", in, got, want)
		}
	}
	// The X-prefix rule: "-main" must never start with '_' or a digit.
	if got := munge("-main"); got[0] == '_' {
		t.Errorf("munge(-main) starts with underscore: %q", got)
	}
}

// compileAndRun pushes src through the full pipeline — compile (analyze
// + compile-time eval), emit, go build — and returns the binary's
// stdout. PrintLastValue is on: output ends with pr-str of the last
// top-level value.
func compileAndRun(t *testing.T, src string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping compile-and-run in -short mode")
	}
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := eval.Out
	eval.Out = io.Discard // compile-time side effects don't belong to the run
	forms, err := CompileReader(strings.NewReader(src), "test.clj")
	eval.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dir := t.TempDir()
	if err := WriteModule(dir, forms, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("write module: %v", err)
	}
	bin := filepath.Join(dir, "prog"+exeSuffix)
	if err := GoBuild(dir, bin); err != nil {
		t.Fatalf("go build: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return string(out)
}

func expectRun(t *testing.T, src, want string) {
	t.Helper()
	if got := compileAndRun(t, src); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// --- S1 ports ---------------------------------------------------------------

func TestFactRecursive(t *testing.T) {
	expectRun(t, `
(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(fact 10)
`, "3628800\n")
}

func TestLetNestedIfDo(t *testing.T) {
	expectRun(t, `
(let* [a 1
       b (if (< a 2) (+ a 9) 0)]
  (do (println "side")
      (+ a b)))
`, "side\n11\n")
}

func TestLoopRecur100k(t *testing.T) {
	expectRun(t, `
(loop* [i 1 acc 0]
  (if (< 100000 i) acc (recur (+ i 1) (+ acc i))))
`, "5000050000\n")
}

func TestClosureCaptureEscapingLet(t *testing.T) {
	expectRun(t, `
(def add (let* [base 100] (fn* [x] (+ base x))))
[(add 6) (add 7)]
`, "[106 107]\n")
}

// --- S5 ports (the recur/loop edge cases, incl. the capture fix) -------------

func TestClosureOverLoopLocal(t *testing.T) {
	// Clojure closures capture the VALUE at the creating iteration:
	// 0 1 2 — the naive by-reference emission would print 2 2 2 (or the
	// final carrier value). S5 case 1.
	expectRun(t, `
(def fs (loop* [i 0 acc nil]
          (if (< i 3)
            (recur (+ i 1) (cons (fn* [] i) acc))
            acc)))
[((first (next (next fs)))) ((first (next fs))) ((first fs))]
`, "[0 1 2]\n")
}

func TestClosureInLoopBindingInit(t *testing.T) {
	// An init-position closure keeps seeing the INITIAL value forever
	// (binding-var/carrier split, S5 case 1b).
	expectRun(t, `
(loop* [i 0 f (fn* [] i)]
  (if (< i 2) (recur (+ i 1) f) (f)))
`, "0\n")
}

func TestClosureInRecurArgs(t *testing.T) {
	// A closure created in recur args captures that iteration's value
	// (S5 case 1b, second form).
	expectRun(t, `
(loop* [i 0 f (fn* [] i)]
  (if (< i 3) (recur (+ i 1) (fn* [] i)) (f)))
`, "2\n")
}

func TestLoopShadowing(t *testing.T) {
	expectRun(t, `
(let* [x 3] [(loop* [x 5] x) x])
`, "[5 3]\n")
}

func TestSimultaneousRebinding(t *testing.T) {
	// 5 swaps: sequential assignment would collapse to [2 2] (S5 case 4).
	expectRun(t, `
(loop* [n 0 a 1 b 2]
  (if (< n 5) (recur (+ n 1) b a) [a b]))
`, "[2 1]\n")
}

func TestFnLevelRecur(t *testing.T) {
	expectRun(t, `
((fn* [n acc] (if (< n 1) acc (recur (- n 1) (+ acc n)))) 10 0)
`, "55\n")
}

func TestFnLevelRecurConstantStack(t *testing.T) {
	expectRun(t, `
((fn* [n acc] (if (< n 1) acc (recur (- n 1) (+ acc n)))) 100000 0)
`, "5000050000\n")
}

func TestClosureOverFnParam(t *testing.T) {
	// Params are recur carriers too (S5 case 5c).
	expectRun(t, `
(def fs ((fn* go [i acc]
           (if (< i 3)
             (recur (+ i 1) (cons (fn* [] i) acc))
             acc))
         0 nil))
[((first (next (next fs)))) ((first (next fs))) ((first fs))]
`, "[0 1 2]\n")
}

func TestLoopAsExpression(t *testing.T) {
	// Two loops inside one call's args (S5 case 6).
	expectRun(t, `
(+ (loop* [i 0] (if (< i 5) (recur (+ i 1)) i))
   (loop* [j 0] (if (< j 7) (recur (+ j 1)) j)))
`, "12\n")
}

func TestRecurUnderMacroShapes(t *testing.T) {
	// recur in the tail of when (if+do) — macro-expansion shapes fall
	// out of the ""-r-value convention (S5 case 3).
	expectRun(t, `
(loop* [i 0 acc 0]
  (if (< i 10)
    (recur (+ i 1) (+ acc i))
    acc))
`, "45\n")
}

// --- fn representation -------------------------------------------------------

func TestMultiArityAndVariadic(t *testing.T) {
	expectRun(t, `
(def f (fn* ([] 0) ([x] x) ([x & r] (first r))))
[(f) (f 9) (f 1 2 3) (f 1)]
`, "[0 9 2 1]\n")
}

func TestVariadicEmptyRestIsNil(t *testing.T) {
	expectRun(t, `
(def f (fn* [x & r] r))
(f 1)
`, "nil\n")
}

func TestFiveArgCall(t *testing.T) {
	expectRun(t, `
(def f (fn* [a b c d e] (+ a b c d e)))
(f 1 2 3 4 5)
`, "15\n")
}

// --- vars, dynamics, quote ---------------------------------------------------

func TestRedefVisibleThroughVar(t *testing.T) {
	expectRun(t, `
(def g (fn* [] 1))
(def call-g (fn* [] (g)))
(def before (call-g))
(def g (fn* [] 2))
[before (call-g)]
`, "[1 2]\n")
}

func TestDynamicBinding(t *testing.T) {
	expectRun(t, `
(def ^:dynamic *x* 1)
(def probe (fn* [] *x*))
[(binding [*x* 2] (probe)) (probe)]
`, "[2 1]\n")
}

func TestSetBangInsideBinding(t *testing.T) {
	expectRun(t, `
(def ^:dynamic *y* 1)
(binding [*y* 2] (set! *y* 5) *y*)
`, "5\n")
}

func TestQuoteAndCollLiterals(t *testing.T) {
	expectRun(t, `
[:a 'foo '(1 2) {:k (+ 1 2)} #{7} "s" 1.5 nil true \c]
`, "[:a foo (1 2) {:k 3} #{7} \"s\" 1.5 nil true \\c]\n")
}

func TestTheVar(t *testing.T) {
	expectRun(t, `
(def x 1)
(var x)
`, "#=(var user/x)\n")
}

func TestMacroExpandedSource(t *testing.T) {
	// Macros (core.clj's defn/when/->) expand during analysis; the
	// emitter sees only post-expansion AST (ADR 0002).
	expectRun(t, `
(defn twice [x] (* x 2))
(when true (-> 5 twice (+ 1)))
`, "11\n")
}

// --- M3-v0 Go interop (ADR 0010, spike S2) -----------------------------

// TestInteropCompiled exercises the AOT direct-call path end-to-end:
// (T,error) → [v err] vector (with the Go zero value in the error branch),
// int→int64 widening, arg coercion (int64→int), and the `!` unwrap. The
// dual-mode-identical bar is the examples/interop diff; this pins the
// shaping in-package.
func TestInteropCompiled(t *testing.T) {
	expectRun(t, `
(require-go '[strconv])
[(strconv/Atoi "123") (strconv/Itoa 42) (strconv/Atoi! "7") (strconv/Atoi "bad")]
`, `[[123 nil] "42" 7 [0 #object[*strconv.NumError]]]`+"\n")
}

// TestInteropEmittedShape asserts the emitted Go is a direct, non-reflective
// call shaped through the rt helpers — never the interpreter's reflect path.
func TestInteropEmittedShape(t *testing.T) {
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := eval.Out
	eval.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(`
(require-go '[strconv])
(strconv/Atoi "123")
`), "test.clj")
	eval.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	src, _, err := EmitMain(forms, Options{})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	s := string(src)
	for _, want := range []string{
		`"strconv"`,             // real import, not reflect
		"strconv.Atoi(",         // direct call
		".(string)",             // arg coercion
		"lang.NewVector(int64(", // [v err] with int widening
		"rt.NormErr(",           // nil-normalized error slot
	} {
		if !strings.Contains(s, want) {
			t.Errorf("emitted source missing %q:\n%s", want, s)
		}
	}
}

func TestDeterministicOutput(t *testing.T) {
	src := `
(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(fact 10)
`
	emitOnce := func() string {
		lang.RemoveNamespace(lang.NewSymbol("user"))
		oldOut := eval.Out
		eval.Out = io.Discard
		forms, err := CompileReader(strings.NewReader(src), "test.clj")
		eval.Out = oldOut
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		formatted, _, err := EmitMain(forms, Options{})
		if err != nil {
			t.Fatalf("emit: %v", err)
		}
		return string(formatted)
	}
	a, b := emitOnce(), emitOnce()
	if a != b {
		t.Fatalf("emission is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
	// Shape assertions: fixed-arity representation and per-call deref.
	for _, want := range []string{"lang.FnFunc1(", ".Get()", "InternVarName"} {
		if !strings.Contains(a, want) {
			t.Errorf("emitted source missing %q:\n%s", want, a)
		}
	}
	if strings.Contains(a, "func() any {\n}()") {
		t.Errorf("IIFE detected in emitted source")
	}
}
