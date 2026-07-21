;; clojure.data/diff on atoms (core/data.cljg, fundamentals audit 2026-07):
;; anything non-collection — numbers, strings, keywords, nil — compares as a
;; unit; strings are atoms, never sequentials; a Double never equals a Long.
;; oracle (clojure 1.12.5, 2026-07-21): each element verified with the
;; `clojure` CLI, e.g. (diff 1 2) => [1 2 nil]; (diff "ab" "ab") =>
;; [nil nil "ab"]; (diff nil 1) => [nil 1 nil]; (diff 1.0 1) => [1.0 1 nil].
(require '[clojure.data :as d])
[(d/diff 1 2)
 (d/diff 1 1)
 (d/diff "ab" "ab")
 (d/diff "a" "b")
 (d/diff nil nil)
 (d/diff nil 1)
 (d/diff :k :k)
 (d/diff 1.0 1)]
;; expect: [[1 2 nil] [nil nil 1] [nil nil "ab"] ["a" "b" nil] [nil nil nil] [nil 1 nil] [nil nil :k] [1.0 1 nil]]
