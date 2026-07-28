;; ADR 0067 second-table batch: quot/rem, min/max, unary -, long, the bit
;; operations, the unchecked (wrapping) arithmetic and the int64 predicates
;; now emit as raw Go inside a typed region instead of a boxed var deref +
;; lang.Apply. Every fn here is a single fixed-arity int64 body, so a
;; compiled binary takes the specialized/lifted path for it — this file is
;; the proof that the unboxed forms compute EXACTLY what the boxed tower
;; computes, edges included: quot/rem truncate toward zero on negatives
;; (never floor), shift counts mask to 6 bits so (bit-shift-left x 64) is x
;; and not 0, arithmetic vs unsigned right shift differ on a negative,
;; unchecked-* wrap silently at both int64 ends, and even?/odd? hold on
;; negative odd values (Go's % is truncating like the JVM's, so -3 % 2 is
;; -1 — odd? must test != 0, never == 1).
;; The throwing edges (integer division by zero, checked unary minus of
;; Long/MIN_VALUE) live in numeric-unboxed-second-table-throws.clj: cljgo's
;; wording for those predates this batch and diverges from the JVM's.
;; oracle (clojure 1.12.5, verified 2026-07-27): expectation vector below,
;; byte-identical.
(defn digit-sum [n] (loop [x n s 0] (if (< x 1) s (recur (quot x 10) (+ s (rem x 10))))))
(defn gcd [a b] (if (zero? b) a (recur b (rem a b))))
(defn trunc [a b] [(quot a b) (rem a b)])
(defn clamp [x] (min 100 (max 0 x)))
(defn mixer [x] (bit-xor (bit-shift-left x 13) (unsigned-bit-shift-right x 7)))
(defn bits [x y] [(bit-and x y) (bit-or x y) (bit-xor x y) (bit-and-not x y) (bit-not x)])
(defn poke [x n] [(bit-set x n) (bit-clear x n) (bit-flip x n)])
(defn shifts [x] [(bit-shift-left x 64) (bit-shift-right x 2) (unsigned-bit-shift-right x 60)])
(defn neg [x] (- x))
(defn wrapped [x] [(unchecked-add x 1) (unchecked-subtract x 1) (unchecked-multiply x 2)
                   (unchecked-inc x) (unchecked-dec x) (unchecked-negate x)])
(defn ident [x] (long (+ x 0)))
(defn classify [n] (if (even? n) :even (if (pos? n) :pos (if (neg? n) :neg :zero))))

[(digit-sum 987654)
 (gcd 1071 462)
 (trunc 7 2) (trunc -7 2) (trunc 7 -2) (trunc -7 -2)
 (clamp 250) (clamp -3) (clamp 42)
 (mixer 1) (mixer -1)
 (bits 12 10)
 (poke 8 3)
 (shifts 1) (shifts -16)
 (neg 7) (neg -9)
 (wrapped 9223372036854775807)
 (wrapped -9223372036854775808)
 (ident 5)
 (mapv classify [-4 -3 0 3 4])]
;; expect: [39 21 [3 1] [-3 -1] [-3 1] [3 -1] 100 0 42 8192 -144115188075847681 [8 14 6 4 -13] [8 0 0] [1 0 0] [-16 -4 15] -7 9 [-9223372036854775808 9223372036854775806 -2 -9223372036854775808 9223372036854775806 -9223372036854775807] [-9223372036854775807 9223372036854775807 0 -9223372036854775807 9223372036854775807 -9223372036854775808] 5 [:even :neg :even :pos :even]]
