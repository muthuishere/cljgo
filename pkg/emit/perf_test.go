package emit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// TestFactorialPerfBudget measures the M2 perf budget (design/00 §1.4:
// emitted factorial within ~10× of handwritten Go). Startup is factored
// out by timing an idle variant of each binary.
//
// Where M2 v0 actually lands (measured on darwin/arm64, 2026-07-12):
//   - naive emission (all calls through variadic nativeFn builtins):
//     ~168× — the []any-per-arithmetic-op trap S6 predicted.
//   - with the ADR 0004 fixed-arity convention + pkg/emit/rt's guarded
//     arithmetic intrinsics + unboxed if-test comparisons: ~35×.
//
// The remaining gap to ~10× is (a) the vendored Var.Get (two atomic
// loads + Box unwrap, ~4 derefs per fact call vs S6's minimal
// atomic.Pointer var), (b) boxing of large intermediates, (c) the rt
// helpers being over the inline budget. (a) is pkg/lang work, (b)/(c)
// are the design/04 §5 primitive-hints/intrinsics rungs — post-M2 by
// design. S6's 7.8× modeled arithmetic as raw Go ops, i.e. it already
// assumed those rungs for the arithmetic; the honest v0 number is ~35×.
//
// The hard limit below (60×) is a regression guard against the naive
// emission, not the budget; tightening it to ~10× tracks the ladder.
func TestFactorialPerfBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf measurement in -short mode")
	}
	const iters = 2_000_000

	cljProg := func(n int) string {
		return fmt.Sprintf(`
(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(loop* [i 0 acc 0]
  (if (< i %d)
    (recur (+ i 1) (+ acc (fact 15)))
    acc))
`, n)
	}
	goProg := func(n int) string {
		return fmt.Sprintf(`package main

import "fmt"

func fact(n int64) int64 {
	if n < 2 {
		return 1
	}
	return n * fact(n-1)
}

func main() {
	var acc int64
	for i := 0; i < %d; i++ {
		acc += fact(15)
	}
	fmt.Println(acc)
}
`, n)
	}

	buildClj := func(name, src string) string {
		lang.RemoveNamespace(lang.NewSymbol("user"))
		oldOut := eval.Out
		eval.Out = io.Discard
		forms, err := CompileReader(strings.NewReader(src), name+".clj")
		eval.Out = oldOut
		if err != nil {
			t.Fatalf("compile %s: %v", name, err)
		}
		dir := t.TempDir()
		if err := WriteModule(dir, forms, Options{PrintLastValue: true}); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		bin := filepath.Join(dir, name+ExeSuffix)
		if err := GoBuild(dir, bin); err != nil {
			t.Fatalf("build %s: %v", name, err)
		}
		return bin
	}
	buildGo := func(name, src string) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module perfraw\n\ngo 1.26\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, name+ExeSuffix)
		if err := GoBuild(dir, bin); err != nil {
			t.Fatalf("build %s: %v", name, err)
		}
		return bin
	}

	run := func(bin string) time.Duration {
		best := time.Duration(1<<62 - 1)
		for i := 0; i < 3; i++ {
			start := time.Now()
			if err := exec.Command(bin).Run(); err != nil {
				t.Fatalf("run %s: %v", bin, err)
			}
			if d := time.Since(start); d < best {
				best = d
			}
		}
		return best
	}

	cljWork := run(buildClj("cljwork", cljProg(iters)))
	cljIdle := run(buildClj("cljidle", cljProg(0)))
	rawWork := run(buildGo("rawwork", goProg(iters)))
	rawIdle := run(buildGo("rawidle", goProg(0)))

	cljNet := cljWork - cljIdle
	rawNet := rawWork - rawIdle
	if rawNet <= 0 {
		t.Skipf("raw baseline too fast to measure (work %v, idle %v)", rawWork, rawIdle)
	}
	ratio := float64(cljNet) / float64(rawNet)
	t.Logf("fact(15) x %d: emitted %v (startup %v), raw Go %v (startup %v) — ratio %.1fx (budget ~10x; see doc comment)",
		iters, cljNet, cljIdle, rawNet, rawIdle, ratio)
	if ratio > 60 {
		t.Fatalf("emitted factorial is %.1fx handwritten Go — regression past the v0 floor (~35x measured; naive emission was ~168x)", ratio)
	}
}
