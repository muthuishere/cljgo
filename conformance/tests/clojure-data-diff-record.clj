;; clojure.data/diff on defrecords (core/data.cljg): records diff as maps —
;; the JVM gets this from its java.util.Map impl, cljgo from diff's
;; predicate fallback (see the header note in core/data.cljg) — and a
;; record vs a plain map with the same keys diffs as all-same even though
;; (= record map) is false.
;; oracle (clojure 1.12.5, 2026-07-21): with (defrecord P [a]):
;;   (diff (->P 1) (->P 2)) => ({:a 1} {:a 2} nil)
;;   (diff (->P 1) {:a 1}) => (nil nil {:a 1})
(require '[clojure.data :as d])
(defrecord P [a])
[(d/diff (->P 1) (->P 2))
 (d/diff (->P 1) {:a 1})]
;; expect: [({:a 1} {:a 2} nil) (nil nil {:a 1})]
