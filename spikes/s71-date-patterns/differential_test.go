package pattern

// The differential stress test: 4,000 generated patterns, formatted on BOTH
// hosts and diffed.
//
// The prototype's hand-written tests can only assert the cases their author
// thought of, and the ..000 bug existed precisely because one case was not
// thought of. This test needs no author insight — it asks the oracle.
//
// Three verdicts per pattern, and only one of them is a bug:
//
//	AGREE    both hosts produced the same string           — fine
//	REFUSED  cljgo refused to compile the pattern          — fine, by design
//	DIVERGE  cljgo produced a DIFFERENT string             — the failure the
//	         whole design rule exists to prevent
//
// A refusal is never a failure here: "errors on 20% of inputs is fine; silently
// mis-formats on 2% is not". What this test measures is that the 2% is zero.
//
// Regenerate the corpus (needs a JVM):
//
//	go run fuzzgen.go > /tmp/s71/patterns.txt
//	clojure -M oracle.clj < /tmp/s71/patterns.txt > testdata/oracle.tsv

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func loadOracle(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("testdata/oracle.tsv")
	if err != nil {
		t.Skipf("no oracle corpus (%v); regenerate with fuzzgen.go + oracle.clj", err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, '\t')
		if i < 0 {
			continue
		}
		out[line[:i]] = line[i+1:]
	}
	return out
}

// TestLayoutStringDesignIsUnfixable pins the REJECTED design's failure as
// evidence, not as a red test. The layout-string approach cannot reach zero
// divergence — Go's Format decides whether to substitute "Mon"/"Jan" based on
// the literal that FOLLOWS them, which a translator emitting a layout string
// cannot control. Two rounds of token fixes took this from 1252 to 108 and it
// stops there. direct_test.go runs the same corpus against the design that
// replaced it and reports 0.
//
// It asserts a CEILING rather than exact equality: any change that pushes
// divergence back up has reintroduced something, and any change that drops it
// materially should be investigated rather than silently absorbed.
func TestLayoutStringDesignIsUnfixable(t *testing.T) {
	oracle := loadOracle(t)
	if len(oracle) == 0 {
		t.Fatal("empty oracle corpus")
	}

	var agree, refused, jvmRejected int
	diverge := map[string][]string{} // "pattern -> got | want", keyed by token class

	for p, want := range oracle {
		if want == "!ERR" {
			// java.time itself rejects it. cljgo must also refuse — accepting
			// a pattern the oracle calls invalid is its own kind of drift.
			jvmRejected++
			if c, err := Compile(p); err == nil {
				diverge["accepted-what-jvm-rejected"] = append(
					diverge["accepted-what-jvm-rejected"],
					p+" -> layout "+c.Layout)
			}
			continue
		}
		c, err := Compile(p)
		if err != nil {
			refused++
			continue
		}
		got := ref.Format(c.Layout)
		if got == want {
			agree++
			continue
		}
		diverge[classify(p)] = append(diverge[classify(p)], p+"\n      got  "+got+"\n      want "+want)
	}

	t.Logf("corpus=%d  agree=%d  refused=%d  jvm-rejected=%d  DIVERGE=%d",
		len(oracle), agree, refused, jvmRejected, countAll(diverge))

	n := countAll(diverge)
	if n == 0 {
		t.Fatal("the layout-string design now agrees everywhere; if that is real, " +
			"re-open the design decision in spikes/s71-date-patterns/RESULTS.md")
	}
	if n > 120 {
		var classes []string
		for k := range diverge {
			classes = append(classes, k)
		}
		sort.Strings(classes)
		t.Errorf("layout-string divergence rose to %d (was 108); regression in %d classes, e.g.\n    %s",
			n, len(classes), strings.Join(first(diverge[classes[0]], 2), "\n    "))
	}
}

// classify buckets a divergence by the tokens present, so a hundred failures
// from one broken token report as one class instead of a hundred lines.
func classify(p string) string {
	var have []string
	for _, tok := range []string{"H", "h", "M", "d", "y", "E", "a", "X", "Z", "S", "m", "s"} {
		if strings.Contains(p, tok) {
			have = append(have, tok)
		}
	}
	return strings.Join(have, "")
}

func countAll(m map[string][]string) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}

func first(s []string, n int) []string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// TestMemoIsRaceFree stresses the memoisation strategy the spike settled on
// under concurrency — the one place a "simple cache" can stop being simple.
func TestMemoIsRaceFree(t *testing.T) {
	pats := []string{"yyyy-MM-dd", "HH:mm:ss", "dd/MM/yyyy HH:mm", "MMM d, yyyy", "hh:mm a"}
	want := map[string]string{}
	for _, p := range pats {
		c, err := Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		want[p] = ref.Format(c.Layout)
	}
	done := make(chan bool)
	for g := 0; g < 32; g++ {
		go func(g int) {
			for i := 0; i < 500; i++ {
				p := pats[(g+i)%len(pats)]
				c, err := Compile(p)
				if err != nil {
					t.Error(err)
					break
				}
				if got := ref.Format(c.Layout); got != want[p] {
					t.Errorf("%q: %q != %q", p, got, want[p])
					break
				}
			}
			done <- true
		}(g)
	}
	for g := 0; g < 32; g++ {
		<-done
	}
	_ = time.Now
}
