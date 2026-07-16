;; format/printf (ADR 0030, spike S14): nil / truthiness edge cases.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   d-nil-throws: (format "%d" nil)
;;   b-nil-is-false: (format "%b" nil)
;;   b-truthy-string: (format "%b" "x")
;;   b-truthy-zero: (format "%b" 0)
;;   b-truthy-false-boxed: (format "%b" false)
[(format "%d" nil)
 (format "%b" nil)
 (format "%b" "x")
 (format "%b" 0)
 (format "%b" false)]
;; expect: ["null" "false" "true" "true" "false"]
