package reader

// Syntax-quote expansion (design/01-reader.md §2), a faithful port of
// clojure.lang.LispReader.SyntaxQuoteReader: `form reads form fully,
// then rewrites it at read time into code that reconstructs it
// (seq/concat/list forms), resolving symbols through the injected
// Resolver.
//
// Gensym IDs come from ONE atomic global counter (RT.nextID
// semantics), NOT from the size of a per-quote map: Glojure mints
// x__0__auto__ in two separate syntax-quotes, breaking hygiene when
// expansions meet (design/01-reader.md §2). The gensym ENVIRONMENT
// (name -> minted symbol) is created fresh per outermost-or-nested
// syntax-quote read and torn down after, so the same x# inside one
// quote is one symbol and x# in two quotes is two symbols.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// globalNextID is the package-level gensym counter, shared by
// auto-gensym (x#) and #() arg hygiene (p1__N#). clojure.core/gensym
// should draw from the same counter via NextID once eval wires it up.
var globalNextID atomic.Int64

// NextID returns the next id from the global gensym counter
// (clojure.lang.RT.nextID equivalent).
func NextID() int64 { return globalNextID.Add(1) }

// WithNextID overrides the gensym id source (used by tests and by a
// host that maintains its own RT.nextID counter).
func WithNextID(f func() int64) Option {
	return func(r *Reader) { r.nextID = f }
}

// maxSyntaxQuoteDepth bounds nested ` recursion; Glojure's fuzzer
// found exponential blowup on deep nesting (design/01-reader.md §2).
const maxSyntaxQuoteDepth = 64

// Symbols used by syntax-quote expansion and dispatch forms.
var (
	symQuote           = lang.NewSymbol("quote")
	symUnquote         = lang.NewSymbol("clojure.core/unquote")
	symUnquoteSplicing = lang.NewSymbol("clojure.core/unquote-splicing")
	symSeq             = lang.NewSymbol("clojure.core/seq")
	symConcat          = lang.NewSymbol("clojure.core/concat")
	symList            = lang.NewSymbol("clojure.core/list")
	symApply           = lang.NewSymbol("clojure.core/apply")
	symVector          = lang.NewSymbol("clojure.core/vector")
	symHashMap         = lang.NewSymbol("clojure.core/hash-map")
	symHashSet         = lang.NewSymbol("clojure.core/hash-set")
	symWithMeta        = lang.NewSymbol("clojure.core/with-meta")
	symAmp             = lang.NewSymbol("&")
	symFnStar          = lang.NewSymbol("fn*")
)

// specials mirrors clojure.lang.Compiler.specials: these symbols pass
// through syntax-quote unqualified, as (quote sym). CLI checks
// (clojure 1.12.5): `if => (quote if); `def => (quote def);
// `fn* => (quote fn*); `& => (quote &); `recur => (quote recur);
// `try => (quote try); `catch => (quote catch). Note `let and `when
// are NOT special (let*/loop* are) and resolve to clojure.core.
var specials = map[string]bool{
	"def": true, "loop*": true, "recur": true, "if": true, "case*": true,
	"let*": true, "letfn*": true, "do": true, "fn*": true, "quote": true,
	"var": true, "clojure.core/import*": true, ".": true, "set!": true,
	"deftype*": true, "reify*": true, "try": true, "throw": true,
	"monitor-enter": true, "monitor-exit": true, "catch": true,
	"finally": true, "new": true, "&": true,
}

func isSpecial(form any) bool {
	sym, ok := form.(*lang.Symbol)
	return ok && specials[sym.FullName()]
}

// isTaggedSeq reports whether form is a seq whose first element is
// the given symbol (LispReader.isUnquote / isUnquoteSplicing shape).
func isTaggedSeq(form any, tag *lang.Symbol) bool {
	seq, ok := form.(lang.ISeq)
	if !ok {
		return false
	}
	first, ok := seq.First().(*lang.Symbol)
	return ok && first.Equals(tag)
}

func isUnquote(form any) bool         { return isTaggedSeq(form, symUnquote) }
func isUnquoteSplicing(form any) bool { return isTaggedSeq(form, symUnquoteSplicing) }

// second returns the second element of a seq ((unquote X) -> X).
func second(form any) any {
	return lang.First(lang.Rest(form))
}

// readSyntaxQuote handles `form: read the next form under a FRESH
// gensym environment, then expand it. Nested ` needs no special code —
// the inner backtick is read (and expanded, with its own env) while
// reading the outer form, matching Clojure's push/pop of GENSYM_ENV.
func (r *Reader) readSyntaxQuote(start Position) (any, error) {
	if r.sqDepth >= maxSyntaxQuoteDepth {
		return nil, r.errAt(start, "syntax-quote nested too deeply (max %d)", maxSyntaxQuoteDepth)
	}
	saved := r.gensymEnv
	r.gensymEnv = map[string]*lang.Symbol{}
	r.sqDepth++
	defer func() {
		r.gensymEnv = saved
		r.sqDepth--
	}()

	form, err := r.readForm()
	if errors.Is(err, ErrEOF) {
		return nil, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w syntax-quoted form", ErrIncomplete)}
	}
	if err != nil {
		return nil, err
	}
	return r.syntaxQuote(start, form)
}

