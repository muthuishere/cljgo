// Package reader implements the Clojure reader: UTF-8 text in,
// pkg/lang persistent data structures out, with :file/:line/:column/
// :end-line/:end-column metadata on every IObj form.
//
// Design contract: design/01-reader.md (this file implements Phase 0,
// plus auto-resolved :: keywords via the injected Resolver). The
// reader is dumb and faithful — it produces data, never evaluates.
package reader

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Resolver supplies namespace context to the reader (auto-resolved
// keywords now; syntax-quote symbol resolution in Phase 1). It is
// injected by the compiler so this package never depends on it.
// Mirror of clojure.lang.LispReader$Resolver per design/01-reader.md §3.
type Resolver interface {
	// CurrentNS returns the symbol naming the current namespace.
	CurrentNS() *lang.Symbol
	// ResolveAlias resolves a namespace alias to the symbol naming
	// the aliased namespace, or nil if the alias is unknown.
	ResolveAlias(sym *lang.Symbol) *lang.Symbol
	// ResolveVar resolves a symbol to the var it names, or nil.
	ResolveVar(sym *lang.Symbol) *lang.Symbol
	// ResolveType resolves a symbol naming a type ("class"), or nil.
	ResolveType(sym *lang.Symbol) *lang.Symbol
}

// Reader reads Clojure forms from a rune stream.
type Reader struct {
	s        *scanner
	resolver Resolver

	// nextID supplies gensym ids (auto-gensym x#, #() params); the
	// default is the package-global atomic counter (NextID).
	nextID func() int64
	// gensymEnv maps x# names to minted symbols; non-nil only while
	// expanding a syntax-quote, fresh per ` (design/01-reader.md §2).
	gensymEnv map[string]*lang.Symbol
	// fnArgs maps %-arg numbers (%& = -1) to param symbols; non-nil
	// only inside #().
	fnArgs map[int]*lang.Symbol
	// sqDepth tracks syntax-quote nesting for the depth limit.
	sqDepth int
}

// Option configures a Reader.
type Option func(*Reader)

// WithFilename sets the file name recorded in position metadata and
// error messages.
func WithFilename(file string) Option {
	return func(r *Reader) { r.s.setFile(file) }
}

// WithResolver injects the namespace resolver used for auto-resolved
// (::) keywords and syntax-quote symbol resolution.
func WithResolver(res Resolver) Option {
	return func(r *Reader) { r.resolver = res }
}

// New creates a Reader over rs.
func New(rs io.RuneScanner, opts ...Option) *Reader {
	r := &Reader{s: newScanner(rs, "NO_SOURCE_FILE"), nextID: NextID}
	for _, o := range opts {
		o(r)
	}
	return r
}

// ReadString reads a single form from src (convenience wrapper used
// by tests, the REPL and the conformance harness).
func ReadString(src string, opts ...Option) (any, error) {
	return New(strings.NewReader(src), opts...).ReadOne()
}

// ReadOne reads and returns the next form. It returns ErrEOF when the
// input is cleanly exhausted (only whitespace/comments remained).
func (r *Reader) ReadOne() (any, error) {
	return r.readForm()
}

// ReadAll reads all remaining forms until clean EOF.
func (r *Reader) ReadAll() ([]any, error) {
	var forms []any
	for {
		f, err := r.readForm()
		if errors.Is(err, ErrEOF) {
			return forms, nil
		}
		if err != nil {
			return nil, err
		}
		forms = append(forms, f)
	}
}

func (r *Reader) errAt(pos Position, format string, args ...any) error {
	return &Error{Pos: pos, Err: fmt.Errorf(format, args...)}
}

func isWhitespace(c rune) bool {
	// Clojure whitespace: Unicode whitespace plus comma.
	return unicode.IsSpace(c) || c == ','
}

// isMacro reports whether c is a reader macro character
// (LispReader.macros table).
func isMacro(c rune) bool {
	switch c {
	case '"', ';', '\'', '@', '^', '`', '~', '(', ')', '[', ']', '{', '}', '\\', '%', '#':
		return true
	}
	return false
}

// isTerminatingMacro reports whether c terminates a token: every
// macro char except # ' % (so a# a'b a%b read as single symbols).
func isTerminatingMacro(c rune) bool {
	return isMacro(c) && c != '#' && c != '\'' && c != '%'
}

func isDigit(c rune) bool { return c >= '0' && c <= '9' }

func (r *Reader) skipWhitespace() {
	for {
		c, err := r.s.Read()
		if err != nil {
			return
		}
		if !isWhitespace(c) {
			r.s.Unread()
			return
		}
	}
}

// skipLine consumes runes through the next newline (or EOF).
func (r *Reader) skipLine() {
	for {
		c, err := r.s.Read()
		if err != nil || c == '\n' {
			return
		}
	}
}

// peekIsDigit reports whether the next rune is an ASCII digit,
// without consuming it.
func (r *Reader) peekIsDigit() bool {
	c, err := r.s.Read()
	if err != nil {
		return false
	}
	r.s.Unread()
	return isDigit(c)
}

// annotate attaches position metadata to IObj forms
// (design/00-architecture.md §4.5): :file :line :column :end-line
// :end-column, where the end position is exclusive (the position just
// past the form). Non-IObj values (numbers, strings, keywords, ...)
// pass through; the analyzer inherits the enclosing form's position.
func (r *Reader) annotate(form any, start Position) any {
	iobj, ok := form.(lang.IObj)
	if !ok {
		return form
	}
	end := r.s.Pos()
	var m any = iobj.Meta()
	m = lang.Assoc(m, lang.KWFile, start.File)
	m = lang.Assoc(m, lang.KWLine, int64(start.Line))
	m = lang.Assoc(m, lang.KWColumn, int64(start.Col))
	m = lang.Assoc(m, lang.KWEndLine, int64(end.Line))
	m = lang.Assoc(m, lang.KWEndColumn, int64(end.Col))
	return iobj.WithMeta(m.(lang.IPersistentMap))
}

// readForm reads exactly one form, skipping whitespace, comments and
// #_ discards. It returns ErrEOF only at clean end of input.
func (r *Reader) readForm() (any, error) {
	for {
		r.skipWhitespace()
		start := r.s.Pos()
		c, err := r.s.Read()
		if err != nil {
			if err == io.EOF {
				return nil, ErrEOF
			}
			return nil, r.errAt(start, "read error: %v", err)
		}

		var form any
		switch {
		case c == '(':
			form, err = r.readList(start)
		case c == '[':
			form, err = r.readVector(start)
		case c == '{':
			form, err = r.readMapLiteral(start)
		case c == ')' || c == ']' || c == '}':
			return nil, r.errAt(start, "Unmatched delimiter: %c", c)
		case c == '"':
			form, err = r.readString(start)
		case c == '\\':
			form, err = r.readChar(start)
		case c == '\'':
			form, err = r.readWrapped(start, "quote")
		case c == '@':
			form, err = r.readWrapped(start, "clojure.core/deref")
		case c == '^':
			form, err = r.readMeta(start)
		case c == ';':
			r.skipLine()
			continue
		case c == '#':
			var again bool
			form, again, err = r.readDispatch(start)
			if err == nil && again {
				continue
			}
		case c == '`':
			form, err = r.readSyntaxQuote(start)
		case c == '~':
			form, err = r.readUnquote(start)
		case c == '%':
			form, err = r.readArg(start)
		case isDigit(c):
			r.s.Unread()
			form, err = r.readNumber(start, "")
		case (c == '+' || c == '-') && r.peekIsDigit():
			form, err = r.readNumber(start, string(c))
		default:
			form, err = r.interpretToken(start, r.readToken(c))
		}
		if err != nil {
			return nil, err
		}
		return r.annotate(form, start), nil
	}
}

// readToken accumulates a token starting with first, stopping before
// whitespace or a terminating macro character.
func (r *Reader) readToken(first rune) string {
	var b strings.Builder
	b.WriteRune(first)
	for {
		c, err := r.s.Read()
		if err != nil {
			break
		}
		if isWhitespace(c) || isTerminatingMacro(c) {
			r.s.Unread()
			break
		}
		b.WriteRune(c)
	}
	return b.String()
}

// readWrapped reads the next form and wraps it as (sym form) — used
// for 'x => (quote x) and @x => (clojure.core/deref x).
func (r *Reader) readWrapped(start Position, sym string) (any, error) {
	form, err := r.readForm()
	if errors.Is(err, ErrEOF) {
		return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: ErrIncomplete}
	}
	if err != nil {
		return nil, err
	}
	return lang.NewList(lang.NewSymbol(sym), form), nil
}

