package s19

// The Q1/Q2 numbers. Shapes:
//   - rendezvous: unbuffered ping (1 goroutine puts, bench goroutine takes)
//   - throughput: buffered(100) producer/consumer, per-item cost
//   - alts n=2 / n=8, one port ready, per-call cost
// Baseline is always the raw Go channel — the tax of each representation
// is (candidate - raw).

import (
	"testing"
)

// --- rendezvous -----------------------------------------------------------

func benchRendezvous(b *testing.B, put func(any), take func() any) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < b.N; i++ {
			put(1)
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		take()
	}
	<-done
}

func BenchmarkRendezvous_Raw(b *testing.B) {
	ch := make(chan any)
	benchRendezvous(b, func(v any) { ch <- v }, func() any { return <-ch })
}

func BenchmarkRendezvous_GoBacked(b *testing.B) {
	c := NewGoBacked(0, PolicyFixed, nil)
	benchRendezvous(b, func(v any) { c.Put(v) }, c.Take)
}

func BenchmarkRendezvous_AsyncChan(b *testing.B) {
	c := NewAsyncChan(0)
	benchRendezvous(b, func(v any) { c.Put(v) }, c.Take)
}

// --- buffered throughput --------------------------------------------------

func benchThroughput(b *testing.B, put func(any), take func() any) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < b.N; i++ {
			put(i)
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		take()
	}
	<-done
}

func BenchmarkThroughput100_Raw(b *testing.B) {
	ch := make(chan any, 100)
	benchThroughput(b, func(v any) { ch <- v }, func() any { return <-ch })
}

func BenchmarkThroughput100_GoBacked(b *testing.B) {
	c := NewGoBacked(100, PolicyFixed, nil)
	benchThroughput(b, func(v any) { c.Put(v) }, c.Take)
}

func BenchmarkThroughput100_GoBackedXform(b *testing.B) {
	c := NewGoBacked(100, PolicyFixed, XfMap(func(v any) any { return v }))
	benchThroughput(b, func(v any) { c.Put(v) }, c.Take)
}

func BenchmarkThroughput100_AsyncChan(b *testing.B) {
	c := NewAsyncChan(100)
	benchThroughput(b, func(v any) { c.Put(v) }, c.Take)
}

// --- alts, one ready port -------------------------------------------------

func benchAltsReflect(b *testing.B, n int) {
	chans := make([]*GoBacked, n)
	ports := make([]any, n)
	for i := range chans {
		chans[i] = NewGoBacked(1, PolicyFixed, nil)
		ports[i] = chans[i]
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chans[i%n].Put(1)
		AltsR(ports, false, nil)
	}
}

func benchAltsHandler(b *testing.B, n int) {
	chans := make([]*AsyncChan, n)
	ports := make([]any, n)
	for i := range chans {
		chans[i] = NewAsyncChan(1)
		ports[i] = chans[i]
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chans[i%n].Put(1)
		AltsH(ports, false, nil)
	}
}

func BenchmarkAlts2_Reflect(b *testing.B) { benchAltsReflect(b, 2) }
func BenchmarkAlts2_Handler(b *testing.B) { benchAltsHandler(b, 2) }
func BenchmarkAlts8_Reflect(b *testing.B) { benchAltsReflect(b, 8) }
func BenchmarkAlts8_Handler(b *testing.B) { benchAltsHandler(b, 8) }

// static select baseline (what a compiled alt! would emit), n=2
func BenchmarkAlts2_StaticSelect(b *testing.B) {
	c1 := make(chan any, 1)
	c2 := make(chan any, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			c1 <- 1
		} else {
			c2 <- 1
		}
		select {
		case v := <-c1:
			_ = v
		case v := <-c2:
			_ = v
		}
	}
}

func BenchmarkRendezvous_GoBacked2(b *testing.B) {
	c := NewGoBacked2(0)
	benchRendezvous(b, func(v any) { c.Put(v) }, c.Take)
}

func BenchmarkThroughput100_GoBacked2(b *testing.B) {
	c := NewGoBacked2(100)
	benchThroughput(b, func(v any) { c.Put(v) }, c.Take)
}
