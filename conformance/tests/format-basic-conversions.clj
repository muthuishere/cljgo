;; format/printf (ADR 0030, spike S14): one conversion, one arg, no flags.
;; Oracle: real `clojure` CLI 1.12.5, re-verified at freeze time
;; (2026-07-16) via a fresh RunOracle() pass over the spike's 80-probe
;; corpus (spikes/s14-format-grammar/corpus.go) -- read-only reference,
;; never edited. Probes in this file:
;;   d-int: (format "%d" 42)
;;   d-negative: (format "%d" -42)
;;   f-default-prec: (format "%f" 3.14)
;;   e-default: (format "%e" 3.14)
;;   g-default: (format "%g" 3.14)
;;   x-basic: (format "%x" 255)
;;   o-basic: (format "%o" 8)
;;   c-char-lit: (format "%c" \A)
;;   b-true: (format "%b" true)
;;   b-false: (format "%b" false)
;;   n-newline: (format "a%nb")
;;   pct-literal: (format "100%%")
;;   s-string: (format "%s" "hello")
;;   s-nil: (format "%s" nil)
;;   s-keyword: (format "%s" :kw)
[(format "%d" 42)
 (format "%d" -42)
 (format "%f" 3.14)
 (format "%e" 3.14)
 (format "%g" 3.14)
 (format "%x" 255)
 (format "%o" 8)
 (format "%c" \A)
 (format "%b" true)
 (format "%b" false)
 (format "a%nb")
 (format "100%%")
 (format "%s" "hello")
 (format "%s" nil)
 (format "%s" :kw)]
;; expect: ["42" "-42" "3.140000" "3.140000e+00" "3.14000" "ff" "10" "A" "true" "false" "a\nb" "100%" "hello" "null" ":kw"]
