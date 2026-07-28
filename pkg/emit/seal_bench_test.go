package emit

// seal_bench_test.go — the committed measurement behind the OPT-IN hard
// seal (ADR 0066 alternative 1, `cljgo build --seal-core`).
//
// It builds the SAME Clojure programs twice — once with the default
// guarded emission (Options{}), once with Options{SealCore: true} — plus a
// handwritten-Go denominator, and reports net emitted work in ms and the
// ratio to raw Go for each. That is the number the owner needs to rule on
// ever making hard-seal the default.
//
// Off by default (it builds six binaries and runs each three times); run it
// with:
//
//	CLJGO_SEAL_BENCH=1 go test ./pkg/emit -run TestSealCoreMeasure -v -timeout 900s

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// sealFactProg is the ADR 0067 typed-region case: fact lifts to a
// `func fnL_…(int64) int64` behind a region entry guard. Hard-seal removes
// the guard's `!rt.CoreDirty()` condition (one relaxed atomic load per
// activation of the boxed wrapper).
func sealFactProg(n int) string {
	return fmt.Sprintf(`
(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(loop* [i 0 acc 0]
  (if (< i %d)
    (recur (+ i 1) (+ acc (fact 15)))
    acc))
`, n)
}

// sealBoxedProg is the boxed-intrinsic case: the accumulator is a double,
// so the int64 inference cannot type it and every (+ acc 1.5) emits the
// guarded rt.Add2(v, x, y) — a var deref decision plus an atomic load PER
// OP. Hard-seal turns each into rt.Add2S(x, y).
func sealBoxedProg(n int) string {
	return fmt.Sprintf(`
(loop* [i 0 acc 0.5]
  (if (< i %d)
    (recur (+ i 1) (+ acc 1.5))
    acc))
`, n)
}

func sealFactGo(n int) string {
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

func sealBoxedGo(n int) string {
	return fmt.Sprintf(`package main

import "fmt"

func main() {
	acc := 0.5
	for i := 0; i < %d; i++ {
		acc += 1.5
	}
	fmt.Println(acc)
}
`, n)
}

func TestSealCoreMeasure(t *testing.T) {
	if os.Getenv("CLJGO_SEAL_BENCH") != "1" {
		t.Skip("set CLJGO_SEAL_BENCH=1 to run the --seal-core measurement")
	}

	buildClj := func(name, src string, opts Options) string {
		lang.RemoveNamespace(lang.NewSymbol("user"))
		oldOut := corelib.Out
		corelib.Out = io.Discard
		forms, err := CompileReader(strings.NewReader(src), name+".clj")
		corelib.Out = oldOut
		if err != nil {
			t.Fatalf("compile %s: %v", name, err)
		}
		dir := t.TempDir()
		opts.PrintLastValue = true
		if err := WriteModule(dir, forms, opts); err != nil {
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
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module sealraw\n\ngo 1.26\n"), 0o644); err != nil {
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
		for i := 0; i < 9; i++ {
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
	// net factors startup out by timing an idle (zero-iteration) variant of
	// the same program — the perf_test.go methodology.
	net := func(work, idle time.Duration) time.Duration { return work - idle }

	cases := []struct {
		name   string
		prog   func(int) string
		goProg func(int) string
		iters  int
	}{
		{"fact15 (typed region)", sealFactProg, sealFactGo, 2_000_000},
		{"float-acc (boxed rt.Add2)", sealBoxedProg, sealBoxedGo, 5_000_000},
	}

	for _, c := range cases {
		offWork := buildClj("offwork", c.prog(c.iters), Options{})
		offIdle := buildClj("offidle", c.prog(0), Options{})
		onWork := buildClj("onwork", c.prog(c.iters), Options{SealCore: true})
		onIdle := buildClj("onidle", c.prog(0), Options{SealCore: true})
		rawWork := buildGo("rawwork", c.goProg(c.iters))
		rawIdle := buildGo("rawidle", c.goProg(0))

		off := net(run(offWork), run(offIdle))
		on := net(run(onWork), run(onIdle))
		raw := net(run(rawWork), run(rawIdle))

		sz := func(p string) int64 {
			fi, err := os.Stat(p)
			if err != nil {
				return 0
			}
			return fi.Size()
		}
		t.Logf("%s (n=%d)", c.name, c.iters)
		t.Logf("  raw Go        : %8.1f ms", raw.Seconds()*1000)
		t.Logf("  --seal-core=0 : %8.1f ms  ratio %.2fx  binary %d B", off.Seconds()*1000, float64(off)/float64(raw), sz(offWork))
		t.Logf("  --seal-core=1 : %8.1f ms  ratio %.2fx  binary %d B", on.Seconds()*1000, float64(on)/float64(raw), sz(onWork))
		t.Logf("  hard-seal delta: %+.1f%%", (float64(on)/float64(off)-1)*100)
	}
}
