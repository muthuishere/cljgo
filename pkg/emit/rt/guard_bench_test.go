package rt

// Spike s43 / ADR 0066 evidence. These benchmarks isolate the arithmetic
// intrinsic's two regimes in ONE binary so the comparison is apples to
// apples (same build, same inliner decisions):
//
//   - *Dirty  = CoreArithDirty true  = the pre-ADR-0066 guarded path
//     (per-call lang.Var.Get + interface-compare against the pristine
//     builtin), which is exactly what every 2-arg core arithmetic call
//     used to pay.
//   - *Clean  = CoreArithDirty false = the sealed fast path this spike
//     adds (one relaxed atomic.Bool load, then the open-coded int64 op).
//
// Run:  go test ./pkg/emit/rt -run x -bench 'Guard' -benchmem
// pprof: go test ./pkg/emit/rt -run x -bench 'GuardAdd2Dirty' \
//          -cpuprofile /tmp/dirty.prof ; go tool pprof -top /tmp/dirty.prof
// then the same for Clean — Var.Get / efaceeq disappear from the Clean top.

import (
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// benchVars registers the Go builtins, snapshots the pristine arithmetic
// values into the package origs, and seals the seven vars — the same state
// rt.Boot leaves behind, minus the AOT coreLoader rt cannot import here.
func benchVars(tb testing.TB) (add, lt *lang.Var) {
	tb.Helper()
	corelib.RegisterAll()
	add = lang.NSCore.FindInternedVar(lang.NewSymbol("+"))
	lt = lang.NSCore.FindInternedVar(lang.NewSymbol("<"))
	origAdd = add.Get()
	origLT = lt.Get()
	add.Seal()
	lt.Seal()
	return add, lt
}

func benchAdd2(b *testing.B, dirty bool) {
	add, _ := benchVars(b)
	lang.CoreArithDirty.Store(dirty)
	defer lang.CoreArithDirty.Store(false)
	var acc any = int64(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc = Add2(add, acc, int64(1))
	}
	if _, ok := acc.(int64); !ok {
		b.Fatalf("acc not int64: %T", acc)
	}
}

func benchLTBool(b *testing.B, dirty bool) {
	_, lt := benchVars(b)
	lang.CoreArithDirty.Store(dirty)
	defer lang.CoreArithDirty.Store(false)
	var n int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if LTBool(lt, int64(i&1023), int64(512)) {
			n++
		}
	}
	_ = n
}

func BenchmarkGuardAdd2Dirty(b *testing.B) { benchAdd2(b, true) }
func BenchmarkGuardAdd2Clean(b *testing.B) { benchAdd2(b, false) }
func BenchmarkGuardLTDirty(b *testing.B)   { benchLTBool(b, true) }
func BenchmarkGuardLTClean(b *testing.B)   { benchLTBool(b, false) }
