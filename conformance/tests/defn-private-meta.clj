;; defn- (fundamentals batch 1): defn with ^:private metadata on the
;; name; the fn itself works exactly like defn's.
;; oracle (clojure 1.12.5): (defn- pf [x] x) => (:private (meta #'pf))
;; is true; (macroexpand-1 '(defn- pf [x] x)) => (clojure.core/defn pf
;; [x] x) with {:private true} on the name symbol.
(defn- pf [x] (+ x 1))
[(pf 41) (:private (meta #'pf))]
;; expect: [42 true]
