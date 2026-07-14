package lang

import "testing"

// Result/Option primitive tests (ADR 0014, spike S11 variant D). Covers
// the type-per-tag predicates, nil-safety, Equiv/Hash value semantics,
// and readable printing.

func TestResultConstructorsAndPredicates(t *testing.T) {
	okv := NewOk(1)
	errv := NewErr("boom")
	justv := NewJust(5)

	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"ok is result", IsResult(okv), true},
		{"err is result", IsResult(errv), true},
		{"just is not result", IsResult(justv), false},
		{"none is not result", IsResult(None), false},
		{"just is option", IsOption(justv), true},
		{"none is option", IsOption(None), true},
		{"ok is not option", IsOption(okv), false},
		{"ok? ok", IsOk(okv), true},
		{"ok? err", IsOk(errv), false},
		{"err? err", IsErr(errv), true},
		{"just? just", IsJust(justv), true},
		{"none? none", IsNone(None), true},
		{"none? just", IsNone(justv), false},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestResultNilSafety(t *testing.T) {
	// (just nil) is a DISTINCT type from none — a nil payload never
	// collapses into absence (the spike's nil-safety-free property).
	jn := NewJust(nil)
	if !IsJust(jn) {
		t.Fatal("(just nil) should be a just")
	}
	if IsNone(jn) {
		t.Fatal("(just nil) must not be none")
	}
	if Equiv(jn, None) {
		t.Fatal("(just nil) must not equal none")
	}
	if ResultPayload(jn) != nil {
		t.Fatalf("payload of (just nil) should be nil, got %v", ResultPayload(jn))
	}
}

func TestResultPayload(t *testing.T) {
	if got := ResultPayload(NewOk(7)); got != int64OrInt(got, 7) {
		t.Fatalf("ok payload: got %v", got)
	}
	if ResultPayload(None) != nil {
		t.Fatal("none payload should be nil")
	}
}

// int64OrInt lets the literal 7 compare regardless of the boxed int
// kind returned by the constructor (payloads box whatever they're given).
func int64OrInt(got any, want int) any {
	switch got.(type) {
	case int64:
		return int64(want)
	default:
		return want
	}
}

func TestResultEquiv(t *testing.T) {
	if !Equiv(NewOk(1), NewOk(1)) {
		t.Error("(= (ok 1) (ok 1)) should hold")
	}
	if Equiv(NewOk(1), NewOk(2)) {
		t.Error("(= (ok 1) (ok 2)) should be false")
	}
	if Equiv(NewOk(1), NewErr(1)) {
		t.Error("(= (ok 1) (err 1)) should be false — different tags")
	}
	if Equiv(NewJust(1), NewOk(1)) {
		t.Error("(= (just 1) (ok 1)) should be false — different tags")
	}
	// Collection payloads compare structurally, not by pointer identity.
	v1 := NewOk(NewVector(int64(1), int64(2)))
	v2 := NewOk(NewVector(int64(1), int64(2)))
	if !Equiv(v1, v2) {
		t.Error("(= (ok [1 2]) (ok [1 2])) should hold structurally")
	}
	if !Equiv(None, None) {
		t.Error("(= none none) should hold")
	}
	// Comparing a tagged value with a non-tagged value must not panic.
	if Equiv(NewOk(1), int64(1)) {
		t.Error("(= (ok 1) 1) should be false")
	}
}

func TestResultIncomparablePayloadEquivNoPanic(t *testing.T) {
	// A raw Go slice payload is not `==`-comparable as a struct field;
	// Equiv must route these types before the fast path (no panic).
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Equiv panicked on incomparable payload: %v", r)
		}
	}()
	a := NewOk([]int{1, 2, 3})
	b := NewOk([]int{1, 2, 3})
	_ = Equiv(a, b)
}

func TestResultHashEq(t *testing.T) {
	// Equal values hash equal; the four tag families are distinguished.
	if HashEq(NewOk(1)) != HashEq(NewOk(1)) {
		t.Error("equal ok values must hash equal")
	}
	if HashEq(NewOk(1)) == HashEq(NewErr(1)) {
		t.Error("ok and err with same payload should differ in hash family")
	}
	if HashEq(None) != HashEq(None) {
		t.Error("none must hash stably")
	}
}

func TestResultPrinting(t *testing.T) {
	cases := []struct {
		val  any
		want string
	}{
		{NewOk(int64(1)), "#cljgo/ok 1"},
		{NewErr(NewKeyword("boom")), "#cljgo/err :boom"},
		{NewJust("hi"), `#cljgo/just "hi"`},               // payload printed readably
		{NewJust(nil), "#cljgo/just nil"},                 // nil payload distinct from none
		{None, "none"},                                    // the sentinel prints bare
		{NewOk(NewOk(int64(2))), "#cljgo/ok #cljgo/ok 2"}, // nesting
	}
	for _, c := range cases {
		if got := PrintString(c.val); got != c.want {
			t.Errorf("PrintString: got %q, want %q", got, c.want)
		}
	}
}
