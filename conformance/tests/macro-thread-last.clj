;; ->> threads as LAST argument. Oracle (clojure 1.12.5):
;; (macroexpand-1 '(->> 5 (- 20) (* 2))) => (* 2 (- 20 5)) => 30.
(->> 5 (- 20) (* 2))
;; expect: 30
