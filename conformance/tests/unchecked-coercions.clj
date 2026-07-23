;; The unchecked truncating coercions: byte/short/int truncate to their
;; JVM widths (8/16/32 bits, values as longs), char masks to 16 bits,
;; long/double truncate/widen numerics — never range-checked, unlike the
;; checked byte/short/int coercions.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23):
;; [(unchecked-byte 300) (unchecked-byte -300) (unchecked-short 70000)
;; (unchecked-char 97) (unchecked-int 4294967296)
;; (unchecked-int 2147483648) (unchecked-long 1.9) (unchecked-float 1.5)
;; (unchecked-double 3)] => [44 -44 4464 \a 0 -2147483648 1 1.5 3.0]
[(unchecked-byte 300)
 (unchecked-byte -300)
 (unchecked-short 70000)
 (unchecked-char 97)
 (unchecked-int 4294967296)
 (unchecked-int 2147483648)
 (unchecked-long 1.9)
 (unchecked-float 1.5)
 (unchecked-double 3)]
;; expect: [44 -44 4464 \a 0 -2147483648 1 1.5 3.0]
