;; case: constant dispatch (v0 = sequential = comparison). Test constants are
;; unevaluated; a list matches any member; a trailing odd clause is default.
;; Oracle (clojure 1.12.5):
;; [(case 2 1 :one 2 :two :default) (case 9 1 :one :default)
;;  (case :b :a 1 (:b :c) 2 :d) (case "hi" "hi" 1 2) (case 'foo foo :sym :no)]
;; => [:two :default 2 1 :sym]
[(case 2 1 :one 2 :two :default)
 (case 9 1 :one :default)
 (case :b :a 1 (:b :c) 2 :d)
 (case "hi" "hi" 1 2)
 (case 'foo foo :sym :no)]
;; expect: [:two :default 2 1 :sym]
