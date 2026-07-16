package eval

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// This file holds the value-rendering helpers ADR 0030 / spike S14 VERDICT.md
// calls "shared regardless of approach": sign/grouping/padding, %s display
// text, and the two conversions with NO Go fmt equivalent at all (%b's Java
// truthiness, %g's round-then-choose-notation algorithm) — hand-written
// once, used by formatTranslateRender in format_builtins.go. Adapted from
// spikes/s14-format-grammar/{common_render,direct}.go (read-only reference).

// formatDisplayString is %s/%S's rendering rule: ANY argument type, via its
// own Java-equivalent toString — never a type-check failure (that's what
// makes %s the safe escape hatch in real Clojure code). Reuses
// lang.ToString for everything, which already renders doubles bit-exactly
// like java.lang.Double.toString (pkg/lang/strconv.go formatFloat) and
// BigInt/Ratio/Char/keywords/etc. the same way pr-str's %s callers expect.
func formatDisplayString(arg any) string {
	return lang.ToString(arg)
}

// formatArgKindName is used only to build IllegalFormatConversionException-
// shaped messages.
func formatArgKindName(arg any) string {
	switch arg.(type) {
	case bool:
		return "java.lang.Boolean"
	case int64:
		return "java.lang.Long"
	case float64:
		return "java.lang.Double"
	case lang.Char:
		return "java.lang.Character"
	case string:
		return "java.lang.String"
	case *lang.BigInt:
		return "clojure.lang.BigInt"
	case *lang.Ratio:
		return "clojure.lang.Ratio"
	case *lang.BigDecimal:
		return "java.math.BigDecimal"
	default:
		return fmt.Sprintf("%T", arg)
	}
}

// formatInsertGrouping inserts ',' every 3 digits from the right of the
// integer PART ONLY (never touches a fractional part after a '.'), matching
// Java's ',' flag. Caller passes just the digit run (no sign).
func formatInsertGrouping(digits string) string {
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

func formatGroupDecimal(s string) string {
	parts := strings.SplitN(s, ".", 2)
	parts[0] = formatInsertGrouping(parts[0])
	return strings.Join(parts, ".")
}

// formatApplySign wraps a non-negative magnitude string with the requested
// sign presentation given the flags and whether the original value was
// negative. The paren flag takes priority over +/space per
// java.util.Formatter.
func formatApplySign(magnitude string, negative bool, d formatDirective) string {
	if negative && d.hasFlag('(') {
		return "(" + magnitude + ")"
	}
	if negative {
		return "-" + magnitude
	}
	if d.hasFlag('+') {
		return "+" + magnitude
	}
	if d.hasFlag(' ') {
		return " " + magnitude
	}
	return magnitude
}

// formatApplyCase uppercases the rendered text for an uppercase conversion
// char (Java: %X/%S/%B/... uppercase the WHOLE result, but uppercasing
// digits/punctuation is a no-op so this is safe).
func formatApplyCase(s string, conv byte) string {
	if formatHasUpperConv(conv) {
		return strings.ToUpper(s)
	}
	return s
}

// formatPadNumeric zero-pads AFTER any sign/paren prefix (Java: "0042",
// "-0042", never "00-42"), or space-pads/left-justifies otherwise.
func formatPadNumeric(s string, d formatDirective) (string, error) {
	if !d.hasWidth || len(s) >= d.width {
		return s, nil
	}
	pad := d.width - len(s)
	if d.hasFlag('-') {
		return s + strings.Repeat(" ", pad), nil
	}
	if d.hasFlag('0') {
		prefixLen := 0
		if len(s) > 0 && (s[0] == '-' || s[0] == '+' || s[0] == ' ' || s[0] == '(') {
			prefixLen = 1
		}
		return s[:prefixLen] + strings.Repeat("0", pad) + s[prefixLen:], nil
	}
	return strings.Repeat(" ", pad) + s, nil
}

// formatDirectB is %b: Java truthiness, not Go's %b (binary). %b is the one
// conversion the shared nil special-case in formatRender does NOT intercept
// (every other conversion prints "null" for nil), so this function alone
// sees a possible nil arg: nil is false, any other non-Boolean is
// unconditionally true, a Boolean is its own value.
func formatDirectB(d formatDirective, arg any) (string, error) {
	var v bool
	switch a := arg.(type) {
	case nil:
		v = false
	case bool:
		v = a
	default:
		v = true // any non-nil, non-Boolean object is truthy for %b
	}
	s := "false"
	if v {
		s = "true"
	}
	return formatPadWidth(formatApplyCase(s, d.conv), d)
}

// formatDirectG is %g: Java's algorithm — round to `precision` significant
// digits, then choose fixed vs. scientific by comparing the POST-ROUNDING
// exponent against [-4, precision). Go's %g picks shortest-round-trip
// representation, a different goal entirely, so there is no delegation
// path here.
func formatDirectG(d formatDirective, arg any) (string, error) {
	fv, ok := arg.(float64)
	if !ok {
		return "", errIllegalConversion(d.conv, formatArgKindName(arg))
	}
	prec := 6
	if d.hasPrec {
		prec = d.precision
	}
	if prec == 0 {
		prec = 1
	}
	neg := math.Signbit(fv)
	mag := math.Abs(fv)

	sci := strconv.FormatFloat(mag, 'e', prec-1, 64)
	exp := formatParseExponent(sci)
	var s string
	if exp >= -4 && exp < prec {
		decPlaces := prec - 1 - exp
		if decPlaces < 0 {
			decPlaces = 0
		}
		s = strconv.FormatFloat(mag, 'f', decPlaces, 64)
	} else {
		s = sci
	}
	s = formatApplyCase(s, d.conv)
	return formatPadNumeric(formatApplySign(s, neg, d), d)
}

func formatParseExponent(sci string) int {
	i := strings.IndexByte(sci, 'e')
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(sci[i+1:])
	return n
}
