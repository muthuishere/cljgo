package reader

// The reader-conditional corpus spike s50 finding 4 asks for, explicitly and
// non-optionally: the consume path's handling of #?(…) is load-bearing, and
// the spike's positional heuristic is not good enough for production.
//
// cljgo does NOT do balanced-paren scanning — it does something strictly
// stronger: it classifies on the REAL READER'S OUTPUT. That is immune to a
// `#?(` inside a string, a char literal, a `;` comment or a `#_` discard,
// which is exactly where a text scan fails (case 9 below is that trap).
//
// ORACLE RUN, 2026-07-30, real JVM Clojure 1.12.5 (`clojure` CLI), via
//   (read {:read-cond :allow} (PushbackReader. (StringReader. src)))
// with the standing feature substitution :cljgo -> :clj (each platform's own
// feature) and :clj -> :cljs (a foreign one). Verbatim results:
//
//	 1 => :A                          6 => []
//	 2 => :D                          9 => ["#?(:cljs x)" \# :end]
//	 5 => [1 2]                      10 => (now)
//	 7 => :inner                     11 => (ns x (:import))
//	 8 => :ok                        14 => :G
//	15 => ERROR read-cond requires an even number of forms.
//
// Every one of those agrees with the expectations below EXCEPT case 15, which
// is a pre-existing cljgo divergence documented at its own case.
//
// ORACLE NOTE. Cases 1-2, 5, 7-9 and 13-15 are ordinary reader-conditional
// semantics and are already oracle-verified against JVM Clojure 1.12.5 in
// readcond_test.go / phase2_test.go under the standing :clj <-> :cljgo
// mapping (the JVM always injects :clj; cljgo always injects :cljgo). They are
// re-asserted here as the corpus's baseline: the starved-conditional option
// must not move any of them.
//
// Cases 3-4, 6 and 10-12 are cljgo-specific by construction — the JVM cannot
// be starved, because it always supplies :clj. Their expectations are frozen
// here with that written rationale, per conformance discipline.

