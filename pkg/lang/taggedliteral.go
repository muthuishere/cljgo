package lang

// TaggedLiteral and ReaderConditional — the two reader data types that
// clojure.core exposes as first-class values (clojure.lang.TaggedLiteral,
// clojure.lang.ReaderConditional). These are cljgo extensions, NOT vendored
// Glojure code, so no EPL header applies.
//
// A TaggedLiteral is the value a data reader would receive: a `:tag` symbol
// and a `:form`. It prints readably as `#tag form` (oracle JVM 1.12.5:
// (pr-str (tagged-literal 'foo 42)) => "#foo 42").
//
// A ReaderConditional carries the whole `#?(...)` / `#?@(...)` body as a
// `:form` list plus a `:splicing?` flag, printing as `#?(...)` or `#?@(...)`
// (oracle: (pr-str (reader-conditional '(:clj 1) false)) => "#?(:clj 1)";
// splicing? true => "#?@(...)").
//
// Both support keyword lookup via ILookup — `(:tag x)`, `(:form x)`,
// `(:splicing? x)` — and value equality via Equiv (same concrete type +
// Equiv on each field), exactly like the JVM records. They are NOT
// collections (no assoc/count), matching the JVM: (assoc tl :k v) and
// (count tl) both throw there.

// Interned keys for TaggedLiteral / ReaderConditional field lookup.
var (
	kwTag      = NewKeyword("tag")
	kwForm     = NewKeyword("form")
	kwSplicing = NewKeyword("splicing?")
)

// TaggedLiteral is clojure.lang.TaggedLiteral: a tag symbol + a form.
type TaggedLiteral struct {
	Tag  any
	Form any
}

// NewTaggedLiteral builds a tagged literal from a tag and a form. Backs
// clojure.core/tagged-literal.
func NewTaggedLiteral(tag, form any) *TaggedLiteral {
	return &TaggedLiteral{Tag: tag, Form: form}
}

// ValAt implements ILookup: :tag and :form return the fields, anything
// else nil.
func (t *TaggedLiteral) ValAt(key any) any {
	return t.ValAtDefault(key, nil)
}

// ValAtDefault implements ILookup with a fallback for unknown keys
// (oracle: (get (tagged-literal 'foo 42) :nope :DEF) => :DEF).
func (t *TaggedLiteral) ValAtDefault(key, def any) any {
	switch {
	case Equiv(key, kwTag):
		return t.Tag
	case Equiv(key, kwForm):
		return t.Form
	default:
		return def
	}
}

// Equiv gives value equality: same type, Equiv tag, Equiv form (oracle:
// (= (tagged-literal 'foo 42) (tagged-literal 'foo 42)) => true).
func (t *TaggedLiteral) Equiv(other any) bool {
	o, ok := other.(*TaggedLiteral)
	if !ok {
		return false
	}
	return Equiv(t.Tag, o.Tag) && Equiv(t.Form, o.Form)
}

// IsTaggedLiteral reports whether x is a TaggedLiteral. Backs
// clojure.core/tagged-literal?.
func IsTaggedLiteral(x any) bool {
	_, ok := x.(*TaggedLiteral)
	return ok
}

// ReaderConditional is clojure.lang.ReaderConditional: the `#?(...)` body
// form plus a splicing flag.
type ReaderConditional struct {
	Form     any
	Splicing bool
}

// NewReaderConditional builds a reader conditional from a form and a
// splicing flag. Backs clojure.core/reader-conditional.
func NewReaderConditional(form any, splicing bool) *ReaderConditional {
	return &ReaderConditional{Form: form, Splicing: splicing}
}

// ValAt implements ILookup: :form and :splicing? return the fields.
func (r *ReaderConditional) ValAt(key any) any {
	return r.ValAtDefault(key, nil)
}

// ValAtDefault implements ILookup with a fallback for unknown keys.
func (r *ReaderConditional) ValAtDefault(key, def any) any {
	switch {
	case Equiv(key, kwForm):
		return r.Form
	case Equiv(key, kwSplicing):
		return r.Splicing
	default:
		return def
	}
}

// Equiv gives value equality: same type, Equiv form, equal splicing flag.
func (r *ReaderConditional) Equiv(other any) bool {
	o, ok := other.(*ReaderConditional)
	if !ok {
		return false
	}
	return r.Splicing == o.Splicing && Equiv(r.Form, o.Form)
}

// IsReaderConditional reports whether x is a ReaderConditional. Backs
// clojure.core/reader-conditional?.
func IsReaderConditional(x any) bool {
	_, ok := x.(*ReaderConditional)
	return ok
}
