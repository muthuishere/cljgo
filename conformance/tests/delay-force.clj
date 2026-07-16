;; delay / force / delay? / realized? (ADR 0022 batch/harness-misc): the body
;; is not evaluated until forced, forced at most once (memoized), force on a
;; non-delay returns it unchanged.
;; oracle (clojure 1.12.5):
;;   (force (delay (+ 1 2))) => 3; (delay? (delay 1)) => true;
;;   (delay? 5) => false; (force 42) => 42; realized? flips false->true on
;;   first force; a side-effecting body runs once for two forces.
[(force (delay (+ 1 2)))
 (delay? (delay 1))
 (delay? 5)
 (force 42)
 (let [n (volatile! 0)
       d (delay (vswap! n inc) 7)
       before (realized? d)
       v1 (force d)
       after (realized? d)
       v2 (force d)]
   [before v1 after v2 @n])]
;; expect: [3 true false 42 [false 7 true 7 1]]