// readDispatch handles the # dispatch character. again=true means the
// caller should continue its read loop (comment or discard consumed).
func (r *Reader) readDispatch(start Position) (form any, again bool, err error) {
	c, err := r.s.Read()
	if err != nil {
		return nil, false, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w dispatch character", ErrIncomplete)}
	}
	switch c {
	case '{':
		f, err := r.readSet(start)
		return f, false, err
	case '_':
		// Discard the next form; stacked #_#_ works because the
		// discarded form is read with full recursion.
		if _, err := r.readForm(); err != nil {
			if errors.Is(err, ErrEOF) {
				return nil, false, &Error{Pos: r.s.Pos(), Start: &start, Err: ErrIncomplete}
			}
			return nil, false, err
		}
		return nil, true, nil
	case '!':
		// #! line comment (shebang).
		r.skipLine()
		return nil, true, nil
	case '<':
		return nil, false, r.errAt(start, "Unreadable form")
	case '\'':
		// #'x => (var x). CLI check: (read-string "#'x") => (var x).
		f, err := r.readWrapped(start, "var")
		return f, false, err
	case '(':
		f, err := r.readFnLiteral(start)
		return f, false, err
	case '"':
		f, err := r.readRegex(start)
		return f, false, err
	case '^':
		// #^ legacy metadata, alias of ^. CLI check:
		// (meta (read-string "#^:private [a]")) => {:private true}.
		f, err := r.readMeta(start)
		return f, false, err
	case '#':
		f, err := r.readSymbolicValue(start)
		return f, false, err
	case '?', ':', '=':
		return nil, false, r.errAt(start, "#%c reader macro is not yet implemented (reader Phase 2)", c)
	default:
		// A letter after # begins a tagged literal (#tag form). cljgo's
		// Result/Option values print/read as #cljgo/ok, #cljgo/err,
		// #cljgo/just, #cljgo/none (ADR 0014 D4).
		if isSymbolLead(c) {
			r.s.Unread()
			f, err := r.readTaggedLiteral(start)
			return f, false, err
		}
		return nil, false, r.errAt(start, "No dispatch macro for: %c", c)
	}
}

