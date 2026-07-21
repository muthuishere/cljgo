;; defonce (fundamentals batch 1): initializes only when the var has no
;; root binding — a second defonce is a nil no-op, and the value stays.
;; oracle (clojure 1.12.5): (defonce d1 1) => #'user/d1; (defonce d1 2)
;; => nil; d1 => 1.
(defonce dd 1)
(def second-defonce (defonce dd 2))
[dd (nil? second-defonce)]
;; expect: [1 true]
