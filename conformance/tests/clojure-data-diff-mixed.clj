;; clojure.data/diff across partitions (core/data.cljg): values in different
;; equality partitions (map vs sequential, map vs nil) compare as atoms;
;; nested values recurse into their own partitions.
;; oracle (clojure 1.12.5, 2026-07-21): each element verified with the
;; `clojure` CLI, e.g. (diff {:a 1} [1 2]) => [{:a 1} [1 2] nil];
;; (diff {:a [1 2]} {:a [1 3]}) => ({:a [nil 2]} {:a [nil 3]} {:a [1]}).
(require '[clojure.data :as d])
[(d/diff {:a 1} [1 2])
 (d/diff {:a 1} nil)
 (d/diff {:a [1 2]} {:a [1 3]})]
;; expect: [[{:a 1} [1 2] nil] [{:a 1} nil nil] ({:a [nil 2]} {:a [nil 3]} {:a [1]})]
