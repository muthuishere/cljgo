;; or must not evaluate later forms once one is truthy. Oracle
;; (clojure 1.12.5): *hits* stays 0.
(def ^:dynamic *hits* 0)
(binding [*hits* 0]
  (or 1 (set! *hits* 99))
  *hits*)
;; expect: 0
