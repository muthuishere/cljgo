package lang

// S4 spike: smoke tests + micro-benchmarks over the severed runtime's
// core behaviors — persistent vector build, HAMT assoc, lazy-seq
// realization, atom CAS under contention.

import (
	"sync"
	"testing"
)

const smokeN = 10_000

func TestSmokeVectorBuild10k(t *testing.T) {
	var v Conser = NewVector()
	for i := 0; i < smokeN; i++ {
		v = v.Cons(int64(i))
	}
	pv := v.(*Vector)
	if pv.Count() != smokeN {
		t.Fatalf("Count() = %d; want %d", pv.Count(), smokeN)
	}
	for _, i := range []int{0, 31, 32, 1024, smokeN - 1} {
		if got := pv.Nth(i); got != int64(i) {
			t.Errorf("Nth(%d) = %v; want %d", i, got, i)
		}
	}
	// Persistence: assoc'ing index 0 must not disturb the original.
	v2 := pv.AssocN(0, int64(-1))
	if pv.Nth(0) != int64(0) || v2.Nth(0) != int64(-1) {
		t.Error("AssocN mutated the original vector")
	}
}

func TestSmokeHAMTAssoc10k(t *testing.T) {
	var m Associative = NewPersistentHashMap()
	for i := 0; i < smokeN; i++ {
		m = m.Assoc(int64(i), int64(i*2))
	}
	if Count(m) != smokeN {
		t.Fatalf("Count = %d; want %d", Count(m), smokeN)
	}
	for _, i := range []int64{0, 1, 4095, smokeN - 1} {
		if got := m.ValAt(i); got != i*2 {
			t.Errorf("ValAt(%d) = %v; want %d", i, got, i*2)
		}
	}
	// without
	m2 := m.(IPersistentMap).Without(int64(0))
	if m2.ValAt(int64(0)) != nil || m.ValAt(int64(0)) == nil {
		t.Error("Without broke persistence")
	}
}

func TestSmokeLazySeqRealization(t *testing.T) {
	calls := 0
	var mk func(i int) ISeq
	mk = func(i int) ISeq {
		return NewLazySeq(func() any {
			calls++
			if i >= smokeN {
				return nil
			}
			return NewCons(int64(i), mk(i+1))
		})
	}
	s := mk(0)
	if calls != 0 {
		t.Fatalf("lazy-seq body ran eagerly (%d calls)", calls)
	}
	// realize fully
	n := 0
	var last any
	for seq := Seq(s); seq != nil; seq = seq.Next() {
		last = seq.First()
		n++
	}
	if n != smokeN || last != int64(smokeN-1) {
		t.Errorf("realized %d elems (last %v); want %d (last %d)", n, last, smokeN, smokeN-1)
	}
	// realize-at-most-once: re-walking must not re-run bodies
	before := calls
	for seq := Seq(s); seq != nil; seq = seq.Next() {
	}
	if calls != before {
		t.Errorf("re-walk re-ran %d lazy bodies", calls-before)
	}
}

func TestSmokeAtomCAS(t *testing.T) {
	a := NewAtom(int64(0))
	inc := FnFunc(func(args ...any) any { return args[0].(int64) + 1 })
	const workers, per = 8, 1000
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				a.Swap(inc, nil)
			}
		}()
	}
	wg.Wait()
	if got := a.Deref(); got != int64(workers*per) {
		t.Errorf("atom = %v after %d contended swaps; want %d", got, workers*per, workers*per)
	}
	// compare-and-set identity semantics
	cur := a.Deref()
	if !a.CompareAndSet(cur, int64(-1)) {
		t.Error("CompareAndSet with current value failed")
	}
	if a.CompareAndSet(cur, int64(-2)) {
		t.Error("CompareAndSet with stale value succeeded")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks

func BenchmarkVectorCons10k(b *testing.B) {
	for b.Loop() {
		var v Conser = NewVector()
		for i := 0; i < smokeN; i++ {
			v = v.Cons(int64(i))
		}
	}
}

func BenchmarkVectorTransientConj10k(b *testing.B) {
	for b.Loop() {
		tv := NewVector().AsTransient().(ITransientVector)
		for i := 0; i < smokeN; i++ {
			tv = tv.Conj(int64(i)).(ITransientVector)
		}
		_ = tv.Persistent()
	}
}

func BenchmarkHAMTAssoc10k(b *testing.B) {
	for b.Loop() {
		var m Associative = NewPersistentHashMap()
		for i := 0; i < smokeN; i++ {
			m = m.Assoc(int64(i), int64(i))
		}
	}
}

func BenchmarkHAMTGet(b *testing.B) {
	var m Associative = NewPersistentHashMap()
	for i := 0; i < smokeN; i++ {
		m = m.Assoc(int64(i), int64(i))
	}
	b.ResetTimer()
	i := int64(0)
	for b.Loop() {
		_ = m.ValAt(i % smokeN)
		i++
	}
}

func BenchmarkLazySeqRealize10k(b *testing.B) {
	for b.Loop() {
		var mk func(i int) ISeq
		mk = func(i int) ISeq {
			return NewLazySeq(func() any {
				if i >= smokeN {
					return nil
				}
				return NewCons(int64(i), mk(i+1))
			})
		}
		for seq := Seq(mk(0)); seq != nil; seq = seq.Next() {
		}
	}
}

func BenchmarkAtomSwapUncontended(b *testing.B) {
	a := NewAtom(int64(0))
	inc := FnFunc(func(args ...any) any { return args[0].(int64) + 1 })
	for b.Loop() {
		a.Swap(inc, nil)
	}
}

func BenchmarkKeywordIntern(b *testing.B) {
	for b.Loop() {
		_ = InternKeyword("bench", "hot-keyword")
	}
}

func BenchmarkKeywordIdentityCompare(b *testing.B) {
	k1 := InternKeyword("bench", "cmp")
	k2 := InternKeyword("bench", "cmp")
	n := 0
	for b.Loop() {
		if k1 == k2 {
			n++
		}
	}
	_ = n
}
