package emit

import (
	"io"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit/rt"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

// TestUnboxedOpsAreSealed is the correctness interlock for the ADR 0067
// unboxed tables. A typed region open-codes these ops in raw Go and never
// derefs the operator var, so redefinition liveness rests ENTIRELY on the
// var being sealed: the redef trips lang.CoreArithDirty, every guarded
// region's `if !rt.CoreDirty()` fails, and execution takes the boxed
// emission that reads the var per call.
//
// Adding an op to intUnboxArith2/1, intUnboxPred1 or intUnboxCmp without
// adding it to rt.SealedCoreNames would therefore make `with-redefs` of
// that op silently invisible inside compiled hot loops — the one failure
// mode ADR 0066/0067 exist to rule out. This test fails instead.
func TestUnboxedOpsAreSealed(t *testing.T) {
	sealed := map[string]bool{}
	for _, n := range rt.SealedCoreNames {
		sealed[n] = true
	}
	check := func(table string, names []string) {
		for _, n := range names {
			if !sealed[n] {
				t.Errorf("%s open-codes clojure.core/%s but rt.SealedCoreNames does not seal it: "+
					"a (with-redefs [%s …]) would not be seen inside a guarded typed region (ADR 0066/0067)", table, n, n)
			}
		}
	}
	check("intUnboxArith2", keys(intUnboxArith2))
	check("intUnboxArith1", keys(intUnboxArith1))
	check("intUnboxPred1", keys(intUnboxPred1))
	check("intUnboxCmp", keys(intUnboxCmp))
	check("intrinsic2", keys(intrinsic2))
	check("testIntrinsics", keys(testIntrinsics))
}

// TestSealedCoreNamesResolve guards the other direction: a name in the
// seal list that no longer exists in clojure.core (renamed, dropped) would
// silently stop sealing — rt.Boot skips a var it cannot find, so nothing
// else would notice. Booting the interpreter interns the same clojure.core
// the compiled seal walks.
func TestSealedCoreNamesResolve(t *testing.T) {
	repl.New(nil, io.Discard, io.Discard)
	for _, n := range rt.SealedCoreNames {
		if lang.NSCore.FindInternedVar(lang.NewSymbol(n)) == nil {
			t.Errorf("clojure.core/%s is in rt.SealedCoreNames but is not interned — rt.Boot would silently skip sealing it", n)
		}
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
