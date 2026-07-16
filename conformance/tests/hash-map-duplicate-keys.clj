;; hash-map dedupes repeated keys, last value wins — it must route through
;; NewPersistentArrayMapAsIfByAssoc (the same as-if-by-assoc path map literals
;; use), not a bare NewMap that keeps every key/val pair verbatim.
;; oracle (clojure 1.12.5): (hash-map :a 1 :a 2) => {:a 2}
(hash-map :a 1 :a 2)
;; expect: {:a 2}
