package reader

// Phase 1 dispatch form tests: #'x, #(...), #"...", ##Inf/-Inf/NaN,
// #^. The #() shapes were verified against the real Clojure CLI
// (1.12.5, darwin/arm64); cited inline as "CLI check". Gensym ids are
// normalized (normalizeGensyms, from the S8 spike) before comparing.

import (
	"math"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

func readPrNorm(t *testing.T, src string) string {
	t.Helper()
	return normalizeGensyms(readPr(t, src))
}

func TestVarQuote(t *testing.T) {
	// CLI check: (read-string "#'x") => (var x); S8 golden: `#'x
	// expands identically to `(var x).
	if got := readPr(t, "#'x"); got != "(var x)" {
		t.Errorf("#'x => %s, want (var x)", got)
	}
	if got := readPr(t, "#'foo/bar"); got != "(var foo/bar)" {
		t.Errorf("#'foo/bar => %s, want (var foo/bar)", got)
	}
}

func TestFnLiteral(t *testing.T) {
	// All CLI checks (ids differ per run; normalized on both sides,
	// numbering by order of first appearance in the printed form):
	tests := []struct{ src, want string }{
		// #(+ % %2) => (fn* [p1__139# p2__140#] (+ p1__139# p2__140#))
		{"#(+ % %2)", "(fn* [p1__1# p2__2#] (+ p1__1# p2__2#))"},
		// #(apply + %&) => (fn* [& rest__143#] (apply + rest__143#))
		{"#(apply + %&)", "(fn* [& rest__1#] (apply + rest__1#))"},
		// #(do %3) => (fn* [p1__147# p2__148# p3__146#] (do p3__146#))
		// — gap args are minted at build time, so p3's raw id is the
		// LOWEST; normalization is by appearance order, like the JVM's.
		{"#(do %3)", "(fn* [p1__1# p2__2# p3__3#] (do p3__3#))"},
		// #() => (fn* [] ())
		{"#()", "(fn* [] ())"},
		// #(+ % %) => (fn* [p1__153#] (+ p1__153# p1__153#)) — same
		// arg registered once.
		{"#(+ % %)", "(fn* [p1__1#] (+ p1__1# p1__1#))"},
		// #(%) => (fn* [p1__193#] (p1__193#))
		{"#(%)", "(fn* [p1__1#] (p1__1#))"},
		// #(apply + %2 %&) => (fn* [p1__153# p2__151# & rest__152#]
		// (apply + p2__151# rest__152#))
		{"#(apply + %2 %&)", "(fn* [p1__1# p2__2# & rest__3#] (apply + p2__2# rest__3#))"},
	}
	for _, tt := range tests {
		if got := readPrNorm(t, tt.src); got != tt.want {
			t.Errorf("read %q =>\n got: %s\nwant: %s", tt.src, got, tt.want)
		}
	}
}

func TestFnLiteralErrors(t *testing.T) {
	// CLI check: #(#(inc %)) => "Nested #()s are not allowed".
	if err := mustErr(t, "#(#(inc %))"); !strings.Contains(err.Error(), "Nested #()s are not allowed") {
		t.Errorf("nested #(): error %q", err)
	}
	// CLI check: #(str %-1) => "arg literal must be %, %& or %integer".
	if err := mustErr(t, "#(str %-1)"); !strings.Contains(err.Error(), "arg literal must be %, %& or %integer") {
		t.Errorf("#(str %%-1): error %q", err)
	}
	// CLI check: "#(" => EOF while reading.
	if err := mustErr(t, "#("); !strings.Contains(err.Error(), "EOF while reading") {
		t.Errorf("#(: error %q", err)
	}
}

func TestPercentOutsideFnLiteral(t *testing.T) {
	// CLI checks: (read-string "%") => %; "%2" => %2; "%&" => %&
	// (plain symbols — % is a non-terminating macro char).
	for _, src := range []string{"%", "%2", "%&", "%foo"} {
		v := mustRead(t, src)
		sym, ok := v.(*lang.Symbol)
		if !ok || sym.FullName() != src {
			t.Errorf("read %q => %s, want symbol %s", src, lang.PrintString(v), src)
		}
	}
}

func TestRegexLiteral(t *testing.T) {
	// Raw pattern capture, no compilation at read time
	// (design/01-reader.md §4).
	v := mustRead(t, `#"a+b"`)
	re, ok := v.(*Regex)
	if !ok {
		t.Fatalf(`#"a+b" => %T, want *reader.Regex`, v)
	}
	if re.Pattern != "a+b" {
		t.Errorf("pattern %q, want a+b", re.Pattern)
	}
	// CLI check: (pr (read-string "#\"a\\\"b\"")) => #"a\"b" — the
	// backslash escape is kept verbatim in the pattern source.
	v = mustRead(t, `#"a\"b"`)
	if p := v.(*Regex).Pattern; p != `a\"b` {
		t.Errorf("escaped pattern %q, want a\\\"b", p)
	}
	if got := lang.PrintString(v); got != `#"a\"b"` {
		t.Errorf("regex prints %s, want #\"a\\\"b\"", got)
	}
	// A Java-only pattern (lookbehind) must still READ fine.
	if p := mustRead(t, `#"(?<=x)y"`).(*Regex).Pattern; p != "(?<=x)y" {
		t.Errorf("java-regex pattern %q", p)
	}
	// CLI check: "#\"ab" => "EOF while reading regex".
	if err := mustErr(t, `#"ab`); !strings.Contains(err.Error(), "EOF while reading regex") {
		t.Errorf("unterminated regex: error %q", err)
	}
}

func TestSymbolicValues(t *testing.T) {
	if v := mustRead(t, "##Inf"); !math.IsInf(v.(float64), 1) {
		t.Errorf("##Inf => %v", v)
	}
	if v := mustRead(t, "##-Inf"); !math.IsInf(v.(float64), -1) {
		t.Errorf("##-Inf => %v", v)
	}
	if v := mustRead(t, "##NaN"); !math.IsNaN(v.(float64)) {
		t.Errorf("##NaN => %v", v)
	}
	// CLI checks: ##Foo => "Unknown symbolic value: ##Foo"; "##" =>
	// EOF while reading.
	if err := mustErr(t, "##Foo"); !strings.Contains(err.Error(), "Unknown symbolic value: ##Foo") {
		t.Errorf("##Foo: error %q", err)
	}
	if err := mustErr(t, "##"); !strings.Contains(err.Error(), "EOF while reading") {
		t.Errorf("##: error %q", err)
	}
	// ## followed by a non-symbol.
	if err := mustErr(t, `##"x"`); !strings.Contains(err.Error(), "Invalid token: ##") {
		t.Errorf("##\"x\": error %q", err)
	}
}

func TestLegacyMetaDispatch(t *testing.T) {
	// CLI check: (meta (read-string "#^:private [a]")) => {:private true}.
	v := mustRead(t, "#^:private [a]")
	m := v.(lang.IObj).Meta()
	if m == nil || m.ValAt(lang.KWPrivate) != true {
		t.Errorf("#^:private [a] meta => %s", lang.PrintString(m))
	}
	if got := lang.PrintString(v); got != "[a]" {
		t.Errorf("#^:private [a] => %s, want [a]", got)
	}
}
