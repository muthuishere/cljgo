;; when-let, like if-let, accepts exactly one binding pair. Ours silently
;; accepted (and ignored) extra binding pairs instead of throwing.
;; oracle (clojure 1.12.5): (macroexpand '(when-let [x (range 5) y (range 5)]))
;; throws IllegalArgumentException ("when-let requires exactly 1 binding form")
(try (macroexpand '(when-let [x (range 5) y (range 5)])) :nothrow (catch Exception _e :threw))
;; expect: :threw
