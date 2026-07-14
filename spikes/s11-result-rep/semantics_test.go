package s11

import (
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// D1 semantic gates, per candidate:
//   1. (= (ok 1) (ok 1)) is true; (= (ok 1) (err 1)) is false
//   2. (ok nil) and none are distinguishable — REQUIRED
//   3. results work as persistent-map keys (fresh key finds stored value)
//   4. pr-str emits the D4 tagged-literal form #cljgo/ok 5

type candidate struct {
	name    string
	ok      func(any) any
	err     func(any) any
	none    any
	isOk    func(any) bool
	isErr   func(any) bool
	unwrap  func(any) any
	andThen func(any, func(any) any) any
}

var candidates = []candidate{
	{"A_TaggedPtr", OkA, ErrA, NoneA, IsOkA, IsErrA, UnwrapA, AndThenA},
	{"B_Vector", OkB, ErrB, NoneB, IsOkB, IsErrB, UnwrapB, AndThenB},
	{"C_StructVal", OkC, ErrC, NoneC, IsOkC, IsErrC, UnwrapC, AndThenC},
	{"D_TypePerTag", MkOkD, MkErrD, NoneD, IsOkD, IsErrD, UnwrapD, AndThenD},
}

func TestEquivSemantics(t *testing.T) {
	for _, c := range candidates {
		t.Run(c.name, func(t *testing.T) {
			if !lang.Equiv(c.ok(1), c.ok(1)) {
				t.Error("(= (ok 1) (ok 1)) must be true")
			}
			if lang.Equiv(c.ok(1), c.ok(2)) {
				t.Error("(= (ok 1) (ok 2)) must be false")
			}
			if lang.Equiv(c.ok(1), c.err(1)) {
				t.Error("(= (ok 1) (err 1)) must be false")
			}
			// Clojure number-category equality must flow through: (= 1 1.0)
			// is false, so (= (ok 1) (ok 1.0)) is false.
			if lang.Equiv(c.ok(1), c.ok(1.0)) {
				t.Error("(= (ok 1) (ok 1.0)) must be false (number categories)")
			}
			// HashEq contract: equal values hash equal, ok/err hash apart.
			if lang.HashEq(c.ok(1)) != lang.HashEq(c.ok(1)) {
				t.Error("equal oks must hash equal")
			}
			if lang.HashEq(c.ok(1)) == lang.HashEq(c.err(1)) {
				t.Error("(ok 1) and (err 1) should hash apart")
			}
		})
	}
}

func TestNilSafety(t *testing.T) {
	for _, c := range candidates {
		t.Run(c.name, func(t *testing.T) {
			okNil := c.ok(nil)
			if lang.IsNil(okNil) {
				t.Error("(ok nil) must not be nil")
			}
			if lang.Equiv(okNil, c.none) {
				t.Error("(ok nil) and none must be distinguishable")
			}
			if lang.Equiv(okNil, nil) {
				t.Error("(ok nil) must not equal nil")
			}
			if !c.isOk(okNil) {
				t.Error("(ok? (ok nil)) must be true")
			}
			if c.unwrap(okNil) != nil {
				t.Error("(unwrap (ok nil)) must be nil")
			}
			if !lang.Equiv(c.none, c.none) {
				t.Error("(= none none) must be true")
			}
		})
	}
}

func TestMapKey(t *testing.T) {
	for _, c := range candidates {
		t.Run(c.name, func(t *testing.T) {
			m := lang.NewPersistentHashMap(c.ok(1), "one", c.err(2), "two", c.none, "none")
			if got := m.ValAt(c.ok(1)); got != "one" {
				t.Errorf("fresh (ok 1) key lookup: got %v, want \"one\"", got)
			}
			if got := m.ValAt(c.err(2)); got != "two" {
				t.Errorf("fresh (err 2) key lookup: got %v, want \"two\"", got)
			}
			if got := m.ValAt(c.none); got != "none" {
				t.Errorf("none key lookup: got %v, want \"none\"", got)
			}
			if got := m.ValAt(c.ok(2)); got != nil {
				t.Errorf("(ok 2) must not be found: got %v", got)
			}
		})
	}
}

func TestRailwayShortCircuit(t *testing.T) {
	for _, c := range candidates {
		t.Run(c.name, func(t *testing.T) {
			boom := c.err("boom")
			r := c.andThen(c.andThen(boom, func(v any) any { t.Error("must not run"); return v }),
				func(v any) any { t.Error("must not run"); return v })
			if !lang.Equiv(r, boom) {
				t.Error("err must pass through and-then unchanged")
			}
		})
	}
}

func TestPrinting(t *testing.T) {
	cases := []struct {
		val  any
		want string
	}{
		{OkA(5), "#cljgo/ok 5"},
		{ErrA("x"), `#cljgo/err "x"`},
		{NoneA, "#cljgo/none nil"},
		{OkC(5), "#cljgo/ok 5"},
		{MkOkD(5), "#cljgo/ok 5"},
		{MkErrD(5), "#cljgo/err 5"},
		{NoneD, "#cljgo/none nil"},
		// Candidate B prints as a plain vector — NOT a tagged literal.
		// Reader support would require printer special-casing on a
		// structural (not type) match: recorded as a D4 liability.
		{OkB(5), "[:cljgo.result/ok 5]"},
	}
	for _, tc := range cases {
		if got := lang.PrintString(tc.val); got != tc.want {
			t.Errorf("PrintString(%v): got %q, want %q", tc.val, got, tc.want)
		}
	}
}

// Value-struct candidates (C, D) ride lang.Equiv's `a == b` fast path.
// When the payload is an uncomparable Go value (slice/map/func), Go's
// runtime PANICS on ==. Pointer (A) and vector (B) representations never
// hit it (iface compare is pointer compare). This is the deal-breaker
// probe for by-value boxing.
func TestUncomparablePayloadHazard(t *testing.T) {
	probe := func(a, b any) (panicked bool) {
		defer func() { panicked = recover() != nil }()
		lang.Equiv(a, b)
		return
	}
	slice := []int{1, 2, 3}
	if probe(OkA(slice), OkA(slice)) {
		t.Error("A: pointer boxing must not panic on uncomparable payload")
	}
	if probe(OkB(slice), OkB(slice)) {
		t.Error("B: vector rep must not panic on uncomparable payload")
	}
	if !probe(OkC(slice), OkC(slice)) {
		t.Log("C: expected panic did not fire (compiler behavior changed?)")
	} else {
		t.Log("C: CONFIRMED — (= (ok []int) (ok []int)) panics via == fast path")
	}
	if !probe(MkOkD(slice), MkOkD(slice)) {
		t.Log("D: expected panic did not fire (compiler behavior changed?)")
	} else {
		t.Log("D: CONFIRMED — (= (ok []int) (ok []int)) panics via == fast path")
	}
}

// Result nested in a collection must compare structurally: (= [(ok 1)] [(ok 1)]).
func TestNestedInCollection(t *testing.T) {
	for _, c := range candidates {
		t.Run(c.name, func(t *testing.T) {
			v1 := lang.NewVector(c.ok(1), c.none)
			v2 := lang.NewVector(c.ok(1), c.none)
			if !lang.Equiv(v1, v2) {
				t.Error("(= [(ok 1) none] [(ok 1) none]) must be true")
			}
		})
	}
}
