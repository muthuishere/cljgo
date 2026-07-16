;; Reader metadata on vector/map/set literals (`^:foo {}` etc.) must survive
;; evaluation. analyzeVector/analyzeMap/analyzeSet keep the read form (with
;; its meta) on ast.Node.Form, but eval's OpVector/OpMap/OpSet previously
;; rebuilt a brand-new collection from the evaluated items and threw that
;; metadata away.
;; oracle (clojure 1.12.5):
;; [(:foo (meta ^:foo {})) (:foo (meta ^:foo [1 2])) (:foo (meta ^:foo #{1 2}))]
;; => [true true true]
[(:foo (meta ^:foo {})) (:foo (meta ^:foo [1 2])) (:foo (meta ^:foo #{1 2}))]
;; expect: [true true true]
