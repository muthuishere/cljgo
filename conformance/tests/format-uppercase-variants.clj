;; format/printf (ADR 0030, spike S14): uppercase variants (only b,h,s,c,x,e,g,a,t have them in Java).
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   S-upper: (format "%S" "abc")
;;   B-upper: (format "%B" true)
;;   C-upper: (format "%C" \a)
;;   X-upper: (format "%X" 255)
;;   E-upper: (format "%E" 3.14)
;;   G-upper: (format "%G" 3.14)
[(format "%S" "abc")
 (format "%B" true)
 (format "%C" \a)
 (format "%X" 255)
 (format "%E" 3.14)
 (format "%G" 3.14)]
;; expect: ["ABC" "TRUE" "A" "FF" "3.140000E+00" "3.14000"]
