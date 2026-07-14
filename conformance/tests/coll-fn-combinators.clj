;; comp/partial/complement/constantly/identity/fnil/juxt/every-pred/some-fn.
;; oracle (clojure 1.12.5):
;;   ((comp inc inc) 5) => 7
;;   ((partial + 10) 5) => 15
;;   ((complement even?) 3) => true
;;   ((constantly 42) 1 2 3) => 42
;;   (identity 7) => 7
;;   ((fnil inc 0) nil) => 1
;;   ((juxt inc dec) 5) => [6 4]
;;   ((every-pred even? pos?) 4) => true
;;   ((some-fn even? neg?) 3) => false
[((comp inc inc) 5)
 ((partial + 10) 5)
 ((complement even?) 3)
 ((constantly 42) 1 2 3)
 (identity 7)
 ((fnil inc 0) nil)
 ((juxt inc dec) 5)
 ((every-pred even? pos?) 4)
 ((some-fn even? neg?) 3)]
;; expect: [7 15 true 42 7 1 [6 4] true false]
