package eval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// internFormatBuiltins wires `format`/`printf` (ADR 0030, spike S14) into
// clojure.core: Java's java.util.Formatter grammar, implemented
// translate-then-delegate — parse the format string once, delegate the
// fmt.Sprintf-compatible conversions (d x o c s f e, and their uppercase
// forms) through a strict per-verb flag allow-list, hand-render the
// conversions/flags Go's fmt has no equivalent for (%b's Java truthiness
// semantics, %g's round-then-choose-notation algorithm, the ','/'(' flags,
// %n, argument indexing). Both modes get this identically: format/printf
// are plain nativeFn Vars, the same shape as println/pr-str/str, so AOT
// compilation needs no emitter changes (a generic Var-call already handles
// any IFn).
//
// Adapted from spikes/s14-format-grammar/{spec,translate,common_render,
// errors}.go (candidate A, ratified 80/80 against real JVM Clojure 1.12.5 —
// see VERDICT.md) — read as a reference only, never edited; the spike's
// stand-in types (plain rune/*big.Int/*big.Rat) are replaced here with
// cljgo's actual runtime types (lang.Char, *lang.BigInt, *lang.Ratio), and
// %s's display text reuses lang.ToString (pkg/lang/strconv.go's formatFloat
// is a bit-exact-verified java.lang.Double.toString, better than the
// spike's own approximation) instead of re-deriving Java double rendering.
func (e *Evaluator) internFormatBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	def("format", func(args ...any) any {
		if len(args) < 1 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: format", len(args)))
		}
		f, ok := args[0].(string)
		if !ok {
			panic(fmt.Errorf("format expects a string as the first argument, got: %s", lang.PrintString(args[0])))
		}
		s, err := formatRender(f, args[1:])
		if err != nil {
			panic(err)
		}
		return s
	})
	def("printf", func(args ...any) any {
		if len(args) < 1 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: printf", len(args)))
		}
		f, ok := args[0].(string)
		if !ok {
			panic(fmt.Errorf("printf expects a string as the first argument, got: %s", lang.PrintString(args[0])))
		}
		s, err := formatRender(f, args[1:])
		if err != nil {
			panic(err)
		}
		fmt.Fprint(Out, s)
		return nil
	})
}

// --- format errors: Java-exception-shaped messages ---------------------

// formatError mirrors the SIMPLE CLASS NAME of the java.util.* exception
// java.util.Formatter would throw for the same misuse, so a catch/message
// grep reads the same as real Clojure's stack trace would.
type formatError struct {
	class string
	msg   string
}

func (e *formatError) Error() string { return e.class + ": " + e.msg }

func errUnknownConversion(conv byte) error {
	return &formatError{"UnknownFormatConversionException", fmt.Sprintf("Conversion = '%c'", conv)}
}

func errIllegalConversion(conv byte, typeName string) error {
	return &formatError{"IllegalFormatConversionException", fmt.Sprintf("%c != %s", conv, typeName)}
}

func errMissingArg(conv byte) error {
	return &formatError{"MissingFormatArgumentException", fmt.Sprintf("Format specifier '%%%c'", conv)}
}

func errDuplicateFlags(flags string) error {
	return &formatError{"DuplicateFormatFlagsException", "Flags = '" + flags + "'"}
}

func errIllegalFlags(flags string) error {
	return &formatError{"IllegalFormatFlagsException", "Flags = '" + flags + "'"}
}

// --- directive parsing ---------------------------------------------------

// formatDirective is one parsed %-conversion from a Java/Clojure format
// string. Argument-index resolution and flag/width/precision extraction are
// shared regardless of conversion; only value rendering differs below.
type formatDirective struct {
	literal string // non-empty for a plain-text run; conv==0 in that case

	explicitIndex int  // 1-based; 0 = not given (use implicit/relative)
	relative      bool // "%<" — reuse the previous directive's argument

	flags     string // raw flag chars, e.g. "-+0,("
	hasWidth  bool
	width     int
	hasPrec   bool
	precision int
	conv      byte // 's','d','f','e','g','x','o','c','b','n','%', or upper-case variant
}

