package reader

// Phase 1 dispatch forms (design/01-reader.md §1): #(...) anonymous
// fn, #"..." regex, ## symbolic values. (#'x is readWrapped("var");
// #^ reuses readMeta.)

import (
	"fmt"
	"math"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Regex is the value of a #"pattern" literal: the RAW pattern text,
// exactly as written between the quotes (backslash escapes preserved).
// Per design/01-reader.md §4 we do NOT compile in the reader — Go's
// RE2 rejects Java-regex features (backreferences, lookbehind) that a
// program may never actually run; compilation happens lazily at
// runtime (e.g. via lang.CachedCompileRegexp). pkg/lang has no
// dedicated regex value type (its printer special-cases
// *regexp.Regexp only), so the raw-pattern type lives here.
type Regex struct {
	Pattern string
}

// String prints like Clojure's pr on a Pattern: #"pattern" with the
// pattern source verbatim. CLI check: (pr (read-string "#\"a\\\"b\""))
// => #"a\"b" (the backslash-escape is preserved, not re-escaped).
func (re Regex) String() string {
	return `#"` + re.Pattern + `"`
}

// readRegex ports LispReader.RegexReader: consume runes until an
// unescaped '"', keeping backslash + following rune verbatim.
func (r *Reader) readRegex(start Position) (any, error) {
	var b strings.Builder
	for {
		c, err := r.s.Read()
		if err != nil {
			return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w regex", ErrIncomplete)}
		}
		if c == '"' {
			return Regex{Pattern: b.String()}, nil
		}
		b.WriteRune(c)
		if c == '\\' {
			c, err = r.s.Read()
			if err != nil {
				return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w regex", ErrIncomplete)}
			}
			b.WriteRune(c)
		}
	}
}

// readSymbolicValue ports LispReader.SymbolicValueReader (##Inf,
// ##-Inf, ##NaN). CLI checks: (read-string "##Inf") => Infinity;
// "##Foo" => "Unknown symbolic value: ##Foo"; ##"x" (non-symbol) =>
// "Invalid token: ##...".
func (r *Reader) readSymbolicValue(start Position) (any, error) {
	form, err := r.readForm()
	if err != nil {
		if err == ErrEOF {
			return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w symbolic value", ErrIncomplete)}
		}
		return nil, err
	}
	sym, ok := form.(*lang.Symbol)
	if !ok {
		return nil, r.errAt(start, "Invalid token: ##%s", lang.PrintString(form))
	}
	switch sym.FullName() {
	case "Inf":
		return math.Inf(1), nil
	case "-Inf":
		return math.Inf(-1), nil
	case "NaN":
		return math.NaN(), nil
	}
	return nil, r.errAt(start, "Unknown symbolic value: ##%s", sym.FullName())
}

// readFnLiteral ports LispReader.FnReader: #(body) => (fn* [args] body)
// with %/%1..%n/%& registered in r.fnArgs while the body is read as a
// normal list ('(' is pushed back). Args are hygienic gensyms
// pN__<id># / rest__<id># — the trailing # is deliberate, it keeps
// `#(...) hygienic. CLI checks (clojure 1.12.5):
//
//	#(+ % %2)     => (fn* [p1__139# p2__140#] (+ p1__139# p2__140#))
//	#(apply + %&) => (fn* [& rest__143#] (apply + rest__143#))
//	#(do %3)      => (fn* [p1__147# p2__148# p3__146#] (do p3__146#))
//	#()           => (fn* [] ())
//	#(+ % %)      => (fn* [p1__153#] (+ p1__153# p1__153#))
//	#(#(inc %))   => error "Nested #()s are not allowed"
func (r *Reader) readFnLiteral(start Position) (any, error) {
	if r.fnArgs != nil {
		return nil, r.errAt(start, "Nested #()s are not allowed")
	}
	r.fnArgs = map[int]*lang.Symbol{}
	defer func() { r.fnArgs = nil }()

	r.s.Unread() // put '(' back; body reads as a normal list
	form, err := r.readForm()
	if err != nil {
		return nil, err
	}

	maxArg := 0
	for n := range r.fnArgs {
		if n > maxArg {
			maxArg = n
		}
	}
	var args []any
	for i := 1; i <= maxArg; i++ {
		s := r.fnArgs[i]
		if s == nil {
			// Gap in the used args (e.g. #(do %3)): mint an unused
			// positional param so arity is still 3.
			s = r.garg(i)
		}
		args = append(args, s)
	}
	if rest := r.fnArgs[-1]; rest != nil {
		args = append(args, symAmp, rest)
	}
	return lang.NewList(symFnStar, lang.NewVector(args...), form), nil
}

// garg mints a hygienic #() parameter symbol (LispReader.garg):
// p<n>__<id># for positional args, rest__<id># for %&.
func (r *Reader) garg(n int) *lang.Symbol {
	prefix := "rest"
	if n != -1 {
		prefix = fmt.Sprintf("p%d", n)
	}
	return lang.NewSymbol(fmt.Sprintf("%s__%d#", prefix, r.nextID()))
}

// readArg ports LispReader.ArgReader ('%'). Outside #(), % reads as a
// plain symbol token (CLI check: (read-string "%") => %; %foo is one
// symbol because % is non-terminating). Inside #(): % alone => arg 1,
// %& => rest arg, %<positive int> => arg n, anything else => error
// (CLI check: #(str %-1) => "arg literal must be %, %& or %integer").
func (r *Reader) readArg(start Position) (any, error) {
	if r.fnArgs == nil {
		return r.interpretToken(start, r.readToken('%'))
	}
	c, err := r.s.Read()
	if err != nil {
		return r.registerArg(1), nil
	}
	r.s.Unread()
	if isWhitespace(c) || isTerminatingMacro(c) {
		return r.registerArg(1), nil
	}
	n, err := r.readForm()
	if err != nil {
		return nil, err
	}
	if s, ok := n.(*lang.Symbol); ok && s.Equals(symAmp) {
		return r.registerArg(-1), nil
	}
	if i, ok := n.(int64); ok && i >= 1 {
		return r.registerArg(int(i)), nil
	}
	return nil, r.errAt(start, "arg literal must be %%, %%& or %%integer")
}

// registerArg returns the param symbol for arg n, minting it on first
// use so repeated % / %2 share one symbol.
func (r *Reader) registerArg(n int) *lang.Symbol {
	s := r.fnArgs[n]
	if s == nil {
		s = r.garg(n)
		r.fnArgs[n] = s
	}
	return s
}
