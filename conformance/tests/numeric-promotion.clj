;; Integer overflow: checked +/-/* throw (Batch 2), promoting +'/-'/*' and
;; the unchecked-* variants do not. Promotion must agree interpreted vs
;; compiled (dual-mode, ADR 0002) — the prime ops are the same host builtin.
[(+' 9223372036854775807 1)
 (*' 9223372036854775807 2)
 (-' -9223372036854775808 1)
 (inc' 9223372036854775807)
 (dec' -9223372036854775808)
 (unchecked-add 9223372036854775807 1)
 (unchecked-multiply 9223372036854775807 2)
 (+ 1 2 3)]
;; expect: [9223372036854775808N 18446744073709551614N -9223372036854775809N 9223372036854775808N -9223372036854775809N -9223372036854775808 -2 6]
