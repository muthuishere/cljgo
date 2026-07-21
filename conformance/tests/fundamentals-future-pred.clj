;; future? (fundamentals audit 2026-07): the one missing member of the
;; future family — true only for futures, not promises/delays/values.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (future? (future 1)) => true (and @ it => 1)
;;   (future? 1) => false
;;   (future? (promise)) => false
;;   (future? (delay 1)) => false
(def fu (future 1))
[(future? fu) (deref fu) (future? 1) (future? (promise)) (future? (delay 1))]
;; expect: [true 1 false false false]
