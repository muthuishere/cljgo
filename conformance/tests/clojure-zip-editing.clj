;; clojure.zip editing (fundamentals audit 2026-07): embedded core/zip.cljg.
;; Covers replace, edit, insert-left, insert-right, insert-child,
;; append-child, remove, and root reflecting accumulated edits — including
;; the walk/edit sequences from clojure/zip.clj's own comment block
;; (replace-*-with-/ over '[[a * b] + [c * d]], remove-then-walk, remove at
;; a leftmost child collapsing into the parent, end? after remove+next).
;; seq-zip and xml-zip round-trip their make-node constructors (xml-zip's
;; assoc :content, seq-zip's meta-preserving with-meta).
;; oracle (clojure 1.12.5, `clojure` CLI): this exact vector prints
;; byte-identically on the JVM (verified 2026-07-21).
(require '[clojure.zip :as z])
(def dz (z/vector-zip '[[a * b] + [c * d]]))
[(-> dz z/down z/right z/right z/down z/right (z/replace '/) z/root)
 (-> dz z/next z/next (z/edit str) z/next z/next z/next (z/replace '/) z/root)
 (-> [1 2] z/vector-zip z/down (z/edit + 10) z/root)
 (-> [1 2] z/vector-zip z/down (z/insert-left 0) z/root)
 (-> [1 2] z/vector-zip z/down (z/insert-right 9) z/root)
 (-> [[2 3]] z/vector-zip z/down (z/insert-child 1) z/root)
 (-> [[1 2]] z/vector-zip z/down (z/append-child 3) z/root)
 (-> dz z/next z/next z/next z/next z/next z/next z/next z/next z/next z/remove z/root)
 (-> dz z/next z/next z/next z/next z/next z/next z/next z/next z/next z/remove (z/insert-right 'e) z/root)
 (-> dz z/next z/next z/next z/next z/next z/next z/next z/next z/next z/remove z/up (z/append-child 'e) z/root)
 (z/end? (-> dz z/next z/next z/next z/next z/next z/next z/next z/next z/next z/remove z/next))
 (-> dz z/next z/remove z/next z/remove z/root)
 (loop [loc dz] (if (z/end? loc) (z/root loc) (recur (z/next (if (= '* (z/node loc)) (z/replace loc '/) loc)))))
 (loop [loc dz] (if (z/end? loc) (z/root loc) (recur (z/next (if (= '* (z/node loc)) (z/remove loc) loc)))))
 (-> '(1 (2 3)) z/seq-zip z/down z/right z/down (z/replace 9) z/root)
 (-> {:tag :a :content [{:tag :b :content ["x"]}]} z/xml-zip z/down (z/append-child "y") z/root)
 (-> dz z/down z/remove z/node)]
;; expect: [[[a * b] + [c / d]] [["a" * b] / [c * d]] [11 2] [0 1 2] [1 9 2] [[1 2 3]] [[1 2 3]] [[a * b] + [c *]] [[a * b] + [c * e]] [[a * b] + [c * e]] true [[c * d]] [[a / b] + [c / d]] [[a b] + [c d]] (1 (9 3)) {:tag :a, :content [{:tag :b, :content ["x" "y"]}]} [+ [c * d]]]
