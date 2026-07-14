package eval_test

import (
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// TestBootUnderBudget times the full boot of design/00 §6 (M1): Go
// builtins → bootstrap defmacro → embedded core.clj loaded into
// clojure.core → user refers core's publics. Budget is 250ms (ADR 0019):
// every core.clj def is macroexpanded through the tree-walk evaluator, so
// interpreter-boot cost grows ~linearly with the size of clojure.core (the
// seq/coll library and future core growth). The point is catching a
// *pathological* regression (O(n²) blowup, runaway realization, per-form
// I/O), not the expected linear cost of a larger core (BenchmarkBoot is the
// precise benchmark). Compiled-binary startup (<50ms, design/00 §5) is a
// separate budget, unaffected by interpret-time boot.
func TestBootUnderBudget(t *testing.T) {
	start := time.Now()
	ev := eval.New()
	elapsed := time.Since(start)
	t.Logf("boot (builtins + defmacro + core.clj): %v", elapsed)
	if elapsed > 250*time.Millisecond {
		t.Fatalf("boot took %v, budget 250ms (design/00 §6 M1, ADR 0019)", elapsed)
	}

	// Boot must leave the macros live: core.clj's macros are interned in
	// clojure.core with the :macro flag set.
	for _, name := range []string{"defmacro", "defn", "when", "when-not", "if-not", "and", "or", "->", "->>", "cond", "let", "loop", "fn"} {
		v := lang.NSCore.FindInternedVar(lang.NewSymbol(name))
		if v == nil {
			t.Fatalf("boot did not intern clojure.core/%s", name)
		}
		if !v.IsMacro() {
			t.Errorf("clojure.core/%s is not marked :macro", name)
		}
	}
	if ns := ev.CurrentNS().Name().Name(); ns != "user" {
		t.Errorf("boot should land in user, got %s", ns)
	}
}

func BenchmarkBoot(b *testing.B) {
	for i := 0; i < b.N; i++ {
		eval.New()
	}
}
