package eval

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// internStringBuiltins interns the clojure.core regex functions
// (re-pattern, re-matcher, re-find, re-matches, re-seq, re-groups) plus
// clojure.core/subs, and the private `-str-*` host primitives that
// core/string.cljg (the clojure.string namespace) is built on.
//
// All public names are real clojure.core / clojure.string fns — precedence-
// safe additions, never renames (CLAUDE.md precedence principle). Kept in
// this file so builtins.go gains exactly one call line inside internBuiltins.
//
// REGEX ENGINE CAVEAT: cljgo compiles #"..." patterns with Go's `regexp`
// package, which is RE2 — linear-time, but WITHOUT the Java-regex features
// java.util.regex supports: no backreferences (\1), no lookahead/lookbehind
// ((?=...) (?<=...)), no possessive quantifiers, no atomic groups, and
// named-group syntax is (?P<name>...) not (?<name>...). Patterns using only
// the common syntax (character classes, anchors, quantifiers, alternation,
// capturing groups) behave identically to JVM Clojure; patterns using the
// RE2-unsupported features that pass on the JVM will panic at match time
// with a compile error. String-index results (index-of, re match offsets)
// are byte offsets into the UTF-8 string, which equal char offsets for
// ASCII; JVM Clojure returns UTF-16 code-unit indices.
func (e *Evaluator) internStringBuiltins(
	def func(string, func(...any) any) *lang.Var,
	defPrivate func(string, func(args ...any) any),
) {
	// --- clojure.core regex fns -------------------------------------

	// re-pattern: (re-pattern s) => a #"..." pattern. Idempotent on a
	// pattern (Clojure returns it unchanged). We keep the raw-pattern
	// reader.Regex value; it compiles lazily via CachedCompileRegexp.
	def("re-pattern", func(args ...any) any {
		a := oneArg("re-pattern", args)
		switch v := a.(type) {
		case reader.Regex:
			return v
		case string:
			return reader.Regex{Pattern: v}
		default:
			panic(fmt.Errorf("re-pattern expects a string or pattern, got: %s", lang.PrintString(a)))
		}
	})

	// re-matcher: (re-matcher re s) => a stateful matcher. re-find/1 and
	// re-groups walk it, exactly like java.util.regex.Matcher.
	def("re-matcher", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: re-matcher", len(args)))
		}
		re := compilePattern("re-matcher", args[0])
		s := strArg("re-matcher", args[1])
		return lang.NewRegexpMatcher(re, s)
	})

	// re-find: (re-find m) advances a matcher and returns its match;
	// (re-find re s) returns the first match of re in s. Return shape:
	// nil if no match, the whole-match string when the pattern has no
	// groups, or [whole g1 g2 ...] when it does (nil for an unmatched
	// optional group). Matches JVM clojure.core/re-find.
	def("re-find", func(args ...any) any {
		switch len(args) {
		case 1:
			m, ok := args[0].(*lang.RegexpMatcher)
			if !ok {
				panic(fmt.Errorf("re-find expects a matcher, got: %s", lang.PrintString(args[0])))
			}
			if !m.Find() {
				return nil
			}
			return matchResult(m)
		case 2:
			re := compilePattern("re-find", args[0])
			s := strArg("re-find", args[1])
			m := lang.NewRegexpMatcher(re, s)
			if !m.Find() {
				return nil
			}
			return matchResult(m)
		default:
			panic(fmt.Errorf("wrong number of args (%d) passed to: re-find", len(args)))
		}
	})

	// re-matches: (re-matches re s) matches the ENTIRE string. nil if the
	// whole string does not match; otherwise the same shape as re-find.
	def("re-matches", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: re-matches", len(args)))
		}
		re := compilePattern("re-matches", args[0])
		s := strArg("re-matches", args[1])
		m := lang.NewRegexpMatcher(re, s)
		if !m.Matches() {
			return nil
		}
		return matchResult(m)
	})

	// re-groups: (re-groups m) returns the groups of the LAST match on a
	// matcher — whole-match string with no groups, else [whole g1 g2 ...].
	def("re-groups", func(args ...any) any {
		m, ok := oneArg("re-groups", args).(*lang.RegexpMatcher)
		if !ok {
			panic(fmt.Errorf("re-groups expects a matcher, got: %s", lang.PrintString(args[0])))
		}
		return matchResult(m)
	})

	// re-seq: (re-seq re s) => a seq of successive matches; each element
	// follows the re-find shape (string with no groups, vector with).
	def("re-seq", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: re-seq", len(args)))
		}
		re := compilePattern("re-seq", args[0])
		s := strArg("re-seq", args[1])
		m := lang.NewRegexpMatcher(re, s)
		var out []any
		for m.Find() {
			out = append(out, matchResult(m))
		}
		if len(out) == 0 {
			return nil
		}
		return lang.NewList(out...)
	})

	// subs: clojure.core/subs — (subs s start) / (subs s start end).
	// Byte-offset substring (ASCII-equivalent to JVM's UTF-16 indices).
	// Out-of-range panics like Java's String.substring.
	def("subs", func(args ...any) any {
		if len(args) != 2 && len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: subs", len(args)))
		}
		s := strArg("subs", args[0])
		start, ok := lang.AsInt(args[1])
		if !ok {
			panic(fmt.Errorf("subs: start index must be an integer, got: %s", lang.PrintString(args[1])))
		}
		end := len(s)
		if len(args) == 3 {
			end, ok = lang.AsInt(args[2])
			if !ok {
				panic(fmt.Errorf("subs: end index must be an integer, got: %s", lang.PrintString(args[2])))
			}
		}
		if start < 0 || end > len(s) || start > end {
			panic(fmt.Errorf("String index out of range: %d", start))
		}
		return s[start:end]
	})

	// --- clojure.string host primitives (private) -------------------

	defPrivate("-str-upper-case", func(args ...any) any {
		return strings.ToUpper(strArg("upper-case", oneArg("upper-case", args)))
	})
	defPrivate("-str-lower-case", func(args ...any) any {
		return strings.ToLower(strArg("lower-case", oneArg("lower-case", args)))
	})
	defPrivate("-str-capitalize", func(args ...any) any {
		s := strArg("capitalize", oneArg("capitalize", args))
		if s == "" {
			return s
		}
		r := []rune(s)
		return string(unicode.ToUpper(r[0])) + strings.ToLower(string(r[1:]))
	})
	defPrivate("-str-trim", func(args ...any) any {
		return strings.TrimSpace(strArg("trim", oneArg("trim", args)))
	})
	defPrivate("-str-triml", func(args ...any) any {
		return strings.TrimLeftFunc(strArg("triml", oneArg("triml", args)), unicode.IsSpace)
	})
	defPrivate("-str-trimr", func(args ...any) any {
		return strings.TrimRightFunc(strArg("trimr", oneArg("trimr", args)), unicode.IsSpace)
	})
	defPrivate("-str-trim-newline", func(args ...any) any {
		return strings.TrimRight(strArg("trim-newline", oneArg("trim-newline", args)), "\n\r")
	})
	defPrivate("-str-reverse", func(args ...any) any {
		r := []rune(strArg("reverse", oneArg("reverse", args)))
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return string(r)
	})
	defPrivate("-str-blank?", func(args ...any) any {
		a := oneArg("blank?", args)
		if a == nil {
			return true
		}
		s, ok := a.(string)
		if !ok {
			panic(fmt.Errorf("blank? expects a string, got: %s", lang.PrintString(a)))
		}
		for _, r := range s {
			if !unicode.IsSpace(r) {
				return false
			}
		}
		return true
	})
	defPrivate("-str-starts-with?", func(args ...any) any {
		return strings.HasPrefix(strArg("starts-with?", args[0]), coerceStr("starts-with?", args[1]))
	})
	defPrivate("-str-ends-with?", func(args ...any) any {
		return strings.HasSuffix(strArg("ends-with?", args[0]), coerceStr("ends-with?", args[1]))
	})
	defPrivate("-str-includes?", func(args ...any) any {
		return strings.Contains(strArg("includes?", args[0]), coerceStr("includes?", args[1]))
	})
	// -str-index-of / -str-last-index-of: (… s needle) or (… s needle from).
	// needle is a string or char. Return the byte index or nil.
	defPrivate("-str-index-of", func(args ...any) any {
		return indexOf("index-of", args, false)
	})
	defPrivate("-str-last-index-of", func(args ...any) any {
		return indexOf("last-index-of", args, true)
	})

	// -str-split: (-str-split re s limit) => vector, Java Pattern.split
	// semantics (trailing empties dropped when limit == 0; limit > 0 caps
	// the piece count; limit < 0 keeps everything).
	defPrivate("-str-split", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: split", len(args)))
		}
		re := compilePattern("split", args[0])
		s := strArg("split", args[1])
		limit, ok := lang.AsInt(args[2])
		if !ok {
			panic(fmt.Errorf("split: limit must be an integer, got: %s", lang.PrintString(args[2])))
		}
		return javaSplit(re, s, int(limit))
	})

	// -str-replace / -str-replace-first: (… s match replacement).
	// match may be a string, char, or #"..." pattern; replacement a string
	// or char (a string when match is a pattern, with $1 group refs).
	defPrivate("-str-replace", func(args ...any) any {
		return replaceImpl("replace", args, false)
	})
	defPrivate("-str-replace-first", func(args ...any) any {
		return replaceImpl("replace-first", args, true)
	})
}

