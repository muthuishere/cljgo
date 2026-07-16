;; Multi-namespace load order (ADR 0042): deps load at the require SITE,
;; interleaved with the entry's own side effects, depth-first, exactly
;; once (the second require of conf.chain-b must not reload it).
;; oracle (Clojure 1.12.5, 2026-07-17, clojure -Sdeps '{:paths ["."]}'
;; -M multi-ns-load-order.clj from conformance/tests): prints
;; entry-start / load chain-b / load chain-a / entry-end; value 30.
(println "entry-start")
(require '[conf.chain-a :as a])
(require '[conf.chain-b :as b])
(println "entry-end")
(+ a/a-val b/b-val)
;; expect: 30
