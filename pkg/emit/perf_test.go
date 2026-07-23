package emit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/corelib"
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
// The hard limit below (60× locally, CLJGO_PERF_RATIO_MAX elsewhere — ADR
// 0024) is a regression guard against the naive emission, not the budget;
// tightening it to ~10× tracks the ladder.
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
		oldOut := corelib.Out
		corelib.Out = io.Discard
		forms, err := CompileReader(strings.NewReader(src), name+".clj")
		corelib.Out = oldOut
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
	maxRatio := perfRatioMax(t)
	ratio := float64(cljNet) / float64(rawNet)
	t.Logf("fact(15) x %d: emitted %v (startup %v), raw Go %v (startup %v) — ratio %.1fx (max %.0fx; see doc comment)",
		iters, cljNet, cljIdle, rawNet, rawIdle, ratio, maxRatio)
	if ratio > maxRatio {
		t.Fatalf("emitted factorial is %.1fx handwritten Go — regression past the v0 floor (~35x measured; naive emission was ~168x)", ratio)
	}
}

// defaultPerfRatioMax is the local ceiling: ~35x is measured on the owner's
// machine, so 60x leaves real headroom before the gate fires.
const defaultPerfRatioMax = 60

// perfRatioMax is the emitted-vs-handwritten-Go ceiling, overridable via
// CLJGO_PERF_RATIO_MAX.
//
// Host-relative for the same reason as the boot budget (ADR 0024). Although a
// ratio cancels raw host speed in principle, this one divides two independently
// measured net times, so on a contended shared runner both jitter and the
// quotient swings: a macos runner measured 63.2x where local measures ~35x,
// then passed on rerun — flaky, not broken. CI sets 120: still far under the
// ~168x naive-emission regression this gate exists to catch, so it keeps
// biting.
func perfRatioMax(t *testing.T) float64 {
	t.Helper()
	s := os.Getenv("CLJGO_PERF_RATIO_MAX")
	if s == "" {
		return defaultPerfRatioMax
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Don't silently fall back: a typo'd ceiling must not look like a pass.
		t.Fatalf("CLJGO_PERF_RATIO_MAX=%q is not a number: %v", s, err)
	}
	return v
}

// TestCoreReducePerfBudget is the clojure.core-mediated perf gate ADR 0037
// decision #5 mandates — and which, until now, did not exist.
//
// TestFactorialPerfBudget above measures USER code only: fn*, arithmetic and
// recur, all of which the emitter turns into direct Go. That path was already
// fast, and its greenness is exactly why a 16.54x regression against a
// competitor stayed invisible — nothing in CI ran a workload that went THROUGH
// clojure.core. This one does: (reduce + (range N)) touches the seq machinery,
// the IFn dispatch on the reducing fn, and the boxing of every intermediate —
// the costs ADR 0045 and the benchmark suite both point at.
//
// It asserts a WALL-CLOCK TOTAL, not a ratio against handwritten Go. The
// ratio shape was tried first and rejected on measurement: raw Go sums two
// million int64s in well under a millisecond, so dividing by that denominator
// swung the same unchanged code between 75x and 157x run to run. A total also
// matches how this project reports performance everywhere else — whole
// wall-clock, never boot-subtracted, because that is what a user experiences.
//
// It is a REGRESSION gate, not a target. The known fix path
// (IReduce/internal-reduce on range and vectors, native map/filter/take,
// Apply2 fast paths) should make this number FALL; lower the budget with it.
func TestCoreReducePerfBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf measurement in -short mode")
	}
	const n = 2_000_000

	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(fmt.Sprintf("(reduce + (range %d))\n", n)), "corereduce.clj")
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dir := t.TempDir()
	if err := WriteModule(dir, forms, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("write: %v", err)
	}
	bin := filepath.Join(dir, "corereduce"+ExeSuffix)
	if err := GoBuild(dir, bin); err != nil {
		t.Fatalf("build: %v", err)
	}

	best := time.Duration(1<<62 - 1)
	for i := 0; i < 3; i++ {
		start := time.Now()
		if err := exec.Command(bin).Run(); err != nil {
			t.Fatalf("run: %v", err)
		}
		if d := time.Since(start); d < best {
			best = d
		}
	}

	budget := coreReduceBudget(t)
	t.Logf("(reduce + (range %d)) compiled: %v total wall clock (budget %v)", n, best, budget)
	if best > budget {
		t.Fatalf("clojure.core-mediated reduce took %v, past the %v budget — a core-path regression (ADR 0037 #5)", best, budget)
	}
}

