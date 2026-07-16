package format14

import "strings"

// Renderer turns one resolved (directive, argument) pair into text. The two
// candidate implementations (direct.go / translate.go) each provide one;
// everything else (parsing, argument-index resolution, flag validation) is
// shared here so the comparison in run_test.go is apples-to-apples.
type Renderer func(d Directive, arg any) (string, error)

// Format runs a parsed directive list through one Renderer.
func Format(render Renderer, f string, args []any) (string, error) {
	dirs, err := ParseFormat(f)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	implicitNext := 0
	lastIndex := -1

	for _, d := range dirs {
		if d.Literal != "" {
			sb.WriteString(d.Literal)
			continue
		}
		conv := d.Conv
		if conv == '%' {
			sb.WriteByte('%')
			continue
		}
		if conv == 'n' {
			sb.WriteByte('\n')
			continue
		}
		if hasDuplicateFlag(d.Flags) {
			return "", errDuplicateFlags(d.Flags)
		}
		if d.HasFlag('-') && d.HasFlag('0') {
			return "", errIllegalFlags(d.Flags)
		}
		if noUpperForm[conv] {
			return "", errUnknownConversion(conv)
		}

		var argIdx int
		switch {
		case d.ExplicitIndex > 0:
			argIdx = d.ExplicitIndex - 1
			lastIndex = argIdx
		case d.Relative:
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

		// null is special-cased by java.util.Formatter BEFORE any
		// conversion-specific type check: every conversion except %b prints
		// the literal string "null" for a nil argument (confirmed against
		// the oracle — d-nil-throws was our WRONG prior assumption; Java's
		// Formatter special-cases null ahead of the per-conversion dispatch).
		if arg == nil && lowerConvOf(conv) != 'b' {
			out, err := padWidth("null", d)
			if err != nil {
				return "", err
			}
			sb.WriteString(out)
			continue
		}

		rendered, err := render(d, arg)
		if err != nil {
			return "", err
		}
		sb.WriteString(rendered)
	}
	return sb.String(), nil
}

func hasDuplicateFlag(flags string) bool {
	seen := map[byte]bool{}
	for i := 0; i < len(flags); i++ {
		if seen[flags[i]] {
			return true
		}
		seen[flags[i]] = true
	}
	return false
}

func lowerConvOf(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

// padWidth applies the shared width/justify logic (used for the "null"
// special-case, and reused by both renderers for their final pad step).
func padWidth(s string, d Directive) (string, error) {
	if !d.HasWidth || len(s) >= d.Width {
		return s, nil
	}
	pad := strings.Repeat(" ", d.Width-len(s))
	if d.HasFlag('-') {
		return s + pad, nil
	}
	return pad + s, nil
}
