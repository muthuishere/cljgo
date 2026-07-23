// S40 probe — streaming file/byte I/O as a cljgo reducible.
//
// Throwaway spike code (ADR 0027). Models the cljgo runtime contracts
// (IReduceInit + IFn.Invoke + Reduced short-circuit) faithfully enough to
// price the wrapper tax honestly, without importing pkg/lang.
package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"time"
)

// ---------------------------------------------------------------------------
// Runtime shims — mirror pkg/lang exactly enough to charge the real tax.
//
//	IFn.Invoke(args ...any) any        (pkg/lang/interfaces.go:43)
//	IReduceInit.ReduceInit(f, init)    (pkg/lang/interfaces.go:52)
//	Reduced + IsReduced short-circuit  (as LongRange.ReduceInit does)
// ---------------------------------------------------------------------------

type IFn interface {
	Invoke(args ...any) any
}

type IReduceInit interface {
	ReduceInit(f IFn, init any) any
}

// Reduced is the wrap the transducer machinery uses to signal "stop now".
type Reduced struct{ val any }

func (r *Reduced) Deref() any { return r.val }

func isReduced(x any) bool { _, ok := x.(*Reduced); return ok }

// stepFn adapts a Go closure into an IFn, boxing exactly like a cljgo fn
// value does when reduce calls it: variadic any args, any return.
type stepFn func(acc, x any) any

func (s stepFn) Invoke(args ...any) any { return s(args[0], args[1]) }

// ---------------------------------------------------------------------------
// The blessed form: LinesReducible — an io/lines value.
//
// Implements IReduceInit. Opens the file at reduce time, streams it through a
// bufio scan, feeds each line (as a Go string = cljgo string) to the step fn,
// honours Reduced, and closes the file when the reduction ends. Constant
// memory: only one line buffer is ever live.
// ---------------------------------------------------------------------------

type LinesReducible struct {
	path  string
	gzip  bool
	bufKB int
}

// linesReadLast records how many lines the most recent reduction actually
// pulled from disk — used to prove `take` short-circuits the file read.
var linesReadLast int64

func (lr *LinesReducible) ReduceInit(f IFn, init any) any {
	fh, err := os.Open(lr.path)
	if err != nil {
		panic(err)
	}
	defer fh.Close()

	var r io.Reader = fh
	if lr.gzip {
		gz, err := gzip.NewReader(fh)
		if err != nil {
			panic(err)
		}
		defer gz.Close()
		r = gz
	}

	sc := bufio.NewScanner(bufio.NewReaderSize(r, lr.bufKB*1024))
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // tolerate long lines

	var n int64
	acc := init
	for sc.Scan() {
		n++
		// sc.Text() allocates a string per line (cljgo strings are Go
		// strings; the reducer sees an immutable string just as it would
		// from any seq). This is the honest per-line cost.
		acc = f.Invoke(acc, sc.Text())
		if isReduced(acc) {
			acc = acc.(*Reduced).Deref()
			break
		}
	}
	if err := sc.Err(); err != nil {
		panic(err)
	}
	linesReadLast = n
	return acc
}

func lines(path string) *LinesReducible {
	return &LinesReducible{path: path, bufKB: 256}
}
func gzLines(path string) *LinesReducible {
	return &LinesReducible{path: path, gzip: true, bufKB: 256}
}

// reduce — the cljgo core entry that dispatches to IReduceInit. Modelled here
// so the tax measurement goes through the same interface indirection the real
// `reduce` does.
func reduce(f IFn, init any, coll IReduceInit) any {
	return coll.ReduceInit(f, init)
}

// ---------------------------------------------------------------------------
// Transducers — modelled exactly as cljgo's step-fn composition. Each takes an
// IFn (the downstream step) and returns an IFn. `into []` = reduce with the
// composed step over a conj-to-slice reducer.
// ---------------------------------------------------------------------------

func xfilter(pred func(any) bool) func(IFn) IFn {
	return func(rf IFn) IFn {
		return stepFn(func(acc, x any) any {
			if pred(x) {
				return rf.Invoke(acc, x)
			}
			return acc
		})
	}
}

func xmap(fn func(any) any) func(IFn) IFn {
	return func(rf IFn) IFn {
		return stepFn(func(acc, x any) any {
			return rf.Invoke(acc, fn(x))
		})
	}
}

func xtake(n int) func(IFn) IFn {
	return func(rf IFn) IFn {
		left := n
		return stepFn(func(acc, x any) any {
			if left <= 0 {
				return &Reduced{acc}
			}
			left--
			res := rf.Invoke(acc, x)
			if left <= 0 {
				return &Reduced{deref(res)}
			}
			return res
		})
	}
}

