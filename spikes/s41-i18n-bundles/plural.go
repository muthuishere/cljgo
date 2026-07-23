package main

// plural.go — a pragmatic CLDR/ICU plural-category selector.
//
// Real ICU/CLDR defines SIX categories — zero, one, two, few, many,
// other — and a per-language rule set derived from the CLDR
// `plurals.xml` operands (n, i, v, w, f, t). See
// https://cldr.unicode.org/index/cldr-spec/plural-rules and
// https://unicode-org.github.io/icu/userguide/format_parse/messages/ .
//
// This spike ships the two rules that cover the demo locales and names
// the gap: a real bri.i18n would either vendor a generated CLDR table
// (all ~200 locales) or bind to golang.org/x/text/feature/plural (pure
// Go, CLDR-backed). We deliberately hand-roll two so the mechanism is
// visible and dependency-free.

// PluralCategory is a CLDR keyword.
type PluralCategory string

const (
	Zero  PluralCategory = "zero"
	One   PluralCategory = "one"
	Two   PluralCategory = "two"
	Few   PluralCategory = "few"
	Many  PluralCategory = "many"
	Other PluralCategory = "other"
)

// pluralRule maps a count to a category for one language family.
type pluralRule func(n int) PluralCategory

// pluralRules is the (tiny) built-in table, keyed by the language
// subtag. A production impl replaces this map with CLDR-generated data.
var pluralRules = map[string]pluralRule{
	// English CLDR rule: one -> i=1 and v=0 (i.e. integer 1); else other.
	// (We only handle integers here, so: n==1 -> one.)
	"en": func(n int) PluralCategory {
		if n == 1 {
			return One
		}
		return Other
	},
	// French CLDR rule: one -> i=0 or i=1 (0 and 1 are "one"); else other.
	"fr": func(n int) PluralCategory {
		if n == 0 || n == 1 {
			return One
		}
		return Other
	},
}

// categoryFor picks a CLDR category for count in the given language,
// falling back to the English rule for unknown languages.
func categoryFor(lang string, n int) PluralCategory {
	if r, ok := pluralRules[lang]; ok {
		return r(n)
	}
	return pluralRules["en"](n)
}

// selectPlural resolves a count to the winning message for a plural
// bundle value, honouring an explicit `zero` special-case when present
// (ICU allows literal `=0`/`zero`; we accept the `zero` keyword) before
// falling to the CLDR category, then to `other`.
func selectPlural(lang string, n int, forms map[PluralCategory]string) (string, bool) {
	// Explicit zero form wins for n==0 if the author supplied one
	// (matches ICU's `=0` / explicit `zero` convenience).
	if n == 0 {
		if s, ok := forms[Zero]; ok {
			return s, true
		}
	}
	cat := categoryFor(lang, n)
	if s, ok := forms[cat]; ok {
		return s, true
	}
	if s, ok := forms[Other]; ok {
		return s, true
	}
	return "", false
}
