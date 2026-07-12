;; Collection literals evaluate their elements (single-element set:
;; set print order is undefined).
[[(+ 1 2)] {:a (+ 1 2)} #{(* 2 3)}]
;; expect: [[3] {:a 3} #{6}]