// matchResult renders a matcher's current match the way clojure.core does:
// the whole-match string when the pattern has no capturing groups, else a
// vector [whole g1 g2 …] with nil for any unmatched optional group.
func matchResult(m *lang.RegexpMatcher) any {
	gc := m.GroupCount()
	if gc == 0 {
		return m.Group()
	}
	parts := make([]any, gc+1)
	for i := 0; i <= gc; i++ {
		parts[i] = m.GroupInt(i)
	}
	return lang.NewVector(parts...)
}

// compilePattern turns a reader.Regex (or raw pattern string) into a
// compiled RE2 regexp via the shared cache.
func compilePattern(op string, a any) *regexp.Regexp {
	switch v := a.(type) {
	case reader.Regex:
		return lang.CachedCompileRegexp(v.Pattern)
	case string:
		return lang.CachedCompileRegexp(v)
	default:
		panic(fmt.Errorf("%s expects a pattern (#\"...\"), got: %s", op, lang.PrintString(a)))
	}
}

// strArg requires a string argument.
func strArg(op string, a any) string {
	s, ok := a.(string)
	if !ok {
		panic(fmt.Errorf("%s expects a string, got: %s", op, lang.PrintString(a)))
	}
	return s
}

// coerceStr accepts a string or a char (single-rune string).
func coerceStr(op string, a any) string {
	switch v := a.(type) {
	case string:
		return v
	case lang.Char:
		return string(rune(v))
	default:
		panic(fmt.Errorf("%s expects a string or char, got: %s", op, lang.PrintString(a)))
	}
}

