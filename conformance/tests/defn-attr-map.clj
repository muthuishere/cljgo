;; defn attr-map parity: the documented (defn name doc-string? attr-map?
;; ...) form, plus a TRAILING attr-map, plus the ambiguity case where the
;; body IS a map. clojure.core/defn normalizes a single-arity [params] into
;; the multi-arity shape BEFORE taking a trailing attr-map, which is what
;; keeps (defn f [x] {:a 1}) from losing its body. A trailing attr-map wins
;; over a leading one on key conflicts. defn- inherits all of it.
;;
;; oracle (clojure 1.12.5, `clojure -M -e "<the forms below>"`):
;; [1 5 nil ([& xs]) "Return the least argument." "just a doc" ([x]) 2 1 nil
;;  42 "d" 1 2 3 :lead :trail :trail {:a 9} nil ([x]) {:b 2} nil 4 1 true]
(defn least
  "Return the least argument."
  {:arglists '([& xs])}
  ([] nil)
  ([a] a)
  ([a b] (if (neg? (compare a b)) a b)))
(defn doc-only "just a doc" [x] (inc x))
(defn attr-only {:a 1} [x] (inc x))
(defn doc+attr "d" {:a 1} [x] (* 2 x))
(defn trailing-multi ([x] (inc x)) {:c 3})
(defn both {:k :lead :shared :lead} ([x] x) {:k2 :trail :shared :trail})
(defn body-is-map [x] {:a x})
(defn trailing-single [x] (inc x) {:b 2})
(defn- pf {:p 1} [x] x)
[(least 3 1) (least 5) (least)
 (:arglists (meta #'least)) (:doc (meta #'least))
 (:doc (meta #'doc-only)) (:arglists (meta #'doc-only))
 (attr-only 1) (:a (meta #'attr-only)) (:doc (meta #'attr-only))
 (doc+attr 21) (:doc (meta #'doc+attr)) (:a (meta #'doc+attr))
 (trailing-multi 1) (:c (meta #'trailing-multi))
 (:k (meta #'both)) (:k2 (meta #'both)) (:shared (meta #'both))
 (body-is-map 9) (:a (meta #'body-is-map)) (:arglists (meta #'body-is-map))
 (trailing-single 1) (:b (meta #'trailing-single))
 (pf 4) (:p (meta #'pf)) (:private (meta #'pf))]
;; expect: [1 5 nil ([& xs]) "Return the least argument." "just a doc" ([x]) 2 1 nil 42 "d" 1 2 3 :lead :trail :trail {:a 9} nil ([x]) {:b 2} nil 4 1 true]