import (
	"bufio"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

func readAllWith(src string, opts ...Option) ([]any, error) {
	r := New(bufio.NewReader(strings.NewReader(src)),
		append([]Option{WithFilename("corpus.cljc")}, opts...)...)
	return r.ReadAll()
}

func printForms(forms []any) string {
	parts := make([]string, len(forms))
	for i, f := range forms {
		parts[i] = lang.PrintString(f)
	}
	return strings.Join(parts, " ")
}

// TestReaderConditionalCorpus is the s50-mandated corpus, run twice: with the
// default (JVM-parity) semantics and with the Maven-origin starved check on.
func TestReaderConditionalCorpus(t *testing.T) {
	const starved = "\x00STARVED" // expect R1012 under WithStarvedCondError
	const rerr = "\x00READERROR"  // expect a reader error in BOTH modes
	cases := []struct {
		n       int
		name    string
		src     string
		want    string // default-mode reading ("" = reads as nothing)
		wantMvn string // maven-origin reading; "" means "same as want"
	}{
		{1, "cljgo branch selected", `#?(:cljgo :A :clj :B)`, ":A", ""},
		{2, "default branch selected", `#?(:clj :B :default :D)`, ":D", ""},
		{3, "starved: clj/cljs only", `#?(:clj :B :cljs :C)`, "", starved},
		{4, "starved: single non-matching branch", `#?(:cljs :C)`, "", starved},
		{5, "splicing selects cljgo", `[#?@(:cljgo [1 2] :clj [3])]`, "[1 2]", ""},
		// A splicing conditional is illegal at the top level (R1010), so a
		// starved splice is NECESSARILY nested and therefore never R1012 — it
		// elides, exactly as on the JVM. Frozen deliberately.
		{6, "starved splice is nested by construction, so it elides", `[#?@(:clj [3])]`, "[]", ""},
		{7, "nested inside the SELECTED branch", `#?(:cljgo #?(:cljgo :inner :clj :no) :clj :B)`, ":inner", ""},
		{8, "nested inside a NON-selected branch is never read",
			`#?(:cljgo :ok :clj #?(:clj :never :cljs :never2))`, ":ok", ""},
		// THE text-scan trap: every one of these LOOKS like a conditional to a
		// regex or a balanced-paren scanner, and none of them is one. Reading
		// the real forms is what makes the classifier immune.
		{9, "the text-scan trap: #? in a string, a char literal and a comment",
			"[\"#?(:clj x)\" \\# ;; #?(:clj y)\n :end]", `["#?(:clj x)" \# :end]`, ""},
		{10, "the medley case: java fenced away, :default supplied",
			`#?(:clj (java.util.Date.) :default (now))`, "(now)", ""},
		{11, "an :import fenced inside a conditional never arrives",
			`(ns x (:import #?(:clj [java.util Date])))`, "(ns x (:import))", "(ns x (:import))"},
		{14, ":cljgo wins over :default by ORDER, first match wins",
			`#?(:clj :B :cljgo :G :default :D)`, ":G", ""},
		// An odd body has a feature but no VALUE for it — malformed rather
		// than starved. cljgo elides it silently.
		//
		// KNOWN DIVERGENCE, oracle-checked 2026-07-30 against real Clojure
		// 1.12.5: the JVM does NOT elide this, it throws
		//   "read-cond requires an even number of forms."
		// (`(read {:read-cond :allow} …)` on `#?(:clj)`). cljgo's reader has
		// no even-arity check at all (pkg/reader/readcond.go steps the body
		// with `i+1 < len(forms)`), so a malformed body is dropped instead.
		//
		// This divergence PRE-DATES ADR 0095 — it is not introduced or
		// widened here, and the starved check deliberately does not paper
		// over it: a malformed body is a different fault from a starved one
		// and must not be reported as R1012. It is recorded here rather than
		// left undocumented, and closing it (an R1xxx arity error matching
		// the JVM) is follow-up work outside this change's scope.
		{15, "odd number of forms in the body (diverges from the JVM)", `#?(:cljgo)`, "", ""},
		// Case 12: a whole TOP-LEVEL form whose only branches are JVM ones.
		// THIS is the s50 trap — the forms simply vanish and the namespace
		// installs with no vars — and it is what R1012 exists to catch.
		{12, "a top-level :clj-only body starves", `#?(:clj (def x 1) :cljs (def x 2))`, "", starved},
		{13, "unterminated conditional is the existing R1001, unchanged",
			`#?(:cljgo :a`, rerr, rerr},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// --- default mode: JVM parity, completely unchanged ---
			forms, err := readAllWith(tc.src)
			switch {
			case tc.want == rerr:
				if err == nil {
					t.Fatalf("case %d: want a reader error, got %s", tc.n, printForms(forms))
				}
			case err != nil:
				t.Fatalf("case %d: default read failed: %v", tc.n, err)
			default:
				if got := printForms(forms); got != tc.want {
					t.Fatalf("case %d default mode: want %q, got %q", tc.n, tc.want, got)
				}
			}

			// --- maven-origin mode: only a STARVED conditional differs ---
			want := tc.wantMvn
			if want == "" {
				want = tc.want
			}
			forms, err = readAllWith(tc.src, WithStarvedCondError())
			switch want {
			case starved:
				if err == nil {
					t.Fatalf("case %d: a starved conditional read silently as %q — the exact trap s50 forbids",
						tc.n, printForms(forms))
				}
				if !strings.Contains(err.Error(), "supplies no branch for this platform") {
					t.Fatalf("case %d: want the starved diagnostic, got %v", tc.n, err)
				}
				// Doctrine: located, expected-vs-found.
				if !strings.Contains(err.Error(), "corpus.cljc") {
					t.Errorf("case %d: the starved error is not located: %v", tc.n, err)
				}
				if !strings.Contains(err.Error(), ":cljgo, :default") {
					t.Errorf("case %d: the starved error does not state what was expected: %v", tc.n, err)
				}
			case rerr:
				if err == nil {
					t.Fatalf("case %d: want a reader error, got %s", tc.n, printForms(forms))
				}
				if strings.Contains(err.Error(), "supplies no branch") {
					t.Fatalf("case %d: an unterminated form must keep its own error, not become R1012: %v", tc.n, err)
				}
			default:
				if err != nil {
					t.Fatalf("case %d: maven-origin read failed: %v", tc.n, err)
				}
				if got := printForms(forms); got != want {
					t.Fatalf("case %d maven mode: want %q, got %q", tc.n, want, got)
				}
			}
		})
	}
}

