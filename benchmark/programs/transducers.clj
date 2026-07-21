;; Transducer pipeline — no intermediate collections
(transduce
  (comp (map #(* % %))
        (filter even?))
  + 0
  (range 100000))
