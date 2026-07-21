;; clojure.data/diff on sequentials (core/data.cljg): compared by index as
;; associative collections, results as vectors padded with nil; lists and
;; vectors are the same partition; equal sequentials of different concrete
;; type return [nil nil a].
;; oracle (clojure 1.12.5, 2026-07-21): each element verified with the
;; `clojure` CLI, e.g. (diff [1 2 3] [1 9 3 4]) =>
;; [[nil 2] [nil 9 nil 4] [1 nil 3]]; (diff '(1 2) [1 2]) => [nil nil (1 2)].
(require '[clojure.data :as d])
[(d/diff [1 2 3] [1 9 3 4])
 (d/diff [1 2] [1 2 3])
 (d/diff [nil] [nil nil])
 (d/diff (list 1 2) [1 2])
 (d/diff [1 2 3] (list 1 5 3))]
;; expect: [[[nil 2] [nil 9 nil 4] [1 nil 3]] [nil [nil nil 3] [1 2]] [nil [nil nil] [nil]] [nil nil (1 2)] [[nil 2] [nil 5] [1 nil 3]]]
