package eval_test

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// sym/list/vec/evalAll/mustErr come from eval_test.go and kw from
// testclj_test.go (same package). require-go libspecs are quoted so their
// symbols/keywords arrive unevaluated, exactly as at the REPL.
func requireGo(t *testing.T, e *eval.Evaluator, spec any) {
	t.Helper()
	evalAll(t, e, list(sym("require-go"), list(sym("quote"), spec)))
}

func vecOf(t *testing.T, v any) lang.IPersistentVector {
	t.Helper()
	pv, ok := v.(lang.IPersistentVector)
	if !ok {
		t.Fatalf("expected a vector, got %T: %s", v, lang.PrintString(v))
	}
	return pv
}

func TestHostSingleReturn(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strings")))
	if got := evalAll(t, e, list(sym("strings/ToUpper"), "hi")); got != "HI" {
		t.Errorf("(strings/ToUpper \"hi\") = %v, want HI", got)
	}
}

func TestHostIntCoercion(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strings")))
	// strings.Repeat(s string, count int); Clojure int64 → Go int.
	if got := evalAll(t, e, list(sym("strings/Repeat"), "ab", int64(2))); got != "abab" {
		t.Errorf("(strings/Repeat \"ab\" 2) = %v, want abab", got)
	}
}

func TestHostItoa(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strconv")))
	if got := evalAll(t, e, list(sym("strconv/Itoa"), int64(42))); got != "42" {
		t.Errorf("(strconv/Itoa 42) = %v, want \"42\"", got)
	}
}

func TestHostVErrHappy(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strconv")))
	// (strconv/Atoi "42") → [42 nil]: value normalized to int64, error slot nil.
	got := evalAll(t, e, list(sym("strconv/Atoi"), "42"))
	pv := vecOf(t, got)
	if pv.Count() != 2 {
		t.Fatalf("Atoi ok vector count = %d, want 2: %s", pv.Count(), lang.PrintString(got))
	}
	if pv.Nth(0) != int64(42) {
		t.Errorf("Atoi ok value = %v (%T), want int64(42)", pv.Nth(0), pv.Nth(0))
	}
	if pv.Nth(1) != nil {
		t.Errorf("Atoi ok error slot = %v, want nil", pv.Nth(1))
	}
}

func TestHostVErrError(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strconv")))
	// (strconv/Atoi "x") → [0 <err>]: Go returns (0, err); the value is the
	// passed-through Go zero (int64 0, NOT nil — shaping is exact and the
	// AOT emitter emits the same 0), the error slot is a truthy Go error.
	got := evalAll(t, e, list(sym("strconv/Atoi"), "x"))
	pv := vecOf(t, got)
	if pv.Count() != 2 {
		t.Fatalf("Atoi err vector count = %d, want 2: %s", pv.Count(), lang.PrintString(got))
	}
	if pv.Nth(0) != int64(0) {
		t.Errorf("Atoi err value = %v (%T), want int64(0)", pv.Nth(0), pv.Nth(0))
	}
	if !lang.IsTruthy(pv.Nth(1)) {
		t.Errorf("Atoi err slot = %v, want a truthy error", pv.Nth(1))
	}
}

func TestHostThrowHappy(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strconv")))
	// (strconv/Atoi! "42") → 42 (unwrapped), no vector.
	if got := evalAll(t, e, list(sym("strconv/Atoi!"), "42")); got != int64(42) {
		t.Errorf("(strconv/Atoi! \"42\") = %v (%T), want int64(42)", got, got)
	}
}

func TestHostThrowError(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strconv")))
	// (strconv/Atoi! "x") → the Go error is panicked and recovered into an error.
	err := mustErr(t, e, list(sym("strconv/Atoi!"), "x"))
	if !strings.Contains(err.Error(), "invalid syntax") {
		t.Errorf("throw error = %v, want the strconv parse error", err)
	}
}

func TestHostConstRef(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("math")))
	// math/Pi is an OpHostRef to a const value (not a func).
	if got := evalAll(t, e, sym("math/Pi")); got != 3.141592653589793 {
		t.Errorf("math/Pi = %v (%T), want 3.141592653589793", got, got)
	}
}

func TestHostFnAsValue(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strings")))
	// (def f strings/ToUpper) (f "x") → "X": fn-as-value via OpHostRef.
	evalAll(t, e, list(sym("def"), sym("f"), sym("strings/ToUpper")))
	if got := evalAll(t, e, list(sym("f"), "x")); got != "X" {
		t.Errorf("(f \"x\") = %v, want X", got)
	}
}

func TestHostAsAlias(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strconv"), kw("as"), sym("sc")))
	if got := evalAll(t, e, list(sym("sc/Itoa"), int64(7))); got != "7" {
		t.Errorf("(sc/Itoa 7) = %v, want \"7\"", got)
	}
}

func TestHostStringPath(t *testing.T) {
	e := eval.New()
	// String path (may contain slashes); default alias is the last segment.
	requireGo(t, e, vec("fmt"))
	if got := evalAll(t, e, list(sym("fmt/Sprintf"), "%d-%s", int64(3), "x")); got != "3-x" {
		t.Errorf("(fmt/Sprintf ...) = %v, want 3-x", got)
	}
}

// TestHostPrecedenceClojureWins is the CLAUDE.md non-negotiable: a
// require-go alias never shadows a Clojure namespace or ns-alias.
func TestHostPrecedenceClojureWins(t *testing.T) {
	e := eval.New()
	requireGo(t, e, vec(sym("strconv"), kw("as"), sym("sc")))
	// Before: sc is only a require-go alias → host call works.
	if got := evalAll(t, e, list(sym("sc/Itoa"), int64(1))); got != "1" {
		t.Fatalf("(sc/Itoa 1) = %v, want \"1\"", got)
	}
	// Introduce a Clojure alias sc → clojure.core. Now sc is a Clojure
	// ns-alias, so it wins: resolveHost yields false and sc/Itoa resolves
	// (and fails) as a Clojure var, never as the Go member.
	evalAll(t, e, list(sym("alias"), list(sym("quote"), sym("sc")), list(sym("quote"), sym("clojure.core"))))
	err := mustErr(t, e, list(sym("sc/Itoa"), int64(1)))
	if !strings.Contains(err.Error(), "no such var") {
		t.Errorf("after Clojure alias, sc/Itoa error = %v, want a Clojure 'no such var'", err)
	}
	// A real clojure.core call through a colliding require-go alias still
	// hits Clojure, proving the alias never hijacks the namespace.
	requireGo(t, e, vec(sym("strings"), kw("as"), sym("clojure.core")))
	if got := evalAll(t, e, list(sym("clojure.core/+"), int64(2), int64(3))); got != int64(5) {
		t.Errorf("(clojure.core/+ 2 3) = %v, want 5 (Clojure wins)", got)
	}
}
