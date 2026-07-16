;; format/printf (ADR 0030, spike S14): %s of a Double (Java's Double.toString vs Go's %v).
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   s-double-whole: (format "%s" 3.0)
;;   s-double-frac: (format "%s" 3.14)
;;   s-double-large-sci: (format "%s" 1.2345E9)
;;   s-double-small-sci: (format "%s" 1.5E-5)
[(format "%s" 3.0)
 (format "%s" 3.14)
 (format "%s" 1.2345E9)
 (format "%s" 1.5E-5)]
;; expect: ["3.0" "3.14" "1.2345E9" "1.5E-5"]
