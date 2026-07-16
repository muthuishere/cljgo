;; Two SEPARATELY-READ `#"..."` literals with identical pattern text are
;; NOT `=` — java.util.regex.Pattern has no .equals override, so equality
;; is identity-only. Same instance (bound once, referenced twice) IS `=`.
;; cljgo's reader.Regex must therefore carry identity (a pointer), never a
;; plain struct value: Go struct equality would make any two same-pattern
;; literals compare `==` true, which is wrong.
;; harness: eval — pkg/emit has no reader.Regex constant emission yet (see
;;   regex-core.clj's waiver); the compiled path can't build this file.
;; Regression: clojure-test-suite core_test/eq.cljc + not_eq.cljc (jank
;; suite, ADR 0022).
;; Oracle (clojure 1.12.5): [false true]
[(= #"my regex" #"my regex")
 (let [r #"x"] (= r r))]
;; expect: [false true]
