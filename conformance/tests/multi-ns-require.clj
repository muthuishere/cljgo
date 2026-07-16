;; Multi-namespace require (ADR 0042): a file-backed namespace loads at
;; the require site; cross-ns fns and macros (expanded at analysis) work.
;; oracle (Clojure 1.12.5, 2026-07-17, clojure -Sdeps '{:paths ["."]}'
;; -M multi-ns-require.clj from conformance/tests): prints hello! and 9,
;; (load-file …) value 49.
(require '[conf.helper :as h])
(println (h/exclaim "hello"))
(println (h/square 3))
(h/square 7)
;; expect: 49
