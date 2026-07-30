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
	"strings"

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
	// No branch matched. On the JVM (and for cljgo project code) that is
	// legal: the conditional reads as nothing. Under WithStarvedCondError —
	// set only for Maven-origin source, ADR 0095 §4.1 — a STARVED
	// conditional (branches present, none selectable) is a hard error
	// instead, because a silently-empty namespace out of a jar blames the
	// wrong code later. A conditional with NO branches at all, #?(), is
	// vacuous rather than starved and stays legal.
	//
	// The FIRST check below is the top-level one (nestDepth is back to 0 once
	// our own body read has returned): whole top-level forms vanishing, leaving
	// a namespace with no vars — the shape s50 warns about.
	//
	// Nesting is covered by two further checks, each keyed on the elision
	// SILENTLY CHANGING A SHAPE rather than on nesting as such — a starved
	// conditional whose removal breaks nothing is still the portable-library
	// fencing idiom, and erroring on it would reject exactly the libraries this
	// exists to consume. `#?(:clj (java.util.Date.) :default (now))` is not
	// starved at all (:default matches), and `(:import #?(:clj …))` is starved
	// but harmless. The remaining documented false NEGATIVE: a starved
	// conditional inside a SELECTED branch of another conditional is still
	// elided silently, because the body is read before selection and firing
	// there would be a false positive on an UNSELECTED branch.
	//
	// SHAPE-BREAKING elision comes first, at any depth. A starved conditional
	// directly inside a vector, map or set silently changes that collection's
	// element count — `[a 1 b #?(:clj x)]` becomes a 3-element binding vector.
	// Real camel-snake-kebab 0.4.3 has exactly that, and eliding it made the
	// namespace classify as usable and then fail to compile with an error
	// naming a library the user never wrote. That is a false "usable" claim,
	// so it is a hard error rather than a documented false negative.
	//
	// SPLICING conditionals are exempt: `#?@(…)` contributes a SEQUENCE, so a
	// starved one contributes zero elements — it removes nothing and the
	// enclosing collection keeps its shape. Real tools.cli 1.1.230 has
	// `#?@(:cljr (req …))` inside a `let` binding vector at cli.cljc:108, and
	// JVM Clojure elides it to nothing on every non-cljr platform too.
	if r.starvedCollError && !splicing && len(forms) >= 2 {
		switch r.enclosingColl() {
		case "vector", "map", "set":
			return nil, false, r.errAt(start,
				"reader conditional supplies no branch for this platform, and eliding it would change the shape of the enclosing %s; expected one of :cljgo, :default; found %s",
				r.enclosingColl(), featureList(forms))
		}
	}
	if r.starvedCondError && r.nestDepth == 0 && len(forms) >= 2 {
		return nil, false, r.errAt(start,
			"reader conditional supplies no branch for this platform; expected one of :cljgo, :default; found %s",
			featureList(forms))
	}
	// NESTED starved conditional inside a LIST — medley 1.4.0 core.cljc:181,
	// `(instance? #?(:clj clojure.lang.PersistentQueue :cljs …) x)`. Eliding it
	// changes the CALL'S ARITY, and the user then reads
	// "macroexpanding instance?: wrong number of args (1) passed to:
	// clojure.core/instance?" — a diagnostic that names neither the reader
	// conditional nor the library. Same shape-breaking argument as the
	// vector/map/set rule above, so it shares that rule's wording (and, in
	// pkg/deps, its "never recoverable by re-reading with elision" handling).
	//
	// The fire condition is SHIFTING, not nesting: it fires only when forms
	// FOLLOW the elided conditional in the list, because those forms silently
	// slide into its place. That is what keeps the portable-library fencing
	// idioms — the whole point of consuming .cljc at all — readable:
	//
	//   - a TRAILING conditional is exempt. `(def ^:private max-number
	//     #?(:clj Long/MAX_VALUE :cljs js/Number.MAX_VALUE))` (real
	//     com.stuartsierra/dependency 1.0.0, dependency.cljc:148) stays a
	//     well-formed def, and `(defn- editable? [coll] #?(:clj … :cljs …))`
	//     (real medley 1.4.0, core.cljc:79) a well-formed defn. Both libraries
	//     load and run; erroring there would reject them outright.
	//   - a list whose HEAD IS A KEYWORD is an ns-clause fence —
	//     `(ns x (:import #?(:clj [java.util Date])))`. Nothing is CALLED, so
	//     no arity breaks; the JVM-only import simply never arrives. Frozen as
	//     corpus case 11.
	//   - anywhere inside another conditional's BODY. The body is read in full
	//     BEFORE a branch is selected, so a starved conditional there may live
	//     in a branch cljgo would never have taken (corpus case 8). Erroring
	//     would be a false positive on correct portable code. A starved
	//     conditional inside a SELECTED branch therefore stays the documented
	//     false negative it already was.
	//   - inside a #_ discard, a string or a comment: nothing survives to be
	//     shifted.
	//
	// Stated rather than hidden, the trailing exemption's false NEGATIVE: a
	// trailing starved argument of a real call, `(f 1 #?(:clj 2))`, still
	// elides to `(f 1)` silently. Distinguishing that from `(def x #?(…))`
	// needs analysis the reader does not have.
	if r.starvedCondError && !splicing && len(forms) >= 2 &&
		r.enclosingColl() == "list" && !r.insideConditionalBody() && r.discardDepth == 0 {
		if _, headIsKeyword := r.enclosingHead().(lang.Keyword); !headIsKeyword {
			i := len(r.nestPending) - 1
			r.nestPending[i] = append(r.nestPending[i], pendingStarved{
				pos:   start,
				count: r.nestCounts[i],
				feats: featureList(forms),
			})
		}
	}
	return nil, true, nil
}

// featureList renders the feature keywords actually present in a starved
// conditional's branch list, for the R1012 "found:" detail.
func featureList(forms []any) string {
	var feats []string
	for i := 0; i+1 < len(forms); i += 2 {
		if k, ok := forms[i].(lang.Keyword); ok {
			feats = append(feats, ":"+k.Name())
		}
	}
	if len(feats) == 0 {
		return "no feature keywords"
	}
	return strings.Join(feats, ", ")
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