// isSymbolLead reports whether c can begin a reader tag symbol (a subset
// of Clojure's symbol-lead chars, enough for #cljgo/... tags).
func isSymbolLead(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		c == '_' || c == '.' || c == '*' || c == '+' || c == '!' ||
		c == '-' || c == '?' || c == '$' || c == '%' || c == '&' || c == '='
}

// readTaggedLiteral reads `tag form` (the leading '#' already consumed by
// readDispatch) and constructs the value for a known tag. cljgo owns the
// namespaced `cljgo/...` tags for Result/Option (ADR 0014); other tags
// are not yet supported.
func (r *Reader) readTaggedLiteral(start Position) (any, error) {
	tagForm, err := r.readForm()
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w tagged literal", ErrIncomplete)}
		}
		return nil, err
	}
	sym, ok := tagForm.(*lang.Symbol)
	if !ok {
		return nil, r.errAt(start, "Reader tag must be a symbol")
	}
	val, err := r.readForm()
	if err != nil {
		if errors.Is(err, ErrEOF) {
			return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w tagged literal %s", ErrIncomplete, sym.FullName())}
		}
		return nil, err
	}
	switch sym.FullName() {
	case "cljgo/ok":
		return lang.NewOk(val), nil
	case "cljgo/err":
		return lang.NewErr(val), nil
	case "cljgo/just":
		return lang.NewJust(val), nil
	case "cljgo/none":
		// The following form is ignored by convention (none is the
		// nullary sentinel; it also prints simply as `none`).
		return lang.None, nil
	}
	return nil, r.errAt(start, "No reader function for tag #%s", sym.FullName())
}

