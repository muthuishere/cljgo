package lang

import "testing"

// A descending LongRange must agree with itself on every path.
//
// Before this test existed, `(range 6 1 -1)` reported Count() == 5 and mapped to
// five elements through the chunked path, while the seq path (First/Next — what
// seq, vec, doall, some, filter, first and doseq all walk) stopped after ONE
// element, and Reduce/ReduceInit stopped before the first step. Nothing threw:
// callers got a short answer that looked like a real one. These tests pin all
// three paths against the same expected slice so the three cannot drift apart
// again.

// walk realizes a seq the way `seq`/`vec`/`doall` do — First + Next to nil.
func walk(s ISeq) []int64 {
	var out []int64
	for s != nil {
		out = append(out, s.First().(int64))
		s = s.Next()
	}
	return out
}

func equalInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLongRangeDescendingSeqPath(t *testing.T) {
	cases := []struct {
		name             string
		start, end, step int64
		want             []int64
	}{
		{"step -1", 6, 1, -1, []int64{6, 5, 4, 3, 2}},
		{"step -1 to zero", 5, 0, -1, []int64{5, 4, 3, 2, 1}},
		{"step -2", 10, 0, -2, []int64{10, 8, 6, 4, 2}},
		{"single element", 3, 2, -1, []int64{3}},
		{"across zero", 2, -3, -1, []int64{2, 1, 0, -1, -2}},
		// The ascending cases were always right; they are here so a future fix
		// to the descending side cannot quietly break them.
		{"ascending step 1", 0, 5, 1, []int64{0, 1, 2, 3, 4}},
		{"ascending step 2", 0, 10, 2, []int64{0, 2, 4, 6, 8}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewLongRange(tc.start, tc.end, tc.step)

			got := walk(r.Seq())
			if !equalInt64s(got, tc.want) {
				t.Errorf("seq path: got %v, want %v", got, tc.want)
			}

			// Count() and the seq path must agree — the split between them is
			// exactly what made the original bug silent.
			if c, ok := r.(Counted); ok {
				if c.Count() != len(tc.want) {
					t.Errorf("Count() = %d, but the seq path yields %d elements",
						c.Count(), len(tc.want))
				}
			}
		})
	}
}

func TestLongRangeDescendingReduce(t *testing.T) {
	// sum via Reduce (no init) and ReduceInit must both see every element.
	sum := NewFnFunc2(func(acc, x any) any { return acc.(int64) + x.(int64) })

	r := NewLongRange(6, 1, -1) // 6+5+4+3+2 = 20
	want := int64(20)

	if red, ok := r.(IReduce); ok {
		if got := red.Reduce(sum); got != want {
			t.Errorf("Reduce: got %v, want %v", got, want)
		}
	}
	if red, ok := r.(IReduceInit); ok {
		if got := red.ReduceInit(sum, int64(0)); got != want {
			t.Errorf("ReduceInit: got %v, want %v", got, want)
		}
	}
}

func TestLongRangeEmptyDescending(t *testing.T) {
	// end >= start with a negative step is empty, and must stay empty.
	for _, tc := range [][3]int64{{1, 6, -1}, {3, 3, -1}} {
		r := NewLongRange(tc[0], tc[1], tc[2])
		if got := walk(r.Seq()); len(got) != 0 {
			t.Errorf("NewLongRange(%d,%d,%d): got %v, want empty", tc[0], tc[1], tc[2], got)
		}
	}
}
