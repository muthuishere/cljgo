;; Batch A3: byte-array / short-array / bytes? / make-array, completing
;; the ADR 0025 array family. A cljgo byte array is []int8 (the JVM's
;; SIGNED byte, so [1 -1] round-trips) and fills via the wrapping cast
;; (Number.byteValue() semantics: 200 => -56, oracle-identical). bytes?
;; is true only for byte arrays. make-array over a well-known class
;; (ADR 0036 ClassRef) builds nil-filled object arrays, nested per
;; dimension, exactly as the JVM's Class-typed make-array does.
;; Divergence (documented at the def site): the JVM's 2-arity
;; (byte-array n init) demands a Byte-typed init; cljgo's single-fixnum
;; tower accepts its int64 — that form is therefore not frozen here.
;; Oracle (clojure 1.12.5): verified 2026-07-23.
[(vec (byte-array 3))
 (vec (byte-array [1 2 3]))
 (vec (byte-array [1 -1]))
 (vec (byte-array [200]))
 (vec (short-array 2))
 (vec (short-array [1 2]))
 (bytes? (byte-array 1))
 (bytes? (int-array 1))
 (bytes? "s")
 (bytes? nil)
 (vec (make-array Long 3))
 (count (make-array String 2))
 (vec (map vec (make-array Object 2 3)))]
;; expect: [[0 0 0] [1 2 3] [1 -1] [-56] [0 0] [1 2] true false false false [nil nil nil] 2 [[nil nil nil] [nil nil nil]]]