// indexOf backs clojure.string/index-of and last-index-of.
func indexOf(op string, args []any, last bool) any {
	if len(args) != 2 && len(args) != 3 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	s := strArg(op, args[0])
	needle := coerceStr(op, args[1])
	var idx int
	if len(args) == 3 {
		from, ok := lang.AsInt(args[2])
		if !ok {
			panic(fmt.Errorf("%s: from-index must be an integer, got: %s", op, lang.PrintString(args[2])))
		}
		if last {
			// search the prefix up to and including from
			end := int(from) + len(needle)
			if end > len(s) {
				end = len(s)
			}
			if int(from) < 0 {
				return nil
			}
			idx = strings.LastIndex(s[:end], needle)
		} else {
			if int(from) < 0 {
				from = 0
			}
			if int(from) > len(s) {
				return nil
			}
			rel := strings.Index(s[from:], needle)
			if rel < 0 {
				return nil
			}
			idx = rel + int(from)
		}
	} else if last {
		idx = strings.LastIndex(s, needle)
	} else {
		idx = strings.Index(s, needle)
	}
	if idx < 0 {
		return nil
	}
	return int64(idx)
}

// javaSplit ports java.util.regex.Pattern.split(input, limit), which is
// what clojure.string/split delegates to.
func javaSplit(re *regexp.Regexp, s string, limit int) *lang.Vector {
	matches := re.FindAllStringIndex(s, -1)
	matchLimited := limit > 0
	var pieces []string
	index := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		if !matchLimited || len(pieces) < limit-1 {
			// no empty leading piece for a zero-width match at position 0
			if index == 0 && start == 0 && start == end {
				continue
			}
			pieces = append(pieces, s[index:start])
			index = end
		} else if len(pieces) == limit-1 {
			pieces = append(pieces, s[index:])
			index = end
		}
	}
	if index == 0 {
		return lang.NewVector(s)
	}
	if !matchLimited || len(pieces) < limit {
		pieces = append(pieces, s[index:])
	}
	resultSize := len(pieces)
	if limit == 0 {
		for resultSize > 0 && pieces[resultSize-1] == "" {
			resultSize--
		}
	}
	out := make([]any, resultSize)
	for i := 0; i < resultSize; i++ {
		out[i] = pieces[i]
	}
	return lang.NewVector(out...)
}

// replaceImpl backs clojure.string/replace and replace-first. It dispatches
// on the match type: string/char => literal replacement; #"..." => regex
// replacement with $1 group references (Go/RE2 $-expansion).
func replaceImpl(op string, args []any, first bool) any {
	if len(args) != 3 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), op))
	}
	s := strArg(op, args[0])
	switch match := args[1].(type) {
	case reader.Regex:
		re := lang.CachedCompileRegexp(match.Pattern)
		repl := strArg(op, args[2])
		if !first {
			return re.ReplaceAllString(s, repl)
		}
		loc := re.FindStringSubmatchIndex(s)
		if loc == nil {
			return s
		}
		return s[:loc[0]] + string(re.ExpandString(nil, repl, s, loc)) + s[loc[1]:]
	case string, lang.Char:
		m := coerceStr(op, match)
		r := coerceStr(op, args[2])
		if first {
			return strings.Replace(s, m, r, 1)
		}
		return strings.ReplaceAll(s, m, r)
	default:
		panic(fmt.Errorf("%s: match must be a string, char, or pattern, got: %s", op, lang.PrintString(args[1])))
	}
}
