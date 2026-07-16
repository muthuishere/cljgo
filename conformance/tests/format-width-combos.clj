;; format/printf (ADR 0030, spike S14): more width/x/o combos.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   x-width-zero-pad: (format "%08x" 255)
;;   o-width: (format "%6o|" 8)
;;   d-width-plain: (format "%6d|" 42)
[(format "%08x" 255)
 (format "%6o|" 8)
 (format "%6d|" 42)]
;; expect: ["000000ff" "    10|" "    42|"]
