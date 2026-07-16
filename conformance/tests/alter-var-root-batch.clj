;; alter-var-root (design/08 batch E, ADR 0022): (alter-var-root #'v f &
;; args) applies f to the current root plus args, sets and returns the
;; new root — the primitive derive/underive ride on internally.
;; oracle (clojure 1.12.5): (def ^:private cnt 0)
;; (alter-var-root #'cnt inc) => 1; (alter-var-root #'cnt + 10) => 11.
(do
  (def ^:private avr-cnt 0)
  [(alter-var-root #'avr-cnt inc)
   (alter-var-root #'avr-cnt + 10)
   avr-cnt])
;; expect: [1 11 11]
