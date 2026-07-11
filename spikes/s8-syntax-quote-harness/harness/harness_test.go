package harness

import (
	"errors"
	"strings"
	"testing"
)

const sampleGolden = `;; comment header

IN: ` + "`x" + `
OK: (quote user/x)

IN: ` + "`(x# x#)" + `
OK: (clojure.core/seq (clojure.core/concat (clojure.core/list (quote x__154__auto__)) (clojure.core/list (quote x__154__auto__))))

IN: ` + "`~@x" + `
ERR: IllegalStateException
`

func TestParseGolden(t *testing.T) {
	cases, err := ParseGolden(strings.NewReader(sampleGolden))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 3 {
		t.Fatalf("want 3 cases, got %d", len(cases))
	}
	if cases[0].Input != "`x" || cases[0].Golden != "(quote user/x)" {
		t.Errorf("case 0 parsed wrong: %+v", cases[0])
	}
	// golden must be normalized at load time: __154__ -> __1__
	if !strings.Contains(cases[1].Golden, "x__1__auto__") || strings.Contains(cases[1].Golden, "154") {
		t.Errorf("case 1 golden not normalized: %s", cases[1].Golden)
	}
	if !cases[2].WantErr || cases[2].ErrKind != "IllegalStateException" {
		t.Errorf("case 2 err parsed wrong: %+v", cases[2])
	}
}

func TestParseGoldenTruncated(t *testing.T) {
	_, err := ParseGolden(strings.NewReader("IN: `x\n"))
	if err == nil {
		t.Fatal("want error for IN: without OK:/ERR:")
	}
}

func TestRunPassAndFail(t *testing.T) {
	cases, err := ParseGolden(strings.NewReader(sampleGolden))
	if err != nil {
		t.Fatal(err)
	}

	// Fake candidate reader that matches the golden semantics but mints its
	// own (different) gensym numbers — normalization must reconcile them.
	perfect := func(src string) (string, error) {
		switch src {
		case "`x":
			return "(quote user/x)", nil
		case "`(x# x#)":
			return "(clojure.core/seq (clojure.core/concat (clojure.core/list (quote x__9001__auto__)) (clojure.core/list (quote x__9001__auto__))))", nil
		case "`~@x":
			return "", errors.New("splice not in list")
		}
		return "", errors.New("unexpected input")
	}
	rep := Run(cases, perfect)
	if rep.Failed != 0 {
		var b strings.Builder
		rep.Print(&b, true)
		t.Fatalf("perfect reader should pass all:\n%s", b.String())
	}

	// A reader that breaks hygiene (two DIFFERENT gensyms for the same x#)
	// must fail case 1 even after normalization.
	unhygienic := func(src string) (string, error) {
		if src == "`(x# x#)" {
			return "(clojure.core/seq (clojure.core/concat (clojure.core/list (quote x__1__auto__)) (clojure.core/list (quote x__2__auto__))))", nil
		}
		return perfect(src)
	}
	rep = Run(cases, unhygienic)
	if rep.Failed != 1 || rep.Results[1].Pass {
		t.Fatalf("unhygienic reader must fail exactly the x# case, got %+v", rep)
	}

	// The stub reader fails everything except error-expecting cases pass
	// only if it errors — it doesn't, so all 3 fail.
	stub := func(string) (string, error) { return "NOT IMPLEMENTED", nil }
	rep = Run(cases, stub)
	if rep.Failed != 3 {
		t.Fatalf("stub should fail all 3, failed %d", rep.Failed)
	}
}
