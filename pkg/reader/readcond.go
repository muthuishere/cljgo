package reader

// Reader conditionals (design/01-reader.md §Phase 2), a faithful port
// of clojure.lang.LispReader$ConditionalReader:
//
//	#?(:cljgo x :default z)   selecting  => x    (feature :cljgo matches)
//	#?@(:cljgo [a b])         splicing   => a b  (spliced into the parent)
//
// PLATFORM FEATURE DECISION: cljgo is its own Clojure platform, so its
// reader feature is :cljgo (never :clj — that is the JVM's feature, and
// claiming it would silently pull JVM-only branches). :default always
// matches, as a last resort. Unlike JVM Clojure, cljgo does not gate
// reader conditionals behind an opt-in :read-cond flag for FILE and REPL
// reading: they are always processed there, regardless of extension
// (deliberate divergence — ADR 0068 addendum). clojure.core/read-string,
// however, mirrors the JVM's opts protocol exactly: conditionals are an
// error without {:read-cond :allow}/{:read-cond :preserve}
// (WithReadCondForbid), {:features #{...}} adds selectable features
// (WithFeatures), and :preserve reads the conditional as a
// lang.ReaderConditional data value (WithReadCondPreserve, ADR 0050).
//
// The mechanism (first-branch-wins, whole-body-read-then-select,
// non-keyword-feature rejection, top-level-splice rejection, elision of
// an unmatched non-splicing conditional) was verified against clojure
// 1.12.5 with {:read-cond :allow}. The :clj vs :cljgo mapping is the
// mirror image of the JVM oracle (JVM always injects :clj); cited per
// case in readcond_test.go.

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// kwDefault always matches; kwCljgo is cljgo's platform feature.
var (
	kwDefault = lang.NewKeyword("default")
	kwCljgo   = lang.NewKeyword("cljgo")
)

// Reader-conditional policy (condMode). The zero value condAllow keeps
// cljgo's always-process behavior for files and the REPL; condForbid and
// condPreserve back clojure.core/read-string's JVM-parity opts protocol
// (WithReadCondForbid / WithReadCondPreserve).
const (
	condAllow = iota
	condForbid
	condPreserve
)

// matchesFeature reports whether a reader-conditional feature keyword is
// satisfied: the platform feature :cljgo and :default always match, plus
// any caller-supplied WithFeatures keywords (the JVM likewise always
// includes its platform :clj alongside {:features #{...}} — oracle
// 1.12.5: (read-string {:read-cond :allow :features #{:cljs}}
// "#?(:clj 2)") => 2).
func (r *Reader) matchesFeature(feat lang.Keyword) bool {
	return feat == kwCljgo || feat == kwDefault || r.condFeatures[feat]
}

// readConditional reads a reader conditional whose leading "#?" has been
// consumed. It returns again=true when no branch matched (the form is
// elided, exactly like a #_ discard). A matched selecting branch returns
// its form; a matched splicing branch returns a spliceForms sentinel for
// readDelimited to splice into the enclosing collection.
func (r *Reader) readConditional(start Position, spliceOK bool) (form any, again bool, err error) {
	incomplete := func() (any, bool, error) {
		return nil, false, &Error{Pos: r.s.Pos(), Start: &start, Err: fmt.Errorf("%w reader conditional", ErrIncomplete)}
	}

	// JVM parity for read-string without {:read-cond :allow/:preserve}
	// (oracle 1.12.5: (read-string "#?(:clj 1)") throws this exact
	// message). Never set for file/REPL reading (see condMode's doc).
	if r.condMode == condForbid {
		return nil, false, r.errAt(start, "Conditional read not allowed")
	}

	// Optional '@' => splicing conditional.
	splicing := false
	c, e := r.s.Read()
	if e != nil {
		return incomplete()
	}
	if c == '@' {
		splicing = true
	} else {
		r.s.Unread()
	}

	// A preserved conditional is ONE data value, so top-level splicing is
	// fine there (oracle 1.12.5: (read-string {:read-cond :preserve}
	// "#?@(:clj [1 2])") => the #?@ object, while {:read-cond :allow}
	// throws).
	if splicing && !spliceOK && r.condMode != condPreserve {
		return nil, false, r.errAt(start, "Reader conditional splicing not allowed at the top level.")
	}

	// The body must be a list: #?(...).
	r.skipWhitespace()
	oc, e := r.s.Read()
	if e != nil {
		return incomplete()
	}
	if oc != '(' {
		return nil, false, r.errAt(start, "read-cond body must be a list")
	}

	// {:read-cond :preserve}: don't select at all — the whole body reads
	// as ONE lang.ReaderConditional data value (ADR 0050), with every
	// tagged literal inside preserved as a lang.TaggedLiteral (oracle
	// 1.12.5: (read-string {:read-cond :preserve} "#?(:clj #inst ...)")
	// keeps a TaggedLiteral in :form; #inst OUTSIDE a conditional still
	// resolves normally). tagPreserve gates readTaggedLiteral.
	if r.condMode == condPreserve {
		r.tagPreserve++
		forms, err := r.readDelimited("reader conditional", ')', start)
		r.tagPreserve--
		if err != nil {
			return nil, false, err
		}
		return lang.NewReaderConditional(lang.NewList(forms...), splicing), false, nil
	}

	// Read the entire body first (matching Clojure, which errors on
	// malformed forms even in unselected branches). Unknown TAGGED literals
	// in the body — e.g. jank's #cpp in a branch cljgo will elide — are
	// suppress-read as nil rather than erroring (Clojure reads unselected
	// branches in a tag-suppressing mode); tagSuppress gates readTaggedLiteral.
	r.tagSuppress++
	forms, err := r.readDelimited("reader conditional", ')', start)
	r.tagSuppress--
	if err != nil {
		return nil, false, err
	}

	// Scan feature/form pairs left to right; first match wins.
	for i := 0; i+1 < len(forms); i += 2 {
		feat, ok := forms[i].(lang.Keyword)
		if !ok {
			return nil, false, r.errAt(start, "Feature should be a keyword: %s", featureString(forms[i]))
		}
		if !r.matchesFeature(feat) {
			continue
		}
		val := forms[i+1]
		if !splicing {
			return val, false, nil
		}
		items, ok := spliceItems(val)
		if !ok {
			return nil, false, r.errAt(start, "Spliced form list in read-cond-splicing must implement clojure.lang.Sequential")
		}
		return spliceForms{items: items}, false, nil
	}
	// No branch matched: the conditional reads as nothing.
	return nil, true, nil
}

// spliceItems returns the elements of a matched splicing branch's value.
// The value must be a sequential collection (list or vector).
func spliceItems(val any) ([]any, bool) {
	switch v := val.(type) {
	case lang.IPersistentVector:
		items := make([]any, 0, v.Count())
		for i := 0; i < v.Count(); i++ {
			items = append(items, lang.MustNth(v, i))
		}
		return items, true
	case lang.ISeq:
		var items []any
		for s := lang.Seq(v); s != nil; s = s.Next() {
			items = append(items, s.First())
		}
		return items, true
	default:
		return nil, false
	}
}

// featureString renders a non-keyword feature for the error message,
// matching Clojure's toString-based formatting (a bad string feature
// "clj" prints as clj, not "clj").
func featureString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return lang.PrintString(v)
}
