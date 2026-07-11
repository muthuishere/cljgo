package s6

import (
	"testing"

	"github.com/muthuishere/cljgo/spikes/s6-var-indirection/lang"
)

const (
	fibN     = int64(30)
	fibWant  = int64(832040)
	factN    = int64(20)
	factWant = int64(2432902008176640000)
)

// --- correctness gates -----------------------------------------------------

func TestAllVariantsAgree(t *testing.T) {
	fibs := map[string]int64{
		"raw":     RawFib(fibN),
		"boxed":   BoxedFib(fibN).(int64),
		"fn":      lang.Apply1(fnFib, fibN).(int64),
		"var":     lang.Apply1(VarFib.Deref(), fibN).(int64),
		"ptrvar":  lang.Apply1(PtrVarFib.Deref(), fibN).(int64),
		"mutex":   lang.Apply1(MutexVarFib.Deref(), fibN).(int64),
		"rwmutex": lang.Apply1(RWMutexVarFib.Deref(), fibN).(int64),
		"hoist":   CallHoisted(HoistVarFib, fibN).(int64),
		"fixed":   FixedVarFib.Deref1()(fibN).(int64),
	}
	for name, got := range fibs {
		if got != fibWant {
			t.Errorf("fib(%d) %s = %d, want %d", fibN, name, got, fibWant)
		}
	}
	facts := map[string]int64{
		"raw":   RawFact(factN),
		"boxed": BoxedFact(factN).(int64),
		"fn":    lang.Apply1(fnFact, factN).(int64),
		"var":   lang.Apply1(VarFact.Deref(), factN).(int64),
		"hoist": CallHoisted(HoistVarFact, factN).(int64),
		"fixed": FixedVarFact.Deref1()(factN).(int64),
	}
	for name, got := range facts {
		if got != factWant {
			t.Errorf("fact(%d) %s = %d, want %d", factN, name, got, factWant)
		}
	}
}

// TestRedefSemantics documents the variant-4 vs variant-5 liveness difference.
func TestRedefSemantics(t *testing.T) {
	v := lang.NewVar(nil)
	calls := 0
	var origSelf lang.Fn
	origSelf = func(args ...any) any { // hoisted-style: recurses on itself
		n := args[0].(int64)
		calls++
		if n == 0 {
			return int64(0)
		}
		return lang.Apply1(origSelf, n-1)
	}
	v.Set(origSelf)

	// re-def mid-flight is impossible to observe under hoisting within one
	// top-level call; but a re-def IS visible on the next top-level call:
	v.Set(func(args ...any) any { return int64(42) })
	if got := lang.Apply1(v.Deref(), int64(5)).(int64); got != 42 {
		t.Errorf("re-def not visible on next top-level call: got %d", got)
	}
}

// --- full-program benchmarks ------------------------------------------------

var sink any

func BenchmarkFib30_1_Raw(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = RawFib(fibN)
	}
}

func BenchmarkFib30_2_Boxed(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = BoxedFib(fibN)
	}
}

func BenchmarkFib30_3_FnApply1(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = lang.Apply1(fnFib, fibN)
	}
}

func BenchmarkFib30_4_VarPerCall_AtomicValue(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = lang.Apply1(VarFib.Deref(), fibN)
	}
}

func BenchmarkFib30_4p_VarPerCall_AtomicPointer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = lang.Apply1(PtrVarFib.Deref(), fibN)
	}
}

func BenchmarkFib30_4m_VarPerCall_Mutex(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = lang.Apply1(MutexVarFib.Deref(), fibN)
	}
}

func BenchmarkFib30_4r_VarPerCall_RWMutex(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = lang.Apply1(RWMutexVarFib.Deref(), fibN)
	}
}

func BenchmarkFib30_5_VarHoisted(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = CallHoisted(HoistVarFib, fibN)
	}
}

func BenchmarkFib30_6_FixedArityVarPerCall(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = FixedVarFib.Deref1()(fibN)
	}
}

func BenchmarkFact20_1_Raw(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = RawFact(factN)
	}
}

func BenchmarkFact20_2_Boxed(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = BoxedFact(factN)
	}
}

func BenchmarkFact20_3_FnApply1(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = lang.Apply1(fnFact, factN)
	}
}

func BenchmarkFact20_4_VarPerCall(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = lang.Apply1(VarFact.Deref(), factN)
	}
}

func BenchmarkFact20_5_VarHoisted(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = CallHoisted(HoistVarFact, factN)
	}
}

func BenchmarkFact20_6_FixedArityVarPerCall(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = FixedVarFact.Deref1()(factN)
	}
}

// --- deref-only microbenchmarks (settle the Var representation) -------------

var (
	microAtomic  = lang.NewVar(func(args ...any) any { return nil })
	microPtr     = lang.NewPtrVar(func(args ...any) any { return nil })
	microMutex   = lang.NewMutexVar(func(args ...any) any { return nil })
	microRWMutex = lang.NewRWMutexVar(func(args ...any) any { return nil })
)

func BenchmarkDeref_AtomicValue(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = microAtomic.Deref()
	}
}

func BenchmarkDeref_AtomicPointer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = microPtr.DerefFn()
	}
}

func BenchmarkDeref_Mutex(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = microMutex.Deref()
	}
}

func BenchmarkDeref_RWMutex(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sink = microRWMutex.Deref()
	}
}

func BenchmarkDeref_AtomicValue_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		var local any
		for pb.Next() {
			local = microAtomic.Deref()
		}
		_ = local
	})
}

func BenchmarkDeref_Mutex_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		var local any
		for pb.Next() {
			local = microMutex.Deref()
		}
		_ = local
	})
}
