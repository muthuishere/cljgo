;; clojure.zip constructors + navigation (fundamentals audit 2026-07):
;; embedded core/zip.cljg, a straight port of clojure/zip.clj (EPL 1.0) onto
;; core.clj primitives. Covers zipper (custom map zipper), seq-zip,
;; vector-zip, xml-zip, node, branch?, children, make-node, path, lefts,
;; rights, down, up, right, rightmost, left, leftmost, next, prev, end?,
;; root — the read-only half of the 28 publics (editing in
;; clojure-zip-editing.clj, error paths in clojure-zip-errors.clj).
;; oracle (clojure 1.12.5, `clojure` CLI): this exact vector prints
;; byte-identically on the JVM (verified 2026-07-21); the depth-first z/next
;; walk of '[[a * b] + [c * d]] and the end-loc behaviors ([node :end] stays
;; put under next, root at end returns the node) match Huet-zipper semantics.
(require '[clojure.zip :as z])
(def dz (z/vector-zip '[[a * b] + [c * d]]))
(def mz (z/zipper map? :kids (fn [n ks] (assoc n :kids (vec ks))) {:v 1 :kids [{:v 2} {:v 3}]}))
[(z/node (z/vector-zip [1 [2 3]]))
 (-> [1 [2 3]] z/vector-zip z/down z/right z/down z/node)
 (-> '(1 (2 3)) z/seq-zip z/down z/right z/down z/node)
 (-> {:tag :a :content [{:tag :b :content ["x"]} "y"]} z/xml-zip z/down z/down z/node)
 (z/branch? dz)
 (-> dz z/down z/branch?)
 (z/children (z/vector-zip [1 [2]]))
 (z/make-node dz [1] '(9 8))
 (-> mz z/down z/right z/node)
 (-> dz z/down z/right z/right z/down z/path)
 (-> dz z/down z/right z/right z/down z/right z/lefts)
 (-> dz z/down z/right z/right z/down z/right z/rights)
 (-> dz z/down z/rights)
 (-> dz z/down z/right z/left z/node)
 (-> dz z/down z/rightmost z/node)
 (-> dz z/down z/rightmost z/leftmost z/node)
 (-> dz z/down z/left)
 (-> dz z/down z/right z/right z/right)
 (z/up dz)
 (-> dz z/down z/up z/node)
 (loop [loc dz acc []] (if (z/end? loc) acc (recur (z/next loc) (conj acc (z/node loc)))))
 (z/end? dz)
 (-> dz z/next z/next z/next z/prev z/node)
 (z/prev dz)
 (let [end (loop [loc dz] (if (z/end? loc) loc (recur (z/next loc))))]
   [(z/end? end) (z/node (z/next end)) (z/root end)])]
;; expect: [[1 [2 3]] 2 2 "x" true true (1 [2]) [9 8] {:v 3} [[[a * b] + [c * d]] [c * d]] (c) (d) (+ [c * d]) [a * b] [c * d] [a * b] nil nil nil [[a * b] + [c * d]] [[[a * b] + [c * d]] [a * b] a * b + [c * d] c * d] false a nil [true [[a * b] + [c * d]] [[a * b] + [c * d]]]]
