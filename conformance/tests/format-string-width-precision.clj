;; format/printf (ADR 0030, spike S14): width / precision on strings.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   s-width: (format "%10s|" "hi")
;;   s-left-width: (format "%-10s|" "hi")
;;   s-precision-truncate: (format "%.3s" "hello")
;;   s-width-precision: (format "%10.3s|" "hello")
[(format "%10s|" "hi")
 (format "%-10s|" "hi")
 (format "%.3s" "hello")
 (format "%10.3s|" "hello")]
;; expect: ["        hi|" "hi        |" "hel" "       hel|"]
