;; clojure.string/blank? (and trim/triml/trimr) are specified in terms of
;; java.lang.Character.isWhitespace, which EXCLUDES the non-breaking-space
;; family (U+00A0 NBSP, U+2007 FIGURE SPACE, U+202F NARROW NBSP) even though
;; those runes carry the Unicode White_Space property (so Go's
;; unicode.IsSpace wrongly says true for them). Regular space and ordinary
;; whitespace mixes are still blank.
;; Regression: clojure-test-suite string_test/blank_qmark.cljc (jank suite,
;; ADR 0022).
;; Oracle (clojure 1.12.5): [true false false true]
[(clojure.string/blank? " ")
 (clojure.string/blank? " ")
 (clojure.string/blank? " ")
 (clojure.string/blank? "\t \n")]
;; expect: [true false false true]
