;; min-key's 3+-arity walk compares the running winner to each new element
;; with `<=`, NOT `<` (clojure.repl/source min-key on the JVM). This only
;; shows up with ##NaN in the mix, where every comparison is false: once a
;; NaN becomes the running winner, `<=` against the next element also comes
;; back false, so the loop KEEPS the NaN instead of falling through to
;; (wrongly) adopt the new element — order of NaN's appearance changes the
;; result, and a naive `<`-only fold gets it wrong in exactly this case.
;; Regression: clojure-test-suite core_test/min_key.cljc (jank suite, ADR
;; 0022).
;; Oracle (clojure 1.12.5): [##-Inf ##NaN]
[(min-key identity ##-Inf 1 ##NaN)
 (min-key identity ##-Inf ##NaN 1)]
;; expect: [##-Inf ##NaN]
