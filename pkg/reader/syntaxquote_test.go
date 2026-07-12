package reader

// Syntax-quote unit tests for behavior the S8 goldens don't pin down:
// resolver alias/type paths, the global gensym counter, EOF errors,
// and the depth limit. Oracle checks were run against the real
// Clojure CLI (1.12.5, darwin/arm64) in a `user` ns with
// (require '[clojure.string :as str]); cited inline as "CLI check".

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// typeResolver extends testResolver with one type mapping
// (String -> java.lang.String) and one var (inc -> clojure.core/inc).
type typeResolver struct{ testResolver }

func (typeResolver) ResolveType(s *lang.Symbol) *lang.Symbol {
	if s.FullName() == "String" {
		return lang.NewSymbol("java.lang.String")
	}
	return nil
}

func (typeResolver) ResolveVar(s *lang.Symbol) *lang.Symbol {
	if s.FullName() == "inc" {
		return lang.InternSymbol("clojure.core", "inc")
	}
	return nil
}

// printForm is a pr-str-compatible printer for the forms these tests
// compare (design/01-reader.md task: do NOT patch pkg/lang; keep the
// workaround local). Sole divergence found in lang.PrintString for
// Phase 1 forms: an EMPTY list prints "(nil)" instead of "()" — its
// ISeq branch prints seq.First() of the non-nil *lang.EmptyList
// (pkg/lang/strconv.go Print). Everything else (symbols, keywords,
// numbers, strings, chars, bools, nested lists, vectors, maps, sets,
// with-meta forms) matched Clojure pr-str byte-for-byte across all 58
// S8 goldens. Collections recurse here so an empty list nested inside
// #() bodies prints correctly; leaves delegate to lang.PrintString.
func printForm(v any) string {
	switch x := v.(type) {
	case lang.IPersistentVector:
		parts := make([]string, 0, x.Count())
		for i := 0; i < x.Count(); i++ {
			parts = append(parts, printForm(lang.MustNth(x, i)))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case lang.ISeq, lang.IPersistentList:
		var parts []string
		for s := lang.Seq(x); s != nil; s = s.Next() {
			parts = append(parts, printForm(s.First()))
		}
		return "(" + strings.Join(parts, " ") + ")"
	default:
		return lang.PrintString(v)
	}
}

func readPr(t *testing.T, src string, opts ...Option) string {
	t.Helper()
	v, err := newTestReader(src, opts...).ReadOne()
	if err != nil {
		t.Fatalf("read %q: unexpected error: %v", src, err)
	}
	return printForm(v)
}

func TestSyntaxQuoteSymbolResolution(t *testing.T) {
	tests := []struct{ src, want string }{
		// CLI check: `str/join => (quote clojure.string/join)
		// (with the str => clojure.string alias).
		{"`str/join", "(quote clojure.string/join)"},
		// CLI check: `String/valueOf => (quote java.lang.String/valueOf)
		// (Classname/staticMethod requalified by the type's full name).
		{"`String/valueOf", "(quote java.lang.String/valueOf)"},
		// CLI check: `String. => (quote java.lang.String.) (ctor:
		// resolve the class, re-append the dot).
		{"`String.", "(quote java.lang.String.)"},
		// CLI check: `foo. => (quote foo.) (unresolvable ctor keeps
		// only the NAME of the resolution — not user/foo.).
		{"`foo.", "(quote foo.)"},
		// CLI check: `.foo => (quote .foo) (method names left as-is).
		{"`.foo", "(quote .foo)"},
		// CLI check: `java.lang.String => (quote java.lang.String)
		// (dotted name = class name, already resolved).
		{"`java.lang.String", "(quote java.lang.String)"},
		// CLI check: `Foo/bar => (quote Foo/bar) (unknown ns as-is).
		{"`Foo/bar", "(quote Foo/bar)"},
		// Unqualified var resolution through ResolveVar.
		{"`inc", "(quote clojure.core/inc)"},
		// Unresolvable => qualified with CurrentNS.
		{"`zzz", "(quote user/zzz)"},
	}
	for _, tt := range tests {
		if got := readPr(t, tt.src, WithResolver(typeResolver{})); got != tt.want {
			t.Errorf("read %q => %s, want %s", tt.src, got, tt.want)
		}
	}
}

func TestSyntaxQuoteGensymGlobalCounter(t *testing.T) {
	// Two separate reads (separate Readers) must never mint the same
	// gensym: the id source is one global atomic counter, NOT a
	// per-reader or per-map-size scheme (the Glojure collision bug —
	// design/01-reader.md §2).
	pr1 := readPr(t, "`x#")
	pr2 := readPr(t, "`x#")
	if pr1 == pr2 {
		t.Errorf("two separate reads minted the same gensym: %s", pr1)
	}
	for _, pr := range []string{pr1, pr2} {
		if !strings.HasPrefix(pr, "(quote x__") || !strings.HasSuffix(pr, "__auto__)") {
			t.Errorf("gensym shape: got %s, want (quote x__<id>__auto__)", pr)
		}
	}
}

func TestSyntaxQuoteGensymEnvPerQuote(t *testing.T) {
	// Same x# within one syntax-quote = same symbol; a nested
	// syntax-quote gets its OWN env (matches Clojure's GENSYM_ENV
	// push/pop). CLI check: `(x# `(x#)) => the two x# differ.
	pr := readPr(t, "`(x# `(x#))")
	ids := gensymRe.FindAllString(pr, -1)
	if len(ids) < 2 {
		t.Fatalf("expected at least 2 gensym occurrences, got %q", pr)
	}
	if ids[0] == ids[len(ids)-1] {
		t.Errorf("nested syntax-quote shared the outer gensym env: %s", pr)
	}
}

func TestGensymLiteralOutsideSyntaxQuote(t *testing.T) {
	// CLI check: (read-string "x#") => x# (a plain symbol; the gensym
	// machinery only engages inside `).
	v := mustRead(t, "x#")
	sym, ok := v.(*lang.Symbol)
	if !ok || sym.FullName() != "x#" {
		t.Errorf("read x# => %s, want symbol x#", lang.PrintString(v))
	}
}

func TestSyntaxQuoteFnLiteralHygiene(t *testing.T) {
	// #() params end in # ON PURPOSE: inside ` they hit the
	// auto-gensym path and stay hygienic. CLI check: `#(b %) =>
	// ... (quote p1__147__148__auto__) ... (param renamed
	// p<n>__<id>__<id2>__auto__, same symbol in argv and body).
	pr := readPr(t, "`#(b %)")
	if strings.Contains(pr, "#") && !strings.Contains(pr, "__auto__") {
		t.Fatalf("`#(b %%) params not gensym'd: %s", pr)
	}
	occ := gensymRe.FindAllString(pr, -1)
	if len(occ) != 2 || occ[0] != occ[1] {
		t.Errorf("param must appear as the SAME auto-gensym twice, got %v in %s", occ, pr)
	}
}

func TestSyntaxQuoteMetadataPreserved(t *testing.T) {
	// User metadata survives (with-meta wrap) while the reader's five
	// position keys are stripped — S8 goldens cover ^:private and
	// ^{:doc "d"}; this pins the stacked-meta + tag-shorthand case.
	// CLI check: `^String x =>
	// (clojure.core/with-meta (quote user/x) (clojure.core/apply
	// clojure.core/hash-map (clojure.core/seq (clojure.core/concat
	// (clojure.core/list :tag) (clojure.core/list (quote user/String)))))).
	got := readPr(t, "`^String x")
	want := "(clojure.core/with-meta (quote user/x) (clojure.core/apply clojure.core/hash-map (clojure.core/seq (clojure.core/concat (clojure.core/list :tag) (clojure.core/list (quote user/String))))))"
	if got != want {
		t.Errorf("`^String x =>\n got: %s\nwant: %s", got, want)
	}

	// Position-only meta must NOT produce a with-meta wrap.
	if pr := readPr(t, "`(a)"); strings.Contains(pr, "with-meta") {
		t.Errorf("position metadata leaked into expansion: %s", pr)
	}
}

func TestSyntaxQuoteErrors(t *testing.T) {
	// CLI checks: "`" and "`~" => EOF while reading; "~" => EOF while
	// reading character. All must be incomplete-input errors here.
	for _, src := range []string{"`", "~", "`~", "`(a"} {
		err := mustErr(t, src)
		if !strings.Contains(err.Error(), "EOF while reading") {
			t.Errorf("read %q: error %q, want an EOF-while-reading error", src, err)
		}
	}
	// `~@x => "splice not in list" (S8 golden ERR case; message per
	// LispReader's IllegalStateException).
	if err := mustErr(t, "`~@x"); !strings.Contains(err.Error(), "splice not in list") {
		t.Errorf("read `~@x: error %q, want splice not in list", err)
	}
}

func TestSyntaxQuoteDepthLimit(t *testing.T) {
	src := strings.Repeat("`", maxSyntaxQuoteDepth+1) + "x"
	if err := mustErr(t, src); !strings.Contains(err.Error(), "nested too deeply") {
		t.Errorf("depth limit: error %q, want nested too deeply", err)
	}
	// Modest nesting still reads. (Deliberately far below the limit:
	// nested syntax-quote expansion grows EXPONENTIALLY in output
	// size — real Clojure blows up the same way — so reading 64
	// nested backquotes is astronomically large even though the
	// depth check itself passes. The limit exists to fail fast with
	// a clear error instead of consuming all memory on adversarial
	// input that would error anyway at the 65th level.)
	if _, err := newTestReader(strings.Repeat("`", 8) + "x").ReadOne(); err != nil {
		t.Errorf("depth 8 should be allowed: %v", err)
	}
}

func TestUnquoteOutsideSyntaxQuote(t *testing.T) {
	// S8 golden: ~x => (clojure.core/unquote x); CLI check: ~@xs =>
	// (clojure.core/unquote-splicing xs). Read unconditionally, like
	// Clojure — the analyzer rejects them later.
	if got := readPr(t, "~x"); got != "(clojure.core/unquote x)" {
		t.Errorf("~x => %s", got)
	}
	if got := readPr(t, "~@xs"); got != "(clojure.core/unquote-splicing xs)" {
		t.Errorf("~@xs => %s", got)
	}
}

func TestSyntaxQuoteWithoutResolver(t *testing.T) {
	// Without an injected Resolver, symbols pass through unqualified
	// (the REPL/compiler always injects one; this pins the fallback).
	v, err := New(strings.NewReader("`x")).ReadOne()
	if err != nil {
		t.Fatalf("read `x without resolver: %v", err)
	}
	if got := lang.PrintString(v); got != "(quote x)" {
		t.Errorf("`x without resolver => %s, want (quote x)", got)
	}
}

func TestWithNextID(t *testing.T) {
	n := int64(100)
	r := New(strings.NewReader("`x#"), WithNextID(func() int64 { n++; return n }))
	v, err := r.ReadOne()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := lang.PrintString(v); got != "(quote x__101__auto__)" {
		t.Errorf("WithNextID: got %s, want (quote x__101__auto__)", got)
	}
}
