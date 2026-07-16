package format14

import (
	"fmt"
	"strings"
)

// TranslateRender is candidate A: translate-then-delegate. Where Go's fmt
// verb means the SAME thing as the Java conversion+flags (d/x/o/c/s/f/e with
// -,+,space,0,# and width/precision), it builds a Go format verb and hands
// off to fmt.Sprintf. Where Go's grammar has no equivalent at all (',',  '(',
// %b's boolean meaning, %g's algorithm) it hand-renders exactly like
// direct.go — there's no way to "mostly" delegate those; either Go's verb
// means the same thing or you write it yourself.
func TranslateRender(d Directive, arg any) (string, error) {
	switch lowerConvOf(d.Conv) {
	case 's':
		return translateS(d, arg)
	case 'd':
		return translateD(d, arg)
	case 'x':
		return translateHexOctal(d, arg, "x")
	case 'o':
		return translateHexOctal(d, arg, "o")
	case 'c':
		return translateC(d, arg)
	case 'b':
		return directB(d, arg) // no Go equivalent at all — same hand code as direct
	case 'f':
		return translateF(d, arg)
	case 'e':
		return translateE(d, arg)
	case 'g':
		return directG(d, arg) // Go's %g is a different algorithm — cannot delegate
	default:
		return "", errUnknownConversion(d.Conv)
	}
}

// goFlags allowed per Go verb, filtered so we never hand Go's fmt a
// flag/verb combo it rejects with "%!verb(BADFLAG)" noise. Combos Java
// supports but Go's fmt doesn't at all (',', '(') are handled by hand-written
// post-processing below, never passed through to Sprintf.
var goFlags = map[string]string{"d": "-+ 0", "x": "-0#", "o": "-0#", "s": "-", "c": "-", "f": "-+ 0", "e": "-+ 0"}

func buildVerb(d Directive, conv string) string {
	var flags strings.Builder
	allowed := goFlags[conv]
	for i := 0; i < len(d.Flags); i++ {
		if strings.IndexByte(allowed, d.Flags[i]) >= 0 {
			flags.WriteByte(d.Flags[i])
		}
	}
	verb := "%" + flags.String()
	if d.HasWidth {
		verb += fmt.Sprintf("%d", d.Width)
	}
	if d.HasPrec {
		verb += fmt.Sprintf(".%d", d.Precision)
	}
	return verb + conv
}

func translateS(d Directive, arg any) (string, error) {
	s := toDisplayString(arg)
	return applyCase(fmt.Sprintf(buildVerb(d, "s"), s), d.Conv), nil
}

func translateD(d Directive, arg any) (string, error) {
	iv, ok := arg.(int64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	if !d.HasFlag(',') && !d.HasFlag('(') {
		return fmt.Sprintf(buildVerb(d, "d"), iv), nil
	}
	// ',' and '(' have no Go fmt equivalent: render the bare magnitude via Go,
	// hand-apply grouping/parens, then hand-pad (Go's own width computation
	// would be wrong once grouping changes the digit count).
	neg := iv < 0
	mag := iv
	if neg {
		mag = -iv
	}
	digits := fmt.Sprintf("%d", mag)
	if d.HasFlag(',') {
		digits = insertGrouping(digits)
	}
	return padNumeric(applySign(digits, neg, d), d)
}

func translateHexOctal(d Directive, arg any, conv string) (string, error) {
	iv, ok := arg.(int64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	// Go's %x/%o on an UNSIGNED operand never emits a '-' — casting to
	// uint64 first reproduces Java's two's-complement bit pattern for
	// negatives "for free" via Go's own formatter.
	u := uint64(iv)
	verb := buildVerb(d, conv)
	if hasUpperConv(d.Conv) {
		verb = strings.Replace(verb, conv, strings.ToUpper(conv), 1)
	}
	return fmt.Sprintf(verb, u), nil
}

func translateC(d Directive, arg any) (string, error) {
	r, ok := arg.(rune)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	s := fmt.Sprintf(buildVerb(d, "c"), r)
	return applyCase(s, d.Conv), nil
}

func translateF(d Directive, arg any) (string, error) {
	fv, ok := arg.(float64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	if !d.HasFlag(',') && !d.HasFlag('(') {
		return fmt.Sprintf(buildVerb(d, "f"), fv), nil
	}
	neg := fv < 0
	mag := fv
	if neg {
		mag = -fv
	}
	prec := 6
	if d.HasPrec {
		prec = d.Precision
	}
	s := fmt.Sprintf("%.*f", prec, mag)
	if d.HasFlag(',') {
		s = groupDecimal(s)
	}
	return padNumeric(applySign(s, neg, d), d)
}

func translateE(d Directive, arg any) (string, error) {
	fv, ok := arg.(float64)
	if !ok {
		return "", errIllegalConversion(d.Conv, argKindName(arg))
	}
	s := fmt.Sprintf(buildVerb(d, "e"), fv)
	return applyCase(s, d.Conv), nil
}
