package reader

// Reader Phase 2 conformance: namespaced maps, reader conditionals, and
// the #uuid / #inst tagged literals (design/01-reader.md §Phase 2).
//
// Every expectation is frozen against real JVM Clojure 1.12.5 (darwin,
// `clojure` CLI, the semantic oracle), cited inline as "CLI check".
// Namespaced maps and #uuid/#inst are reproduced directly. Reader
// conditionals are cited by ANALOGY: the JVM always injects its own
// platform feature :clj, so cljgo's :cljgo branch cannot be produced on
// the CLI directly — the JVM oracle run with :clj is the mirror of the
// cljgo run with :cljgo (feature present => its value; feature absent =>
// :default or elision). Both were checked via
//   clojure -e "(read-string {:read-cond :allow :features #{:clj}} \"...\")"

import (
	"errors"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// ---------------------------------------------------------------------------
// Namespaced maps

func TestNamespacedMapGolden(t *testing.T) {
	// Printed with the *print-namespace-maps* sugar (batch A2 — root true,
	// matching clojure.main): a map whose keys all share one namespace
	// prints as #:ns{...}, exactly the CLI's own printing byte-for-byte
	// (CLI 2026-07-23: (prn {:foo/a 1 'foo/x 2}) => #:foo{:a 1, x 2}).
	tests := []struct{ src, want string }{
		// CLI: (read-string "#:foo{:a 1 :b 2}") entries => :foo/a 1, :foo/b 2.
		{"#:foo{:a 1 :b 2}", "#:foo{:a 1, :b 2}"},
		// CLI: already-qualified keys stay, :_/d strips to :d (mixed
		// namespaces => plain map printing).
		{"#:foo{:a 1 :foo/b 2 :bar/c 3 :_/d 4}", "{:foo/a 1, :foo/b 2, :bar/c 3, :d 4}"},
		// CLI: bare symbol key gets the namespace too (foo/x).
		{"#:foo{:a 1 x 2}", "#:foo{:a 1, x 2}"},
		// CLI: (read-string "#::{:a 1}") in ns user => {:user/a 1}.
		{"#::{:a 1}", "#:user{:a 1}"},
		// #::alias resolves via the resolver (str -> clojure.string here).
		{"#::str{:a 1}", "#:clojure.string{:a 1}"},
		// CLI: non-qualifiable key (a number) is unchanged.
		{"#:foo{1 2}", "{1 2}"},
		// Whitespace between the namespace and the map is allowed.
		{"#:foo {:a 1}", "#:foo{:a 1}"},
		{"#:foo{}", "{}"},
	}
	for _, tt := range tests {
		if got := readPr(t, tt.src); got != tt.want {
			t.Errorf("read %q => %s, want %s", tt.src, got, tt.want)
		}
	}
}

func TestNamespacedMapErrors(t *testing.T) {
	// CLI: #::bogus{...} => "Unknown auto-resolved namespace alias: bogus".
	if err := mustErr(t, "#::bogus{:a 1}"); !strings.Contains(err.Error(), "Unknown auto-resolved namespace alias: bogus") {
		t.Errorf("#::bogus: %v", err)
	}
	// CLI: #:foo[...] => "Namespaced map must specify a map".
	if err := mustErr(t, "#:foo[1 2]"); !strings.Contains(err.Error(), "Namespaced map must specify a map") {
		t.Errorf("#:foo[..]: %v", err)
	}
	// CLI: #:foo{:a 1 :foo/a 2} => "Duplicate key: :foo/a" (checked after
	// qualification).
	if err := mustErr(t, "#:foo{:a 1 :foo/a 2}"); !strings.Contains(err.Error(), "Duplicate key: :foo/a") {
		t.Errorf("dup key: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Reader conditionals

func TestReaderConditionalGolden(t *testing.T) {
	// The cljgo platform feature is :cljgo (never :clj). Analogy to the
	// JVM oracle noted in the file header.
	tests := []struct{ src, want string }{
		// cljgo of JVM `#?(:clj :j :default :d)` => :j.
		{"#?(:cljgo :yes :default :no)", ":yes"},
		// JVM (with :clj): `#?(:cljs :c :default :d)` => :d — cljgo skips :clj.
		{"#?(:clj :j :default :d)", ":d"},
		// First matching branch wins; :default only if reached.
		{"#?(:default :d :cljgo :y)", ":d"},
		{"#?(:foo 1 :cljgo 2 :default 3)", "2"},
		// Selection inside a collection.
		{"(a #?(:clj b :cljgo c) d)", "(a c d)"},
		// Splicing: matched branch splices its elements.
		{"[1 #?@(:cljgo [2 3]) 4]", "[1 2 3 4]"},
		// Splicing: unmatched branch splices nothing.
		{"[1 #?@(:clj [2 3]) 4]", "[1 4]"},
		// Unmatched selecting conditional inside a collection is elided.
		{"[1 #?(:clj 2) 3]", "[1 3]"},
	}
	for _, tt := range tests {
		if got := readPr(t, tt.src); got != tt.want {
			t.Errorf("read %q => %s, want %s", tt.src, got, tt.want)
		}
	}
}

func TestReaderConditionalTopLevelElision(t *testing.T) {
	// CLI: (read-string {:read-cond :allow} "#?(:cljs :c)") => "EOF while
	// reading" — an unmatched top-level conditional reads as nothing, so
	// the stream is exhausted. cljgo's analog: #?(:clj ...) with no match.
	_, err := readOne(t, "#?(:clj :j)")
	if !errors.Is(err, ErrEOF) {
		t.Errorf("#?(:clj :j) top-level => %v, want ErrEOF (elided)", err)
	}
}

func TestReaderConditionalErrors(t *testing.T) {
	// CLI: (read-string {:read-cond :allow} "#?[:clj 1]") => "read-cond
	// body must be a list".
	if err := mustErr(t, "#?[:clj 1]"); !strings.Contains(err.Error(), "read-cond body must be a list") {
		t.Errorf("#?[..]: %v", err)
	}
	// CLI: non-keyword feature => "Feature should be a keyword: clj".
	if err := mustErr(t, `#?("clj" 1 :default 2)`); !strings.Contains(err.Error(), "Feature should be a keyword: clj") {
		t.Errorf("string feature: %v", err)
	}
	// CLI: #?@ at top level => "Reader conditional splicing not allowed at
	// the top level.".
	if err := mustErr(t, "#?@(:cljgo [1 2])"); !strings.Contains(err.Error(), "splicing not allowed at the top level") {
		t.Errorf("top-level splice: %v", err)
	}
	// CLI: splicing a non-sequential value => "Spliced form list ...".
	if err := mustErr(t, "[#?@(:cljgo 5)]"); !strings.Contains(err.Error(), "Spliced form list in read-cond-splicing") {
		t.Errorf("bad splice value: %v", err)
	}
	// CLI: (read-string {:read-cond :allow :features #{:cljgo}}
	// "{:a 1 :b #?(:cljr X :lpy Y)}") => "Map literal must contain an even
	// number of forms" — an unmatched conditional in map-VALUE position
	// elides just the value, leaving the map odd. This is exactly why the
	// suite's reduce.cljc interop map cannot read under #{:cljgo :default}
	// (its values gate on :cljr/:lpy/:phel/:cljs/:clj with no :default).
	if err := mustErr(t, "{:a 1 :b #?(:clj X :lpy Y)}"); !strings.Contains(err.Error(), "Map literal must contain an even number of forms") {
		t.Errorf("elided map value: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tagged literals: #uuid, #inst

func TestUUIDLiteral(t *testing.T) {
	// CLI: (pr-str #uuid "550E8400-...") => #uuid "550e8400-..." (lowercased).
	v := mustRead(t, `#uuid "550E8400-E29B-41D4-A716-446655440000"`)
	u, ok := v.(*UUID)
	if !ok {
		t.Fatalf("#uuid => %T, want *reader.UUID", v)
	}
	if u.Value() != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("uuid value %q", u.Value())
	}
	if got := lang.PrintString(v); got != `#uuid "550e8400-e29b-41d4-a716-446655440000"` {
		t.Errorf("uuid prints %s", got)
	}
	// CLI: a malformed UUID string throws.
	if err := mustErr(t, `#uuid "not-a-uuid"`); !strings.Contains(err.Error(), "Invalid UUID string") {
		t.Errorf("bad uuid: %v", err)
	}
	if err := mustErr(t, `#uuid 5`); !strings.Contains(err.Error(), "UUID literal expects a string") {
		t.Errorf("non-string uuid: %v", err)
	}
}

func TestInstLiteral(t *testing.T) {
	// v0 preserves canonical timestamp text; CLI reads and prints an
	// already-canonical instant unchanged.
	v := mustRead(t, `#inst "2020-01-01T00:00:00.000-00:00"`)
	if _, ok := v.(Inst); !ok {
		t.Fatalf("#inst => %T, want reader.Inst", v)
	}
	if got := lang.PrintString(v); got != `#inst "2020-01-01T00:00:00.000-00:00"` {
		t.Errorf("inst prints %s", got)
	}
}
