;; Full bit-* surface over 64-bit longs, with Java-style shift masking
;; (1 << 64 == 1). Deterministic; identical in both harness modes.
[(bit-and 12 10)
 (bit-or 12 10)
 (bit-xor 12 10)
 (bit-not 0)
 (bit-and-not 15 9)
 (bit-shift-left 1 4)
 (bit-shift-right -8 1)
 (unsigned-bit-shift-right -1 60)
 (bit-flip 2r0000 3)
 (bit-set 0 2)
 (bit-clear 15 1)
 (bit-test 5 0)
 (bit-test 5 1)
 (bit-shift-left 1 64)]
;; expect: [8 14 6 -1 6 16 -4 15 8 4 13 true false 1]