func deref(x any) any {
	if r, ok := x.(*Reduced); ok {
		return r.val
	}
	return x
}

func comp(xs ...func(IFn) IFn) func(IFn) IFn {
	return func(rf IFn) IFn {
		out := rf
		for i := len(xs) - 1; i >= 0; i-- {
			out = xs[i](out)
		}
		return out
	}
}

// intoVec — (into [] xform coll): transduce with a conj reducer.
func intoVec(xform func(IFn) IFn, coll IReduceInit) []any {
	var out []any
	conj := stepFn(func(acc, x any) any {
		out = append(out, x)
		return acc
	})
	reduce(xform(conj), nil, coll)
	return out
}

// ---------------------------------------------------------------------------
// Writer side: spitLines — stream a producer of lines to disk, buffered,
// constant memory. Optional gzip.
// ---------------------------------------------------------------------------

func spitLines(path string, gzOut bool, produce func(emit func(string))) (int64, error) {
	fh, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer fh.Close()

	var w io.Writer = fh
	bw := bufio.NewWriterSize(fh, 256*1024)

	var gz *gzip.Writer
	if gzOut {
		gz = gzip.NewWriter(bw)
		w = gz
	} else {
		w = bw
	}

	var bytesW int64
	emit := func(s string) {
		n, _ := io.WriteString(w, s)
		m, _ := io.WriteString(w, "\n")
		bytesW += int64(n + m)
	}
	produce(emit)

	if gz != nil {
		if err := gz.Close(); err != nil {
			return bytesW, err
		}
	}
	if err := bw.Flush(); err != nil {
		return bytesW, err
	}
	return bytesW, nil
}

// ---------------------------------------------------------------------------
// Test data generation
// ---------------------------------------------------------------------------

// genFile writes ~targetMB of lines: "<id>\t<random word> <int>\n".
// Returns (path, lineCount, sumOfInts, byteSize).
func genFile(path string, targetMB int) (int64, int64, int64) {
	fh, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer fh.Close()
	bw := bufio.NewWriterSize(fh, 1<<20)
	defer bw.Flush()

	rng := rand.New(rand.NewSource(42))
	words := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"}
	target := int64(targetMB) << 20
	var written, lineCount, sum int64
	buf := make([]byte, 0, 128)
	for written < target {
		v := int64(rng.Intn(1000))
		buf = buf[:0]
		buf = strconv.AppendInt(buf, lineCount, 10)
		buf = append(buf, '\t')
		buf = append(buf, words[rng.Intn(len(words))]...)
		buf = append(buf, ' ')
		buf = strconv.AppendInt(buf, v, 10)
		buf = append(buf, '\n')
		bw.Write(buf)
		written += int64(len(buf))
		lineCount++
		sum += v
	}
	return lineCount, sum, written
}

// gzipFile compresses src -> dst.
func gzipFile(src, dst string) int64 {
	in, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer out.Close()
	bw := bufio.NewWriterSize(out, 1<<20)
	gz := gzip.NewWriter(bw)
	if _, err := io.Copy(gz, bufio.NewReaderSize(in, 1<<20)); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil { // writes gzip trailer into bw
		panic(err)
	}
	if err := bw.Flush(); err != nil { // flush trailer to disk (was the EOF bug)
		panic(err)
	}
	fi, _ := out.Stat()
	return fi.Size()
}

// ---------------------------------------------------------------------------
// Reductions used across the criteria. Parses the trailing int of each line.
// ---------------------------------------------------------------------------

func trailingInt(line string) int64 {
	// last space-delimited token
	i := len(line) - 1
	for i >= 0 && line[i] != ' ' {
		i--
	}
	v, _ := strconv.ParseInt(line[i+1:], 10, 64)
	return v
}

// wrapped reduction: count + sum through the reducible + boxed step fn.
func sumViaReducible(coll IReduceInit) (int64, int64) {
	var count int64
	step := stepFn(func(acc, x any) any {
		count++
		return acc.(int64) + trailingInt(x.(string))
	})
	sum := reduce(step, int64(0), coll).(int64)
	return count, sum
}

// count-only reductions isolate the pure dispatch tax (interface Invoke + arg
// box + Reduced check per line) from the int-boxing-of-accumulator tax.
func countViaReducible(coll IReduceInit) int64 {
	var count int64
	step := stepFn(func(acc, x any) any { count++; return acc })
	reduce(step, nil, coll)
	return count
}

func countViaRawScanner(path string) int64 {
	fh, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer fh.Close()
	sc := bufio.NewScanner(bufio.NewReaderSize(fh, 256*1024))
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var count int64
	for sc.Scan() {
		_ = sc.Text()
		count++
	}
	return count
}

