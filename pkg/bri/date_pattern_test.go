package bri

// The differential evidence behind ADR 0113: 4,000 generated patterns,
// formatted on BOTH hosts and diffed.
//
// Hand-written tests can only assert the cases their author thought of, and
// three real bugs in the s71 prototype existed precisely because a case was not
// thought of — H printing "09" where the JVM prints "9", unbounded EEEEEEE and
// aa runs the JVM rejects, and Locale.ROOT advice that was itself wrong. None
// were reachable from the hand-written suite. This test needs no author
// insight: it asks the oracle.
//
// Three verdicts per pattern, and only one is a bug:
//
//	AGREE    both hosts produced the same string   — fine
//	REFUSED  cljgo refused to compile the pattern  — fine, BY DESIGN
//	DIVERGE  cljgo produced a DIFFERENT string     — the failure the whole
//	         design rule exists to prevent
//
// A refusal is never a failure here: "a pattern language that errors on 20% of
// inputs is fine; one that silently mis-formats on 2% is not". What this
// measures is that the 2% is zero.
//
// Regenerate the corpus (needs a JVM):
//
//	go run spikes/s71-date-patterns/fuzzgen.go > /tmp/patterns.txt
//	clojure -M pkg/bri/testdata/date-pattern-oracle.clj < /tmp/patterns.txt \
//	    > pkg/bri/testdata/date-pattern-oracle.tsv

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/diag"
)

// dpRef is the instant the oracle rendered: 2026-07-31T09:04:05.123Z. A Friday
// in July at 09:04 — chosen so a single instant exercises unpadded vs padded
// hours, a three-letter and a full month, a weekday, and a non-zero fraction.
var dpRef = time.Date(2026, 7, 31, 9, 4, 5, 123000000, time.UTC)

