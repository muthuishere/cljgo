package eval_test

import (
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// TestBootUnderBudget times the full boot of design/00 §6 (M1): Go
// builtins → bootstrap defmacro → embedded core.clj loaded into
// clojure.core → user refers core's publics. Budget is 100ms with lots
// of headroom (measured ~1ms on darwin/arm64); the point is catching a
// regression that makes boot do something pathological, not a precise
// benchmark (BenchmarkBoot is that).
func TestBootUnderBudget(t *testing.T) {
	start := time.Now()
	ev := eval.New()
	elapsed := time.Since(start)
	t.Logf("boot (builtins + defmacro + core.clj): %v", elapsed)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("boot took %v, budget 100ms (design/00 §6 M1)", elapsed)
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
