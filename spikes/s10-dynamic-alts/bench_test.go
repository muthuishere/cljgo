package alts

import (
	"reflect"
	"testing"
)

// Steady-state pattern for all benchmarks: n buffered(1) channels, exactly
// one made ready per iteration (round-robin), then one alts-read over all n.
// This measures the per-operation cost of each mechanism, including case
// construction — which is what emitted alts! code pays every call.

func benchDynamicAlts(b *testing.B, n int) {
	chs := make([]chan any, n)
	ops := make([]AltOp, n)
	for i := range chs {
		chs[i] = make(chan any, 1)
		ops[i] = AltOp{Chan: chs[i]}
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		chs[i%n] <- i
		if _, _, ok := Alts(ops, AltOpts{}); !ok {
			b.Fatal("unexpected not-ok")
		}
		i++
	}
}

func BenchmarkDynamicAlts2(b *testing.B)  { benchDynamicAlts(b, 2) }
func BenchmarkDynamicAlts8(b *testing.B)  { benchDynamicAlts(b, 8) }
func BenchmarkDynamicAlts32(b *testing.B) { benchDynamicAlts(b, 32) }

// Raw reflect.Select with pre-built cases — isolates how much of Alts's
// cost is per-call case construction vs reflect.Select itself.
func BenchmarkRawReflectSelect2Prebuilt(b *testing.B) {
	c1 := make(chan any, 1)
	c2 := make(chan any, 1)
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(c1)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(c2)},
	}
	chs := []chan any{c1, c2}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		chs[i%2] <- i
		_, _, recvOK := reflect.Select(cases)
		if !recvOK {
			b.Fatal("recv not ok")
		}
		i++
	}
}

// The fast path the architecture bets on: static alt! -> a real select.
func BenchmarkStaticSelect2(b *testing.B) {
	c1 := make(chan any, 1)
	c2 := make(chan any, 1)
	chs := []chan any{c1, c2}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		chs[i%2] <- i
		select {
		case v := <-c1:
			_ = v
		case v := <-c2:
			_ = v
		}
		i++
	}
}

// Alternative mechanism: goroutine-per-channel fan-in to one merged channel.
// Setup cost (n goroutines) is paid once; steady-state cost is the extra
// hop through the merged channel + forced goroutine handoff.
func benchFanIn(b *testing.B, n int) {
	srcs := make([]chan any, n)
	merged := make(chan any) // unbuffered: honest alts!-like rendezvous
	stop := make(chan struct{})
	for i := range srcs {
		srcs[i] = make(chan any, 1)
		go func(c chan any) {
			for {
				select {
				case v := <-c:
					select {
					case merged <- v:
					case <-stop:
						return
					}
				case <-stop:
					return
				}
			}
		}(srcs[i])
	}
	b.Cleanup(func() { close(stop) })
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		srcs[i%n] <- i
		<-merged
		i++
	}
}

func BenchmarkFanIn2(b *testing.B)  { benchFanIn(b, 2) }
func BenchmarkFanIn8(b *testing.B)  { benchFanIn(b, 8) }
func BenchmarkFanIn32(b *testing.B) { benchFanIn(b, 32) }

// Non-blocking miss (default taken) — the polling-loop worst case.
func BenchmarkDynamicAltsDefaultMiss8(b *testing.B) {
	n := 8
	ops := make([]AltOp, n)
	for i := range ops {
		ops[i] = AltOp{Chan: make(chan any)}
	}
	opts := AltOpts{HasDefault: true, Default: "d"}
	b.ReportAllocs()
	for b.Loop() {
		if _, ch, _ := Alts(ops, opts); ch != DefaultPort {
			b.Fatal("expected default")
		}
	}
}
