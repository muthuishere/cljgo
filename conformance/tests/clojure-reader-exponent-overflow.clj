;; A float literal whose exponent overflows/underflows float64 saturates to
;; +-Infinity/0.0 instead of being an "Invalid number" reader error — Go's
;; strconv.ParseFloat returns the correctly-signed value alongside its
;; ErrRange error, matching Java's Double.parseDouble (pkg/reader/number.go's
;; matchNumber — shared by clojure.core's reader and clojure.edn/read-string
;; alike; not edn-specific). oracle (clojure 1.12.5):
;;   (read-string "1e400") => ##Inf
;;   (read-string "-1e400") => ##-Inf
;;   (read-string "1e-400") => 0.0
;;   (clojure.edn/read-string "1e400") => ##Inf
(require '[clojure.edn :as edn])
[(edn/read-string "1e400")
 (edn/read-string "-1e400")
 (edn/read-string "1e-400")]
;; expect: [##Inf ##-Inf 0.0]
