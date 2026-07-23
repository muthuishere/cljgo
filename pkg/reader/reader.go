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
	// tagSuppress > 0 while reading the body of a reader conditional: an
	// unknown tagged literal (e.g. jank's #cpp, in a branch cljgo elides)
	// reads as nil instead of erroring, matching Clojure's suppress-read of
	// unselected branches. See readConditional / readTaggedLiteral.
	tagSuppress int

	// condMode is the reader-conditional policy (readcond.go). File loads
	// and the REPL keep the zero value condAllow — cljgo processes
	// conditionals in normal reading, a documented divergence from the
	// JVM's .cljc-only gate (ADR 0068 addendum). clojure.core/read-string
	// mirrors the JVM's opts protocol: condForbid without {:read-cond
	// :allow/:preserve}, condPreserve for {:read-cond :preserve}.
	condMode int
	// condFeatures holds extra feature keywords a caller supplied via
	// WithFeatures ({:features #{...}}); the platform feature :cljgo and
	// :default always match regardless (mirroring the JVM, which always
	// includes its :clj — oracle 1.12.5: (read-string {:read-cond :allow
	// :features #{:cljs}} "#?(:clj 2)") => 2).
	condFeatures map[lang.Keyword]bool
	// tagPreserve > 0 while reading the body of a PRESERVED conditional:
	// every tagged literal — even a known one like #inst — reads as a
	// lang.TaggedLiteral value instead of resolving (oracle 1.12.5:
	// (read-string {:read-cond :preserve} "#?(:clj #inst \"...\")") keeps
	// a TaggedLiteral in :form). See readConditional / readTaggedLiteral.
	tagPreserve int

	// ednStrict enables clojure.edn's tighter reader rules (WithEDNStrict):
	// real JVM clojure.edn is a RESTRICTED reader, stricter than
	// clojure.core's LispReader in a few specific spots (oracle-verified,
	// clojure-test-suite edn_test/read_string.cljc): `#!` is not a
	// registered comment macro there (throws "No dispatch macro for: !"
	// instead of consuming a shebang line), and `::kw` auto-resolution
	// throws "Invalid token" (edn has no notion of a current namespace).
	// cljgo additionally rejects 3+-part (two-slash) symbols/keywords in
	// this mode as a DEVIATION from real JVM edn (which is lenient there);
	// the suite's own :default branch expects the stricter behavior. Only
	// clojure.edn/read-string sets this; clojure.core's reader (REPL, file
	// loads, syntax-quote, ...) is completely unaffected.
	ednStrict bool

	// tagReaders backs clojure.edn/read-string's `:readers` option
	// (map[tag-full-name]lang.IFn): consulted BEFORE the built-in
	// #uuid/#inst/#cljgo/... tags, so a caller-supplied reader can override
	// them (oracle: (edn/read-string {:readers {'uuid (constantly
	// :override)}} "#uuid \"...\"") => :override). nil outside the opts
	// arity.
	tagReaders map[string]lang.IFn
	// defaultReader backs clojure.edn/read-string's `:default` option: an
	// (fn [tag value]) invoked for a tag with no built-in handler, no
	// *data-readers* entry, and no :readers override. nil outside the opts
	// arity (or when :default wasn't supplied), in which case an unhandled
	// tag is a reader error as usual.
	defaultReader lang.IFn
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

// WithEDNStrict enables clojure.edn's restricted-reader rules (see the
// Reader.ednStrict field doc). Used only by clojure.edn/read-string.
func WithEDNStrict() Option {
	return func(r *Reader) { r.ednStrict = true }
}

// WithReadCondForbid makes a reader conditional a read error
// ("Conditional read not allowed"), the JVM's default outside .cljc
// files and read-string {:read-cond :allow/:preserve}. Used by
// clojure.core/read-string when opts carry no :read-cond permission.
func WithReadCondForbid() Option {
	return func(r *Reader) { r.condMode = condForbid }
}

// WithReadCondPreserve makes a reader conditional read as a
// lang.ReaderConditional data value instead of selecting a branch
// (read-string {:read-cond :preserve}, ADR 0050). Tagged literals
// inside the preserved body read as lang.TaggedLiteral values.
func WithReadCondPreserve() Option {
	return func(r *Reader) { r.condMode = condPreserve }
}

// WithFeatures adds feature keywords a matching branch may select, on
// top of the always-present platform feature :cljgo and :default
// (read-string's {:features #{...}} option).
func WithFeatures(feats ...lang.Keyword) Option {
	return func(r *Reader) {
		if r.condFeatures == nil {
			r.condFeatures = make(map[lang.Keyword]bool, len(feats))
		}
		for _, f := range feats {
			r.condFeatures[f] = true
		}
	}
}

// WithTagReaders installs a tag -> reader-fn table backing
// clojure.edn/read-string's `:readers` option, keyed by the tag symbol's
// full name (e.g. "my/foo", or "uuid"/"inst" to override a built-in tag).
func WithTagReaders(readers map[string]lang.IFn) Option {
	return func(r *Reader) { r.tagReaders = readers }
}

