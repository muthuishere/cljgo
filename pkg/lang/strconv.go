package lang

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// formatFloat renders a float64 exactly like Java's Double.toString
// (JDK 19+ / Ryū spec), which is what Clojure's str and pr both emit
// for finite doubles. Verified bit-exactly against the real oracle
// (clojure 1.12.5 on JDK 26, Double/toString over Double/longBitsToDouble):
// 8510 mixed random doubles plus an exhaustive scan of the 60000
// smallest subnormals and 40000 log-uniform random subnormals — zero
// divergences. Rules (each confirmed by the oracle):
//   - 1e-3 <= |v| < 1e7: plain decimal, always with a '.' (21.5, 1.0,
//     100000.0, 9999999.9, 0.001).
//   - otherwise: scientific d.dddE±x — shortest round-trip digits,
//     uppercase E, no '+' and no zero padding on the exponent, mantissa
//     always contains '.' (1.0E7, 1.0E-4, -5.647638473894739E258).
//   - zero: 0.0 / -0.0 (sign preserved).
//   - Infinity / -Infinity / NaN (Java names; Clojure pr prints ##Inf
//     etc. — handled in Print, not here, because str keeps Java names).
func formatFloat(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	case math.IsNaN(v):
		return "NaN"
	}
	abs := math.Abs(v)
	if abs != 0 && (abs >= 1e7 || abs < 1e-3) {
		s := strconv.FormatFloat(v, 'E', -1, 64)
		mant, exp, _ := strings.Cut(s, "E")
		// JDK quirk (observed on JDK 26, e.g. Double/toString of
		// Double/MIN_VALUE is "4.9E-324", not the strictly shorter
		// "5E-324"): a subnormal is never shortened to a single
		// significant digit unless its 2-digit correct rounding ends
		// in 0. Exhaustively verified over the smallest 60000
		// subnormals + 40000 random subnormals (only bit patterns
		// 1,2,10,12,14,16,18,20 are affected).
		if abs < 2.2250738585072014e-308 && !strings.Contains(mant, ".") {
			two := strconv.FormatFloat(v, 'E', 1, 64)
			if m2, _, _ := strings.Cut(two, "E"); !strings.HasSuffix(m2, "0") {
				mant, exp, _ = strings.Cut(two, "E")
			}
		}
		neg := strings.HasPrefix(exp, "-")
		exp = strings.TrimLeft(strings.TrimLeft(exp, "+-"), "0")
		if neg {
			exp = "-" + exp
		}
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		return mant + "E" + exp
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0" // Double.toString always keeps a fractional digit (1.0, -0.0)
	}
	return s
}

// floatValue unwraps a float32/float64 for the printer.
func floatValue(x interface{}) (float64, bool) {
	switch v := x.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	}
	return 0, false
}

