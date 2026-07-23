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

// TestS42Measure is the spike-s42 / ADR 0067 before/after measurement: it
// compiles each numeric kernel twice — inference OFF (the boxed baseline)
// and ON (unboxed int64) — and reports wall time and total process
// mallocs for both. Run explicitly:
//
//	go test ./pkg/emit -run TestS42Measure -v
func TestS42Measure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf measurement in -short mode")
	}
	kernels := []struct{ name, src string }{
		{"fact15x2M", `(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))
(loop* [i 0 acc 0] (if (< i 2000000) (recur (+ i 1) (+ acc (fact 15))) acc))`},
		{"fib35", `(def fib (fn* fib [n] (if (< n 2) n (+ (fib (- n 2)) (fib (- n 1))))))
(fib 35)`},
		{"loopsum10M", `(loop* [i 0 acc 0] (if (< i 10000000) (recur (+ i 1) (+ acc i)) acc))`},
	}

	build := func(name, src string, on bool) string {
		numInferEnabled = on
		lang.RemoveNamespace(lang.NewSymbol("user"))
		oldOut := corelib.Out
		corelib.Out = io.Discard
		forms, err := CompileReader(strings.NewReader(src), name+".clj")
		corelib.Out = oldOut
		if err != nil {
			t.Fatalf("%s compile: %v", name, err)
		}
		dir := t.TempDir()
		if err := WriteModule(dir, forms, Options{PrintLastValue: true}); err != nil {
			t.Fatalf("%s write: %v", name, err)
		}
		bin := filepath.Join(dir, name+ExeSuffix)
		if err := GoBuild(dir, bin); err != nil {
			t.Fatalf("%s build: %v", name, err)
		}
		return bin
	}
	bestWall := func(bin string) time.Duration {
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
	mallocs := func(bin string) uint64 {
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), "CLJGO_ALLOC_REPORT=1")
		var errBuf strings.Builder
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			t.Fatalf("alloc run %s: %v", bin, err)
		}
		for _, line := range strings.Split(errBuf.String(), "\n") {
			if strings.HasPrefix(line, "CLJGO_MALLOCS=") {
				v, _ := strconv.ParseUint(strings.TrimPrefix(line, "CLJGO_MALLOCS="), 10, 64)
				return v
			}
		}
		t.Fatalf("no CLJGO_MALLOCS line from %s: %q", bin, errBuf.String())
		return 0
	}

	defer func() { numInferEnabled = true }()
	fmt.Printf("\n=== spike s42 / ADR 0067 — boxed baseline vs unboxed int64 ===\n")
	fmt.Printf("%-14s %12s %12s %8s %14s %14s %8s\n", "kernel", "wall(off)", "wall(on)", "speedup", "mallocs(off)", "mallocs(on)", "drop")
	for _, k := range kernels {
		offBin := build(k.name, k.src, false)
		onBin := build(k.name, k.src, true)
		wOff, wOn := bestWall(offBin), bestWall(onBin)
		mOff, mOn := mallocs(offBin), mallocs(onBin)
		speed := float64(wOff) / float64(wOn)
		drop := float64(mOff-mOn) / 1e6
		fmt.Printf("%-14s %12v %12v %7.2fx %14d %14d %6.1fM\n",
			k.name, wOff.Round(time.Millisecond), wOn.Round(time.Millisecond), speed, mOff, mOn, drop)
	}
	fmt.Println()
}
