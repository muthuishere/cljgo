//go:build ignore

// fuzzgen — the s71 differential stress test, generator half.
//
// The design rule is "never silently mis-format". The prototype's own tests
// cannot prove that: they assert the cases I thought of, and the ..000 bug
// existed precisely because I had not thought of it. The only thing that can
// prove it is a DIFFERENTIAL test against the oracle — generate patterns,
// format the same instant on both hosts, and diff the strings.
//
//	go run fuzzgen.go > /tmp/s71/patterns.txt   # one pattern per line
//	clojure -M oracle.clj < patterns.txt        # pattern<TAB>jvm-output
//	go test -run TestDifferential               # compares
//
// Patterns are assembled from the SUPPORTED token set plus literal noise, so
// most should compile; the ones that do not are checked to be refusals rather
// than wrong answers.
package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// tokens the translator claims to support, with their legal run lengths.
var toks = []struct {
	ch   string
	runs []int
}{
	{"y", []int{2, 4}},
	{"M", []int{1, 2, 3, 4}},
	{"d", []int{1, 2}},
	{"E", []int{1, 3, 4}},
	{"H", []int{1, 2}},
	{"h", []int{1, 2}},
	{"m", []int{1, 2}},
	{"s", []int{1, 2}},
	{"a", []int{1}},
	{"X", []int{1, 2, 3}},
	{"Z", []int{1}},
}

// separators that may appear between tokens, including the two that make the
// fraction legal and several that do not.
var seps = []string{"-", "/", ":", " ", ".", ",", "'T'", "", "_", "'at' "}

func main() {
	r := rand.New(rand.NewSource(20260731))
	seen := map[string]bool{}
	for len(seen) < 4000 {
		n := 1 + r.Intn(6)
		var b strings.Builder
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(seps[r.Intn(len(seps))])
			}
			t := toks[r.Intn(len(toks))]
			b.WriteString(strings.Repeat(t.ch, t.runs[r.Intn(len(t.runs))]))
		}
		// Sprinkle fractions: sometimes legal (after . or ,), sometimes not.
		if r.Intn(4) == 0 {
			b.WriteString([]string{".", ",", " ", ""}[r.Intn(4)])
			b.WriteString(strings.Repeat("S", 1+r.Intn(3)))
		}
		p := b.String()
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		fmt.Println(p)
	}
}
