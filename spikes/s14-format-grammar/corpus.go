package format14

// Probe is one (format-string, args) pair, expressed twice: once as Go-native
// args for the two candidate implementations, once as literal Clojure source
// for the oracle. Keeping both in one struct is the single-source-of-truth
// choice — no chance of the oracle and the prototype silently drifting onto
// different arg values.
type Probe struct {
	Name string
	Fmt  string
	// ArgsClj is the literal Clojure source for the args, space-separated,
	// spliced directly after Fmt in `(format Fmt ArgsClj)`. Empty = no args.
	ArgsClj string
	// ArgsGo mirrors ArgsClj as Go-native values, same order.
	ArgsGo []any
	// WantErr marks a probe we expect Java/Clojure to THROW on (the oracle
	// captures the exception's simple class name instead of stdout).
	WantErr bool
}

// Corpus is the full probe set: the 2 real assertions from the jank-derived
// suite (clojure-test-suite/test/clojure/core_test/format.cljc) plus a
// systematic sweep of the Java Formatter grammar.
var Corpus = []Probe{
	// ---- A. baseline, straight from the upstream suite --------------------
	{Name: "suite-passthrough", Fmt: "test"},
	{Name: "suite-s-int", Fmt: "%s", ArgsClj: `1`, ArgsGo: []any{int64(1)}},

	// ---- B. one conversion, one arg, no flags ------------------------------
	{Name: "d-int", Fmt: "%d", ArgsClj: `42`, ArgsGo: []any{int64(42)}},
	{Name: "d-negative", Fmt: "%d", ArgsClj: `-42`, ArgsGo: []any{int64(-42)}},
	{Name: "f-default-prec", Fmt: "%f", ArgsClj: `3.14`, ArgsGo: []any{float64(3.14)}},
	{Name: "e-default", Fmt: "%e", ArgsClj: `3.14`, ArgsGo: []any{float64(3.14)}},
	{Name: "g-default", Fmt: "%g", ArgsClj: `3.14`, ArgsGo: []any{float64(3.14)}},
	{Name: "x-basic", Fmt: "%x", ArgsClj: `255`, ArgsGo: []any{int64(255)}},
	{Name: "o-basic", Fmt: "%o", ArgsClj: `8`, ArgsGo: []any{int64(8)}},
	{Name: "c-int-codepoint", Fmt: "%c", ArgsClj: `65`, ArgsGo: []any{int64(65)}},
	{Name: "c-char-lit", Fmt: "%c", ArgsClj: `\A`, ArgsGo: []any{'A'}},
	{Name: "b-true", Fmt: "%b", ArgsClj: `true`, ArgsGo: []any{true}},
	{Name: "b-false", Fmt: "%b", ArgsClj: `false`, ArgsGo: []any{false}},
	{Name: "n-newline", Fmt: "a%nb"},
	{Name: "pct-literal", Fmt: "100%%"},
	{Name: "s-string", Fmt: "%s", ArgsClj: `"hello"`, ArgsGo: []any{"hello"}},
	{Name: "s-nil", Fmt: "%s", ArgsClj: `nil`, ArgsGo: []any{nil}},
	{Name: "s-keyword", Fmt: "%s", ArgsClj: `:kw`, ArgsGo: []any{clojureStringer(":kw")}},

	// ---- C. uppercase variants (only b,h,s,c,x,e,g,a,t have them in Java) --
	{Name: "S-upper", Fmt: "%S", ArgsClj: `"abc"`, ArgsGo: []any{"abc"}},
	{Name: "B-upper", Fmt: "%B", ArgsClj: `true`, ArgsGo: []any{true}},
	{Name: "C-upper", Fmt: "%C", ArgsClj: `\a`, ArgsGo: []any{'a'}},
	{Name: "X-upper", Fmt: "%X", ArgsClj: `255`, ArgsGo: []any{int64(255)}},
	{Name: "E-upper", Fmt: "%E", ArgsClj: `3.14`, ArgsGo: []any{float64(3.14)}},
	{Name: "G-upper", Fmt: "%G", ArgsClj: `3.14`, ArgsGo: []any{float64(3.14)}},
	{Name: "D-invalid-no-upper", Fmt: "%D", ArgsClj: `1`, ArgsGo: []any{int64(1)}, WantErr: true},
	{Name: "O-invalid-no-upper", Fmt: "%O", ArgsClj: `8`, ArgsGo: []any{int64(8)}, WantErr: true},

	// ---- D. flags on numeric conversions ------------------------------------
	{Name: "d-plus-flag", Fmt: "%+d", ArgsClj: `42`, ArgsGo: []any{int64(42)}},
	{Name: "d-space-flag", Fmt: "% d", ArgsClj: `42`, ArgsGo: []any{int64(42)}},
	{Name: "d-paren-flag-neg", Fmt: "%(d", ArgsClj: `-42`, ArgsGo: []any{int64(-42)}},
	{Name: "d-paren-flag-pos", Fmt: "%(d", ArgsClj: `42`, ArgsGo: []any{int64(42)}},
	{Name: "d-comma-grouping", Fmt: "%,d", ArgsClj: `1234567`, ArgsGo: []any{int64(1234567)}},
	{Name: "d-zero-pad", Fmt: "%010d", ArgsClj: `42`, ArgsGo: []any{int64(42)}},
	{Name: "d-left-justify", Fmt: "%-10d|", ArgsClj: `42`, ArgsGo: []any{int64(42)}},
	{Name: "x-alt-form", Fmt: "%#x", ArgsClj: `255`, ArgsGo: []any{int64(255)}},
	{Name: "o-alt-form", Fmt: "%#o", ArgsClj: `8`, ArgsGo: []any{int64(8)}},
	{Name: "f-plus-flag", Fmt: "%+.2f", ArgsClj: `3.14159`, ArgsGo: []any{float64(3.14159)}},
	{Name: "f-comma-grouping", Fmt: "%,.2f", ArgsClj: `1234567.891`, ArgsGo: []any{float64(1234567.891)}},
	{Name: "f-paren-neg", Fmt: "%(.2f", ArgsClj: `-3.14159`, ArgsGo: []any{float64(-3.14159)}},
	{Name: "flags-dup-error", Fmt: "%--d", ArgsClj: `1`, ArgsGo: []any{int64(1)}, WantErr: true},
	{Name: "flags-minus-zero-conflict", Fmt: "%-05d", ArgsClj: `1`, ArgsGo: []any{int64(1)}, WantErr: true},

	// ---- E. width / precision on strings ------------------------------------
	{Name: "s-width", Fmt: "%10s|", ArgsClj: `"hi"`, ArgsGo: []any{"hi"}},
	{Name: "s-left-width", Fmt: "%-10s|", ArgsClj: `"hi"`, ArgsGo: []any{"hi"}},
	{Name: "s-precision-truncate", Fmt: "%.3s", ArgsClj: `"hello"`, ArgsGo: []any{"hello"}},
	{Name: "s-width-precision", Fmt: "%10.3s|", ArgsClj: `"hello"`, ArgsGo: []any{"hello"}},

	// ---- F. width / precision on f/e/g --------------------------------------
	{Name: "f-width", Fmt: "%10.2f|", ArgsClj: `3.14159`, ArgsGo: []any{float64(3.14159)}},
	{Name: "f-left-width", Fmt: "%-10.2f|", ArgsClj: `3.14159`, ArgsGo: []any{float64(3.14159)}},
	{Name: "f-zero-precision", Fmt: "%.0f", ArgsClj: `3.7`, ArgsGo: []any{float64(3.7)}},
	{Name: "e-width-precision", Fmt: "%12.3e|", ArgsClj: `31415.9`, ArgsGo: []any{float64(31415.9)}},
	{Name: "g-precision", Fmt: "%.2g", ArgsClj: `31415.9`, ArgsGo: []any{float64(31415.9)}},
	{Name: "g-small", Fmt: "%g", ArgsClj: `0.0000012345`, ArgsGo: []any{float64(0.0000012345)}},

	// ---- G. argument indexing ------------------------------------------------
	{Name: "idx-reorder", Fmt: "%2$s %1$s", ArgsClj: `"a" "b"`, ArgsGo: []any{"a", "b"}},
	{Name: "idx-reuse", Fmt: "%1$s-%1$s", ArgsClj: `"x"`, ArgsGo: []any{"x"}},
	{Name: "idx-relative", Fmt: "%1$s %<s", ArgsClj: `"a" "b"`, ArgsGo: []any{"a", "b"}},
	{Name: "idx-mixed-implicit-then-explicit", Fmt: "%s %2$s", ArgsClj: `"a" "b"`, ArgsGo: []any{"a", "b"}},
	{Name: "idx-out-of-range", Fmt: "%3$s", ArgsClj: `"a" "b"`, ArgsGo: []any{"a", "b"}, WantErr: true},

	// ---- H. nil / truthiness edge cases --------------------------------------
	{Name: "d-nil-throws", Fmt: "%d", ArgsClj: `nil`, ArgsGo: []any{nil}, WantErr: true},
	{Name: "b-nil-is-false", Fmt: "%b", ArgsClj: `nil`, ArgsGo: []any{nil}},
	{Name: "b-truthy-string", Fmt: "%b", ArgsClj: `"x"`, ArgsGo: []any{"x"}},
	{Name: "b-truthy-zero", Fmt: "%b", ArgsClj: `0`, ArgsGo: []any{int64(0)}},
	{Name: "b-truthy-false-boxed", Fmt: "%b", ArgsClj: `false`, ArgsGo: []any{false}},

	// ---- I. BigInt / Ratio ----------------------------------------------------
	{Name: "d-bigint-small", Fmt: "%d", ArgsClj: `100N`, ArgsGo: []any{bigIntFromString("100")}},
	{Name: "d-bigint-huge", Fmt: "%d", ArgsClj: `100000000000000000000N`, ArgsGo: []any{bigIntFromString("100000000000000000000")}},
	{Name: "s-ratio", Fmt: "%s", ArgsClj: `1/3`, ArgsGo: []any{ratioFromString("1/3")}},
	{Name: "d-ratio-throws", Fmt: "%d", ArgsClj: `1/3`, ArgsGo: []any{ratioFromString("1/3")}, WantErr: true},
	{Name: "f-ratio-throws", Fmt: "%f", ArgsClj: `1/3`, ArgsGo: []any{ratioFromString("1/3")}, WantErr: true},

	// ---- J. two's-complement hex/octal on negative ints ------------------------
	{Name: "x-negative-long", Fmt: "%x", ArgsClj: `-1`, ArgsGo: []any{int64(-1)}},
	{Name: "o-negative-long", Fmt: "%o", ArgsClj: `-1`, ArgsGo: []any{int64(-1)}},

	// ---- K. error conditions ----------------------------------------------------
	{Name: "unknown-conversion", Fmt: "%q", ArgsClj: `1`, ArgsGo: []any{int64(1)}, WantErr: true},
	{Name: "missing-arg", Fmt: "%s %s", ArgsClj: `"only-one"`, ArgsGo: []any{"only-one"}, WantErr: true},
	{Name: "d-on-double-throws", Fmt: "%d", ArgsClj: `3.14`, ArgsGo: []any{float64(3.14)}, WantErr: true},
	{Name: "f-on-integer-throws", Fmt: "%f", ArgsClj: `3`, ArgsGo: []any{int64(3)}, WantErr: true},

	// ---- L. more width/x/o combos -----------------------------------------------
	{Name: "x-width-zero-pad", Fmt: "%08x", ArgsClj: `255`, ArgsGo: []any{int64(255)}},
	{Name: "o-width", Fmt: "%6o|", ArgsClj: `8`, ArgsGo: []any{int64(8)}},
	{Name: "d-width-plain", Fmt: "%6d|", ArgsClj: `42`, ArgsGo: []any{int64(42)}},

	// ---- N. %s of a Double (Java's Double.toString vs Go's %v) --------------------
	{Name: "s-double-whole", Fmt: "%s", ArgsClj: `3.0`, ArgsGo: []any{float64(3.0)}},
	{Name: "s-double-frac", Fmt: "%s", ArgsClj: `3.14`, ArgsGo: []any{float64(3.14)}},
	{Name: "s-double-large-sci", Fmt: "%s", ArgsClj: `1.2345E9`, ArgsGo: []any{float64(1.2345e9)}},
	{Name: "s-double-small-sci", Fmt: "%s", ArgsClj: `1.5E-5`, ArgsGo: []any{float64(1.5e-5)}},

	// ---- M. multiple mixed conversions in one string -----------------------------
	{Name: "mixed-line", Fmt: "%s is %d years old (%.1f%%)", ArgsClj: `"Alice" 30 12.345`, ArgsGo: []any{"Alice", int64(30), float64(12.345)}},
	{Name: "printf-style-log", Fmt: "[%-5s] %s", ArgsClj: `"WARN" "disk low"`, ArgsGo: []any{"WARN", "disk low"}},
}
