package format14

import (
	"math"
	"strconv"
	"strings"
)

// DirectRender is candidate B: a hand-rolled Java-format-string interpreter
// that never calls Go's fmt.Sprintf verb formatting. Every conversion is
// rendered from strconv primitives plus hand-written sign/grouping/padding
// logic, so behavior that Go's fmt disagrees with (two's-complement hex on
// negatives, comma grouping, paren-negative, boolean %b, %n) costs no more
// code than the "compatible" conversions — there's no shortcut to lose.
func DirectRender(d Directive, arg any) (string, error) {
	switch lowerConvOf(d.Conv) {
	case 's':
		return directS(d, arg)
	case 'd':
		return directD(d, arg)
	case 'x':
		return directHexOctal(d, arg, 16)
	case 'o':
		return directHexOctal(d, arg, 8)
	case 'c':
		return directC(d, arg)
	case 'b':
		return directB(d, arg)
	case 'f':
		return directF(d, arg)
	case 'e':
		return directE(d, arg)
	case 'g':
		return directG(d, arg)
	default:
		return "", errUnknownConversion(d.Conv)
	}
}

func directS(d Directive, arg any) (string, error) {
	s := toDisplayString(arg)
	if d.HasPrec && d.Precision < len(s) {
		s = s[:d.Precision]
	}
	return padWidth(applyCase(s, d.Conv), d)
}

func directD(d Directive, arg any) (string, error) {
	iv, ok := arg.(int64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	neg := iv < 0
	mag := iv
	if neg {
		mag = -iv
	}
	digits := strconv.FormatInt(mag, 10)
	if d.HasFlag(',') {
		digits = insertGrouping(digits)
	}
	return padNumeric(applySign(digits, neg, d), d)
}

func directHexOctal(d Directive, arg any, base int) (string, error) {
	iv, ok := arg.(int64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	digits := strconv.FormatUint(uint64(iv), base) // two's-complement: Java prints the bit pattern for negatives, never a '-'
	digits = applyCase(digits, d.Conv)
	if d.HasFlag('#') {
		switch base {
		case 16:
			if hasUpperConv(d.Conv) {
				digits = "0X" + digits
			} else {
				digits = "0x" + digits
			}
		case 8:
			digits = "0" + digits
		}
	}
	return padPrefixed(digits, d)
}

func directC(d Directive, arg any) (string, error) {
	r, ok := arg.(rune)
	if !ok {
		// Java's %c requires Character/Byte/Short/int — a Clojure Long
		// (our int64) does NOT qualify, confirmed against the oracle.
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	return padWidth(applyCase(string(r), d.Conv), d)
}

func directB(d Directive, arg any) (string, error) {
	var v bool
	switch a := arg.(type) {
	case nil:
		v = false
	case bool:
		v = a
	default:
		v = true // Java: any non-null, non-Boolean object is truthy for %b
	}
	s := "false"
	if v {
		s = "true"
	}
	return padWidth(applyCase(s, d.Conv), d)
}

func directF(d Directive, arg any) (string, error) {
	fv, ok := arg.(float64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	prec := 6
	if d.HasPrec {
		prec = d.Precision
	}
	neg := math.Signbit(fv)
	mag := math.Abs(fv)
	// NOTE: strconv rounds half-to-even; Java's Formatter rounds HALF_UP.
	// Documented divergence — see VERDICT.md.
	s := strconv.FormatFloat(mag, 'f', prec, 64)
	if d.HasFlag(',') {
		s = groupDecimal(s)
	}
	return padNumeric(applySign(s, neg, d), d)
}

func directE(d Directive, arg any) (string, error) {
	fv, ok := arg.(float64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	prec := 6
	if d.HasPrec {
		prec = d.Precision
	}
	neg := math.Signbit(fv)
	mag := math.Abs(fv)
	s := strconv.FormatFloat(mag, 'e', prec, 64)
	s = applyCase(s, d.Conv)
	return padNumeric(applySign(s, neg, d), d)
}

func directG(d Directive, arg any) (string, error) {
	fv, ok := arg.(float64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	prec := 6
	if d.HasPrec {
		prec = d.Precision
	}
	if prec == 0 {
		prec = 1
	}
	neg := math.Signbit(fv)
	mag := math.Abs(fv)

	sci := strconv.FormatFloat(mag, 'e', prec-1, 64)
	exp := parseExponent(sci)
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
	s = applyCase(s, d.Conv)
	return padNumeric(applySign(s, neg, d), d)
}

func parseExponent(sci string) int {
	i := strings.IndexByte(sci, 'e')
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(sci[i+1:])
	return n
}

func groupDecimal(s string) string {
	parts := strings.SplitN(s, ".", 2)
	parts[0] = insertGrouping(parts[0])
	return strings.Join(parts, ".")
}

// padNumeric zero-pads AFTER any sign/paren prefix (Java: "0042", "-0042",
// never "00-42"), or space-pads/left-justifies otherwise.
func padNumeric(s string, d Directive) (string, error) {
	if !d.HasWidth || len(s) >= d.Width {
		return s, nil
	}
	pad := d.Width - len(s)
	if d.HasFlag('-') {
		return s + strings.Repeat(" ", pad), nil
	}
	if d.HasFlag('0') {
		prefixLen := 0
		if len(s) > 0 && (s[0] == '-' || s[0] == '+' || s[0] == ' ' || s[0] == '(') {
			prefixLen = 1
		}
		return s[:prefixLen] + strings.Repeat("0", pad) + s[prefixLen:], nil
	}
	return strings.Repeat(" ", pad) + s, nil
}

// padPrefixed is padNumeric's sibling for x/o, which prefix "0x"/"0X"/"0"
// instead of a sign.
func padPrefixed(s string, d Directive) (string, error) {
	if !d.HasWidth || len(s) >= d.Width {
		return s, nil
	}
	pad := d.Width - len(s)
	if d.HasFlag('-') {
		return s + strings.Repeat(" ", pad), nil
	}
	if d.HasFlag('0') {
		prefixLen := 0
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			prefixLen = 2
		} else if strings.HasPrefix(s, "0") && len(s) > 1 {
			prefixLen = 1
		}
		return s[:prefixLen] + strings.Repeat("0", pad) + s[prefixLen:], nil
	}
	return strings.Repeat(" ", pad) + s, nil
}
