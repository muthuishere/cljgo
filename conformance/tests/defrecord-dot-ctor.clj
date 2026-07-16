;; (T. a b) on a defrecord/deftype name is the positional constructor —
;; sugar for (->T a b). Previously the analyzer sent every `.`-terminated
;; operator to the Go struct-ctor path, so record dot-ctors died with
;; "unable to resolve Go type" (precedence principle: a Clojure-defined
;; type wins over interop; suite dissoc.cljc).
;; oracle (clojure 1.12.5): [1 2 {:b 2}]
(defrecord DotCtorRec [a b])
(let [r (DotCtorRec. 1 2)]
  [(:a r) (:b r) (into {} (dissoc r :a))])
;; expect: [1 2 {:b 2}]