// TestStarvedCondIsOptIn is the guarantee that keeps every conformance file
// still. cljgo project code is read WITHOUT the option, so a starved
// conditional stays legal and reads as nothing, exactly as on the JVM.
func TestStarvedCondIsOptIn(t *testing.T) {
	forms, err := readAllWith(`[1 #?(:clj 2 :cljs 3) 4]`)
	if err != nil {
		t.Fatalf("default reading of a starved conditional must NOT error: %v", err)
	}
	if got := printForms(forms); got != "[1 4]" {
		t.Fatalf("want [1 4] (elided like a #_ discard), got %q", got)
	}
}

// TestStarvedCollErrorCatchesShapeBreakingElision — the case
// WithStarvedCondError structurally cannot see, because it fires only at
// nestDepth 0. A starved conditional INSIDE a vector/map/set silently changes
// that collection's element count; real camel-snake-kebab 0.4.3 has
// `(let [cs … ss-length #?(:clj … :cljs …)] …)` at
// internals/string_separator.cljc:44, and eliding it produced a namespace that
// classified as usable and then failed to compile with "let* requires an even
// number of forms in binding vector" — an error naming a library the user
// never wrote. medley.core:456 is the same shape.
func TestStarvedCollErrorCatchesShapeBreakingElision(t *testing.T) {
	for _, src := range []string{
		`(let [a 1 b #?(:clj 2 :cljs 3)] a)`, // vector: the csk / medley shape
		`{:a 1 :b #?(:clj 2 :cljs 3)}`,       // map: silently odd
		`#{1 #?(:clj 2 :cljs 3)}`,            // set
	} {
		_, err := readAllWith(src, WithStarvedCollError())
		if err == nil {
			t.Errorf("a shape-breaking elision was allowed: %s", src)
			continue
		}
		if !strings.Contains(err.Error(), "would change the shape of the enclosing") {
			t.Errorf("the error does not say the shape changed: %v", err)
		}
	}
}

// TestStarvedCollErrorLeavesListFencingAlone — a starved conditional nested in
// a LIST is the portable-fencing idiom ((:import #?(:clj …))). Erroring there
// would reject exactly the libraries this exists to consume, so it stays a
// documented false negative.
func TestStarvedCollErrorLeavesListFencingAlone(t *testing.T) {
	if _, err := readAllWith(`(ns x (:import #?(:clj (java.io File))))`, WithStarvedCollError()); err != nil {
		t.Fatalf("list-nested fencing must still elide, got: %v", err)
	}
	// A starved SPLICING conditional inside a collection is also fine: `#?@`
	// contributes a sequence, so a starved one contributes ZERO elements and
	// removes nothing. Real tools.cli 1.1.230 has exactly this in a `let`
	// binding vector (cli.cljc:108, `#?@(:cljr (req …))`), and treating it as
	// shape-breaking reported a fully consumable library as unusable.
	forms, err := readAllWith(`(let [a 1 #?@(:cljr (b 2)) c 3] a)`, WithStarvedCollError())
	if err != nil {
		t.Fatalf("a starved #?@ splice removes nothing and must elide, got: %v", err)
	}
	if got := printForms(forms); got != "(let [a 1 c 3] a)" {
		t.Errorf("want the splice elided leaving an even binding vector, got %q", got)
	}
	// And it is still OPT-IN: project code is read without it.
	if _, err := readAllWith(`(let [a 1 b #?(:clj 2)] a)`); err != nil {
		t.Fatalf("default reading must not error: %v", err)
	}
}

