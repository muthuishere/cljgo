package lang

// Channel-op perf budget (openspec core-async-first-class 1.6, ADR 0024
// host-relative discipline). The wrapper tax is measured as a RATIO
// against raw Go channels on the same host in the same process, so the
// budget transfers across machines the way ADR 0024's absolute
// wall-clock ceilings could not. Reference numbers (S19, darwin/arm64):
// rendezvous 137.3 ns vs 100.5 ns raw (1.37×), buffered 31.0 vs 25.9
// (1.20×), alts n=2 101.5 ns vs 31.6 ns static select (3.2×).
//
// Ceilings (env-overridable, tasks.md 1.6): wrapper ≤ 1.5× raw
// (CLJGO_CHAN_TAX_MAX), alts n=2 ≤ 5× a static Go select
// (CLJGO_ALTS_TAX_MAX). These catch a pathological regression (a lock
// on the take path, an allocation per op, a handler-queue rewrite —
// S19's rejected candidate was 2.7–5.6×), not scheduler noise; CI can
// loosen via the env vars exactly as ADR 0024 prescribes.

import (
	"os"
	"strconv"
	"testing"
)

func BenchmarkRawChanRendezvous(b *testing.B) {
	ch := make(chan any)
	go func() {
		for range b.N {
			<-ch
		}
	}()
	b.ResetTimer()
	for range b.N {
		ch <- int64(1)
	}
}

func BenchmarkChanRendezvous(b *testing.B) {
	c := NewChan(0)
	go func() {
		for range b.N {
			ChanRecv(c)
		}
	}()
	b.ResetTimer()
	for range b.N {
		ChanSend(c, int64(1))
	}
}

func BenchmarkRawChanBuffered(b *testing.B) {
	ch := make(chan any, 100)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range b.N {
			<-ch
		}
	}()
	b.ResetTimer()
	for range b.N {
		ch <- int64(1)
	}
	<-done
}

func BenchmarkChanBuffered(b *testing.B) {
	c := NewChan(100)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range b.N {
			ChanRecv(c)
		}
	}()
	b.ResetTimer()
	for range b.N {
		ChanSend(c, int64(1))
	}
	<-done
}

func BenchmarkStaticSelect2(b *testing.B) {
	c1 := make(chan any, 1)
	c2 := make(chan any, 1)
	for range b.N {
		c1 <- int64(1)
		select {
		case <-c1:
		case <-c2:
		}
	}
}

func BenchmarkAlts2(b *testing.B) {
	c1 := NewChan(1)
	c2 := NewChan(1)
	ports := []any{c1, c2}
	b.ResetTimer()
	for range b.N {
		ChanSend(c1, int64(1))
		Alts(ports, false, nil, false)
	}
}

func BenchmarkAlts8(b *testing.B) {
	chans := make([]*Channel, 8)
	ports := make([]any, 8)
	for i := range chans {
		chans[i] = NewChan(1)
		ports[i] = chans[i]
	}
	b.ResetTimer()
	for i := range b.N {
		ChanSend(chans[i%8], int64(1))
		Alts(ports, false, nil, false)
	}
}

// budgetRatio reads an env-overridable ceiling (ADR 0024).
func budgetRatio(env string, def float64) float64 {
	if s := os.Getenv(env); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v
		}
	}
	return def
}

// TestChanOpBudget pins the wrapper tax vs raw Go channels and the alts
// tax vs a static Go select (openspec core-async-first-class 1.6).
func TestChanOpBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf measurement in -short mode")
	}
	if raceEnabled {
		t.Skip("skipping perf measurement under -race: instrumentation overhead is asymmetric between raw and wrapper ops")
	}

	best := func(bench func(b *testing.B)) float64 {
		bestNs := 1e18
		for range 3 {
			r := testing.Benchmark(bench)
			if ns := float64(r.NsPerOp()); ns < bestNs {
				bestNs = ns
			}
		}
		return bestNs
	}

	rawR := best(BenchmarkRawChanRendezvous)
	wrapR := best(BenchmarkChanRendezvous)
	rawB := best(BenchmarkRawChanBuffered)
	wrapB := best(BenchmarkChanBuffered)
	sel2 := best(BenchmarkStaticSelect2)
	alts2 := best(BenchmarkAlts2)

	chanMax := budgetRatio("CLJGO_CHAN_TAX_MAX", 1.5)
	altsMax := budgetRatio("CLJGO_ALTS_TAX_MAX", 5.0)

	t.Logf("rendezvous: wrapper %.1f ns vs raw %.1f ns = %.2fx (ceiling %.2fx)",
		wrapR, rawR, wrapR/rawR, chanMax)
	t.Logf("buffered:   wrapper %.1f ns vs raw %.1f ns = %.2fx (ceiling %.2fx)",
		wrapB, rawB, wrapB/rawB, chanMax)
	t.Logf("alts n=2:   %.1f ns vs static select %.1f ns = %.2fx (ceiling %.2fx)",
		alts2, sel2, alts2/sel2, altsMax)

	if ratio := wrapR / rawR; ratio > chanMax {
		t.Errorf("rendezvous wrapper tax %.2fx exceeds ceiling %.2fx (S19 measured 1.37x)", ratio, chanMax)
	}
	if ratio := wrapB / rawB; ratio > chanMax {
		t.Errorf("buffered wrapper tax %.2fx exceeds ceiling %.2fx (S19 measured 1.20x)", ratio, chanMax)
	}
	if ratio := alts2 / sel2; ratio > altsMax {
		t.Errorf("alts n=2 tax %.2fx over static select exceeds ceiling %.2fx (S19 measured 3.2x)", ratio, altsMax)
	}
}
