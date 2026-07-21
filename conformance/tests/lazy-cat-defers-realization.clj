;; lazy-cat (fundamentals batch 1): concat of lazy-seq-wrapped colls —
;; the arms are NOT evaluated until the result is walked; () with no args.
;; oracle (clojure 1.12.5): defining (lazy-cat (spy 1) (spy 2)) records
;; nothing; (doall lc) => (1 2) and the spy log is [1 2]; (lazy-cat) => ().
(def side (atom []))
(defn spy [x] (swap! side conj x) [x])
(def lc (lazy-cat (spy 1) (spy 2)))
(def before-force @side)
[(vec (doall lc)) before-force @side (lazy-cat)]
;; expect: [[1 2] [] [1 2] ()]
