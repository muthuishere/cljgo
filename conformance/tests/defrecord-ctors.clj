;; defrecord constructors and printing: positional ->R, map->R (extra keys
;; kept), assoc preserves the record type, records print as #ns.R{...}.
;; Verified vs Clojure CLI 1.12.5:
;;   (defrecord R [a b]) =>
;;   [#user.R{:a 1, :b 2} #user.R{:a 1, :b 2, :c 3} #user.R{:a 9, :b 2} 3]
;; expect: [#user.R{:a 1, :b 2} #user.R{:a 1, :b 2, :c 3} #user.R{:a 9, :b 2} 3]
(defrecord R [a b])

[(->R 1 2)
 (map->R {:a 1 :b 2 :c 3})
 (assoc (->R 1 2) :a 9)
 (:c (map->R {:a 1 :b 2 :c 3}))]
