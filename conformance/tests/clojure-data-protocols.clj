;; clojure.data's protocol publics (core/data.cljg): equality-partition
;; classifies a value's diff partition; diff-similar subdiffs two values of
;; the same partition. Both are real protocol methods (EqualityPartition /
;; Diff), extensible by user code.
;; oracle (clojure 1.12.5, 2026-07-21): each element verified with the
;; `clojure` CLI, e.g. (equality-partition {:a 1}) => :map;
;; (equality-partition '(1 2)) => :sequential;
;; (diff-similar [1 2] [1 3]) => [[nil 2] [nil 3] [1]].
(require '[clojure.data :as d])
[(d/equality-partition {:a 1})
 (d/equality-partition [1])
 (d/equality-partition #{1})
 (d/equality-partition (list 1 2))
 (d/equality-partition 1)
 (d/equality-partition "s")
 (d/equality-partition nil)
 (d/diff-similar [1 2] [1 3])]
;; expect: [:map :sequential :set :sequential :atom :atom :atom [[nil 2] [nil 3] [1]]]
