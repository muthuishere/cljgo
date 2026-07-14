package reader

// Reader Phase 0 tests (design/01-reader.md §1, §5 v0).
//
// Questionable behaviors were verified against the real Clojure CLI
// (clojure 1.12.5, darwin/arm64) with
//   clojure -M -e '(pr (read-string "..."))'
// Each such check is cited inline as "CLI check: ...".

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// testResolver is a stub compiler resolver: current ns "user", one
// alias str => clojure.string.
type testResolver struct{}

func (testResolver) CurrentNS() *lang.Symbol { return lang.NewSymbol("user") }
func (testResolver) ResolveAlias(s *lang.Symbol) *lang.Symbol {
	if s.Name() == "str" {
		return lang.NewSymbol("clojure.string")
	}
	return nil
}
func (testResolver) ResolveVar(s *lang.Symbol) *lang.Symbol  { return nil }
func (testResolver) ResolveType(s *lang.Symbol) *lang.Symbol { return nil }

func newTestReader(src string, opts ...Option) *Reader {
	all := append([]Option{WithFilename("test.clj"), WithResolver(testResolver{})}, opts...)
	return New(strings.NewReader(src), all...)
}

func readOne(t *testing.T, src string) (any, error) {
	t.Helper()
	return newTestReader(src).ReadOne()
}

func mustRead(t *testing.T, src string) any {
	t.Helper()
	v, err := readOne(t, src)
	if err != nil {
		t.Fatalf("read %q: unexpected error: %v", src, err)
	}
	return v
}

func mustErr(t *testing.T, src string) error {
	t.Helper()
	v, err := readOne(t, src)
	if err == nil {
		t.Fatalf("read %q: expected error, got %v", src, lang.PrintString(v))
	}
	if errors.Is(err, ErrEOF) {
		t.Fatalf("read %q: malformed input must not surface as clean ErrEOF", src)
	}
	var re *Error
	if !errors.As(err, &re) {
		t.Fatalf("read %q: error is not a positioned *reader.Error: %v", src, err)
	}
	return err
}

// eqForm compares type identity plus structural equality (Equiv
// ignores metadata, as Clojure equality does).
func eqForm(a, b any) bool {
	if fmt.Sprintf("%T", a) != fmt.Sprintf("%T", b) {
		return false
	}
	return lang.Equiv(a, b)
}

func bigDec(t *testing.T, s string) *lang.BigDecimal {
	t.Helper()
	bd, err := lang.NewBigDecimal(s)
	if err != nil {
		t.Fatalf("bad test BigDecimal %q: %v", s, err)
	}
	return bd
}

// ---------------------------------------------------------------------------
// Literals

func TestNilBooleans(t *testing.T) {
	if v := mustRead(t, "nil"); v != nil {
		t.Errorf("nil => %v", v)
	}
	if v := mustRead(t, "true"); v != true {
		t.Errorf("true => %v", v)
	}
	if v := mustRead(t, "false"); v != false {
		t.Errorf("false => %v", v)
	}
	// nil form is distinguishable from clean EOF.
	if _, err := readOne(t, "   "); !errors.Is(err, ErrEOF) {
		t.Errorf("whitespace-only input: want ErrEOF, got %v", err)
	}
}

func TestIntegers(t *testing.T) {
	tests := []struct {
		src  string
		want any
	}{
		{"0", int64(0)},
		{"-0", int64(0)},
		{"+0", int64(0)},
		{"42", int64(42)},
		{"-17", int64(-17)},
		{"+5", int64(5)}, // CLI check: (read-string "+5") => 5
		{"0xff", int64(255)},
		{"0XFF", int64(255)},
		{"017", int64(15)},
		{"06", int64(6)}, // CLI check: (read-string "06") => 6 (octal)
		{"2r1010", int64(10)},
		{"-2r1010", int64(-10)}, // CLI check: (read-string "-2r1010") => -10
		{"36rZZ", int64(1295)},  // CLI check: (read-string "36rZZ") => 1295
		{"36rzz", int64(1295)},
		{"12N", lang.NewBigIntFromInt64(12)},
		{"0N", lang.NewBigIntFromInt64(0)},
		{"0xFFN", lang.NewBigIntFromInt64(255)}, // CLI check: 255N
		// int64 boundaries: -2^63 stays a fixed int, like Long/MIN_VALUE.
		{"9223372036854775807", int64(9223372036854775807)},
		{"-9223372036854775808", int64(-9223372036854775808)},
		// Beyond int64 without N => BigInt.
		// CLI check: (read-string "99999999999999999999999") => ...N (BigInt).
		{"99999999999999999999999", mustBigInt("99999999999999999999999")},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if !eqForm(got, tt.want) {
			t.Errorf("read %q => %T %v, want %T %v", tt.src, got, got, tt.want, tt.want)
		}
	}
}

