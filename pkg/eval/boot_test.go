package eval_test

import (
	"os"
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// defaultBootBudget is ADR 0019's ceiling, calibrated on the owner's
// machine. CLJGO_BOOT_BUDGET overrides it per host (ADR 0024).
const defaultBootBudget = 250 * time.Millisecond

// bootBudget is the wall-clock ceiling for TestBootUnderBudget: the ADR 0019
// default, overridable via CLJGO_BOOT_BUDGET (any time.ParseDuration string).
//
// The budget is host-relative by design (ADR 0024). The same code boots in
// 181ms locally, 349ms on a macos runner and 3.55s on an ubuntu one — so an
// absolute number would test the runner, not cljgo. CI sets 5s: loose enough
// to absorb runner variance, still ~20x under any real pathology.
func bootBudget(t *testing.T) time.Duration {
	t.Helper()
	s := os.Getenv("CLJGO_BOOT_BUDGET")
	if s == "" {
		return defaultBootBudget
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		// Don't silently fall back: a typo'd budget must not look like a pass.
		t.Fatalf("CLJGO_BOOT_BUDGET=%q is not a duration: %v", s, err)
	}
	return d
}

// TestBootUnderBudget times the full boot of design/00 §6 (M1): Go
// builtins → bootstrap defmacro → embedded core.clj loaded into
// clojure.core → user refers core's publics. Budget is 250ms (ADR 0019),
// host-relative via CLJGO_BOOT_BUDGET (ADR 0024): every core.clj def is
// macroexpanded through the tree-walk evaluator, so interpreter-boot cost
// grows ~linearly with the size of clojure.core (the seq/coll library and
// future core growth). The point is catching a *pathological* regression
// (O(n²) blowup, runaway realization, per-form I/O), not the expected linear
// cost of a larger core (BenchmarkBoot is the precise benchmark).
// Compiled-binary startup (<50ms, design/00 §5) is a separate budget,
// unaffected by interpret-time boot.
func TestBootUnderBudget(t *testing.T) {
	budget := bootBudget(t)
	start := time.Now()
	ev := eval.New()
	elapsed := time.Since(start)
	t.Logf("boot (builtins + defmacro + core.clj): %v (budget %v)", elapsed, budget)
	if elapsed > budget {
		t.Fatalf("boot took %v, budget %v (design/00 §6 M1, ADR 0019/0024)", elapsed, budget)
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
