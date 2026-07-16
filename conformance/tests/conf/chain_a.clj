;; Dependency namespace (ADR 0042): requires conf.chain-b — exercises
;; dep-of-dep loading, ns-name munging (- → _), and load-once.
(ns conf.chain-a
  (:require [conf.chain-b :as b]))
(println "load chain-a")
(def a-val (+ b/b-val 10))