func mustBigInt(s string) *lang.BigInt {
	bi, err := lang.NewBigInt(s)
	if err != nil {
		panic(err)
	}
	return bi
}

func TestFloats(t *testing.T) {
	tests := []struct {
		src  string
		want any
	}{
		{"2.5", 2.5},
		{"-2.5", -2.5},
		{"1e3", 1000.0}, // CLI check: (read-string "1e3") => 1000.0
		{"1.", 1.0},     // CLI check: (read-string "1.") => 1.0
		{"1.5e-2", 0.015},
		{"+1.5", 1.5},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if !eqForm(got, tt.want) {
			t.Errorf("read %q => %T %v, want %v", tt.src, got, got, tt.want)
		}
	}
}

func TestBigDecimals(t *testing.T) {
	// CLI checks: (read-string "12M") => 12M; (read-string "1.0M") => 1.0M.
	for _, tt := range []struct {
		src, dec string
	}{
		{"12M", "12"},
		{"1.0M", "1.0"},
		{"-1.5e3M", "-1.5e3"},
	} {
		got := mustRead(t, tt.src)
		if !eqForm(got, bigDec(t, tt.dec)) {
			t.Errorf("read %q => %T %v, want BigDecimal %s", tt.src, got, got, tt.dec)
		}
	}
}

func TestRatios(t *testing.T) {
	tests := []struct {
		src  string
		want any
	}{
		{"3/4", lang.NewRatio(3, 4)},
		{"-1/2", lang.NewRatio(-1, 2)},
		{"+1/2", lang.NewRatio(1, 2)},
		// Ratios reduce; whole values collapse to integers.
		// CLI checks: (read-string "6/8") => 3/4; (read-string "4/2") => 2.
		{"6/8", lang.NewRatio(3, 4)},
		{"4/2", int64(2)},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if !eqForm(got, tt.want) {
			t.Errorf("read %q => %T %v, want %T %v", tt.src, got, got, tt.want, tt.want)
		}
	}
}

func TestInvalidNumbers(t *testing.T) {
	tests := []struct {
		src, wantSub string
	}{
		{"08", "Invalid number: 08"}, // CLI check: invalid (not octal)
		{"1a", "Invalid number: 1a"},
		{"0x", "Invalid number: 0x"},
		{"2r102", "Invalid number"}, // CLI check: radix error in Clojure
		{"1/0", "Divide by zero"},   // CLI check: "Divide by zero"
		{"1.2.3", "Invalid number"},
	}
	for _, tt := range tests {
		err := mustErr(t, tt.src)
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("read %q: error %q does not contain %q", tt.src, err, tt.wantSub)
		}
	}
}

// ---------------------------------------------------------------------------
// Strings — Clojure escape semantics, deliberately NOT strconv.Unquote
// (the Glojure bug called out in design/01-reader.md §4).

func TestStrings(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`"hello"`, "hello"},
		{`""`, ""},
		{`"a\tb"`, "a\tb"},
		{`"a\rb"`, "a\rb"},
		{`"a\nb"`, "a\nb"},
		{`"a\\b"`, `a\b`},
		{`"a\"b"`, `a"b`},
		{`"a\bb"`, "a\bb"},
		{`"a\fb"`, "a\fb"},
		// CLI check: (read-string "\"\\u0041\"") => "A".
		{`"\u0041"`, "A"},
		{`"\u00411"`, "A1"}, // exactly 4 hex digits; 5th is content
		// Octal escapes are bare digits (no 'o' prefix in strings).
		// CLI check: (read-string "\"\\101\"") => "A".
		{`"\101"`, "A"},
		{`"\377"`, "ÿ"},
		{`"\0"`, "\x00"},
		{`"\1018"`, "A8"},                  // max 3 octal digits, then content
		{"\"multi\nline\"", "multi\nline"}, // raw newline inside string
		{`"esc\tA\377"`, "esc\tAÿ"},        // doc §5 golden form content
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if got != tt.want {
			t.Errorf("read %s => %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestStringEscapeErrors(t *testing.T) {
	tests := []struct {
		src, wantSub string
	}{
		// Go's strconv.Unquote accepts \a \v \x and \'; Clojure does NOT.
		{`"\a"`, `Unsupported escape character: \a`},
		{`"\v"`, `Unsupported escape character: \v`},
		{`"\x41"`, `Unsupported escape character: \x`},
		{`"\'"`, `Unsupported escape character: \'`},
		// 'o' is char-literal syntax only.
		// CLI check: (read-string "\"\\o101\"") => Unsupported escape \o.
		{`"\o101"`, `Unsupported escape character: \o`},
		// CLI check: (read-string "\"\\8\"") => "Invalid digit: 8".
		{`"\8"`, "Invalid digit: 8"},
		// CLI check: (read-string "\"\\400\"") => octal range error.
		{`"\400"`, "Octal escape sequence must be in range [0, 377]"},
		{`"\u00g1"`, "Invalid digit: g"},
		{`"\u12"`, "Invalid character length: 2, should be: 4"},
		{`"\uzz"`, `Invalid unicode escape: \uz`},
	}
	for _, tt := range tests {
		err := mustErr(t, tt.src)
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("read %s: error %q does not contain %q", tt.src, err, tt.wantSub)
		}
	}
}