// defaultCoreReduceBudget locks in today's measured cost with headroom, NOT a
// target: ~110ms measured on the owner's machine (2026-07-22) for
// (reduce + (range 2e6)) including startup, so 350ms leaves ~3x before the
// gate fires. A budget loose enough to sleep through a real regression would
// not be a gate at all. Lower it as the IReduce/Apply2 work lands.
const defaultCoreReduceBudget = 350 * time.Millisecond

// coreReduceBudget is the clojure.core-mediated wall-clock budget, overridable
// via CLJGO_CORE_REDUCE_BUDGET (ADR 0024 host-relative budgets — CI runners
// are slower and noisier than a laptop).
func coreReduceBudget(t *testing.T) time.Duration {
	t.Helper()
	s := os.Getenv("CLJGO_CORE_REDUCE_BUDGET")
	if s == "" {
		return defaultCoreReduceBudget
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		// Don't silently fall back: a typo'd budget must not look like a pass.
		t.Fatalf("CLJGO_CORE_REDUCE_BUDGET=%q is not a duration: %v", s, err)
	}
	return d
}

// TestCorePipelinePerfBudget guards the map/filter path, which the reduce gate
// above is structurally blind to: (reduce + (range N)) seeds only `range`, so it
// rides the chunked-reduce fast path and never exercises `map`/`filter`. The
// core audit (2026-07-23) found exactly this blind spot — map/filter dropped
// chunking, degrading `range -> map/filter -> reduce` ~3.3x, and no gate caught
// it. This one runs (count (filter odd? (map inc (range N)))): map, filter, and
// count all over a chunked source — the pipeline the chunk-aware fast path (ADR
// 0063) exists to keep fast. Like the reduce gate it is a WALL-CLOCK TOTAL
// regression gate, not a target — lower the budget as the path gets faster.
func TestCorePipelinePerfBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf measurement in -short mode")
	}
	const n = 2_000_000

	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(fmt.Sprintf("(count (filter odd? (map inc (range %d))))\n", n)), "corepipeline.clj")
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dir := t.TempDir()
	if err := WriteModule(dir, forms, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("write: %v", err)
	}
	bin := filepath.Join(dir, "corepipeline"+ExeSuffix)
	if err := GoBuild(dir, bin); err != nil {
		t.Fatalf("build: %v", err)
	}

	best := time.Duration(1<<62 - 1)
	for i := 0; i < 3; i++ {
		start := time.Now()
		if err := exec.Command(bin).Run(); err != nil {
			t.Fatalf("run: %v", err)
		}
		if d := time.Since(start); d < best {
			best = d
		}
	}

	budget := corePipelineBudget(t)
	t.Logf("(count (filter odd? (map inc (range %d)))) compiled: %v total wall clock (budget %v)", n, best, budget)
	if best > budget {
		t.Fatalf("clojure.core map/filter pipeline took %v, past the %v budget — a core-path regression (ADR 0063)", best, budget)
	}
}

// defaultCorePipelineBudget locks in today's measured cost with headroom, NOT a
// target: ~240ms measured (2026-07-23) for (count (filter odd? (map inc (range
// 2e6)))) including startup, with chunk-aware map/filter AND a chunk-aware
// `count` (the ADR 0063 follow-up — count now advances a whole chunk at a time
// instead of one Next() node per element, ~11% off the pipeline); 550ms leaves
// ~2x before the gate fires. Lower it as map/filter/count get faster.
const defaultCorePipelineBudget = 550 * time.Millisecond

// corePipelineBudget mirrors coreReduceBudget: host-relative (ADR 0024),
// overridable via CLJGO_CORE_PIPELINE_BUDGET for slower/noisier CI runners.
func corePipelineBudget(t *testing.T) time.Duration {
	t.Helper()
	s := os.Getenv("CLJGO_CORE_PIPELINE_BUDGET")
	if s == "" {
		return defaultCorePipelineBudget
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatalf("CLJGO_CORE_PIPELINE_BUDGET=%q is not a duration: %v", s, err)
	}
	return d
}