// raw baseline: identical work in straight Go, no interface/box/Reduced.
func sumViaRawScanner(path string) (int64, int64) {
	fh, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer fh.Close()
	sc := bufio.NewScanner(bufio.NewReaderSize(fh, 256*1024))
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var count, sum int64
	for sc.Scan() {
		count++
		sum += trailingInt(sc.Text())
	}
	if err := sc.Err(); err != nil {
		panic(err)
	}
	return count, sum
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

var pass = true

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		pass = false
	}
	fmt.Printf("[%s] %s — %s\n", status, name, detail)
}

func peakHeapMB() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

// heapSampler polls live heap every 2ms (ReadMemStats is stop-the-world, so a
// tight loop would wreck throughput — a low-rate ticker samples the peak
// without meaningfully slowing the measured work). Returns a stop func that
// returns the peak MB observed.
func heapSampler() func() float64 {
	var peak float64
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if h := peakHeapMB(); h > peak {
					peak = h
				}
			}
		}
	}()
	return func() float64 { close(done); return peak }
}

func mbps(bytes int64, d time.Duration) float64 {
	return (float64(bytes) / (1 << 20)) / d.Seconds()
}

func main() {
	dir, err := os.MkdirTemp("", "s40")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	fmt.Printf("== S40 streaming file I/O probe ==\ntmp: %s\n\n", dir)

	// --- Criterion 1 + 2 + 5: generate 200MB, measure read/write, tax ---
	main200 := dir + "/data200.txt"
	fmt.Println("generating 200MB test file...")
	genStart := time.Now()
	lineCount, wantSum, byteSize := genFile(main200, 200)
	fmt.Printf("  %d lines, %d bytes, gen %.2fs\n\n", lineCount, byteSize, time.Since(genStart).Seconds())

	// Criterion 1: reducible read, constant memory, MB/s.
	runtime.GC()
	before := peakHeapMB()
	t0 := time.Now()
	cnt, sum := sumViaReducible(lines(main200))
	rdDur := time.Since(t0)
	after := peakHeapMB()
	runtime.GC()
	check("C1 reducible correctness", cnt == lineCount && sum == wantSum,
		fmt.Sprintf("count=%d sum=%d (want %d/%d)", cnt, sum, lineCount, wantSum))
	fmt.Printf("     read throughput: %.0f MB/s (%.2fs), heap %.1f→%.1f MB (Δ%.1f)\n\n",
		mbps(byteSize, rdDur), rdDur.Seconds(), before, after, after-before)

	// --- Criterion 1: constant-memory proof across sizes ---
	fmt.Println("C1 constant-memory sweep (peak heap during reduce):")
	for _, mb := range []int{50, 100, 200} {
		p := fmt.Sprintf("%s/sweep%d.txt", dir, mb)
		_, _, bs := genFile(p, mb)
		runtime.GC()
		stop := heapSampler()
		st := time.Now()
		sumViaReducible(lines(p))
		dur := time.Since(st)
		peak := stop()
		fmt.Printf("     %3d MB file → peak heap %.1f MB, %.0f MB/s\n", mb, peak, mbps(bs, dur))
		os.Remove(p)
	}
	fmt.Print("     (flat peak heap as file grows 4× ⇒ constant memory)\n\n")

	// Criterion 2: tax ratio, raw vs wrapped, averaged over N passes. Two
	// workloads: count-only (pure dispatch tax) and sum (adds acc int-boxing).
	const passes = 5
	var rawC, wrapC, rawS, wrapS time.Duration
	for i := 0; i < passes; i++ {
		t := time.Now()
		rc := countViaRawScanner(main200)
		rawC += time.Since(t)
		t = time.Now()
		wc := countViaReducible(lines(main200))
		wrapC += time.Since(t)

		t = time.Now()
		rsc, rs := sumViaRawScanner(main200)
		rawS += time.Since(t)
		t = time.Now()
		wsc, ws := sumViaReducible(lines(main200))
		wrapS += time.Since(t)
		if i == 0 {
			check("C2 raw==wrapped result", rc == wc && rsc == wsc && rs == ws,
				fmt.Sprintf("count raw/wrap=%d/%d sum raw/wrap=%d/%d", rc, wc, rs, ws))
		}
	}
	rawCavg, wrapCavg := rawC/passes, wrapC/passes
	rawSavg, wrapSavg := rawS/passes, wrapS/passes
	ratioC := float64(wrapCavg) / float64(rawCavg)
	ratioS := float64(wrapSavg) / float64(rawSavg)
	fmt.Printf("     count-only  raw %.0f MB/s vs wrapped %.0f MB/s → dispatch tax %.2f×\n",
		mbps(byteSize, rawCavg), mbps(byteSize, wrapCavg), ratioC)
	fmt.Printf("     sum(parse)  raw %.0f MB/s vs wrapped %.0f MB/s → +acc-box tax %.2f×\n",
		mbps(byteSize, rawSavg), mbps(byteSize, wrapSavg), ratioS)
	check("C2 tax characterised", ratioC > 1.0 && ratioS > 1.0,
		fmt.Sprintf("dispatch %.2f×, dispatch+accbox %.2f× (informational, ADR 0024)", ratioC, ratioS))
	fmt.Println()

	// Criterion 3: transducer pipeline, take short-circuits the file read.
	linesReadLast = 0
	pipe := comp(
		xfilter(func(x any) bool { return trailingInt(x.(string))%2 == 0 }),
		xmap(func(x any) any { return trailingInt(x.(string)) + 1 }),
		xtake(5),
	)
	got := intoVec(pipe, lines(main200))
	check("C3 into [] (filter/map/take) semantics", len(got) == 5,
		fmt.Sprintf("got %v", got))
	check("C3 take short-circuits file read", linesReadLast < lineCount/10,
		fmt.Sprintf("read %d of %d lines then stopped", linesReadLast, lineCount))
	fmt.Println()

	// Criterion 4: gzip pass-through through the SAME lines API.
	gzPath := dir + "/data200.txt.gz"
	fmt.Println("gzipping 200MB file for codec test...")
	gzSize := gzipFile(main200, gzPath)
	tg := time.Now()
	gc, gs := sumViaReducible(gzLines(gzPath))
	gzDur := time.Since(tg)
	check("C4 gzip pass-through (same API)", gc == lineCount && gs == wantSum,
		fmt.Sprintf("count=%d sum=%d, %.1fMB gz → %.0f MB/s decompressed",
			gc, gs, float64(gzSize)/(1<<20), mbps(byteSize, gzDur)))

	// io/copy + io/tee sanity (Reader→Writer, and fan to a second sink).
	copyDst := dir + "/copy.txt"
	teeDst := dir + "/tee.txt"
	func() {
		in, _ := os.Open(main200)
		defer in.Close()
		cf, _ := os.Create(copyDst)
		defer cf.Close()
		tf, _ := os.Create(teeDst)
		defer tf.Close()
		bw := bufio.NewWriterSize(cf, 1<<20)
		tw := bufio.NewWriterSize(tf, 1<<20)
		defer bw.Flush()
		defer tw.Flush()
		n, _ := io.Copy(bw, io.TeeReader(bufio.NewReaderSize(in, 1<<20), tw))
		check("C4 io/copy + io/tee", n == byteSize, fmt.Sprintf("copied %d bytes to 2 sinks", n))
	}()
	fmt.Println()

	// Criterion 5: writer sink, constant memory, MB/s. Re-emit main200's ints.
	sinkPath := dir + "/sink.txt"
	runtime.GC()
	wbefore := peakHeapMB()
	stopW := heapSampler()
	tw := time.Now()
	nbytes, err := spitLines(sinkPath, false, func(emit func(string)) {
		// stream a large produced seq — never materialised
		for i := int64(0); i < lineCount; i++ {
			emit("line " + strconv.FormatInt(i, 10) + " payload data here")
		}
	})
	wrDur := time.Since(tw)
	wpeak := stopW()
	if err != nil {
		panic(err)
	}
	fi, _ := os.Stat(sinkPath)
	check("C5 writer sink correctness", fi.Size() == nbytes && nbytes > 0,
		fmt.Sprintf("%d bytes written", nbytes))
	fmt.Printf("     write throughput: %.0f MB/s (%.2fs), heap %.1f→peak %.1f MB (Δ%.1f)\n\n",
		mbps(nbytes, wrDur), wrDur.Seconds(), wbefore, wpeak, wpeak-wbefore)

	// gzip writer sink sanity
	gzSink := dir + "/sink.txt.gz"
	gzBytes, err := spitLines(gzSink, true, func(emit func(string)) {
		for i := int64(0); i < 100000; i++ {
			emit("compressed line " + strconv.FormatInt(i, 10))
		}
	})
	if err != nil {
		panic(err)
	}
	gzfi, _ := os.Stat(gzSink)
	check("C5 gzip writer sink", gzfi.Size() > 0 && gzfi.Size() < gzBytes,
		fmt.Sprintf("%d logical bytes → %d gz bytes", gzBytes, gzfi.Size()))

	fmt.Println()
	if pass {
		fmt.Println("== ALL PASS ==")
	} else {
		fmt.Println("== SOME FAILED ==")
		os.Exit(1)
	}
}
