package lang

// Spike s72/s73 — what a keyword-keyed map costs to BUILD and to READ, across
// sizes, and what crossing the array-map -> hash-map threshold costs.
//
// This is the substrate under every Clojure map literal and under bri's
// per-request map, so its shape decides a lot that looks unrelated. Two
// questions:
//
//  1. Does build cost grow linearly with entry count, or worse? NewMap above
//     the threshold builds by N successive PERSISTENT assocs, each copying a
//     path through the trie — so the suspicion is N log N with a large
//     constant, where Clojure's own PersistentHashMap.create uses a transient.
//
//  2. What does the threshold itself cost? hashmapThreshold is 16 array slots
//     = 8 entries. bri's request map has 10, so it lands one step over the
//     line. If the step is large, "how many keys does the request map have" is
//     a performance decision nobody knew they were making.

import (
	"fmt"
	"testing"
)

func kwKeyVals(n int) []any {
	kvs := make([]any, 0, 2*n)
	for i := 0; i < n; i++ {
		kvs = append(kvs, NewKeyword(fmt.Sprintf("key-%d", i)), int64(i))
	}
	return kvs
}

// BenchmarkMapBuildBySize — build cost across an order of magnitude. The
// interesting rows are 8 (last array map) and 9 (first hash map).
func BenchmarkMapBuildBySize(b *testing.B) {
	for _, n := range []int{4, 6, 7, 8, 9, 10, 16, 32, 64, 128} {
		kvs := kwKeyVals(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = NewMap(kvs...)
			}
		})
	}
}

// BenchmarkMapGetBySize — read cost across the same sizes, one lookup of a key
// that is present. An array map is a linear scan and a hash map is a trie
// walk, so the threshold trades build cost against read cost; both halves have
// to be on the table before anyone moves it.
func BenchmarkMapGetBySize(b *testing.B) {
	for _, n := range []int{4, 6, 7, 8, 9, 10, 16, 32, 64, 128} {
		kvs := kwKeyVals(n)
		m := NewMap(kvs...)
		probe := kvs[len(kvs)-2] // the LAST key: worst case for a linear scan
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = Get(m, probe)
			}
		})
	}
}

// BenchmarkMapAssocBySize — one assoc onto an existing map. This is the
// operation middleware performs (each layer conj'ing onto the request map), so
// it compounds with the stack depth.
func BenchmarkMapAssocBySize(b *testing.B) {
	extra := NewKeyword("added-key")
	for _, n := range []int{4, 8, 9, 16, 64} {
		m := NewMap(kwKeyVals(n)...)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.Assoc(extra, int64(1))
			}
		})
	}
}