func TestUnterminatedString(t *testing.T) {
	err := mustErr(t, "  \"abc")
	msg := err.Error()
	if !strings.Contains(msg, "EOF while reading string") {
		t.Errorf("unterminated string: %q lacks what was open", msg)
	}
	if !strings.Contains(msg, "starting at line 1 column 3") {
		t.Errorf("unterminated string: %q lacks start position", msg)
	}
}

// ---------------------------------------------------------------------------
// Characters

func TestChars(t *testing.T) {
	tests := []struct {
		src  string
		want rune
	}{
		{`\a`, 'a'},
		{`\A`, 'A'},
		{`\8`, '8'},
		{`\λ`, 'λ'},
		{`\(`, '('},
		{`\\`, '\\'},
		{`\newline`, '\n'},
		{`\space`, ' '},
		{`\tab`, '\t'},
		{`\backspace`, '\b'},
		{`\formfeed`, '\f'},
		{`\return`, '\r'},
		{`\u`, 'u'}, // CLI check: (read-string "\\u") => \u
		{`\o`, 'o'},
		{`\o101`, 'A'}, // CLI check: (read-string "\\o101") => \A
		{`\o7`, 7},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		ch, ok := got.(lang.Char)
		if !ok || rune(ch) != tt.want {
			t.Errorf("read %q => %T %v, want Char %q", tt.src, got, got, tt.want)
		}
	}
}

func TestCharErrors(t *testing.T) {
	tests := []struct {
		src, wantSub string
	}{
		{`\u123`, `Invalid unicode character: \u123`},
		{`\u123z`, "Invalid digit: z"},
		// CLI check: (read-string "\\uD800") => Invalid character constant: \ud800.
		{`\uD800`, `Invalid character constant: \ud800`},
		// CLI check: (read-string "\\o400") => octal range error.
		{`\o400`, "Octal escape sequence must be in range [0, 377]"},
		{`\o1234`, "Invalid octal escape sequence length: 4"},
		{`\o18`, "Invalid digit: 8"},
		{`\bogus`, `Unsupported character: \bogus`},
	}
	for _, tt := range tests {
		err := mustErr(t, tt.src)
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("read %q: error %q does not contain %q", tt.src, err, tt.wantSub)
		}
	}
}

// ---------------------------------------------------------------------------
// Keywords

func TestPlainKeywordNotNamespaceQualified(t *testing.T) {
	// The Glojure bug fix called out in design/01-reader.md §4: Glojure
	// reads :foo as :<current-ns>/foo. Clojure does not.
	// CLI check: (read-string ":foo") => :foo, (namespace :foo) => nil.
	got := mustRead(t, ":foo")
	kw, ok := got.(lang.Keyword)
	if !ok {
		t.Fatalf(":foo => %T, want Keyword", got)
	}
	if kw.Namespace() != nil {
		t.Errorf(":foo namespace => %v, want nil (must NOT be current-ns qualified)", kw.Namespace())
	}
	if kw.Name() != "foo" {
		t.Errorf(":foo name => %q", kw.Name())
	}
}

func TestKeywords(t *testing.T) {
	tests := []struct {
		src, wantStr string
	}{
		{":foo", ":foo"},
		{":foo/bar", ":foo/bar"},
		{":foo.bar/baz", ":foo.bar/baz"},
		// CLI check: (read-string ":foo:bar") => :foo:bar (valid; interior
		// single colons are fine — only :: interior is rejected).
		{":foo:bar", ":foo:bar"},
		// CLI check: (read-string ":3") => :3 (Clojure's reader accepts
		// digit-named keywords even though the spec discourages them).
		{":3", ":3"},
		// CLI check: (read-string ":/") => :/.
		{":/", ":/"},
		{":+", ":+"},
		// Auto-resolved keywords via the injected Resolver.
		// CLI check: (read-string "::foo") => :user/foo.
		{"::foo", ":user/foo"},
		{"::str/trim", ":clojure.string/trim"},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		kw, ok := got.(lang.Keyword)
		if !ok {
			t.Fatalf("read %q => %T, want Keyword", tt.src, got)
		}
		if kw.String() != tt.wantStr {
			t.Errorf("read %q => %s, want %s", tt.src, kw.String(), tt.wantStr)
		}
	}
}