func loadDateOracle(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("testdata/date-pattern-oracle.tsv")
	if err != nil {
		t.Fatalf("oracle corpus missing: %v", err)
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

// TestDatePatternMatchesJVMOracle is the release gate on the pattern language.
// Zero divergences, and zero patterns accepted here that the JVM rejects — the
// second is as important as the first, because accepting more than the oracle
// means a .cljc author can write something that works on cljgo and throws on
// the JVM.
func TestDatePatternMatchesJVMOracle(t *testing.T) {
	oracle := loadDateOracle(t)
	if len(oracle) < 3000 {
		t.Fatalf("corpus too small to be evidence: %d patterns", len(oracle))
	}
	var agree, refused, jvmRejected, wrongAccept int
	var diverge []string
	for p, want := range oracle {
		d, err := dpCompile(p)
		if want == "!ERR" { // the JVM refused this pattern
			jvmRejected++
			if err == nil {
				wrongAccept++
				if len(diverge) < 10 {
					diverge = append(diverge,
						"accepted "+p+" but the JVM rejects it")
				}
			}
			continue
		}
		if err != nil {
			refused++
			continue
		}
		if got := d.format(dpRef); got != want {
			if len(diverge) < 10 {
				diverge = append(diverge, p+"\n      got  "+got+"\n      want "+want)
			}
		} else {
			agree++
		}
	}
	t.Logf("corpus=%d agree=%d refused=%d jvm-rejected=%d wrongly-accepted=%d DIVERGE=%d",
		len(oracle), agree, refused, jvmRejected, wrongAccept, len(diverge))
	for _, d := range diverge {
		t.Errorf("DIVERGENCE: %s", d)
	}
}

// TestDatePatternRefusalsNameTheToken — a refusal that does not say WHICH token
// it refused is only marginally better than a wrong answer.
func TestDatePatternRefusalsNameTheToken(t *testing.T) {
	for _, tc := range []struct{ pattern, want string }{
		{"yyyy-QQ", "Q"},
		{"yyyy-DDD", "D"},
		{"HH:mm z", "z"},
		{"YYYY", "Y"},
		{"yyy", "y"},
		{"EEEEEEE", "E"},
		{"aa", "a"},
		{"yyyy 'unterminated", "unterminated"},
	} {
		_, err := dpCompile(tc.pattern)
		if err == nil {
			t.Errorf("%q was accepted; it must be refused", tc.pattern)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("refusal of %q does not name %q: %v", tc.pattern, tc.want, err)
		}
		if !strings.Contains(err.Error(), tc.pattern) &&
			tc.want != "unterminated" {
			t.Errorf("refusal of %q does not quote the pattern: %v", tc.pattern, err)
		}
	}
}

// TestDatePatternRefusalCarriesG5022 — the code is what `cljgo explain` and the
// --json envelope hang off; a bare message is not enough.
func TestDatePatternRefusalCarriesG5022(t *testing.T) {
	_, err := dpCompile("yyyy-QQ")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	// Assert through the renderer the REPL, `cljgo run`, compiled binaries and
	// the --json envelope all go through, not through the concrete type.
	dg := diag.FromError(err)
	if dg.ErrorCode != "G5022" {
		t.Errorf("refusal carries %q, want G5022", dg.ErrorCode)
	}
	if _, err := diag.Explain("G5022"); err != nil {
		t.Errorf("G5022 has no explain page: %v", err)
	}
	if got := diag.Render(dg); !strings.HasSuffix(got, "help: run `cljgo explain G5022`") {
		t.Errorf("rendered refusal has no help pointer: %s", got)
	}
}

// TestDatePatternRoundTrips — parse is driven by the SAME op list as format, so
// what format writes, parse must read back. Anything else means the two
// descriptions of the pattern have drifted apart.
func TestDatePatternRoundTrips(t *testing.T) {
	for _, p := range []string{
		"yyyy-MM-dd HH:mm:ss",
		"yyyy-MM-dd'T'HH:mm:ss.SSSXXX",
		"dd/MM/yyyy",
		"EEE, d MMM yyyy HH:mm:ss Z",
		"yyyy-MM-dd hh:mm a",
		"MMMM d, yyyy",
		"yy-M-d H:m:s",
	} {
		d, err := dpCompile(p)
		if err != nil {
			t.Fatalf("%q: %v", p, err)
		}
		s := d.format(dpRef)
		got, err := d.parse(s)
		if err != nil {
			t.Errorf("%q: formatted %q but could not read it back: %v", p, s, err)
			continue
		}
		// yy loses the century and a pattern without seconds loses them; compare
		// only what the pattern actually carries, by re-formatting.
		if back := d.format(got.UTC()); back != s {
			t.Errorf("%q: round trip changed the value: %q -> %q", p, s, back)
		}
	}
}

// TestDatePatternParseRefusesIncompletePatterns — the doctrine applied to
// parsing. Defaulting a missing year to 1970, or a missing meridiem to AM, is
// exactly the silent-wrongness this design exists to eliminate.
func TestDatePatternParseRefusesIncompletePatterns(t *testing.T) {
	for _, tc := range []struct{ pattern, want string }{
		{"MM-dd HH:mm", "year"},
		{"yyyy-dd", "month"},
		{"yyyy-MM", "day"},
		{"yyyy-MM-dd hh:mm", "AM/PM"},
	} {
		d, err := dpCompile(tc.pattern)
		if err != nil {
			t.Fatalf("%q should COMPILE (it is formattable): %v", tc.pattern, err)
		}
		_, err = d.parse("whatever")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("parse with %q must be refused naming %q, got: %v",
				tc.pattern, tc.want, err)
		}
	}
}

// TestDatePatternParseRejectsBadInput — a wrong string must not become a
// plausible instant.
func TestDatePatternParseRejectsBadInput(t *testing.T) {
	d, err := dpCompile("yyyy-MM-dd")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"2026-13-01", // month 13
		"2026-02-30", // not a real calendar date
		"2026-01-01x",
		"26-01-01",
		"",
	} {
		if got, err := d.parse(s); err == nil {
			t.Errorf("parse(%q) should have failed, got %v", s, got)
		}
	}
	// The JVM cross-checks a parsed weekday against the parsed date.
	dw, err := dpCompile("EEE yyyy-MM-dd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dw.parse("Mon 2026-07-31"); err == nil {
		t.Error("2026-07-31 is a Friday; a weekday conflict must be an error")
	}
	if _, err := dw.parse("Fri 2026-07-31"); err != nil {
		t.Errorf("the matching weekday must parse: %v", err)
	}
}

// TestDatePatternMemoIsRaceFree — the memo table is the one piece of shared
// mutable state on this path, and bri serves concurrently. Run under -race.
func TestDatePatternMemoIsRaceFree(t *testing.T) {
	const n = 32
	done := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() {
			d, err := dpLookup("yyyy-MM-dd HH:mm:ss")
			if err != nil {
				done <- "err: " + err.Error()
				return
			}
			done <- d.format(dpRef)
		}()
	}
	want := "2026-07-31 09:04:05"
	for i := 0; i < n; i++ {
		if got := <-done; got != want {
			t.Fatalf("concurrent lookup produced %q, want %q", got, want)
		}
	}
	// A refused pattern must be memoised as a refusal, not recompiled and not
	// silently promoted to success.
	if _, err := dpLookup("yyyy-QQ"); err == nil {
		t.Fatal("first lookup of a bad pattern should fail")
	}
	if _, err := dpLookup("yyyy-QQ"); err == nil {
		t.Fatal("memoised lookup of a bad pattern should still fail")
	}
}

func BenchmarkDatePatternFormat(b *testing.B) {
	d, err := dpCompile("yyyy-MM-dd HH:mm:ss")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = d.format(dpRef)
	}
}

func BenchmarkDatePatternLookupAndFormat(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d, err := dpLookup("yyyy-MM-dd HH:mm:ss")
		if err != nil {
			b.Fatal(err)
		}
		_ = d.format(dpRef)
	}
}
