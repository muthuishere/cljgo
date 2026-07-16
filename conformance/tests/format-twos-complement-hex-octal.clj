;; format/printf (ADR 0030, spike S14): two's-complement hex/octal on negative ints.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   x-negative-long: (format "%x" -1)
;;   o-negative-long: (format "%o" -1)
[(format "%x" -1)
 (format "%o" -1)]
;; expect: ["ffffffffffffffff" "1777777777777777777777"]