// readDelimited reads forms until the closing delimiter end. start is
// the position of the opening delimiter, used for unterminated-form
// errors ("the single most useful reader error" — design/01-reader.md §3).
func (r *Reader) readDelimited(what string, end rune, start Position) ([]any, error) {
	unterminated := func(pos Position) error {
		return &Error{
			Pos:   pos,
			Start: &start,
			Err:   fmt.Errorf("%w %s, expected %q to close it", ErrIncomplete, what, string(end)),
		}
	}
	var forms []any
	for {
		r.skipWhitespace()
		pos := r.s.Pos()
		c, err := r.s.Read()
		if err != nil {
			return nil, unterminated(pos)
		}
		if c == end {
			return forms, nil
		}
		r.s.Unread()
		f, err := r.readForm()
		if errors.Is(err, ErrEOF) {
			// A comment or discard consumed the rest of the input.
			return nil, unterminated(r.s.Pos())
		}
		if err != nil {
			return nil, err
		}
		forms = append(forms, f)
	}
}

func (r *Reader) readList(start Position) (any, error) {
	forms, err := r.readDelimited("list", ')', start)
	if err != nil {
		return nil, err
	}
	return lang.NewList(forms...), nil
}

func (r *Reader) readVector(start Position) (any, error) {
	forms, err := r.readDelimited("vector", ']', start)
	if err != nil {
		return nil, err
	}
	return lang.NewVector(forms...), nil
}

func (r *Reader) readMapLiteral(start Position) (any, error) {
	forms, err := r.readDelimited("map", '}', start)
	if err != nil {
		return nil, err
	}
	if len(forms)%2 != 0 {
		return nil, r.errAt(start, "Map literal must contain an even number of forms")
	}
	if err := r.checkDuplicates(start, forms, 2); err != nil {
		return nil, err
	}
	return lang.NewMap(forms...), nil
}

func (r *Reader) readSet(start Position) (any, error) {
	forms, err := r.readDelimited("set", '}', start)
	if err != nil {
		return nil, err
	}
	if err := r.checkDuplicates(start, forms, 1); err != nil {
		return nil, err
	}
	return lang.NewSet(forms...), nil
}

// checkDuplicates rejects duplicate literal keys in map (step 2) and
// set (step 1) literals. CLI checks: (read-string "{:a 1 :a 2}") =>
// "Duplicate key: :a"; (read-string "#{1 1}") => "Duplicate key: 1".
func (r *Reader) checkDuplicates(start Position, forms []any, step int) error {
	seen := lang.NewPersistentHashMap()
	for i := 0; i < len(forms); i += step {
		if seen.ContainsKey(forms[i]) {
			return r.errAt(start, "Duplicate key: %s", lang.PrintString(forms[i]))
		}
		seen = lang.Assoc(seen, forms[i], true).(lang.IPersistentMap)
	}
	return nil
}

// readMeta reads ^meta form. Shorthands per LispReader.MetaReader:
// keyword => {:kw true}; symbol or string => {:tag v}; vector =>
// {:param-tags v} (CLI check on Clojure 1.12.5:
// (meta (read-string "^[String java.lang.Long] x")) =>
// {:param-tags [String java.lang.Long]}). The metadata entries are
// assoc'd over the target's existing meta, so with stacked metadata
// the OUTER ^ wins: CLI check:
// (meta (read-string "^{:a 1} ^{:a 2} x")) => {:a 1}.
func (r *Reader) readMeta(start Position) (any, error) {
	metaPos := r.s.Pos()
	m, err := r.readForm()
	if errors.Is(err, ErrEOF) {
		return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w metadata", ErrIncomplete)}
	}
	if err != nil {
		return nil, err
	}

	var mmap lang.IPersistentMap
	switch mv := m.(type) {
	case lang.Keyword:
		mmap = lang.NewMap(mv, true)
	case *lang.Symbol:
		mmap = lang.NewMap(lang.KWTag, mv)
	case string:
		mmap = lang.NewMap(lang.KWTag, mv)
	case lang.IPersistentVector:
		mmap = lang.NewMap(kwParamTags, mv)
	case lang.IPersistentMap:
		mmap = mv
	default:
		return nil, r.errAt(metaPos, "Metadata must be Symbol,Keyword,String,Map or Vector")
	}

	form, err := r.readForm()
	if errors.Is(err, ErrEOF) {
		return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w metadata", ErrIncomplete)}
	}
	if err != nil {
		return nil, err
	}
	iobj, ok := form.(lang.IObj)
	if !ok {
		// CLI check: (read-string "^:kw 5") =>
		// "Metadata can only be applied to IMetas".
		return nil, r.errAt(start, "Metadata can only be applied to IMetas")
	}

	var ometa any = iobj.Meta()
	for seq := lang.Seq(mmap); seq != nil; seq = seq.Next() {
		e := seq.First().(lang.IMapEntry)
		ometa = lang.Assoc(ometa, e.Key(), e.Val())
	}
	return iobj.WithMeta(ometa.(lang.IPersistentMap)), nil
}

