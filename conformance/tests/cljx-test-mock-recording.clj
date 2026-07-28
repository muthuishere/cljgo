;; cljx.test/mock (ADR 0105): a callable that RECORDS every call. (mock)
;; answers nil, (mock f) delegates to f, (mock {:returns v}) answers v; the
;; call log is read back with calls / call-count / called? / called-with?.
;;
;; This also freezes the NO-LEAK invariant (spike s66 finding 2): each mock
;; owns its log in its own closure, so a fresh mock always starts empty no
;; matter how many mocks were made before it, and nothing global accumulates.
;; The fn-keyed registry s66 originally suggested is NOT used — a compiled fn
;; has no stable identity ((= f1 f2) is true for distinct closures and
;; (identical? f1 f1) is false in a compiled binary), so cljx.test takes s66's
;; sentinel-arg alternative instead.
;;
;; oracle: skip — cljx.test is a cljgo namespace (ADR 0105's cljx.* developer
;; experience tier) with no JVM analog to freeze byte-for-byte. Frozen against
;; cljgo's own output; the REPL-vs-binary bar is enforced by the dual harness,
;; which is exactly what matters for a testing library.
(require '[cljx.test :as x])

(def plain (x/mock))
(def delegating (x/mock (fn [a b] (+ a b))))
(def configured (x/mock {:returns :queued}))

(def plain-result (plain :a))
(def delegating-result (delegating 2 3))
(def configured-result (configured "asha@example.com" "welcome"))
(configured "ravi@example.com" "welcome")

;; a mock made AFTER 50 others still starts empty — no shared registry
(dotimes [_ 50] (x/mock))
(def fresh (x/mock))

[plain-result
 delegating-result
 configured-result
 (x/calls plain)
 (x/calls configured)
 (x/call-count configured)
 (x/called? plain)
 (x/called? fresh)
 (x/called-with? configured "asha@example.com" "welcome")
 (x/called-with? configured "nobody@example.com" "welcome")
 (x/call-count fresh)]
;; expect: [nil 5 :queued [[:a]] [["asha@example.com" "welcome"] ["ravi@example.com" "welcome"]] 2 true false true false 0]