func (d formatDirective) hasFlag(f byte) bool { return strings.IndexByte(d.flags, f) >= 0 }

// formatDirectiveRe is Java's Formatter.fsPattern (java.util.Formatter),
// minus date/time (%t/%T conversions — out of scope, ADR 0030).
var formatDirectiveRe = regexp.MustCompile(`%(\d+\$)?([-#+ 0,(<]*)(\d+)?(\.\d+)?([a-zA-Z%])`)

// parseFormatDirectives splits a Java-grammar format string into literal
// runs and directives, in source order.
func parseFormatDirectives(f string) []formatDirective {
	var out []formatDirective
	idx := 0
	for idx < len(f) {
		loc := formatDirectiveRe.FindStringSubmatchIndex(f[idx:])
		if loc == nil {
			out = append(out, formatDirective{literal: f[idx:]})
			break
		}
		if loc[0] > 0 {
			out = append(out, formatDirective{literal: f[idx : idx+loc[0]]})
		}
		m := make([]string, 6)
		for i := 0; i < 6; i++ {
			if loc[2*i] < 0 {
				m[i] = ""
			} else {
				m[i] = f[idx+loc[2*i] : idx+loc[2*i+1]]
			}
		}
		d := formatDirective{}
		if m[1] != "" {
			n, _ := strconv.Atoi(strings.TrimSuffix(m[1], "$"))
			d.explicitIndex = n
		}
		d.flags = m[2]
		if strings.IndexByte(d.flags, '<') >= 0 {
			d.relative = true
		}
		if m[3] != "" {
			d.hasWidth = true
			d.width, _ = strconv.Atoi(m[3])
		}
		if m[4] != "" {
			d.hasPrec = true
			d.precision, _ = strconv.Atoi(strings.TrimPrefix(m[4], "."))
		}
		d.conv = m[5][0]
		out = append(out, d)
		idx += loc[1]
	}
	return out
}

// formatHasUpperConv reports whether conv is an uppercase letter (Java:
// uppercase the rendered result, e.g. %X, %S, %B — but only for conversions
// that HAVE an uppercase form; %D/%O/%F do not exist and must error).
func formatHasUpperConv(c byte) bool { return c >= 'A' && c <= 'Z' }

// formatNoUpperForm rejects the conversion letters that have NO uppercase
// form in java.util.Formatter — only b,h,s,c,x,e,g,a,t do; d,o,f do not, so
// %D/%O/%F must throw UnknownFormatConversionException, same as real Java.
var formatNoUpperForm = map[byte]bool{'D': true, 'O': true, 'F': true}

