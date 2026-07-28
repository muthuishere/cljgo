;; xorshift64 — pure bit twiddling through let bindings. Every op here
;; (bit-xor, bit-shift-left, unsigned-bit-shift-right) was boxed before.
(defn xorshift [x]
  (let [a (bit-xor x (bit-shift-left x 13))
        b (bit-xor a (unsigned-bit-shift-right a 7))]
    (bit-xor b (bit-shift-left b 17))))

(loop [i 0 s 88172645463325252]
  (if (< i 4000000)
    (recur (inc i) (xorshift s))
    (println s)))