// readUnquote handles ~form => (clojure.core/unquote form) and
// ~@form => (clojure.core/unquote-splicing form). Like Clojure, ~ is
// read unconditionally: S8 golden: ~x at top level =>
// (clojure.core/unquote x).
func (r *Reader) readUnquote(start Position) (any, error) {
	c, err := r.s.Read()
	if err == nil {
		if c == '@' {
			return r.readWrapped(start, "clojure.core/unquote-splicing")
		}
		r.s.Unread()
	}
	return r.readWrapped(start, "clojure.core/unquote")
}

// syntaxQuote is LispReader.SyntaxQuoteReader.syntaxQuote(form).
func (r *Reader) syntaxQuote(start Position, form any) (any, error) {
	var ret any
	switch {
	case isSpecial(form):
		ret = lang.NewList(symQuote, form)

	case isSymbol(form):
		sym, err := r.syntaxQuoteSymbol(start, form.(*lang.Symbol))
		if err != nil {
			return nil, err
		}
		ret = lang.NewList(symQuote, sym)

	case isUnquote(form):
		// ~x collapses to x; NO with-meta wrap (Clojure returns here).
		return second(form), nil

	case isUnquoteSplicing(form):
		// Only legal as an element of a collection walk (sqExpandList).
		// S8 golden: `~@x => ERR (IllegalStateException).
		return nil, r.errAt(start, "splice not in list")

	default:
		var err error
		ret, err = r.syntaxQuoteColl(start, form)
		if err != nil {
			return nil, err
		}
		if ret == nil {
			// Not a collection.
			switch form.(type) {
			case lang.Keyword, string, lang.Char,
				int, int8, int16, int32, int64,
				uint, uint8, uint16, uint32, uint64,
				float32, float64,
				*lang.BigInt, *lang.BigDecimal, *lang.Ratio:
				// Self-evaluating literals stay bare. S8 goldens:
				// `:kw => :kw; `42 => 42; `"str" => "str"; `\c => \c.
				ret = form
			default:
				// Everything else (incl. booleans and nil) is quoted.
				// S8 goldens: `true => (quote true); `nil => (quote nil).
				ret = lang.NewList(symQuote, form)
			}
		}
	}

	// Metadata preservation (design/01-reader.md §2.7): if the form
	// carries meta beyond our position keys, wrap the expansion in
	// (clojure.core/with-meta ret (syntaxQuote meta-minus-positions)).
	// Glojure skips this; we must not — ^:private in a macro template
	// matters. S8 golden: `^:private x => (clojure.core/with-meta
	// (quote user/x) (clojure.core/apply clojure.core/hash-map ...)).
	if iobj, ok := form.(lang.IObj); ok {
		if userMeta := stripPositionMeta(iobj.Meta()); userMeta != nil && userMeta.Count() > 0 {
			expandedMeta, err := r.syntaxQuote(start, userMeta)
			if err != nil {
				return nil, err
			}
			ret = lang.NewList(symWithMeta, ret, expandedMeta)
		}
	}
	return ret, nil
}

func isSymbol(form any) bool {
	_, ok := form.(*lang.Symbol)
	return ok
}

// stripPositionMeta removes the reader's position keys (:file :line
// :column :end-line :end-column). Clojure strips only LINE/COLUMN
// because plain read-string attaches no meta at all; we attach all
// five to every IObj, so all five must be invisible to syntax-quote.
func stripPositionMeta(m lang.IPersistentMap) lang.IPersistentMap {
	if m == nil {
		return nil
	}
	for _, k := range []lang.Keyword{
		lang.KWFile, lang.KWLine, lang.KWColumn, lang.KWEndLine, lang.KWEndColumn,
	} {
		m = lang.Dissoc(m, k).(lang.IPersistentMap)
	}
	return m
}

// syntaxQuoteSymbol implements the Symbol arm of syntaxQuote.
func (r *Reader) syntaxQuoteSymbol(start Position, sym *lang.Symbol) (*lang.Symbol, error) {
	name := sym.Name()
	switch {
	case sym.Namespace() == "" && strings.HasSuffix(name, "#"):
		// Auto-gensym x#: per-syntax-quote environment, global id.
		if r.gensymEnv == nil {
			return nil, r.errAt(start, "Gensym literal not in syntax-quote")
		}
		gs, ok := r.gensymEnv[name]
		if !ok {
			gs = lang.NewSymbol(name[:len(name)-1] + "__" + strconv.FormatInt(r.nextID(), 10) + "__auto__")
			r.gensymEnv[name] = gs
		}
		return gs, nil

	case sym.Namespace() == "" && strings.HasSuffix(name, "."):
		// Ctor form Foo.: resolve Foo, re-append the dot. Only the
		// NAME of the resolution is kept (LispReader uses csym.name),
		// so an unresolvable foo. stays foo., not user/foo. —
		// CLI check: `foo. => (quote foo.).
		csym := r.resolveSymbol(lang.NewSymbol(name[:len(name)-1]))
		return lang.NewSymbol(csym.Name() + "."), nil

	case sym.Namespace() == "" && strings.HasPrefix(name, "."):
		// Method name .foo: left as-is. CLI check: `.foo => (quote .foo).
		return sym, nil

	default:
		if sym.Namespace() != "" && r.resolver != nil {
			// Classname/staticMethod: if the ns part names a type,
			// requalify with the type's full name.
			if t := r.resolver.ResolveType(lang.NewSymbol(sym.Namespace())); t != nil {
				return lang.InternSymbol(t.Name(), name), nil
			}
		}
		return r.resolveSymbol(sym), nil
	}
}

