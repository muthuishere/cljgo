;; doto (side-effect then return the object), while (loop while truthy),
;; dorun (force for effect -> nil), doall (force AND return the seq).
;; Oracle (clojure 1.12.5):
;; [(let [a (doto (atom 0) (reset! 5) (swap! inc))] @a)
;;  (let [c (atom 0)] (while (< @c 3) (swap! c inc)) @c)
;;  (dorun (map identity [1 2 3])) (doall (map inc [1 2 3]))] => [6 3 nil (2 3 4)]
[(let [a (doto (atom 0) (reset! 5) (swap! inc))] @a)
 (let [c (atom 0)] (while (< @c 3) (swap! c inc)) @c)
 (dorun (map identity [1 2 3]))
 (doall (map inc [1 2 3]))]
;; expect: [6 3 nil (2 3 4)]
