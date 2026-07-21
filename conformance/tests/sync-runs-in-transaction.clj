;; sync (fundamentals batch 1): runs its body in a transaction like
;; dosync — alter is accepted inside — returning the body's value; the
;; first argument is the real macro's ignored flags placeholder.
;; oracle (clojure 1.12.5): (sync nil (+ 1 2)) => 3; with r1 = (ref 10),
;; (sync nil (alter r1 + 5)) => 15 and @r1 => 15.
(def r1 (ref 10))
[(sync nil (+ 1 2)) (sync nil (alter r1 + 5)) @r1]
;; expect: [3 15 15]
