;; format/printf (ADR 0030, spike S14): width / precision on f/e/g.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   f-width: (format "%10.2f|" 3.14159)
;;   f-left-width: (format "%-10.2f|" 3.14159)
;;   f-zero-precision: (format "%.0f" 3.7)
;;   e-width-precision: (format "%12.3e|" 31415.9)
;;   g-precision: (format "%.2g" 31415.9)
;;   g-small: (format "%g" 0.0000012345)
[(format "%10.2f|" 3.14159)
 (format "%-10.2f|" 3.14159)
 (format "%.0f" 3.7)
 (format "%12.3e|" 31415.9)
 (format "%.2g" 31415.9)
 (format "%g" 0.0000012345)]
;; expect: ["      3.14|" "3.14      |" "4" "   3.142e+04|" "3.1e+04" "1.23450e-06"]
