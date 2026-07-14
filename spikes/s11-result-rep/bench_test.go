package s11

import (
	"errors"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Payload starts above 255 so int boxing is real (Go's staticuint64s
// caches 0–255; see S6 RESULTS note). Every candidate pays the same
// payload-boxing cost, so differences isolate the representation.
const startVal = 1000

var (
	sinkAny  any
	sinkBool bool
	sinkU32  uint32
	sinkInt  int
	sinkErr  error
)

// ---------------------------------------------------------------------------
// Baseline 1 — raw Go error-check chain (unboxed; the floor).
// ---------------------------------------------------------------------------

var errBoom = errors.New("boom")

func stepRaw(v int) (int, error)  { return v + 1, nil }
func stepRawErr(int) (int, error) { return 0, errBoom }

func chain10Raw(x int) (int, error) {
	v := x
	for i := 0; i < 10; i++ {
		var err error
		v, err = stepRaw(v)
		if err != nil {
			return 0, err
		}
	}
	return v, nil
}

func chain10RawErr(x int) (int, error) {
	v := x
	for i := 0; i < 10; i++ {
		var err error
		if i == 5 {
			v, err = stepRawErr(v)
		} else {
			v, err = stepRaw(v)
		}
		if err != nil {
			return 0, err
		}
	}
	return v, nil
}

func BenchmarkChain_RawGo(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkInt, sinkErr = chain10Raw(startVal)
	}
}

func BenchmarkChainErr_RawGo(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkInt, sinkErr = chain10RawErr(startVal)
	}
}

// ---------------------------------------------------------------------------
// Baseline 2 — panic/recover try-catch style (boxed payloads, for context).
// ---------------------------------------------------------------------------

func stepPanicOk(v any) any { return v.(int) + 1 }
func stepPanicErr(any) any  { panic(errBoom) }

func chain10Panic(x any, failAt int) (res any) {
	defer func() {
		if r := recover(); r != nil {
			res = r
		}
	}()
	v := x
	for i := 0; i < 10; i++ {
		if i == failAt {
			v = stepPanicErr(v)
		} else {
			v = stepPanicOk(v)
		}
	}
	return v
}

func BenchmarkChain_PanicRecover(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10Panic(startVal, -1)
	}
}

func BenchmarkChainErr_PanicRecover(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10Panic(startVal, 5)
	}
}

// ---------------------------------------------------------------------------
// Railway chains: 10 and-then steps, each unwrap → +1 → re-wrap.
// Err variant fails at step 5; steps 6–9 short-circuit through and-then.
// ---------------------------------------------------------------------------

func stepA(v any) any    { return OkA(v.(int) + 1) }
func stepAErr(v any) any { return ErrA(errBoom) }

func chain10(mk func(any) any, andThen func(any, func(any) any) any,
	step, stepErr func(any) any, failAt int) any {
	r := mk(startVal)
	for i := 0; i < 10; i++ {
		if i == failAt {
			r = andThen(r, stepErr)
		} else {
			r = andThen(r, step)
		}
	}
	return r
}

func BenchmarkChain_A_TaggedPtr(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(OkA, AndThenA, stepA, stepAErr, -1)
	}
}

func BenchmarkChainErr_A_TaggedPtr(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(OkA, AndThenA, stepA, stepAErr, 5)
	}
}

func stepB(v any) any    { return OkB(v.(int) + 1) }
func stepBErr(v any) any { return ErrB(errBoom) }

func BenchmarkChain_B_Vector(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(OkB, AndThenB, stepB, stepBErr, -1)
	}
}

func BenchmarkChainErr_B_Vector(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(OkB, AndThenB, stepB, stepBErr, 5)
	}
}

func stepC(v any) any    { return OkC(v.(int) + 1) }
func stepCErr(v any) any { return ErrC(errBoom) }

func BenchmarkChain_C_StructVal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(OkC, AndThenC, stepC, stepCErr, -1)
	}
}

func BenchmarkChainErr_C_StructVal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(OkC, AndThenC, stepC, stepCErr, 5)
	}
}

func stepD(v any) any    { return MkOkD(v.(int) + 1) }
func stepDErr(v any) any { return MkErrD(errBoom) }

