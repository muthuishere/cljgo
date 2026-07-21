;; clojure.data/diff on sets (core/data.cljg): never subdiffed —
;; [only-in-a only-in-b intersection], empty components as nil.
;; Components are kept to <=1 element so the frozen print is
;; order-independent (set print order is host-specific).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (diff #{1 2} #{2 3}) => [#{1} #{3} #{2}]
;;   (diff #{1} #{1}) => [nil nil #{1}]
;;   (diff #{1} #{2}) => [#{1} #{2} nil]
(require '[clojure.data :as d])
[(d/diff #{1 2} #{2 3})
 (d/diff #{1} #{1})
 (d/diff #{1} #{2})]
;; expect: [[#{1} #{3} #{2}] [nil nil #{1}] [#{1} #{2} nil]]
