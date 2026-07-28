package rt

// ADR 0066 alternative 1 (hard seal, opt-in) evidence. Same one-binary,
// apples-to-apples shape as guard_bench_test.go, adding the third regime:
//
//   - *Dirty  = guarded path, flag tripped  (pre-ADR-0066 cost)
//   - *Clean  = guarded path, flag clean    (what ships today)
//   - *Sealed = no var, no flag load at all (what --seal-core emits)
//
// Run: go test ./pkg/emit/rt -run x -bench 'Guard|Sealed' -benchmem

import "testing"

func BenchmarkSealedAdd2(b *testing.B) {
	benchVars(b)
	var acc any = int64(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc = Add2S(acc, int64(1))
	}
	if _, ok := acc.(int64); !ok {
		b.Fatalf("acc not int64: %T", acc)
	}
}

func BenchmarkSealedLT(b *testing.B) {
	benchVars(b)
	var n int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if LTBoolS(int64(i&1023), int64(512)) {
			n++
		}
	}
	_ = n
}

// TestSealedMatchesGuarded freezes the contract that the sealed twins are
// the guarded helpers minus the liveness machinery: for every input where
// no redefinition is in play, they must return the identical value.
func TestSealedMatchesGuarded(t *testing.T) {
	add, lt := benchVars(t)
	cases := [][2]any{
		{int64(3), int64(4)},
		{int64(-9), int64(9223372036854775807)}, // near overflow
		{2.5, int64(2)},
		{int64(7), 0.5},
	}
	for _, c := range cases {
		if got, want := Add2S(c[0], c[1]), Add2(add, c[0], c[1]); got != want {
			t.Fatalf("Add2S%v = %v, guarded Add2 = %v", c, got, want)
		}
		if got, want := LTBoolS(c[0], c[1]), LTBool(lt, c[0], c[1]); got != want {
			t.Fatalf("LTBoolS%v = %v, guarded LTBool = %v", c, got, want)
		}
	}
}