func TestKeywordErrors(t *testing.T) {
	tests := []struct {
		src, wantSub string
	}{
		{":", "Invalid token: :"},   // CLI check: Invalid token
		{"::", "Invalid token: ::"}, // CLI check: Invalid token
		{":foo:", "Invalid token"},
		{"::nope/x", "Invalid token: ::nope/x"}, // unknown alias
		{"::1a/x", "Invalid token"},             // invalid alias symbol
	}
	for _, tt := range tests {
		err := mustErr(t, tt.src)
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("read %q: error %q does not contain %q", tt.src, err, tt.wantSub)
		}
	}
}

func TestAutoResolvedKeywordRequiresResolver(t *testing.T) {
	_, err := New(strings.NewReader("::foo")).ReadOne()
	if err == nil || !strings.Contains(err.Error(), "resolver") {
		t.Errorf("::foo without resolver: want resolver error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Symbols

func TestSymbols(t *testing.T) {
	tests := []struct {
		src      string
		wantNS   string
		wantName string
	}{
		{"foo", "", "foo"},
		{"foo/bar", "foo", "bar"},
		{"clojure.core/+", "clojure.core", "+"},
		{"/", "", "/"},
		{"clojure.core//", "clojure.core", "/"}, // CLI check: valid
		// CLI check: (let [s (read-string "a/b/c")] [(namespace s) (name s)])
		// => ["a" "b/c"] — Symbol.intern splits on the FIRST slash.
		{"a/b/c", "a", "b/c"},
		{"+", "", "+"},
		{"-", "", "-"},
		{"...", "", "..."},
		{"+foo", "", "+foo"},
		{"-foo", "", "-foo"},
		{"x#", "", "x#"},   // gensym suffix: # is non-terminating
		{"a'b", "", "a'b"}, // ' is non-terminating inside a token
		{"%", "", "%"},     // CLI check: (read-string "%") => % (plain symbol outside #())
		{"*ns*", "", "*ns*"},
		{"with-meta", "", "with-meta"},
		{"nil?", "", "nil?"},
		{"→", "", "→"},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		sym, ok := got.(*lang.Symbol)
		if !ok {
			t.Fatalf("read %q => %T, want *Symbol", tt.src, got)
		}
		if sym.Namespace() != tt.wantNS || sym.Name() != tt.wantName {
			t.Errorf("read %q => ns %q name %q, want ns %q name %q",
				tt.src, sym.Namespace(), sym.Name(), tt.wantNS, tt.wantName)
		}
	}
}

func TestSymbolErrors(t *testing.T) {
	// CLI checks: foo: and foo/ both => "Invalid token".
	for _, src := range []string{"foo:", "foo/", "a::b", "ns:/x"} {
		err := mustErr(t, src)
		if !strings.Contains(err.Error(), "Invalid token") {
			t.Errorf("read %q: error %q, want Invalid token", src, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Collections

func TestCollections(t *testing.T) {
	tests := []struct {
		src  string
		want any
	}{
		{"()", lang.NewList()},
		{"(1 2 3)", lang.NewList(int64(1), int64(2), int64(3))},
		{"(a (b c))", lang.NewList(lang.NewSymbol("a"),
			lang.NewList(lang.NewSymbol("b"), lang.NewSymbol("c")))},
		{"[]", lang.NewVector()},
		{"[1 2]", lang.NewVector(int64(1), int64(2))},
		{"[1 [2 [3]]]", lang.NewVector(int64(1),
			lang.NewVector(int64(2), lang.NewVector(int64(3))))},
		{"{}", lang.NewMap()},
		{"{:a 1}", lang.NewMap(lang.NewKeyword("a"), int64(1))},
		{"{:a {:b 2}}", lang.NewMap(lang.NewKeyword("a"),
			lang.NewMap(lang.NewKeyword("b"), int64(2)))},
		{"[1, 2,3]", lang.NewVector(int64(1), int64(2), int64(3))}, // commas are whitespace
		{"(defn add [a b] (+ a b))", lang.NewList( // doc §5 golden form
			lang.NewSymbol("defn"), lang.NewSymbol("add"),
			lang.NewVector(lang.NewSymbol("a"), lang.NewSymbol("b")),
			lang.NewList(lang.NewSymbol("+"), lang.NewSymbol("a"), lang.NewSymbol("b")))},
		{`{:name "muthu" :tags #{:go} :n 42}`, lang.NewMap( // doc §5 golden form
			lang.NewKeyword("name"), "muthu",
			lang.NewKeyword("tags"), lang.NewSet(lang.NewKeyword("go")),
			lang.NewKeyword("n"), int64(42))},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if !lang.Equiv(got, tt.want) {
			t.Errorf("read %q => %s, want %s", tt.src, lang.PrintString(got), lang.PrintString(tt.want))
		}
	}
}

func TestSet(t *testing.T) {
	got := mustRead(t, "#{:go :clj 42}")
	set, ok := got.(*lang.Set)
	if !ok {
		t.Fatalf("#{...} => %T, want *Set", got)
	}
	if set.Count() != 3 {
		t.Fatalf("set count => %d, want 3", set.Count())
	}
	for _, k := range []any{lang.NewKeyword("go"), lang.NewKeyword("clj"), int64(42)} {
		if !set.Contains(k) {
			t.Errorf("set missing %v", k)
		}
	}
}

func TestCollectionErrors(t *testing.T) {
	tests := []struct {
		src, wantSub string
	}{
		{")", "Unmatched delimiter: )"},
		{"]", "Unmatched delimiter: ]"},
		{"}", "Unmatched delimiter: }"},
		{"[1 2)", "Unmatched delimiter: )"},
		{"{:a 1 :b}", "Map literal must contain an even number of forms"},
		// CLI checks: {:a 1 :a 2} => Duplicate key: :a; #{1 1} => Duplicate key: 1.
		{"{:a 1 :a 2}", "Duplicate key: :a"},
		{"#{1 1}", "Duplicate key: 1"},
		// CLI check: (read-string "(1 #_)") => Unmatched delimiter: ).
		{"(1 #_)", "Unmatched delimiter: )"},
	}
	for _, tt := range tests {
		err := mustErr(t, tt.src)
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("read %q: error %q does not contain %q", tt.src, err, tt.wantSub)
		}
	}
}

func TestUnterminatedCollections(t *testing.T) {
	tests := []struct {
		src, what, startAt string
	}{
		{"(1 2", "list", "line 1 column 1"},
		{"  [1 2", "vector", "line 1 column 3"},
		{"{:a 1", "map", "line 1 column 1"},
		{"#{1", "set", "line 1 column 1"},
		{"(1 ; trailing comment", "list", "line 1 column 1"},
		{"(\n  [1 2", "vector", "line 2 column 3"},
	}
	for _, tt := range tests {
		err := mustErr(t, tt.src)
		msg := err.Error()
		if !strings.Contains(msg, "EOF while reading "+tt.what) {
			t.Errorf("read %q: error %q does not say a %s was open", tt.src, msg, tt.what)
		}
		if !strings.Contains(msg, "starting at "+tt.startAt) {
			t.Errorf("read %q: error %q does not say it started at %s", tt.src, msg, tt.startAt)
		}
	}
}

// ---------------------------------------------------------------------------
// Quote / deref reader macros

func TestQuoteDeref(t *testing.T) {
	tests := []struct {
		src, want string
	}{
		{"'a", "(quote a)"}, // CLI check: (read-string "'a") => (quote a)
		{"'(a b)", "(quote (a b))"},
		{"''a", "(quote (quote a))"},
		{"'[1 2]", "(quote [1 2])"},
		// CLI check: (read-string "@foo") => (clojure.core/deref foo).
		{"@foo", "(clojure.core/deref foo)"},
		{"@(atom 1)", "(clojure.core/deref (atom 1))"},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if s := lang.PrintString(got); s != tt.want {
			t.Errorf("read %q => %s, want %s", tt.src, s, tt.want)
		}
	}
	for _, src := range []string{"'", "@"} {
		err := mustErr(t, src)
		if !strings.Contains(err.Error(), "EOF while reading") {
			t.Errorf("read %q: error %q, want EOF while reading", src, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Comments, whitespace, #_ discard

func TestComments(t *testing.T) {
	tests := []struct {
		src  string
		want any
	}{
		{"; a comment\n42", int64(42)},
		{"#! shebang style\n42", int64(42)}, // CLI check: #! is a line comment
		{"42 ; trailing", int64(42)},
		{"; c1\n; c2\n:done", lang.NewKeyword("done")},
	}
	for _, tt := range tests {
		if got := mustRead(t, tt.src); !eqForm(got, tt.want) {
			t.Errorf("read %q => %v, want %v", tt.src, got, tt.want)
		}
	}
	// Comment-only input (even without a trailing newline) is clean EOF.
	if _, err := readOne(t, ";; only a comment"); !errors.Is(err, ErrEOF) {
		t.Errorf("comment-only input: want ErrEOF, got %v", err)
	}
}

func TestDiscard(t *testing.T) {
	tests := []struct {
		src, want string
	}{
		{"#_ 1 2", "2"},
		{"#_1 2", "2"},
		{"#_#_ 1 2 3", "3"}, // CLI check: (read-string "#_#_ 1 2 3") => 3 (stacked)
		{"#_(ignored (deeply)) :kept", ":kept"},
		{"(1 #_2 3)", "(1 3)"},
		{"[1 #_[2 3] 4]", "[1 4]"},
		{"{:a #_:skip 1}", "{:a 1}"},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if s := lang.PrintString(got); s != tt.want {
			t.Errorf("read %q => %s, want %s", tt.src, s, tt.want)
		}
	}
	// Discarded form followed by nothing is clean EOF...
	if _, err := readOne(t, "#_ 42"); !errors.Is(err, ErrEOF) {
		t.Errorf("#_ 42: want ErrEOF, got %v", err)
	}
	// ...but a dangling #_ is a malformed-input error, not ErrEOF.
	err := mustErr(t, "#_")
	if !strings.Contains(err.Error(), "EOF while reading") {
		t.Errorf("#_: error %q, want EOF while reading", err)
	}
}

// ---------------------------------------------------------------------------
// Metadata (^)

func metaOf(t *testing.T, form any) lang.IPersistentMap {
	t.Helper()
	im, ok := form.(lang.IMeta)
	if !ok {
		t.Fatalf("form %T does not carry meta", form)
	}
	return im.Meta()
}

func TestMetaKeywordShorthand(t *testing.T) {
	form := mustRead(t, "^:private x")
	m := metaOf(t, form)
	if lang.Get(m, lang.KWPrivate) != true {
		t.Errorf("^:private x meta => %s, want :private true", lang.PrintString(m))
	}
	if sym := form.(*lang.Symbol); sym.Name() != "x" {
		t.Errorf("target => %v, want x", sym)
	}
}

func TestMetaSymbolAndStringShorthand(t *testing.T) {
	// ^Sym => {:tag Sym}; ^"str" => {:tag "str"}.
	// CLI check: (meta (read-string "^\"[B\" x")) => {:tag "[B"}.
	m := metaOf(t, mustRead(t, "^String s"))
	if tag := lang.Get(m, lang.KWTag); !eqForm(tag, lang.NewSymbol("String")) {
		t.Errorf("^String s: :tag => %v, want symbol String", tag)
	}
	m = metaOf(t, mustRead(t, `^"[B" s`))
	if tag := lang.Get(m, lang.KWTag); tag != "[B" {
		t.Errorf(`^"[B" s: :tag => %v, want "[B"`, tag)
	}
}

func TestMetaVectorShorthand(t *testing.T) {
	// CLI check (Clojure 1.12.5): (meta (read-string "^[String java.lang.Long] x"))
	// => {:param-tags [String java.lang.Long]}.
	m := metaOf(t, mustRead(t, "^[String java.lang.Long] x"))
	want := lang.NewVector(lang.NewSymbol("String"), lang.NewSymbol("java.lang.Long"))
	if pt := lang.Get(m, lang.NewKeyword("param-tags")); !lang.Equiv(pt, want) {
		t.Errorf("^[...] x: :param-tags => %v, want %s", pt, lang.PrintString(want))
	}
}

func TestMetaMapAndMerging(t *testing.T) {
	m := metaOf(t, mustRead(t, "^{:a 1 :b 2} x"))
	if lang.Get(m, lang.NewKeyword("a")) != int64(1) || lang.Get(m, lang.NewKeyword("b")) != int64(2) {
		t.Errorf("^{:a 1 :b 2} x meta => %s", lang.PrintString(m))
	}

	// Stacked metadata: entries of the OUTER ^ win.
	// CLI check: (meta (read-string "^{:a 1} ^{:a 2} x")) => {:a 1}.
	m = metaOf(t, mustRead(t, "^{:a 1} ^{:a 2} x"))
	if got := lang.Get(m, lang.NewKeyword("a")); got != int64(1) {
		t.Errorf("stacked meta: :a => %v, want 1 (outer wins)", got)
	}

	// Mixed shorthands accumulate.
	// CLI check: (meta (read-string "^:private ^String x")) =>
	// {:tag String, :private true}.
	m = metaOf(t, mustRead(t, "^:private ^String x"))
	if lang.Get(m, lang.KWPrivate) != true {
		t.Errorf("^:private ^String x: missing :private true: %s", lang.PrintString(m))
	}
	if tag := lang.Get(m, lang.KWTag); !eqForm(tag, lang.NewSymbol("String")) {
		t.Errorf("^:private ^String x: :tag => %v", tag)
	}
}

func TestMetaOnCollection(t *testing.T) {
	form := mustRead(t, "^:private (f)")
	m := metaOf(t, form)
	if lang.Get(m, lang.KWPrivate) != true {
		t.Errorf("^:private (f): meta %s", lang.PrintString(m))
	}
	// Position metadata is anchored at the ^ (like Clojure, which stamps
	// the line of the ^ into the list's meta).
	if lang.Get(m, lang.KWColumn) != int64(1) {
		t.Errorf("^:private (f): :column => %v, want 1", lang.Get(m, lang.KWColumn))
	}
}

func TestMetaErrors(t *testing.T) {
	// CLI check: (read-string "^:kw 5") => "Metadata can only be applied to IMetas".
	err := mustErr(t, "^:kw 5")
	if !strings.Contains(err.Error(), "Metadata can only be applied to IMetas") {
		t.Errorf("^:kw 5: error %q", err)
	}
	err = mustErr(t, "^5 x")
	if !strings.Contains(err.Error(), "Metadata must be Symbol,Keyword,String,Map or Vector") {
		t.Errorf("^5 x: error %q", err)
	}
	err = mustErr(t, "^:private")
	if !strings.Contains(err.Error(), "EOF while reading") {
		t.Errorf("^:private (no target): error %q", err)
	}
}

// ---------------------------------------------------------------------------
// Position metadata (design/00-architecture.md §4.5)

func posMeta(t *testing.T, form any) (file any, line, col, endLine, endCol any) {
	t.Helper()
	m := metaOf(t, form)
	return lang.Get(m, lang.KWFile), lang.Get(m, lang.KWLine), lang.Get(m, lang.KWColumn),
		lang.Get(m, lang.KWEndLine), lang.Get(m, lang.KWEndColumn)
}

func TestPositions(t *testing.T) {
	form := mustRead(t, "(a b)")
	file, line, col, endLine, endCol := posMeta(t, form)
	if file != "test.clj" || line != int64(1) || col != int64(1) || endLine != int64(1) || endCol != int64(6) {
		t.Errorf("(a b) pos => %v %v:%v-%v:%v, want test.clj 1:1-1:6", file, line, col, endLine, endCol)
	}

	// Inner symbols carry their own positions (end is exclusive).
	seq := lang.Seq(form)
	_, _, colA, _, endColA := posMeta(t, seq.First())
	if colA != int64(2) || endColA != int64(3) {
		t.Errorf("symbol a pos => col %v end-col %v, want 2/3", colA, endColA)
	}
	_, _, colB, _, endColB := posMeta(t, seq.Next().First())
	if colB != int64(4) || endColB != int64(5) {
		t.Errorf("symbol b pos => col %v end-col %v, want 4/5", colB, endColB)
	}
}

func TestPositionsMultiline(t *testing.T) {
	form := mustRead(t, "[1\n  foo]")
	_, line, col, endLine, endCol := posMeta(t, form)
	if line != int64(1) || col != int64(1) || endLine != int64(2) || endCol != int64(7) {
		t.Errorf("vector pos => %v:%v-%v:%v, want 1:1-2:7", line, col, endLine, endCol)
	}
	sym := lang.MustNth(form, 1)
	_, line, col, endLine, endCol = posMeta(t, sym)
	if line != int64(2) || col != int64(3) || endLine != int64(2) || endCol != int64(6) {
		t.Errorf("foo pos => %v:%v-%v:%v, want 2:3-2:6", line, col, endLine, endCol)
	}
}

func TestPositionsAfterCommentAndDiscard(t *testing.T) {
	form := mustRead(t, "; c\n#_skip x")
	_, line, col, _, _ := posMeta(t, form)
	if line != int64(2) || col != int64(8) {
		t.Errorf("x pos => %v:%v, want 2:8", line, col)
	}
}

func TestErrorPositions(t *testing.T) {
	err := mustErr(t, "{:a 1\n  :a 2}")
	var re *Error
	errors.As(err, &re)
	if re.Pos.File != "test.clj" || re.Pos.Line != 1 || re.Pos.Col != 1 {
		t.Errorf("duplicate-key error pos => %v, want test.clj:1:1", re.Pos)
	}

	err = mustErr(t, "\n\n   )")
	errors.As(err, &re)
	if re.Pos.Line != 3 || re.Pos.Col != 4 {
		t.Errorf("unmatched-delimiter pos => %v, want 3:4", re.Pos)
	}

	// Unterminated collection: Start carries where it opened.
	err = mustErr(t, "(1 2")
	errors.As(err, &re)
	if re.Start == nil || re.Start.Line != 1 || re.Start.Col != 1 {
		t.Errorf("unterminated list: Start => %v, want 1:1", re.Start)
	}
	if re.Pos.Col != 5 {
		t.Errorf("unterminated list: Pos => %v, want col 5 (EOF)", re.Pos)
	}
}

// ---------------------------------------------------------------------------
// Phase 2+ syntax is rejected with clear errors (not silently misread)
// (Phase 1 forms — ` ~ #' #() #"" ## #^ — are implemented and tested
// in sqconformance_test.go / syntaxquote_test.go / dispatch_test.go.)

func TestUnimplementedReaderMacros(t *testing.T) {
	tests := []struct {
		src, wantSub string
	}{
		{"#?(:clj 1)", "not yet implemented"},
		{"#::{:a 1}", "not yet implemented"},
		{"#=(+ 1 2)", "not yet implemented"},
		{"#<unreadable>", "Unreadable form"},
		// #tag form is a tagged literal (data reader); an unknown tag is
		// rejected after the tag+form are read (ADR 0014 added #cljgo/...).
		{"#zzz 1", "No reader function for tag"},
	}
	for _, tt := range tests {
		err := mustErr(t, tt.src)
		if !strings.Contains(err.Error(), tt.wantSub) {
			t.Errorf("read %q: error %q does not contain %q", tt.src, err, tt.wantSub)
		}
	}
}

// ---------------------------------------------------------------------------
// ReadAll / ErrEOF contract

func TestReadAll(t *testing.T) {
	forms, err := newTestReader("1 :two [3] ; done").ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(forms) != 3 {
		t.Fatalf("ReadAll => %d forms, want 3", len(forms))
	}
	if forms[0] != int64(1) {
		t.Errorf("forms[0] => %v", forms[0])
	}
	if kw := forms[1].(lang.Keyword); kw.Name() != "two" {
		t.Errorf("forms[1] => %v", forms[1])
	}
	if s := lang.PrintString(forms[2]); s != "[3]" {
		t.Errorf("forms[2] => %s", s)
	}

	for _, src := range []string{"", "   \n\t,,, ", "; just a comment", "#_ discarded"} {
		forms, err := newTestReader(src).ReadAll()
		if err != nil || len(forms) != 0 {
			t.Errorf("ReadAll(%q) => %v forms, err %v; want 0 forms, nil", src, len(forms), err)
		}
	}

	if _, err := newTestReader("(1").ReadAll(); err == nil {
		t.Error("ReadAll of malformed input must error")
	}
}

func TestReadOneSequential(t *testing.T) {
	r := newTestReader("foo bar")
	a, err := r.ReadOne()
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ReadOne()
	if err != nil {
		t.Fatal(err)
	}
	if a.(*lang.Symbol).Name() != "foo" || b.(*lang.Symbol).Name() != "bar" {
		t.Errorf("sequential reads => %v %v", a, b)
	}
	if _, err := r.ReadOne(); !errors.Is(err, ErrEOF) {
		t.Errorf("third read: want ErrEOF, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Golden round-trips: read then print equals Clojure's printed form
// (doc §5 v0 success check; expected strings produced with
//  clojure -M -e '(pr (read-string ...))', Clojure 1.12.5).

func TestGoldenRoundTrip(t *testing.T) {
	tests := []struct {
		src, want string
	}{
		{"(defn add [a b] (+ a b))", "(defn add [a b] (+ a b))"},
		{"[1 -2.5 3/4 0xff 2r1010 36rZZ 12N \\a \\newline]",
			"[1 -2.5 3/4 255 10 1295 12N \\a \\newline]"},
		{"'(a b)", "(quote (a b))"},
		{"@state", "(clojure.core/deref state)"},
		{"(str \"esc\\tA\\377\")", "(str \"esc\\tAÿ\")"},
		{"{:a 1, :b 2}", "{:a 1, :b 2}"}, // lang prints maps with ", " separator
		{"#_(ignored) (kept)", "(kept)"},
	}
	for _, tt := range tests {
		got := mustRead(t, tt.src)
		if s := lang.PrintString(got); s != tt.want {
			t.Errorf("round-trip %q => %s, want %s", tt.src, s, tt.want)
		}
	}
}