// WithDefaultReader installs the fallback (fn [tag value]) backing
// clojure.edn/read-string's `:default` option.
func WithDefaultReader(fn lang.IFn) Option {
	return func(r *Reader) { r.defaultReader = fn }
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

// skipLine consumes runes through the next line terminator (or EOF). Both
// \n and \r end a `;` comment (oracle 1.12.5: (read-string ";foo\r3\n5")
// => 3 — a bare \r terminates the comment same as \n; cljgo previously
// only stopped at \n, so a \r-only line ending swallowed the next form
// too).
func (r *Reader) skipLine() {
	for {
		c, err := r.s.Read()
		if err != nil || c == '\n' || c == '\r' {
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

// annotate attaches position metadata to LIST (seq) forms and SYMBOLS
// (design/00-architecture.md §4.5, narrowed by ADR 0038): :file :line
// :column :end-line :end-column, where the end position is exclusive
// (the position just past the form). Vector/map/set literals and
// non-IObj scalars pass through clean, matching JVM Clojure, where
// data-structure literals carry no reader position metadata (oracle
// 1.12.5: file-loaded (meta '(1 2)) => {:line 1 :column 9}, (meta [1 2])
// / (meta {:a 1}) / (meta #{1}) => nil) — user ^ metadata must not be
// polluted with position keys (group_by.cljc). Symbols keep positions
// as a DELIBERATE deviation (JVM symbols read meta-free): they are what
// `cljgo check` diagnostics point at (A2001's exact column), and no
// conformance behavior observes symbol metadata.
func (r *Reader) annotate(form any, start Position) any {
	switch form.(type) {
	case lang.ISeq, *lang.Symbol:
		// annotated below
	default:
		return form
	}
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

// sentinel is a unique internal marker value returned by readWithDelim.
type sentinel struct{ name string }

// sentReadFinished is returned by readWithDelim when it consumed the
// enclosing collection's closing delimiter in place of a form. It
// mirrors clojure.lang.LispReader's READ_FINISHED: it lets the reader
// loop over discards (#_) and non-matching reader conditionals (#?)
// that sit immediately before the closer without misreading the closer
// as an "Unmatched delimiter".
var sentReadFinished = &sentinel{"read-finished"}

// spliceForms carries the forms produced by a matched splicing reader
// conditional (#?@); readDelimited splices them into the enclosing
// collection. It never escapes to a top-level (non-collection) read.
type spliceForms struct{ items []any }

// readForm reads exactly one form, skipping whitespace, comments and
// #_ discards. It returns ErrEOF only at clean end of input. It is the
// non-collection entry point: splicing (#?@) is disallowed here.
func (r *Reader) readForm() (any, error) {
	return r.readWithDelim(0, false, false)
}

// readWithDelim reads one form. When hasDelim is set and the reader
// encounters returnOn (a collection's closing delimiter), it returns
// sentReadFinished instead of a form. spliceOK reports whether a
// splicing reader conditional (#?@) is permitted here (only directly
// inside a collection).
func (r *Reader) readWithDelim(returnOn rune, hasDelim, spliceOK bool) (any, error) {
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
		if hasDelim && c == returnOn {
			return sentReadFinished, nil
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
			form, again, err = r.readDispatch(start, spliceOK)
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
		if _, ok := form.(spliceForms); ok {
			// The splice sentinel is handled by readDelimited; it is
			// not an IObj and must not be annotated.
			return form, nil
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
// caller should continue its read loop (comment, discard, or a reader
// conditional with no matching branch was consumed). spliceOK reports
// whether a splicing reader conditional (#?@) is allowed in this
// position (only directly inside a collection).
func (r *Reader) readDispatch(start Position, spliceOK bool) (form any, again bool, err error) {
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
		// #! line comment (shebang) — a cljgo core-reader extension for
		// script files (CLI check: (read-string "#!shebang\n42") => 42).
		// clojure.edn's restricted reader has no such macro at all (oracle:
		// (clojure.edn/read-string "#!shebang") throws "No dispatch macro
		// for: !"), so ednStrict rejects it outright instead of comment-
		// skipping.
		if r.ednStrict {
			return nil, false, r.errAt(start, "No dispatch macro for: !")
		}
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
	case '?':
		// Reader conditional: #?(...) selecting, #?@(...) splicing.
		return r.readConditional(start, spliceOK)
	case ':':
		// Namespaced map: #:ns{...}, #::{...}, #::alias{...}.
		f, err := r.readNamespacedMap(start)
		return f, false, err
	case '=':
		return nil, false, r.errAt(start, "#= reader macro is not yet implemented (reader Phase 2)")
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
	// Inside a PRESERVED reader conditional ({:read-cond :preserve}),
	// every tagged literal — even a known #uuid/#inst — reads as a
	// lang.TaggedLiteral data value instead of resolving (oracle 1.12.5:
	// (read-string {:read-cond :preserve} "#?(:clj #inst \"...\")") keeps
	// a TaggedLiteral in :form). See readConditional.
	if r.tagPreserve > 0 {
		return lang.NewTaggedLiteral(sym, val), nil
	}
	// clojure.edn/read-string's `:readers` option (WithTagReaders) takes
	// priority over EVERYTHING below, including the built-in #uuid/#inst
	// tags (oracle: (edn/read-string {:readers {'uuid (constantly
	// :override)}} "#uuid \"...\"") => :override, not a real UUID).
	if r.tagReaders != nil {
		if fn, ok := r.tagReaders[sym.FullName()]; ok {
			return fn.Invoke(val), nil
		}
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
	// A program-registered *data-readers* entry overrides even the built-in
	// #uuid/#inst tags (oracle 1.12.5: (binding [*data-readers* {'inst (fn
	// [v] [:inst v])}] (read-string "#inst \"2020-01-01\"")) => [:inst
	// "2020-01-01"]) — but only for the CORE reader: clojure.edn never
	// consults *data-readers* on the JVM, so the edn-strict mode skips it.
	// The cljgo-owned cljgo/* tags above stay ours (ADR 0014).
	if !r.ednStrict {
		if fn, ok := dataReaderFor(sym); ok {
			return fn.Invoke(val), nil
		}
	}
	switch sym.FullName() {
	case "uuid":
		s, ok := val.(string)
		if !ok {
			return nil, r.errAt(start, "UUID literal expects a string, not %s", lang.PrintString(val))
		}
		u, ok := NewUUID(s)
		if !ok {
			return nil, r.errAt(start, "Invalid UUID string: %s", s)
		}
		return u, nil
	case "inst":
		s, ok := val.(string)
		if !ok {
			return nil, r.errAt(start, "Instant literal expects a string, not %s", lang.PrintString(val))
		}
		inst, err := NewInst(s)
		if err != nil {
			return nil, r.errAt(start, "%v", err)
		}
		return inst, nil
	}
	// clojure.edn/read-string's `:default` option: an (fn [tag value])
	// invoked for a tag with no built-in handler and no *data-readers*
	// entry (oracle: (edn/read-string {:default (fn [_tag v] [:unknown
	// v])} "#foo 42") => [:unknown 42]; a KNOWN tag like #uuid still uses
	// its built-in reader above, :default is only the last resort).
	if r.defaultReader != nil {
		return r.defaultReader.Invoke(sym, val), nil
	}
	// Inside an unselected reader-conditional branch (e.g. jank's #cpp), an
	// unknown tag is suppress-read as nil rather than an error — the branch is
	// elided, so the value is discarded anyway (matches Clojure's suppressing
	// read of unselected branches). Only the value is consumed above.
	if r.tagSuppress > 0 {
		return nil, nil
	}
	// *default-data-reader-fn* — the LAST resort before the error, exactly
	// clojure.core (oracle 1.12.5: (binding [*default-data-reader-fn* (fn
	// [t v] [t v])] (read-string "#foo/bar 1")) => [foo/bar 1]; a KNOWN tag
	// like #inst never reaches it). Core reads only: clojure.edn has its
	// own :default option (defaultReader, above) and ignores this var, as
	// on the JVM.
	if !r.ednStrict {
		if fn, ok := defaultDataReaderFn(); ok {
			return fn.Invoke(sym, val), nil
		}
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
		f, err := r.readWithDelim(end, true, true)
		if errors.Is(err, ErrEOF) {
			// EOF, a comment, or a discard consumed the rest of the input.
			return nil, unterminated(r.s.Pos())
		}
		if err != nil {
			return nil, err
		}
		if f == sentReadFinished {
			return forms, nil
		}
		if sp, ok := f.(spliceForms); ok {
			forms = append(forms, sp.items...)
			continue
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

	if r.ednStrict {
		// clojure.edn has no current-namespace context, so ::foo/::alias/foo
		// auto-resolution is simply an invalid token (oracle: real JVM
		// clojure.edn/read-string "::foo" throws "Invalid token: ::foo" —
		// the SAME error clojure.core's reader would give with no injected
		// Resolver, so this only tightens edn to that no-resolver case).
		if strings.HasPrefix(tok, "::") {
			return invalid()
		}
		// 3+-part (two-or-more-slash) symbols/keywords, e.g. foo/bar/baz:
		// real JVM edn is actually lenient here too (DEVIATION — see the
		// ednStrict field doc), but the clojure-test-suite's :default
		// dialect branch expects rejection, so cljgo is stricter than the
		// JVM oracle by design for this one corner. `ns` (m[1]) already
		// carries its trailing '/'; an EXTRA '/' inside it (once that
		// trailing one is stripped) means the namespace part itself had a
		// slash, i.e. 3+ parts. A bare trailing-slash NAME like "foo//"
		// (ns "foo/", name "/") is legitimately 2 parts and must not trip
		// this — checking `ns`, not the raw token, keeps that case valid.
		if nsBody := strings.TrimSuffix(ns, "/"); strings.Contains(nsBody, "/") {
			return invalid()
		}
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