func formatLowerConv(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

// --- shared dispatch (argument index resolution, nil special-case) ------

// formatRender runs a format string + args through the translate-then-
// delegate renderer; the single entry point `format`/`printf` call.
func formatRender(f string, args []any) (string, error) {
	dirs := parseFormatDirectives(f)

	var sb strings.Builder
	implicitNext := 0
	lastIndex := -1

	for _, d := range dirs {
		if d.literal != "" {
			sb.WriteString(d.literal)
			continue
		}
		conv := d.conv
		if conv == '%' {
			sb.WriteByte('%')
			continue
		}
		if conv == 'n' {
			sb.WriteByte('\n')
			continue
		}
		if formatHasDuplicateFlag(d.flags) {
			return "", errDuplicateFlags(d.flags)
		}
		if d.hasFlag('-') && d.hasFlag('0') {
			return "", errIllegalFlags(d.flags)
		}
		if formatNoUpperForm[conv] {
			return "", errUnknownConversion(conv)
		}

		var argIdx int
		switch {
		case d.explicitIndex > 0:
			argIdx = d.explicitIndex - 1
			lastIndex = argIdx
		case d.relative:
			if lastIndex < 0 {
				return "", errMissingArg(conv)
			}
			argIdx = lastIndex
		default:
			argIdx = implicitNext
			implicitNext++
			lastIndex = argIdx
		}
		if argIdx < 0 || argIdx >= len(args) {
			return "", errMissingArg(conv)
		}
		arg := args[argIdx]

		// nil is special-cased by java.util.Formatter BEFORE any
		// conversion-specific type check: every conversion except %b
		// prints the literal string "null" for a nil argument.
		if arg == nil && formatLowerConv(conv) != 'b' {
			out, err := formatPadWidth("null", d)
			if err != nil {
				return "", err
			}
			sb.WriteString(out)
			continue
		}

		rendered, err := formatTranslateRender(d, arg)
		if err != nil {
			return "", err
		}
		sb.WriteString(rendered)
	}
	return sb.String(), nil
}

func formatHasDuplicateFlag(flags string) bool {
	seen := map[byte]bool{}
	for i := 0; i < len(flags); i++ {
		if seen[flags[i]] {
			return true
		}
		seen[flags[i]] = true
	}
	return false
}

// formatPadWidth applies the shared width/justify logic (used for the
// "null" special-case, and reused by the renderer's final pad step).
func formatPadWidth(s string, d formatDirective) (string, error) {
	if !d.hasWidth || len(s) >= d.width {
		return s, nil
	}
	pad := strings.Repeat(" ", d.width-len(s))
	if d.hasFlag('-') {
		return s + pad, nil
	}
	return pad + s, nil
}

// --- translate-then-delegate renderer (ADR 0030 candidate A) -------------

// formatTranslateRender renders one resolved (directive, non-nil argument)
// pair. Where Go's fmt verb means the SAME thing as the Java
// conversion+flags (d/x/o/c/s/f/e with -,+,space,0,# and width/precision)
// it builds a Go format verb and delegates to fmt.Sprintf. Where Go's
// grammar has no equivalent at all (',', '(', %b's boolean meaning, %g's
// algorithm) it hand-renders.
func formatTranslateRender(d formatDirective, arg any) (string, error) {
	switch formatLowerConv(d.conv) {
	case 's':
		return formatTranslateS(d, arg)
	case 'd':
		return formatTranslateD(d, arg)
	case 'x':
		return formatTranslateHexOctal(d, arg, "x")
	case 'o':
		return formatTranslateHexOctal(d, arg, "o")
	case 'c':
		return formatTranslateC(d, arg)
	case 'b':
		return formatDirectB(d, arg) // no Go equivalent at all
	case 'f':
		return formatTranslateF(d, arg)
	case 'e':
		return formatTranslateE(d, arg)
	case 'g':
		return formatDirectG(d, arg) // Go's %g is a different algorithm — cannot delegate
	default:
		return "", errUnknownConversion(d.conv)
	}
}

// formatGoFlags is the allowed flag set per Go verb, filtered so we never
// hand Go's fmt a flag/verb combo it rejects with "%!verb(BADFLAG)" noise.
// Combos Java supports but Go's fmt doesn't at all (',', '(') are handled
// by hand-written post-processing below, never passed through to Sprintf.
// This allow-list is the invariant: no unfiltered flag ever reaches
// fmt.Sprintf (see format_builtins_test.go).
var formatGoFlags = map[string]string{"d": "-+ 0", "x": "-0#", "o": "-0#", "s": "-", "c": "-", "f": "-+ 0", "e": "-+ 0"}

func formatBuildVerb(d formatDirective, conv string) string {
	var flags strings.Builder
	allowed := formatGoFlags[conv]
	for i := 0; i < len(d.flags); i++ {
		if strings.IndexByte(allowed, d.flags[i]) >= 0 {
			flags.WriteByte(d.flags[i])
		}
	}
	verb := "%" + flags.String()
	if d.hasWidth {
		verb += fmt.Sprintf("%d", d.width)
	}
	if d.hasPrec {
		verb += fmt.Sprintf(".%d", d.precision)
	}
	return verb + conv
}

func formatTranslateS(d formatDirective, arg any) (string, error) {
	s := formatDisplayString(arg)
	return formatApplyCase(fmt.Sprintf(formatBuildVerb(d, "s"), s), d.conv), nil
}

func formatTranslateD(d formatDirective, arg any) (string, error) {
	iv, ok := arg.(int64)
	if !ok {
		return "", errIllegalConversion(d.conv, formatArgKindName(arg))
	}
	if !d.hasFlag(',') && !d.hasFlag('(') {
		return fmt.Sprintf(formatBuildVerb(d, "d"), iv), nil
	}
	// ',' and '(' have no Go fmt equivalent: render the bare magnitude via
	// Go, hand-apply grouping/parens, then hand-pad (Go's own width
	// computation would be wrong once grouping changes the digit count).
	neg := iv < 0
	mag := iv
	if neg {
		mag = -iv
	}
	digits := fmt.Sprintf("%d", mag)
	if d.hasFlag(',') {
		digits = formatInsertGrouping(digits)
	}
	return formatPadNumeric(formatApplySign(digits, neg, d), d)
}

func formatTranslateHexOctal(d formatDirective, arg any, conv string) (string, error) {
	iv, ok := arg.(int64)
	if !ok {
		return "", errIllegalConversion(d.conv, formatArgKindName(arg))
	}
	// Go's %x/%o on an UNSIGNED operand never emits a '-' — casting to
	// uint64 first reproduces Java's two's-complement bit pattern for
	// negatives "for free" via Go's own formatter.
	u := uint64(iv)
	verb := formatBuildVerb(d, conv)
	if formatHasUpperConv(d.conv) {
		verb = strings.Replace(verb, conv, strings.ToUpper(conv), 1)
	}
	return fmt.Sprintf(verb, u), nil
}

func formatTranslateC(d formatDirective, arg any) (string, error) {
	var r rune
	switch v := arg.(type) {
	case lang.Char:
		r = rune(v)
	default:
		// Java's %c requires Character/Byte/Short/int — a plain Clojure
		// Long (cljgo's int64) does NOT qualify, confirmed against the
		// oracle; only a genuine char literal works.
		return "", errIllegalConversion(d.conv, formatArgKindName(arg))
	}
	s := fmt.Sprintf(formatBuildVerb(d, "c"), r)
	return formatApplyCase(s, d.conv), nil
}

func formatTranslateF(d formatDirective, arg any) (string, error) {
	switch v := arg.(type) {
	case float64:
		if !d.hasFlag(',') && !d.hasFlag('(') {
			return fmt.Sprintf(formatBuildVerb(d, "f"), v), nil
		}
		neg := v < 0
		mag := v
		if neg {
			mag = -v
		}
		prec := 6
		if d.hasPrec {
			prec = d.precision
		}
		s := fmt.Sprintf("%.*f", prec, mag)
		if d.hasFlag(',') {
			s = formatGroupDecimal(s)
		}
		return formatPadNumeric(formatApplySign(s, neg, d), d)
	case *lang.BigDecimal:
		return formatBigDecimalF(d, v)
	default:
		return "", errIllegalConversion(d.conv, formatArgKindName(arg))
	}
}

func formatTranslateE(d formatDirective, arg any) (string, error) {
	switch v := arg.(type) {
	case float64:
		s := fmt.Sprintf(formatBuildVerb(d, "e"), v)
		return formatApplyCase(s, d.conv), nil
	case *lang.BigDecimal:
		return formatBigDecimalE(d, v)
	default:
		return "", errIllegalConversion(d.conv, formatArgKindName(arg))
	}
}
