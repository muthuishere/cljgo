package corelib

import (
	cryptorand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	mathrand "math/rand/v2"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// internNumericBuiltins wires cljgo's numeric tower into clojure.core
// (design/08 §5 Batch 2, ADR 0022): the bigint/bigdec coercions, ratio
// accessors, the promoting (+'/-'/*') and unchecked (unchecked-*)
// arithmetic variants, the full bit-* surface, numeric parse-* / rand-*,
// and `==` (cross-category numeric equality, distinct from `=`). The Go
// numeric TYPES are already vendored (pkg/lang/{bigint,bigdecimal,ratio,
// numberops}.go); this is the missing core-fn wiring. Registered as Go
// builtins so BOTH modes have them identically — rt.Boot() interns these
// into clojure.core before an emitted binary's Load() runs, so the
// interpreter and the compiled binary agree by construction (ADR 0002).
//
// Precedence-safe: every name here is a real clojure.core fn, never a
// rename (CLAUDE.md). The checked +/-/* keep Clojure's throw-on-overflow
// (lang.Add/Sub/Multiply already do; the emit intrinsics fall through to
// the same tower path), so promotion is opt-in via the prime variants.
//
// Wired into internBuiltins by ONE line (e.internNumericBuiltins(def)),
// per the merge-friendly discipline. The numeric type PREDICATES
// (number?/rational?/ratio?/int?/…) belong to Batch 1 and are NOT
// defined here; this file only guarantees ratios/bigints/bigdecs are the
// genuine tower types those predicates match.
func internNumericBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// --- BigInt / BigInteger / BigDecimal coercions ----------------------

	// bigint: coerce to clojure.lang.BigInt (prints with an N suffix).
	// Strings parse; floats/ratios/bigdecimals truncate toward zero.
	def("bigint", func(args ...any) any {
		x := oneArg("bigint", args)
		switch v := x.(type) {
		case string:
			bi, err := lang.NewBigInt(strings.TrimSpace(v))
			if err != nil {
				panic(lang.NewIllegalArgumentError("bigint: invalid number: " + v))
			}
			return bi
		case *lang.Ratio:
			return lang.NewBigIntFromGoBigInt(v.BigIntegerValue())
		case *lang.BigDecimal:
			return lang.NewBigIntFromGoBigInt(v.ToBigInteger())
		case nil:
			panic(lang.NewIllegalArgumentError("bigint: cannot convert nil"))
		default:
			return lang.AsBigInt(x)
		}
	})

	// biginteger: coerce to a java.math.BigInteger analogue (*big.Int,
	// prints WITHOUT an N suffix). Same conversions as bigint.
	def("biginteger", func(args ...any) any {
		x := oneArg("biginteger", args)
		switch v := x.(type) {
		case string:
			bn, ok := new(big.Int).SetString(strings.TrimSpace(v), 10)
			if !ok {
				panic(lang.NewIllegalArgumentError("biginteger: invalid number: " + v))
			}
			return bn
		case *big.Int:
			return v
		case *lang.BigInt:
			return v.ToBigInteger()
		case *lang.Ratio:
			return v.BigIntegerValue()
		case *lang.BigDecimal:
			return v.ToBigInteger()
		case nil:
			panic(lang.NewIllegalArgumentError("biginteger: cannot convert nil"))
		default:
			return lang.AsBigInt(x).ToBigInteger()
		}
	})

	// bigdec: coerce to BigDecimal. Strings parse exactly; numbers go
	// through the tower's decimal conversion.
	def("bigdec", func(args ...any) any {
		x := oneArg("bigdec", args)
		switch v := x.(type) {
		case string:
			bd, err := lang.NewBigDecimal(strings.TrimSpace(v))
			if err != nil {
				panic(lang.NewIllegalArgumentError("bigdec: invalid number: " + v))
			}
			return bd
		case nil:
			panic(lang.NewIllegalArgumentError("bigdec: cannot convert nil"))
		default:
			return lang.AsBigDecimal(x)
		}
	})

	// -math-context: private helper behind the `with-precision` macro
	// (core.clj) — builds the *lang.MathContext bound to *math-context*
	// (ADR 0032 follow-on, S16 probes_wp.clj). precision is a Long,
	// rounding is the rounding-mode name as a string (the macro extracts
	// it from a bare symbol like UP/HALF_EVEN at macro-expansion time via
	// `name`, so no java.math.RoundingMode class access is needed here).
	def("-math-context", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -math-context", len(args)))
		}
		precision, ok := args[0].(int64)
		if !ok {
			panic(lang.NewIllegalArgumentError("-math-context: precision must be an integer"))
		}
		rounding, ok := args[1].(string)
		if !ok {
			panic(lang.NewIllegalArgumentError("-math-context: rounding must be a string"))
		}
		mc, err := lang.NewMathContext(int(precision), rounding)
		if err != nil {
			panic(lang.NewIllegalArgumentError(err.Error()))
		}
		return mc
	})

	// --- Ratio accessors -------------------------------------------------
	//
	// numerator/denominator return a BigInteger (*big.Int), matching
	// Clojure (they print without an N suffix). Clojure restricts these to
	// Ratio; a reduced integer literal (e.g. 4/2 => 2) is not a Ratio.

	def("numerator", func(args ...any) any {
		r, ok := oneArg("numerator", args).(*lang.Ratio)
		if !ok {
			panic(lang.NewIllegalArgumentError("numerator requires a Ratio"))
		}
		return r.Numerator()
	})
	def("denominator", func(args ...any) any {
		r, ok := oneArg("denominator", args).(*lang.Ratio)
		if !ok {
			panic(lang.NewIllegalArgumentError("denominator requires a Ratio"))
		}
		return r.Denominator()
	})

	// rationalize: exact rational of a number. Floats/bigdecimals convert
	// via their DECIMAL string (so 0.1 => 1/10, not the binary
	// approximation); integers and ratios pass through, reduced results
	// collapse to integers.
	def("rationalize", func(args ...any) any {
		return rationalize(oneArg("rationalize", args))
	})

	// --- Promoting arithmetic (+'/-'/*'/inc'/dec') -----------------------
	//
	// int64 overflow auto-promotes to BigInt instead of throwing.

	def("+'", func(args ...any) any {
		var acc any = int64(0)
		for i, a := range args {
			if i == 0 {
				acc = a
				continue
			}
			acc = lang.AddP(acc, a)
		}
		return acc
	})
	def("-'", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: -'"))
		}
		if len(args) == 1 {
			return lang.SubP(int64(0), args[0])
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.SubP(acc, a)
		}
		return acc
	})
	def("*'", func(args ...any) any {
		if len(args) == 0 {
			return int64(1)
		}
		acc := args[0]
		for _, a := range args[1:] {
			acc = lang.MultiplyP(acc, a)
		}
		return acc
	})
	def("inc'", func(args ...any) any {
		return lang.AddP(oneArg("inc'", args), int64(1))
	})
	def("dec'", func(args ...any) any {
		return lang.SubP(oneArg("dec'", args), int64(1))
	})

	// abs lives below with the batch-E fns (ADR 0029 cluster E and batch E
	// implemented it concurrently; the keep-both merge briefly registered it
	// twice — one definition kept, oracle notes folded in there).

	// --- Unchecked arithmetic (int64 wraps, no overflow check) -----------

	def("unchecked-add", func(args ...any) any {
		return lang.UncheckedAdd(twoArgs("unchecked-add", args))
	})
	def("unchecked-subtract", func(args ...any) any {
		return lang.UncheckedSubtract(twoArgs("unchecked-subtract", args))
	})
	def("unchecked-multiply", func(args ...any) any {
		return lang.UncheckedMultiply(twoArgs("unchecked-multiply", args))
	})
	def("unchecked-negate", func(args ...any) any {
		return lang.UncheckedNegate(oneArg("unchecked-negate", args))
	})
	def("unchecked-inc", func(args ...any) any {
		return lang.UncheckedAdd(oneArg("unchecked-inc", args), int64(1))
	})
	def("unchecked-dec", func(args ...any) any {
		return lang.UncheckedSubtract(oneArg("unchecked-dec", args), int64(1))
	})
	// unchecked-*-int operate on longs in cljgo (no boxed Integer type);
	// division/remainder honor Clojure's floor-free truncation.
	def("unchecked-add-int", func(args ...any) any {
		return lang.UncheckedAdd(twoArgs("unchecked-add-int", args))
	})
	def("unchecked-subtract-int", func(args ...any) any {
		return lang.UncheckedSubtract(twoArgs("unchecked-subtract-int", args))
	})
	def("unchecked-multiply-int", func(args ...any) any {
		return lang.UncheckedMultiply(twoArgs("unchecked-multiply-int", args))
	})
	def("unchecked-divide-int", func(args ...any) any {
		x, y := twoArgs("unchecked-divide-int", args)
		return lang.AsInt64(x) / lang.AsInt64(y)
	})
	def("unchecked-remainder-int", func(args ...any) any {
		x, y := twoArgs("unchecked-remainder-int", args)
		return lang.AsInt64(x) % lang.AsInt64(y)
	})
	// unchecked-inc-int / unchecked-dec-int / unchecked-negate-int
	// (fundamentals batch A4): same documented cljgo stance as the other
	// -int arithmetic above — they operate on int64 (no boxed Integer
	// type), so results wrap at 64 bits, not the JVM's 32.
	def("unchecked-inc-int", func(args ...any) any {
		return lang.UncheckedAdd(oneArg("unchecked-inc-int", args), int64(1))
	})
	def("unchecked-dec-int", func(args ...any) any {
		return lang.UncheckedSubtract(oneArg("unchecked-dec-int", args), int64(1))
	})
	def("unchecked-negate-int", func(args ...any) any {
		return lang.UncheckedNegate(oneArg("unchecked-negate-int", args))
	})

	// --- Unchecked coercions (fundamentals batch A4) ---------------------
	//
	// The truncating casts, oracle-verified vs JVM 1.12.5 (2026-07-23) —
	// unlike the checked byte/short/int/... coercions above these never
	// range-check, they truncate like Java primitive casts. byte/short/int
	// truncate to their true JVM widths (8/16/32 bits) and come back as
	// cljgo's int64 numbers; char masks to 16 bits like Java's (char)
	// cast. Frozen evidence in conformance/tests/unchecked-coercions.clj.
	// oracle: (unchecked-byte 300) => 44; (unchecked-byte -300) => -44;
	// (unchecked-short 70000) => 4464; (unchecked-char 97) => \a;
	// (unchecked-int 4294967296) => 0;
	// (unchecked-int 2147483648) => -2147483648; (unchecked-long 1.9) => 1;
	// (unchecked-float 1.5) => 1.5; (unchecked-double 3) => 3.0.
	def("unchecked-byte", func(args ...any) any {
		return int64(int8(lang.AsInt64(oneArg("unchecked-byte", args))))
	})
	def("unchecked-short", func(args ...any) any {
		return int64(int16(lang.AsInt64(oneArg("unchecked-short", args))))
	})
	def("unchecked-char", func(args ...any) any {
		x := oneArg("unchecked-char", args)
		if c, ok := x.(lang.Char); ok {
			return c
		}
		return lang.Char(rune(uint16(lang.AsInt64(x))))
	})
	def("unchecked-int", func(args ...any) any {
		return int64(int32(lang.AsInt64(oneArg("unchecked-int", args))))
	})
	def("unchecked-long", func(args ...any) any {
		return lang.UncheckedLongCast(oneArg("unchecked-long", args))
	})
	def("unchecked-float", func(args ...any) any {
		return lang.UncheckedFloatCast(oneArg("unchecked-float", args))
	})
	def("unchecked-double", func(args ...any) any {
		return lang.AsFloat64(oneArg("unchecked-double", args))
	})

	// --- Bit operations (all over 64-bit longs) --------------------------
	//
	// Shift/position counts mask to 6 bits, matching Java/Clojure long
	// shift semantics (1 << 64 == 1, not 0 as raw Go would give).

	bitFold := func(name string, f func(a, b int64) int64) func(args ...any) any {
		return func(args ...any) any {
			if len(args) < 2 {
				panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), name))
			}
			acc := lang.AsInt64(args[0])
			for _, a := range args[1:] {
				acc = f(acc, lang.AsInt64(a))
			}
			return acc
		}
	}
	def("bit-and", bitFold("bit-and", func(a, b int64) int64 { return a & b }))
	def("bit-or", bitFold("bit-or", func(a, b int64) int64 { return a | b }))
	def("bit-xor", bitFold("bit-xor", func(a, b int64) int64 { return a ^ b }))
	def("bit-and-not", bitFold("bit-and-not", func(a, b int64) int64 { return a &^ b }))
	def("bit-not", func(args ...any) any {
		return ^lang.AsInt64(oneArg("bit-not", args))
	})
	def("bit-shift-left", func(args ...any) any {
		x, n := twoArgs("bit-shift-left", args)
		return lang.AsInt64(x) << uint(lang.AsInt64(n)&63)
	})
	def("bit-shift-right", func(args ...any) any {
		x, n := twoArgs("bit-shift-right", args)
		return lang.AsInt64(x) >> uint(lang.AsInt64(n)&63)
	})
	def("unsigned-bit-shift-right", func(args ...any) any {
		x, n := twoArgs("unsigned-bit-shift-right", args)
		return int64(uint64(lang.AsInt64(x)) >> uint(lang.AsInt64(n)&63))
	})
	def("bit-flip", func(args ...any) any {
		x, n := twoArgs("bit-flip", args)
		return lang.AsInt64(x) ^ (int64(1) << uint(lang.AsInt64(n)&63))
	})
	def("bit-set", func(args ...any) any {
		x, n := twoArgs("bit-set", args)
		return lang.AsInt64(x) | (int64(1) << uint(lang.AsInt64(n)&63))
	})
	def("bit-clear", func(args ...any) any {
		x, n := twoArgs("bit-clear", args)
		return lang.AsInt64(x) &^ (int64(1) << uint(lang.AsInt64(n)&63))
	})
	def("bit-test", func(args ...any) any {
		x, n := twoArgs("bit-test", args)
		return (lang.AsInt64(x) & (int64(1) << uint(lang.AsInt64(n)&63))) != 0
	})

	// --- Numeric equality: == (cross-category, distinct from =) ----------

	def("==", func(args ...any) any {
		if len(args) == 0 {
			panic(fmt.Errorf("wrong number of args (0) passed to: =="))
		}
		if !lang.IsNumber(args[0]) {
			panic(lang.NewIllegalArgumentError("== not supported on " + lang.PrintString(args[0])))
		}
		for i := 1; i < len(args); i++ {
			if !lang.IsNumber(args[i]) {
				panic(lang.NewIllegalArgumentError("== not supported on " + lang.PrintString(args[i])))
			}
			if !lang.NumEquiv(args[i-1], args[i]) {
				return false
			}
		}
		return true
	})

	// --- Parsing (parse-long / parse-double / parse-boolean / parse-uuid)
	//
	// All take a string and return nil on a non-parse (Clojure 1.11); a
	// non-string argument throws.

	def("parse-long", func(args ...any) any {
		s := stringArg("parse-long", oneArg("parse-long", args))
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil
		}
		return n
	})
	def("parse-double", func(args ...any) any {
		s := strings.TrimSpace(stringArg("parse-double", oneArg("parse-double", args)))
		switch s {
		case "Infinity", "+Infinity":
			return math.Inf(1)
		case "-Infinity":
			return math.Inf(-1)
		case "NaN", "+NaN", "-NaN":
			return math.NaN()
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return f
	})
	def("parse-boolean", func(args ...any) any {
		switch stringArg("parse-boolean", oneArg("parse-boolean", args)) {
		case "true":
			return true
		case "false":
			return false
		default:
			return nil
		}
	})
	def("parse-uuid", func(args ...any) any {
		s := stringArg("parse-uuid", oneArg("parse-uuid", args))
		u, ok := reader.NewUUID(s)
		if !ok {
			return nil
		}
		return u
	})

	// --- Randomness (rand / rand-int / rand-nth / random-uuid) -----------

	def("rand", func(args ...any) any {
		switch len(args) {
		case 0:
			return mathrand.Float64()
		case 1:
			return lang.AsFloat64(args[0]) * mathrand.Float64()
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: rand", len(args)))
		}
	})
	def("rand-int", func(args ...any) any {
		n := lang.AsInt64(oneArg("rand-int", args))
		return int64(float64(n) * mathrand.Float64())
	})
	def("rand-nth", func(args ...any) any {
		coll := oneArg("rand-nth", args)
		// (nth nil idx) is always nil (never out-of-bounds), so a nil coll
		// never hits the empty-collection error either. Oracle: (rand-nth
		// nil) => nil.
		if coll == nil {
			return nil
		}
		n := lang.Count(coll)
		if n == 0 {
			panic(lang.NewIllegalArgumentError("rand-nth of empty collection"))
		}
		v, _ := lang.Nth(coll, mathrand.IntN(n))
		return v
	})
	def("random-uuid", func(args ...any) any {
		if len(args) != 0 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: random-uuid", len(args)))
		}
		u, _ := reader.NewUUID(randomUUIDString())
		return u
	})

	// abs: absolute value over the whole numeric tower (design/08 batch E).
	// lang.Ops(x).Abs already implements every category faithfully,
	// INCLUDING the JVM's Long/MIN_VALUE 2's-complement oddity (int64Ops.Abs
	// returns x unchanged, matching clojure-test-suite abs.cljc's
	// `r/min-int r/min-int` case) and NaN (float64Ops.Abs => math.Abs(NaN)
	// => NaN). Throws on a non-number, matching the JVM ClassCastException.
	// Oracle (clojure 1.12): (abs -1) => 1; (abs -1/5) => 1/5; (abs -123N)
	// => 123N; (abs -123.456M) => 123.456M; (abs -0.0) => 0.0; (abs ##-Inf)
	// => ##Inf; (abs ##NaN) NaN? => true; (abs nil) throws.
	def("abs", func(args ...any) any {
		x := oneArg("abs", args)
		if !lang.IsNumber(x) {
			panic(fmt.Errorf("abs: not a number: %s", lang.PrintString(x)))
		}
		return lang.Ops(x).Abs(x)
	})

	// shuffle: (shuffle coll) -> a NEW shuffled vector (Fisher-Yates over
	// math/rand/v2, matching rand/rand-int/rand-nth's unseeded source
	// above). Accepts vectors, sets, and seqs/lists (design/08 batch E);
	// throws on anything else (nil, numbers, strings, maps), matching the
	// JVM's `new ArrayList(coll)` requiring a java.util.Collection. Oracle
	// (clojure 1.12, clojure-test-suite shuffle.cljc): (shuffle nil),
	// (shuffle "abc"), (shuffle {}), (shuffle 1) all throw; (shuffle [1 2 3])
	// and (shuffle #{1 2 3}) return a vector of the same count.
	def("shuffle", func(args ...any) any {
		coll := oneArg("shuffle", args)
		var items []any
		switch v := coll.(type) {
		case lang.IPersistentVector:
			items = lang.ToSlice(v)
		case *lang.Set:
			items = lang.ToSlice(v)
		case lang.ISeq:
			items = lang.ToSlice(v)
		default:
			panic(fmt.Errorf("shuffle: not a collection: %s", lang.PrintString(coll)))
		}
		shuffled := make([]any, len(items))
		copy(shuffled, items)
		mathrand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		return lang.NewVector(shuffled...)
	})
}

