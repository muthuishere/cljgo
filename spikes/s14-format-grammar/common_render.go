package format14

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// toDisplayString is %s/%S's rendering rule: ANY argument type, via its own
// "toString" — never a type-check failure (that's what makes %s the safe
// escape hatch in real Clojure code).
func toDisplayString(arg any) string {
	switch v := arg.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return javaDoubleToString(v)
	case rune:
		return string(v)
	case *big.Int:
		return v.String()
	case *big.Rat:
		return v.Num().String() + "/" + v.Denom().String()
	case clojureStringer:
		return v.String()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// javaDoubleToString approximates java.lang.Double.toString — used by %s/%S,
// which prints via the ARGUMENT's own toString, not the %f/%e/%g machinery.
// Go's default float text (strconv/%v: shortest round-trip, no forced ".0")
// disagrees with Java in exactly the cases this handles: whole numbers need
// a trailing ".0", and the sci-notation threshold differs (Java switches at
// 1e-3/1e7; Go's shortest-form switches at different, format-dependent
// points). Full parity (subnormals, exact Java exponent spelling) is a
// known-incomplete tail — see VERDICT.md.
func javaDoubleToString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	abs := math.Abs(f)
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	if abs >= 1e-3 && abs < 1e7 {
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	// Scientific: Go gives "1.2345e+07" / "1e-04"; Java wants "1.2345E7" /
	// "1.0E-4" — strip the '+', drop exponent zero-padding, force a
	// mantissa decimal point.
	s := strconv.FormatFloat(f, 'e', -1, 64)
	parts := strings.SplitN(s, "e", 2)
	mantissa, exp := parts[0], parts[1]
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	sign := ""
	exp = strings.TrimPrefix(exp, "+")
	if strings.HasPrefix(exp, "-") {
		sign = "-"
		exp = exp[1:]
	}
	exp = strings.TrimLeft(exp, "0")
	if exp == "" {
		exp = "0"
	}
	return mantissa + "E" + sign + exp
}

// argKindName is used only to build IllegalFormatConversionException-shaped
// messages; not compared against the oracle (we only compare ExClass, not
// Msg) but kept honest for readability.
func argKindName(arg any) string {
	switch arg.(type) {
	case bool:
		return "java.lang.Boolean"
	case int64:
		return "java.lang.Long"
	case float64:
		return "java.lang.Double"
	case rune:
		return "java.lang.Character"
	case string:
		return "java.lang.String"
	case *big.Int:
		return "clojure.lang.BigInt"
	case *big.Rat:
		return "clojure.lang.Ratio"
	default:
		return fmt.Sprintf("%T", arg)
	}
}

// insertGrouping inserts ',' every 3 digits from the right of the integer
// PART ONLY (never touches a fractional part after a '.'), matching Java's
// ',' flag. Caller passes just the digit run (no sign).
func insertGrouping(digits string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var out []byte
	for i, c := range []byte(digits) {
		if i > 0 && (n-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// applySign wraps a non-negative magnitude string with the requested sign
// presentation given the flags and whether the original value was negative.
// paren flag takes priority over +/space per java.util.Formatter.
func applySign(magnitude string, negative bool, d Directive) string {
	if negative && d.HasFlag('(') {
		return "(" + magnitude + ")"
	}
	if negative {
		return "-" + magnitude
	}
	if d.HasFlag('+') {
		return "+" + magnitude
	}
	if d.HasFlag(' ') {
		return " " + magnitude
	}
	return magnitude
}

// applyCase uppercases the rendered text for an uppercase conversion char
// (Java: %X/%S/%B/... uppercase the WHOLE result, not just letters that
// need it, but uppercasing digits/punctuation is a no-op so this is safe).
func applyCase(s string, conv byte) string {
	if hasUpperConv(conv) {
		return strings.ToUpper(s)
	}
	return s
}
