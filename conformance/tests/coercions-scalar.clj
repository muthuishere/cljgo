;; Batch 1 scalar coercions (ADR 0022, design/08 §5). Oracle (clojure 1.12):
;; (int 5.9) => 5; (char 97) => \a; (double 1) => 1.0; (boolean nil) => false;
;; (num 5) => 5; (long 5) => 5.
[(int 5.9) (long 5) (double 1) (float? (float 1)) (char 97)
 (byte 5) (short 5) (boolean nil) (boolean 0) (num 5)
 (= (int 65) (long 65)) (int? (int 5)) (int? (byte 5))]
;; expect: [5 5 1.0 true \a 5 5 false true 5 true true true]