func BenchmarkChain_D_TypePerTag(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(MkOkD, AndThenD, stepD, stepDErr, -1)
	}
}

func BenchmarkChainErr_D_TypePerTag(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = chain10(MkOkD, AndThenD, stepD, stepDErr, 5)
	}
}

// ---------------------------------------------------------------------------
// Construct + destructure: (ok v) → ok? → unwrap. The let? inner loop.
// ---------------------------------------------------------------------------

func BenchmarkConstructDestructure_A(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		r := OkA(startVal)
		if IsOkA(r) && !IsErrA(r) {
			sinkAny = UnwrapA(r)
		}
	}
}

func BenchmarkConstructDestructure_B(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		r := OkB(startVal)
		if IsOkB(r) && !IsErrB(r) {
			sinkAny = UnwrapB(r)
		}
	}
}

func BenchmarkConstructDestructure_C(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		r := OkC(startVal)
		if IsOkC(r) && !IsErrC(r) {
			sinkAny = UnwrapC(r)
		}
	}
}

func BenchmarkConstructDestructure_D(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		r := MkOkD(startVal)
		if IsOkD(r) && !IsErrD(r) {
			sinkAny = UnwrapD(r)
		}
	}
}

// (ok nil) / none singleton construction — must be alloc-free where the
// representation allows it.
func BenchmarkConstructOkNil_A(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = OkA(nil)
	}
}

func BenchmarkConstructNone_D(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = NoneD
	}
}

// ---------------------------------------------------------------------------
// Equiv: (= (ok 1000) (ok 1000)) through lang.Equiv, plus a not-equal probe.
// ---------------------------------------------------------------------------

var (
	eqA1, eqA2 = OkA(startVal), OkA(startVal)
	eqB1, eqB2 = OkB(startVal), OkB(startVal)
	eqC1, eqC2 = OkC(startVal), OkC(startVal)
	eqD1, eqD2 = MkOkD(startVal), MkOkD(startVal)
	neA        = ErrA(startVal)
)

func BenchmarkEquiv_A(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkBool = lang.Equiv(eqA1, eqA2)
	}
}

func BenchmarkEquiv_B(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkBool = lang.Equiv(eqB1, eqB2)
	}
}

func BenchmarkEquiv_C(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkBool = lang.Equiv(eqC1, eqC2)
	}
}

func BenchmarkEquiv_D(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkBool = lang.Equiv(eqD1, eqD2)
	}
}

func BenchmarkEquivNotEqual_A(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkBool = lang.Equiv(eqA1, neA)
	}
}

// ---------------------------------------------------------------------------
// Map-key hashing: lang.HashEq (what PersistentHashMap.Assoc/ValAt call),
// and a full memo-cache probe: construct a fresh key + ValAt on a 16-entry
// persistent hash map.
// ---------------------------------------------------------------------------

func BenchmarkHashEq_A(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkU32 = lang.HashEq(eqA1)
	}
}

func BenchmarkHashEq_B(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkU32 = lang.HashEq(eqB1)
	}
}

func BenchmarkHashEq_C(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkU32 = lang.HashEq(eqC1)
	}
}

func BenchmarkHashEq_D(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkU32 = lang.HashEq(eqD1)
	}
}

func buildMap(mk func(any) any) lang.IPersistentMap {
	kvs := make([]any, 0, 32)
	for i := 0; i < 16; i++ {
		kvs = append(kvs, mk(startVal+i), i)
	}
	return lang.NewPersistentHashMap(kvs...)
}

var (
	mapA = buildMap(OkA)
	mapB = buildMap(OkB)
	mapC = buildMap(OkC)
	mapD = buildMap(MkOkD)
)

func BenchmarkMapLookup_A(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = mapA.ValAt(OkA(startVal + 7))
	}
}

func BenchmarkMapLookup_B(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = mapB.ValAt(OkB(startVal + 7))
	}
}

func BenchmarkMapLookup_C(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = mapC.ValAt(OkC(startVal + 7))
	}
}

func BenchmarkMapLookup_D(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		sinkAny = mapD.ValAt(MkOkD(startVal + 7))
	}
}
