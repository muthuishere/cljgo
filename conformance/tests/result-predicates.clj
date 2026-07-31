;; ISSUE #171: ok/err/just/none/etc. moved OUT of clojure.core into
;; cljx.meta (ADR 0115, precedence principle) -- required and referred
;; here explicitly, exactly as real user code must.
(require (quote cljx.meta))
(clojure.core/refer (quote cljx.meta))

;; Result/Option predicates (ADR 0014): result?/ok?/err? classify the
;; Result family, option?/just?/none? the Option family — Go type
;; switches under the hood, so a just is not a result and none is not ok.
;; oracle: skip — cljgo Result/Option primitive (no JVM ok/err/just/none)
[(result? (ok 1)) (result? (just 1)) (ok? (ok 1)) (err? (err 1))
 (option? none) (just? (just 1)) (none? none) (ok? none)]
;; expect: [true false true true true true true false]