// ToString converts a value to a string a la Java's .toString method.
func ToString(v interface{}) string {
	switch v := v.(type) {
	case nil:
		return "nil"
	case string:
		return v
	case Char:
		return string(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float32:
		return formatFloat(float64(v))
	case float64:
		return formatFloat(v)
	case uint64, uint32, uint16, uint8, uint, int64, int32, int16, int8, int:
		return fmt.Sprintf("%d", v)
	case *BigInt:
		return v.String()
	case *BigDecimal:
		return v.String()
	}

	////////////////////////////////////////////////////////////////////////////////
	// if v is a Stringer, use its String method
	if s, ok := v.(fmt.Stringer); ok {
		return s.String()
	}

	////////////////////////////////////////////////////////////////////////////////
	// If v is a slice, print it as a vector
	vv := reflect.ValueOf(v)
	if vv.Kind() == reflect.Slice || vv.Kind() == reflect.Array {
		builder := strings.Builder{}
		builder.WriteString("[")
		for i := 0; i < vv.Len(); i++ {
			if i > 0 {
				builder.WriteString(" ")
			}
			// There is a danger here that we will recurse infinitely if the
			// slice contains itself. We should probably check for that, but
			// clojure does not.
			builder.WriteString(ToString(vv.Index(i).Interface()))
		}
		builder.WriteString("]")
		return builder.String()
	}

	// if seq, ok := v.(ISeq); ok {
	// 	builder := strings.Builder{}
	// 	builder.WriteString("(")
	// 	for ; seq != nil; seq = seq.Next() {
	// 		cur := seq.First()
	// 		if builder.Len() > 1 {
	// 			builder.WriteString(" ")
	// 		}
	// 		builder.WriteString(ToString(cur))
	// 	}
	// 	builder.WriteString(")")
	// 	return builder.String()
	// }

	return fmt.Sprintf("#object[%T]", v)
}

// printLength reads *print-length* (VarPrintLength): (0, false) when nil /
// unbound (unlimited, clojure.core's default), else (N, true). cljgo
// addition to the vendored printer — see PROVENANCE.md.
func printLength() (int, bool) {
	v := VarPrintLength.Deref()
	if v == nil {
		return 0, false
	}
	if n, ok := AsNumber(v); ok {
		return int(AsInt64(n)), true
	}
	return 0, false
}

// RTPrintString corresponds to Clojure's RT.printString.
func PrintString(v interface{}) string {
	sb := strings.Builder{}
	Print(v, &sb)
	return sb.String()
}

// Print prints a value to the given io.Writer. Corresponds to
// Clojure's RT.print.
func Print(x interface{}, w io.Writer) {
	if VarPrintInitialized.IsBound() && BooleanCast(VarPrintInitialized.Deref()) {
		VarPrOn.Invoke(x, w)
		return
	}
	readably := BooleanCast(VarPrintReadably.Deref())

	if IsNil(x) {
		io.WriteString(w, "nil")
	} else if f, ok := floatValue(x); ok {
		// Clojure's print-method for Double emits ##Inf / ##-Inf / ##NaN
		// for BOTH print and pr (oracle 1.12.5: (println ##Inf) and
		// (pr-str ##Inf) each give ##Inf; only (str ##Inf) gives
		// "Infinity"). Finite doubles print like Double.toString.
		switch {
		case math.IsInf(f, 1):
			io.WriteString(w, "##Inf")
		case math.IsInf(f, -1):
			io.WriteString(w, "##-Inf")
		case math.IsNaN(f):
			io.WriteString(w, "##NaN")
		default:
			io.WriteString(w, formatFloat(f))
		}
	} else if seq, ok := x.(ISeq); ok {
		// Print the seq of the collection, not the collection itself:
		// an empty list's Seq() is nil, so it prints () rather than
		// (nil) (oracle 1.12.5: (pr-str '()) => "()").
		// *print-length* (printLength, cljgo addition — PROVENANCE.md)
		// bounds the element count exactly like clojure.core: at most N
		// elements then "..." (oracle 1.12.5: (binding [*print-length* 3]
		// (pr-str (range 10))) => "(0 1 2 ...)"). Without the bound an
		// infinite lazy seq would print forever.
		limit, limited := printLength()
		io.WriteString(w, "(")
		n := 0
		for seq := seq.Seq(); seq != nil; seq = seq.Next() {
			if limited && n >= limit {
				io.WriteString(w, "...")
				break
			}
			Print(seq.First(), w)
			n++
			if seq.Next() != nil {
				io.WriteString(w, " ")
			}
		}
		io.WriteString(w, ")")
	} else if s, ok := x.(string); ok {
		if !readably {
			io.WriteString(w, s)
		} else {
			io.WriteString(w, strconv.Quote(s))
		}
	} else if r, ok := x.(*Record); ok {
		// A defrecord prints as `#ns.Name{:a 1, :b 2}` — checked before the
		// generic IPersistentMap branch (a record IS an IPersistentMap).
		printRecord(r, w)
	} else if m, ok := x.(IPersistentMap); ok {
		// *print-length* bounds entries (oracle 1.12.5:
		// (binding [*print-length* 1] (pr-str {:a 1 :b 2})) => "{:a 1, ...}").
		limit, limited := printLength()
		io.WriteString(w, "{")
		n := 0
		for seq := m.Seq(); seq != nil; seq = seq.Next() {
			if limited && n >= limit {
				io.WriteString(w, "...")
				break
			}
			e := seq.First().(IMapEntry)
			Print(e.Key(), w)
			io.WriteString(w, " ")
			Print(e.Val(), w)
			n++
			if seq.Next() != nil {
				io.WriteString(w, ", ")
			}
		}
		io.WriteString(w, "}")
	} else if v, ok := x.(IPersistentVector); ok {
		// *print-length* bounds elements (oracle 1.12.5:
		// (binding [*print-length* 3] (pr-str [1 2 3 4 5])) => "[1 2 3 ...]").
		limit, limited := printLength()
		io.WriteString(w, "[")
		for i := 0; i < v.Count(); i++ {
			if limited && i >= limit {
				io.WriteString(w, "...")
				break
			}
			Print(MustNth(v, i), w)
			if i < v.Count()-1 {
				io.WriteString(w, " ")
			}
		}
		io.WriteString(w, "]")
	} else if s, ok := x.(IPersistentSet); ok {
		// *print-length* bounds elements (oracle 1.12.5:
		// (binding [*print-length* 1] (pr-str #{1 2 3})) => "#{1 ...}").
		limit, limited := printLength()
		io.WriteString(w, "#{")
		n := 0
		for seq := s.Seq(); seq != nil; seq = seq.Next() {
			if limited && n >= limit {
				io.WriteString(w, "...")
				break
			}
			Print(seq.First(), w)
			n++
			if seq.Next() != nil {
				io.WriteString(w, " ")
			}
		}
		io.WriteString(w, "}")
	} else if c, ok := x.(Char); ok {
		if !readably {
			io.WriteString(w, string(c))
		} else {
			io.WriteString(w, CharLiteralFromRune(rune(c)))
		}
	} else if v, ok := x.(*BigDecimal); ok && readably {
		// Java toString: scale-preserving, plain-vs-E per the javadoc
		// algorithm (ADR 0032) — 1.10M prints 1.10M, never 1.1M.
		io.WriteString(w, v.String())
		io.WriteString(w, "M")
	} else if v, ok := x.(*BigInt); ok && readably {
		io.WriteString(w, v.String())
		io.WriteString(w, "N")
	} else if v, ok := x.(*Var); ok {
		io.WriteString(w, "#=(var "+v.Namespace().Name().Name()+"/"+v.Symbol().Name()+")")
	} else if v, ok := x.(*regexp.Regexp); ok {
		io.WriteString(w, "#\""+v.String()+"\"")
	} else {
		io.WriteString(w, ToString(x))
	}
}
