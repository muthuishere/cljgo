;; clojure.data/diff on maps (core/data.cljg): subdiffed where keys match,
;; nested maps recurse, nil values distinguish "present as nil" from
;; "absent" (contains?-based), and per-key results merge into the
;; [only-a only-b in-both] triple (a seq — prints as a list, as on the JVM).
;; oracle (clojure 1.12.5, 2026-07-21): each element verified with the
;; `clojure` CLI, e.g. (diff {:a 1} {:a 2}) => ({:a 1} {:a 2} nil);
;; (diff {:a nil :b 1} {:a nil}) => ({:b 1} nil {:a nil});
;; (diff {1 2} {}) => ({1 2} nil nil).
(require '[clojure.data :as d])
[(d/diff {:a 1} {:a 2})
 (d/diff {:a 1 :b 2} {:a 1 :c 3})
 (d/diff {:a {:b 1 :c 2}} {:a {:b 1 :c 3}})
 (d/diff {:a nil :b 1} {:a nil})
 (d/diff {1 2} {})
 (d/diff {:a 1} {:a 1})]
;; expect: [({:a 1} {:a 2} nil) ({:b 2} {:c 3} {:a 1}) ({:a {:c 2}} {:a {:c 3}} {:a {:b 1}}) ({:b 1} nil {:a nil}) ({1 2} nil nil) [nil nil {:a 1}]]
