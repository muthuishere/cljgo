package pattern

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// The reference instant, chosen so every field is distinguishable:
// 2026-07-31T09:04:05.123Z is a Friday, July, hour 9 (so h and H differ from
// each other only via am/pm), minute 4 and second 5 (so padded and unpadded
// forms differ).
var ref = time.Date(2026, 7, 31, 9, 4, 5, 123000000, time.UTC)

func TestCompileCorrectness(t *testing.T) {
	cases := []struct{ pattern, want string }{
		{"yyyy-MM-dd", "2026-07-31"},
		{"yyyy-MM-dd'T'HH:mm:ss", "2026-07-31T09:04:05"},
		{"yyyy-MM-dd HH:mm:ss.SSS", "2026-07-31 09:04:05.123"},
		{"dd/MM/yyyy", "31/07/2026"},
		{"d/M/yy", "31/7/26"},
		{"MMM d, yyyy", "Jul 31, 2026"},
		{"MMMM d, yyyy", "July 31, 2026"},
		{"EEE, dd MMM yyyy", "Fri, 31 Jul 2026"},
		{"EEEE", "Friday"},
		{"hh:mm a", "09:04 AM"},
		{"yyyy-MM-dd'T'HH:mm:ssXXX", "2026-07-31T09:04:05Z"},
		{"HH:mm:ss,SSS", "09:04:05,123"},
		{"'year' yyyy", "year 2026"},
		{"yyyy'''s' MM", "2026's 07"},
	}
	for _, c := range cases {
		got, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", c.pattern, err)
			continue
		}
		if out := ref.Format(got.Layout); out != c.want {
			t.Errorf("%q -> layout %q -> %q, want %q", c.pattern, got.Layout, out, c.want)
		}
	}
}

// The rule the whole design rests on: refuse loudly rather than approximate.
// Every one of these has a plausible-looking wrong answer, which is exactly
// why silence would be the dangerous outcome.
func TestUnsupportedTokensAreRefusedByName(t *testing.T) {
	for _, p := range []string{
		"G yyyy", "QQ", "DDD", "ww", "W", "F", "kk", "KK",
		"YYYY-MM", "uuuu-MM", "zzz", "VV",
	} {
		c, err := Compile(p)
		if err == nil {
			t.Errorf("Compile(%q) succeeded with layout %q; must refuse", p, c.Layout)
			continue
		}
		// The diagnostic has to name the token, or the user cannot act on it.
		if !strings.Contains(err.Error(), strings.TrimSpace(strings.Split(p, " ")[0])) &&
			!strings.ContainsAny(err.Error(), "GQDwWFkKYuzV") {
			t.Errorf("Compile(%q) error does not name the token: %v", p, err)
		}
	}
}

func TestMalformedRunsAreRefused(t *testing.T) {
	for _, p := range []string{"yyy", "MMMMM", "ddd", "HHH", "SSSSSSSSSS", "'unterminated", "SSS", "HH:mm:ss SSS"} {
		if c, err := Compile(p); err == nil {
			t.Errorf("Compile(%q) succeeded with layout %q; must refuse", p, c.Layout)
		}
	}
}

// ---- the performance question -------------------------------------------
//
// Three strategies for `(date/format inst "yyyy-MM-dd HH:mm:ss")`:
//
//	PerCall     translate the pattern on every call            (naive)
//	Cached      translate once, memoise by pattern string      (runtime cache)
//	Precompiled the layout is already a constant               (comptime ideal)
//
// The third is what the zero-cost mandate demands: a pattern LITERAL is known
// at compile time, so the analyzer should translate it then and emit the Go
// layout as a constant, leaving zero translation in the hot path. These
// benchmarks price the gap that mandate is buying.

const benchPattern = "yyyy-MM-dd HH:mm:ss.SSS"

func BenchmarkFormatPerCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c, err := Compile(benchPattern)
		if err != nil {
			b.Fatal(err)
		}
		_ = ref.Format(c.Layout)
	}
}

func BenchmarkFormatCached(b *testing.B) {
	var mu sync.RWMutex
	cache := map[string]string{}
	get := func(p string) string {
		mu.RLock()
		lay, ok := cache[p]
		mu.RUnlock()
		if ok {
			return lay
		}
		c, err := Compile(p)
		if err != nil {
			b.Fatal(err)
		}
		mu.Lock()
		cache[p] = c.Layout
		mu.Unlock()
		return c.Layout
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ref.Format(get(benchPattern))
	}
}

func BenchmarkFormatPrecompiled(b *testing.B) {
	c, err := Compile(benchPattern)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ref.Format(c.Layout)
	}
}

// The translation step alone, so the ADR can quote it without the Format cost
// mixed in.
func BenchmarkCompileOnly(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Compile(benchPattern); err != nil {
			b.Fatal(err)
		}
	}
}
