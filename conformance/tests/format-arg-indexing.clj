;; format/printf (ADR 0030, spike S14): argument indexing.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   idx-reorder: (format "%2$s %1$s" "a" "b")
;;   idx-reuse: (format "%1$s-%1$s" "x")
;;   idx-relative: (format "%1$s %<s" "a" "b")
;;   idx-mixed-implicit-then-explicit: (format "%s %2$s" "a" "b")
[(format "%2$s %1$s" "a" "b")
 (format "%1$s-%1$s" "x")
 (format "%1$s %<s" "a" "b")
 (format "%s %2$s" "a" "b")]
;; expect: ["b a" "x-x" "a a" "a b"]
