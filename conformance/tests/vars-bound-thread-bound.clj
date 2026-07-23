;; bound? / thread-bound? (fundamentals batch A1): bound? — every var has
;; a value (root binding OR in-effect thread binding, Var.IsBound);
;; thread-bound? — every var has a thread binding in effect on the calling
;; thread (goroutine). A (def name)-without-init var is unbound; a
;; dynamic var counts as thread-bound only inside `binding`.
;; oracle (clojure 1.12.5, 2026-07-23): [true false false false true false true]
(def bx 1)
(def by)
(def ^:dynamic bz 1)
[(bound? #'bx) (bound? #'by) (bound? #'bx #'by)
 (thread-bound? #'bz) (binding [bz 2] (thread-bound? #'bz)) (thread-bound? #'bx)
 (binding [bz 2] (bound? #'bz))]
;; expect: [true false false false true false true]