var kwParamTags = lang.NewKeyword("param-tags")

// symbolPat ports LispReader.symbolPat:
// [:]?([\D&&[^/]].*/)?(/|[\D&&[^/]][^/]*) — Java's \D∩[^/] becomes
// [^0-9/] in Go (RE2 has no class intersection).
var symbolPat = regexp.MustCompile(`^:?([^0-9/].*/)?(/|[^0-9/][^/]*)$`)

// interpretToken turns a non-number token into nil/true/false, a
// keyword, or a symbol.
func (r *Reader) interpretToken(start Position, tok string) (any, error) {
	switch tok {
	case "nil":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return r.matchSymbol(start, tok)
}

// newSymbolSafe converts lang.NewSymbol's validation panic into an
// ok=false. Our symbolPat validation makes panics unreachable for the
// tokens we pass; this is a safety net only (the reader must return
// errors, never panic — design/01-reader.md §4).
func newSymbolSafe(s string) (sym *lang.Symbol, ok bool) {
	defer func() {
		if recover() != nil {
			sym, ok = nil, false
		}
	}()
	return lang.NewSymbol(s), true
}

// matchSymbol ports LispReader.matchSymbol (keywords + symbols).
func (r *Reader) matchSymbol(start Position, tok string) (any, error) {
	invalid := func() (any, error) {
		return nil, r.errAt(start, "Invalid token: %s", tok)
	}
	m := symbolPat.FindStringSubmatch(tok)
	if m == nil {
		return invalid()
	}
	ns, name := m[1], m[2]
	if strings.HasSuffix(ns, ":/") || strings.HasSuffix(name, ":") ||
		strings.Contains(tok[1:], "::") {
		return invalid()
	}

	if strings.HasPrefix(tok, "::") {
		// Auto-resolved keyword: ::foo in the current namespace,
		// ::alias/foo through a namespace alias. CLI check:
		// (read-string "::foo") => :user/foo. Requires the injected
		// Resolver.
		rest := tok[2:]
		if r.resolver == nil {
			return nil, r.errAt(start, "Invalid token (auto-resolved keyword requires a resolver): %s", tok)
		}
		if i := strings.Index(rest, "/"); i >= 0 {
			aliasSym, ok := newSymbolSafe(rest[:i])
			if !ok {
				return invalid()
			}
			nsSym := r.resolver.ResolveAlias(aliasSym)
			if nsSym == nil {
				return invalid()
			}
			return lang.NewKeyword(nsSym.Name() + "/" + rest[i+1:]), nil
		}
		nsSym := r.resolver.CurrentNS()
		if nsSym == nil {
			return invalid()
		}
		return lang.NewKeyword(nsSym.Name() + "/" + rest), nil
	}

	if tok[0] == ':' {
		// Plain keyword: NOT namespace-qualified (:foo stays :foo —
		// the Glojure bug we fix; design/01-reader.md §4). Built with
		// lang.NewKeyword directly, not via Symbol: real Clojure
		// accepts keywords like :3 and :foo:bar that Symbol
		// validation would reject. CLI checks: (read-string ":3") =>
		// :3; (read-string ":foo:bar") => :foo:bar; (read-string
		// ":foo") => :foo with nil namespace.
		return lang.NewKeyword(tok[1:]), nil
	}

	sym, ok := newSymbolSafe(tok)
	if !ok {
		return invalid()
	}
	return sym, nil
}
