package goid

import (
	"sync"
	"testing"
)

// TestGetMatchesStackParse pins the load-bearing property of ADR 0034:
// whatever implementation Get compiles to (fast g-field read or the
// fallback), it returns exactly the ID runtime.Stack reports for the
// same goroutine. A divergence here would mean cross-goroutine dynamic
// binding corruption — the unforgivable failure mode.
func TestGetMatchesStackParse(t *testing.T) {
	if got, want := Get(), getSlow(); got != want {
		t.Fatalf("Get() = %d, stack-parse oracle = %d", got, want)
	}
}

// TestGetMatchesStackParseConcurrent hammers the comparison from many
// goroutines at once (run under -race in CI). Each goroutine checks
// repeatedly so it is likely to be preempted and migrated between Ms
// mid-loop — the pointer-stability property the fast path relies on.
func TestGetMatchesStackParseConcurrent(t *testing.T) {
	const goroutines = 300
	const iters = 50

	var wg sync.WaitGroup
	errs := make(chan string, goroutines)
	seen := make([]int64, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			id := Get()
			seen[slot] = id
			for j := 0; j < iters; j++ {
				if got := Get(); got != id {
					errs <- "Get() changed within one goroutine"
					return
				}
				if oracle := getSlow(); oracle != id {
					errs <- "Get() disagrees with stack-parse oracle"
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	// IDs must be unique per goroutine.
	uniq := make(map[int64]bool, goroutines)
	for _, id := range seen {
		if uniq[id] {
			t.Fatalf("goroutine ID %d observed by two distinct goroutines", id)
		}
		uniq[id] = true
	}
}

func BenchmarkGoidGet(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Get()
	}
}

func BenchmarkGoidGetSlow(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = getSlow()
	}
}