// resolveSymbol ports Compiler.resolveSymbol through the injected
// Resolver: qualified -> resolve ns part as alias; unqualified ->
// resolve as type, then var; unresolvable -> qualify with CurrentNS.
// Without a resolver, symbols are left untouched.
func (r *Reader) resolveSymbol(sym *lang.Symbol) *lang.Symbol {
	if strings.Index(sym.Name(), ".") > 0 {
		// Package-qualified class name: already resolved.
		// CLI check: `java.lang.String => (quote java.lang.String).
		return sym
	}
	if r.resolver == nil {
		return sym
	}
	if ns := sym.Namespace(); ns != "" {
		nsSym := r.resolver.ResolveAlias(lang.NewSymbol(ns))
		if nsSym == nil || nsSym.Name() == ns {
			// Unknown ns stays as written. CLI check: `Foo/bar =>
			// (quote Foo/bar); S8 golden: `foo/bar => (quote foo/bar).
			return sym
		}
		return lang.InternSymbol(nsSym.Name(), sym.Name())
	}
	if t := r.resolver.ResolveType(sym); t != nil {
		return t
	}
	if v := r.resolver.ResolveVar(sym); v != nil {
		return v
	}
	cur := r.resolver.CurrentNS()
	if cur == nil {
		return sym
	}
	// S8 golden: `x in ns user => (quote user/x).
	return lang.InternSymbol(cur.Name(), sym.Name())
}

// syntaxQuoteColl handles the IPersistentCollection arm. It returns
// (nil, nil) when form is not a collection.
func (r *Reader) syntaxQuoteColl(start Position, form any) (any, error) {
	switch f := form.(type) {
	case lang.IPersistentMap:
		// Flatten k/v first (LispReader.flattenMap), then
		// (apply hash-map (seq (concat ...))).
		var kvs []any
		for seq := lang.Seq(f); seq != nil; seq = seq.Next() {
			e := seq.First().(lang.IMapEntry)
			kvs = append(kvs, e.Key(), e.Val())
		}
		elems, err := r.sqExpandList(start, kvs)
		if err != nil {
			return nil, err
		}
		return lang.NewList(symApply, symHashMap, seqConcatForm(elems)), nil

	case lang.IPersistentVector:
		elems, err := r.sqExpandList(start, seqToSlice(f))
		if err != nil {
			return nil, err
		}
		return lang.NewList(symApply, symVector, seqConcatForm(elems)), nil

	case lang.IPersistentSet:
		elems, err := r.sqExpandList(start, seqToSlice(f))
		if err != nil {
			return nil, err
		}
		return lang.NewList(symApply, symHashSet, seqConcatForm(elems)), nil

	case lang.ISeq, lang.IPersistentList:
		items := seqToSlice(form)
		if len(items) == 0 {
			// S8 golden: `() => (clojure.core/list).
			return lang.NewList(symList), nil
		}
		elems, err := r.sqExpandList(start, items)
		if err != nil {
			return nil, err
		}
		return seqConcatForm(elems), nil
	}
	return nil, nil
}

// sqExpandList maps each element to its concat contribution:
// unquote -> (list x), unquote-splicing -> x, else
// (list (syntaxQuote el)) (LispReader.sqExpandList).
func (r *Reader) sqExpandList(start Position, items []any) ([]any, error) {
	out := make([]any, 0, len(items))
	for _, item := range items {
		switch {
		case isUnquote(item):
			out = append(out, lang.NewList(symList, second(item)))
		case isUnquoteSplicing(item):
			out = append(out, second(item))
		default:
			sq, err := r.syntaxQuote(start, item)
			if err != nil {
				return nil, err
			}
			out = append(out, lang.NewList(symList, sq))
		}
	}
	return out, nil
}

// seqConcatForm builds (clojure.core/seq (clojure.core/concat elems...)).
func seqConcatForm(elems []any) any {
	concat := make([]any, 0, len(elems)+1)
	concat = append(concat, symConcat)
	concat = append(concat, elems...)
	return lang.NewList(symSeq, lang.NewList(concat...))
}

func seqToSlice(form any) []any {
	var out []any
	for seq := lang.Seq(form); seq != nil; seq = seq.Next() {
		out = append(out, seq.First())
	}
	return out
}
