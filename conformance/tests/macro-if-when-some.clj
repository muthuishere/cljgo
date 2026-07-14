;; if-some / when-some: bind + branch on non-nil (some?), NOT truthiness — a
;; bound false still takes the "some" branch. Oracle (clojure 1.12.5):
;; [(if-some [x (get {:a 1} :a)] x :none) (if-some [x nil] x :none)
;;  (when-some [x false] :got) (when-some [x nil] :got) (if-some [x false] :f :n)]
;; => [1 :none :got nil :f]
[(if-some [x (get {:a 1} :a)] x :none)
 (if-some [x nil] x :none)
 (when-some [x false] :got)
 (when-some [x nil] :got)
 (if-some [x false] :f :n)]
;; expect: [1 :none :got nil :f]
