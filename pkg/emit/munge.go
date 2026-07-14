package emit

import (
	"fmt"
	"strings"
)

// Munging: Clojure names → Go identifier fragments. The scheme is a
// PUBLIC CONTRACT from M2 on (ADR 0013: emitted output must be a stable,
// go-gettable library surface) — see MUNGING.md for the normative
// description. Token choices follow JVM Clojure's Compiler.CHAR_MAP
// wherever JVM Clojure has one, so names round-trip familiarly across
// implementations; cljgo-specific extensions (`.`, exhaustive escape)
// are documented in MUNGING.md.
var charMap = map[rune]string{
	'-':  "_",
	':':  "_COLON_",
	'+':  "_PLUS_",
	'>':  "_GT_",
	'<':  "_LT_",
	'=':  "_EQ_",
	'~':  "_TILDE_",
	'!':  "_BANG_",
	'@':  "_CIRCA_",
	'#':  "_SHARP_",
	'\'': "_SINGLEQUOTE_",
	'"':  "_DOUBLEQUOTE_",
	'%':  "_PERCENT_",
	'^':  "_CARET_",
	'&':  "_AMP_",
	'*':  "_STAR_",
	'|':  "_BAR_",
	'{':  "_LBRACE_",
	'}':  "_RBRACE_",
	'[':  "_LBRACK_",
	']':  "_RBRACK_",
	'/':  "_SLASH_",
	'\\': "_BSLASH_",
	'?':  "_QMARK_",
	'.':  "_DOT_", // cljgo extension: Go identifiers cannot contain dots
}

// goReserved: Go keywords plus predeclared identifiers a munged name must
// not collide with (they'd shadow or fail to parse).
var goReserved = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true,
	"continue": true, "default": true, "defer": true, "else": true,
	"fallthrough": true, "for": true, "func": true, "go": true,
	"goto": true, "if": true, "import": true, "interface": true,
	"map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true,
	"var": true,
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true, "float32": true,
	"float64": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true,
	"uint64": true, "uintptr": true, "true": true, "false": true,
	"iota": true, "nil": true, "append": true, "cap": true, "clear": true,
	"close": true, "complex": true, "copy": true, "delete": true,
	"imag": true, "len": true, "make": true, "max": true, "min": true,
	"new": true, "panic": true, "print": true, "println": true,
	"real": true, "recover": true,
}

// munge converts a Clojure name to a valid Go identifier per MUNGING.md.
// It is NOT injective ("a-b" and "a_b" collide); callers that mint
// identifiers from munged names must dedup (generator.uniqueGlobal).
func munge(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			if tok, ok := charMap[r]; ok {
				b.WriteString(tok)
			} else {
				b.WriteString(fmt.Sprintf("_u%04x_", r))
			}
		}
	}
	out := b.String()
	if out == "" {
		return "X"
	}
	// A leading digit is not a valid Go identifier start; a leading
	// underscore would make the name unexportable forever (cljs2go's rule).
	if c := out[0]; (c >= '0' && c <= '9') || c == '_' {
		out = "X" + out
	}
	if goReserved[out] {
		out += "_"
	}
	return out
}
