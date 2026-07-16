;; format/printf (ADR 0030, spike S14): multiple mixed conversions in one string.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   mixed-line: (format "%s is %d years old (%.1f%%)" "Alice" 30 12.345)
;;   printf-style-log: (format "[%-5s] %s" "WARN" "disk low")
[(format "%s is %d years old (%.1f%%)" "Alice" 30 12.345)
 (format "[%-5s] %s" "WARN" "disk low")]
;; expect: ["Alice is 30 years old (12.3%)" "[WARN ] disk low"]
