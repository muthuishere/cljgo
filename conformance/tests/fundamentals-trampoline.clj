;; trampoline (fundamentals audit 2026-07): keeps calling returned fns
;; until a non-fn value comes back — deep mutual recursion without stack
;; growth (10000 bounces below).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (trampoline (fn f [n] (if (zero? n) :done #(f (dec n)))) 10000) => :done
;;   (trampoline + 1 2) => 3
;;   (trampoline (fn [] 42)) => 42
;;   (trampoline (fn [] (fn [] 7))) => 7
[(trampoline (fn f [n] (if (zero? n) :done #(f (dec n)))) 10000)
 (trampoline + 1 2)
 (trampoline (fn [] 42))
 (trampoline (fn [] (fn [] 7)))]
;; expect: [:done 3 42 7]
