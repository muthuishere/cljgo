;; format/printf (ADR 0030, spike S14): flags on numeric conversions.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   d-plus-flag: (format "%+d" 42)
;;   d-space-flag: (format "% d" 42)
;;   d-paren-flag-neg: (format "%(d" -42)
;;   d-paren-flag-pos: (format "%(d" 42)
;;   d-comma-grouping: (format "%,d" 1234567)
;;   d-zero-pad: (format "%010d" 42)
;;   d-left-justify: (format "%-10d|" 42)
;;   x-alt-form: (format "%#x" 255)
;;   o-alt-form: (format "%#o" 8)
;;   f-plus-flag: (format "%+.2f" 3.14159)
;;   f-comma-grouping: (format "%,.2f" 1234567.891)
;;   f-paren-neg: (format "%(.2f" -3.14159)
[(format "%+d" 42)
 (format "% d" 42)
 (format "%(d" -42)
 (format "%(d" 42)
 (format "%,d" 1234567)
 (format "%010d" 42)
 (format "%-10d|" 42)
 (format "%#x" 255)
 (format "%#o" 8)
 (format "%+.2f" 3.14159)
 (format "%,.2f" 1234567.891)
 (format "%(.2f" -3.14159)]
;; expect: ["+42" " 42" "(42)" "42" "1,234,567" "0000000042" "42        |" "0xff" "010" "+3.14" "1,234,567.89" "(3.14)"]
