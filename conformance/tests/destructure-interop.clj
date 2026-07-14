;; The ergonomic payoff: [v err] destructuring over a Go (T, error) return.
;; strconv/Atoi "x" yields [0 <err>]; the err slot is truthy so the program
;; branches to :bad. Errors-as-values, read straight out of the binding.
;; oracle: skip — Go interop has no JVM Clojure equivalent
(require-go '[strconv])
(let [[v err] (strconv/Atoi "x")]
  (if err :bad v))
;; expect: :bad