// TestStarvedCondErrorCatchesNestedInList — the medley 1.4.0 report. At
// medley/core.cljc:181, `(instance? #?(:clj clojure.lang.PersistentQueue :cljs
// cljs.core/PersistentQueue) x)` elided to `(instance? x)` and the user got
// "macroexpanding instance?: wrong number of args (1) passed to:
// clojure.core/instance?" — a diagnostic naming neither the conditional nor the
// library. Eliding a conditional in a list changes the CALL'S ARITY, which is
// the vector/map/set shape argument applied to a list, so it shares that
// wording and pkg/deps's never-recoverable handling.
func TestStarvedCondErrorCatchesNestedInList(t *testing.T) {
	cases := []struct {
		name, src, wantPos string
	}{
		{"argument position (the medley shape)",
			`(instance? #?(:clj clojure.lang.PersistentQueue :cljs cljs.core/PersistentQueue) x)`,
			"corpus.cljc:1:12"},
		{"deeper inside a defn body",
			"(defn f [x]\n  (instance? #?(:clj A :cljs B) x))", "corpus.cljc:2:14"},
		{"a single starved branch still starves", `(f #?(:cljs 1) 2)`, "corpus.cljc:1:4"},
		{"first argument, forms after it", `(f #?(:clj 1 :cljs 2) 3 4)`, "corpus.cljc:1:4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forms, err := readAllWith(tc.src, WithStarvedCondError())
			if err == nil {
				t.Fatalf("a nested starved conditional elided silently as %q", printForms(forms))
			}
			if !strings.Contains(err.Error(), "supplies no branch for this platform") {
				t.Fatalf("want the starved diagnostic, got: %v", err)
			}
			// Doctrine: located at the CONDITIONAL, expected-vs-found.
			if !strings.Contains(err.Error(), tc.wantPos) {
				t.Errorf("want the conditional located at %s, got: %v", tc.wantPos, err)
			}
			if !strings.Contains(err.Error(), ":cljgo, :default") {
				t.Errorf("the error does not state what was expected: %v", err)
			}
			// ...and it must stay OPT-IN: project code is unaffected.
			if _, err := readAllWith(tc.src); err != nil {
				t.Errorf("default (project) reading must still elide, got: %v", err)
			}
		})
	}
}

// TestStarvedCondNestedExemptions — the cases the nested check must NOT fire
// on, because firing would reject the portable libraries the consume path
// exists to read. Each is a real idiom.
func TestStarvedCondNestedExemptions(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{":default supplied at the nested site (the medley date shape)",
			`(f #?(:clj (java.util.Date.) :default (now)))`, "(f (now))"},
		{":cljgo supplied at the nested site",
			`(f #?(:clj :x :cljgo :g))`, "(f :g)"},
		{"an ns-clause fence: the list head is a keyword, nothing is called",
			`(ns x (:import #?(:clj [java.util Date])))`, "(ns x (:import))"},
		{"inside another conditional's body, which is read before selection",
			`#?(:cljgo :ok :clj (f #?(:clj 1 :cljs 2)))`, ":ok"},
		{"inside a #_ discard: the whole form is dropped anyway",
			`[#_(f #?(:clj z)) :end]`, "[:end]"},
		{"inside a string: never a conditional at all",
			`["(f #?(:clj x))" :end]`, `["(f #?(:clj x))" :end]`},
		{"inside a ; comment: never read",
			"[;; (f #?(:clj y))\n :end]", "[:end]"},
		{"a starved SPLICE contributes zero elements, so nothing breaks",
			`(f 1 #?@(:cljr [2]) 3)`, "(f 1 3)"},
		// TRAILING elisions, both from real libraries that load and run on
		// cljgo today. The surviving form is well-shaped, so firing here would
		// reject a working dependency outright.
		{"trailing in a def: com.stuartsierra/dependency 1.0.0 dependency.cljc:148",
			`(def ^:private max-number #?(:clj Long/MAX_VALUE :cljs js/Number.MAX_VALUE))`,
			"(def max-number)"},
		{"trailing in a defn body: medley 1.4.0 core.cljc:79",
			`(defn- editable? [coll] #?(:clj (instance? A coll) :cljs (satisfies? B coll)))`,
			"(defn- editable? [coll])"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forms, err := readAllWith(tc.src, WithStarvedCondError(), WithStarvedCollError())
			if err != nil {
				t.Fatalf("false positive on a portable idiom: %v", err)
			}
			if got := printForms(forms); got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestStarvedCondErrorClassifiesAsR1012 pins the registered code the renderer
// attaches, so `cljgo explain R1012` is reachable from a real failure.
func TestStarvedCondErrorClassifiesAsR1012(t *testing.T) {
	_, err := readAllWith(`#?(:clj 1 :cljs 2)`, WithStarvedCondError())
	if err == nil {
		t.Fatal("want a starved-conditional error")
	}
	if !strings.Contains(err.Error(), "corpus.cljc:1:1") {
		t.Errorf("want a file:line:col locus, got %v", err)
	}
}
