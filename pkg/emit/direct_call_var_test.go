package emit

// direct_call_var_test.go — ADR 0064 cross-var direct calls.
//
// A call to a top-level defn of the SAME compilation unit no longer emits
// an unconditional `v.Get()` + `lang.ApplyN`: the def publishes a
// package-level `lang.FnFuncN` handle and arms the var's ADR 0066 seal
// bit, and a matching-arity call site invokes the handle directly while
// that bit reads true. Any root mutation (a second def, alter-var-root,
// with-redefs) disarms the bit through the ONE shared trip site
// (Var.tripIfSealed), and every site falls back to the unchanged var
// path — so redefinition liveness is observably identical.
//
// These tests freeze both halves: the emitted SHAPE (the indirection is
// really gone) and the BEHAVIOR (with-redefs is still seen, compiled).

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// emitSource runs src through compile + EmitMain and returns the Go text.
func emitSource(t *testing.T, src string) string {
	t.Helper()
	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(src), "test.clj")
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	out, _, err := EmitMain(forms, Options{})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	return string(out)
}

// TestDirectVarCallEmittedShape: the handle is declared, published after
// BindRoot, the seal armed after the publish (that order is the release
// edge), and the call site branches on Direct() into a raw handle call.
func TestDirectVarCallEmittedShape(t *testing.T) {
	s := emitSource(t, `
(defn add2 [a b] (+ a b))
(defn caller [x] (add2 x 1))
`)
	// gofmt pads the var block, so match the declaration loosely.
	if !regexp.MustCompile(`fnD_user_add2\s+lang\.FnFunc2`).MatchString(s) {
		t.Errorf("emitted source missing the fnD_user_add2 lang.FnFunc2 handle:\n%s", s)
	}
	for _, want := range []string{
		"v_user_add2.BindRoot(",    // root installed first…
		"fnD_user_add2 = ",         // …then the handle published…
		"v_user_add2.SealDirect()", // …then the bit armed (release edge)
		"v_user_add2.Direct()",     // call site reads the bit
		"fnD_user_add2(",           // direct invocation, no ApplyN
	} {
		if !strings.Contains(s, want) {
			t.Errorf("emitted source missing %q:\n%s", want, s)
		}
	}
	// The publish must come AFTER BindRoot and BEFORE SealDirect.
	bind := strings.Index(s, "v_user_add2.BindRoot(")
	pub := strings.Index(s, "fnD_user_add2 = ")
	seal := strings.Index(s, "v_user_add2.SealDirect()")
	if !(bind < pub && pub < seal) {
		t.Errorf("publish order wrong: BindRoot@%d publish@%d SealDirect@%d", bind, pub, seal)
	}
	// The fallback arm must still be there — this is a guarded fast path,
	// not a hard seal.
	if !strings.Contains(s, "lang.Apply2(") {
		t.Errorf("emitted source dropped the lang.Apply2 fallback arm:\n%s", s)
	}
}

// TestDirectVarCallForwardReference: a declare'd callee resolves too —
// planDirectVars runs before emission, so the call site emitted BEFORE the
// def still uses the handle (which is nil, and Direct() false, until the
// def runs).
func TestDirectVarCallForwardReference(t *testing.T) {
	s := emitSource(t, `
(declare g)
(defn f [x] (g x))
(defn g [x] (* x 10))
`)
	if !strings.Contains(s, "v_user_g.Direct()") || !strings.Contains(s, "fnD_user_g(") {
		t.Errorf("forward reference did not use the direct handle:\n%s", s)
	}
}

// TestDirectVarCallNotEmittedWhenUnsafe: the conservative gates.
func TestDirectVarCallNotEmittedWhenUnsafe(t *testing.T) {
	cases := map[string]string{
		"multi-arity callee": `(defn m ([a] a) ([a b] (+ a b)))
(defn c [x] (m x 1))`,
		"variadic callee": `(defn v [a & r] a)
(defn c [x] (v x 1))`,
		"redefined callee": `(defn r [a] a)
(defn c [x] (r x))
(defn r [a] (inc a))`,
		"dynamic callee": `(def ^:dynamic d (fn [a] a))
(defn c [x] (d x))`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			s := emitSource(t, src)
			if strings.Contains(s, ".Direct()") {
				t.Errorf("%s: emitted a direct call it must not:\n%s", name, s)
			}
		})
	}
}

// TestDirectVarCallArityMismatchFallsBack: a wrong-arity call keeps the
// ApplyN route so the real lang.NewArityError still fires byte-identically.
func TestDirectVarCallArityMismatchFallsBack(t *testing.T) {
	s := emitSource(t, `
(defn two [a b] (+ a b))
(defn c [x] (apply two [x 1]))
`)
	if strings.Contains(s, "v_user_two.Direct()") {
		t.Errorf("apply route must not go direct:\n%s", s)
	}
}

// TestDirectVarCallWithRedefs: the escape hatch, end to end in a COMPILED
// binary. `callh` calls `h` through the direct path; with-redefs
// alter-var-roots h, which disarms the bit, and the call site must observe
// the redefinition — then the restore, then a second root writer
// (alter-var-root) through the same site.
func TestDirectVarCallWithRedefs(t *testing.T) {
	expectRun(t, `
(defn h [a b] (+ a b))
(defn callh [a b] (h a b))
(def before (callh 2 5))
(def during (with-redefs [h (fn [a b] (- a b))] (callh 2 5)))
(def after (callh 2 5))
(alter-var-root #'h (fn [_] (fn [a b] (* a b))))
[before during after (callh 2 5)]
`, "[7 -3 7 10]\n")
}

// TestDirectVarCallMutualRecursion: declare + two defns calling each
// other, both through published handles.
func TestDirectVarCallMutualRecursion(t *testing.T) {
	expectRun(t, `
(declare pong)
(defn ping [n] (if (zero? n) :done (pong (dec n))))
(defn pong [n] (ping (dec n)))
(ping 8)
`, ":done\n")
}

// TestDirectVarCallValuePosition: the callee used as a VALUE still goes
// through the var — the handle is a call-site optimization only.
func TestDirectVarCallValuePosition(t *testing.T) {
	expectRun(t, `
(defn g [x] (* x 10))
(defn twice [fx v] (fx (fx v)))
[(twice g 2) (map g [1 2 3])]
`, "[200 (10 20 30)]\n")
}
