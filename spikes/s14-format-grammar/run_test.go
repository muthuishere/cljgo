package format14

import (
	"fmt"
	"testing"
)

type outcome struct {
	match bool
	note  string
}

// TestCorpusAgainstOracle is the spike's actual measurement: run every probe
// through real JVM Clojure 1.12.5 once (the oracle), then through both
// candidate Go implementations, and report an exact-match rate for each.
// This is what "corpus-verified compatibility report" in README.md's exit
// criterion means — not opinions, counted outcomes.
func TestCorpusAgainstOracle(t *testing.T) {
	oracle, err := RunOracle(Corpus)
	if err != nil {
		t.Fatalf("oracle run failed (is the `clojure` CLI on PATH?): %v", err)
	}

	directResults := map[string]outcome{}
	translateResults := map[string]outcome{}

	for _, p := range Corpus {
		want, ok := oracle[p.Name]
		if !ok {
			t.Fatalf("no oracle result for probe %q", p.Name)
		}
		gotS, gotErr := tryFormat(FormatDirect, p)
		directResults[p.Name] = compare(want, gotS, gotErr)

		gotS2, gotErr2 := tryFormat(FormatTranslate, p)
		translateResults[p.Name] = compare(want, gotS2, gotErr2)
	}

	dMatch, dTotal := tally(directResults)
	tMatch, tTotal := tally(translateResults)

	t.Logf("=== S14 corpus: %d probes ===", len(Corpus))
	t.Logf("direct interpreter      : %d/%d exact matches (%.1f%%)", dMatch, dTotal, 100*float64(dMatch)/float64(dTotal))
	t.Logf("translate-then-delegate : %d/%d exact matches (%.1f%%)", tMatch, tTotal, 100*float64(tMatch)/float64(tTotal))

	t.Logf("--- divergences: direct interpreter ---")
	for _, p := range Corpus {
		if o := directResults[p.Name]; !o.match {
			t.Logf("  %-35s %s", p.Name, o.note)
		}
	}
	t.Logf("--- divergences: translate-then-delegate ---")
	for _, p := range Corpus {
		if o := translateResults[p.Name]; !o.match {
			t.Logf("  %-35s %s", p.Name, o.note)
		}
	}
}

// tryFormat guards against a candidate panicking (rather than erroring) on
// a probe it doesn't handle correctly, so one bad probe can't kill the run.
func tryFormat(f func(string, []any) (string, error), p Probe) (s string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return f(p.Fmt, p.ArgsGo)
}

func compare(want OracleResult, gotS string, gotErr error) outcome {
	if want.Threw {
		if gotErr == nil {
			return outcome{false, fmt.Sprintf("oracle threw %s, candidate returned %q", want.ExClass, gotS)}
		}
		if fe, ok := gotErr.(*FormatError); ok {
			if fe.Class == want.ExClass {
				return outcome{true, ""}
			}
			return outcome{false, fmt.Sprintf("oracle threw %s, candidate threw %s", want.ExClass, fe.Class)}
		}
		// candidate errored but not with our typed FormatError (e.g. a Go
		// panic-turned-error) — count as "detected the failure" but flag the
		// class mismatch for visibility.
		return outcome{false, fmt.Sprintf("oracle threw %s, candidate errored untyped: %v", want.ExClass, gotErr)}
	}
	if gotErr != nil {
		return outcome{false, fmt.Sprintf("oracle succeeded with %q, candidate errored: %v", want.Output, gotErr)}
	}
	if gotS != want.Output {
		return outcome{false, fmt.Sprintf("oracle %q != candidate %q", want.Output, gotS)}
	}
	return outcome{true, ""}
}

func tally(m map[string]outcome) (match, total int) {
	for _, o := range m {
		total++
		if o.match {
			match++
		}
	}
	return
}