// twoArgs asserts a 2-arg builtin's arity and returns both arguments.
func twoArgs(op string, args []any) (any, any) {
	if len(args) != 2 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	return args[0], args[1]
}

// stringArg asserts a parse-* argument is a string (Clojure throws for a
// non-string, distinct from returning nil for an unparseable string).
func stringArg(op string, x any) string {
	s, ok := x.(string)
	if !ok {
		panic(lang.NewIllegalArgumentError(op + " requires a string, got: " + lang.PrintString(x)))
	}
	return s
}

// rationalize returns the exact rational value of x (design/08 §5).
// Floats and bigdecimals convert via their decimal string form so the
// result is the exact decimal (0.1 => 1/10). Reduced integral results
// collapse to int64/BigInt; non-integral to a Ratio.
func rationalize(x any) any {
	var r *big.Rat
	switch v := x.(type) {
	case float64:
		r = decimalRat(strconv.FormatFloat(v, 'g', -1, 64))
	case float32:
		r = decimalRat(strconv.FormatFloat(float64(v), 'g', -1, 32))
	case *lang.BigDecimal:
		r = decimalRat(v.String())
	case *lang.Ratio, *lang.BigInt, *big.Int,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return x // already exact
	case nil:
		panic(lang.NewIllegalArgumentError("rationalize: cannot convert nil"))
	default:
		return x
	}
	if r == nil {
		panic(lang.NewIllegalArgumentError("rationalize: cannot convert " + lang.PrintString(x)))
	}
	if r.IsInt() {
		n := r.Num()
		if n.IsInt64() {
			return n.Int64()
		}
		return lang.NewBigIntFromGoBigInt(n)
	}
	return lang.NewRatioGoBigInt(r.Num(), r.Denom())
}

// decimalRat parses a decimal string ("1.5", "0.1", "1e3") into an exact
// big.Rat, or nil if malformed.
func decimalRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil
	}
	return r
}

// randomUUIDString builds an RFC-4122 v4 UUID string from crypto/rand.
func randomUUIDString() string {
	var b [16]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		panic(fmt.Errorf("random-uuid: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
