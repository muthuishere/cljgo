;; format/printf (ADR 0030, spike S14): BigInt / Ratio.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   s-ratio: (format "%s" 1/3)
[(format "%s" 1/3)]
;; expect: ["1/3"]
