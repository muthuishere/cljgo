;; load-string (fundamentals batch A1): read every form from the string
;; and eval them in order, returning the last value (nil for an empty
;; string). Defs take effect in the current namespace.
;; harness: eval — load-string rides `eval`, which is ADR 0046's
;; bound-and-throwing stub in an AOT binary (the CLJS model: no analyzer
;; is linked into compiled programs).
;; oracle (clojure 1.12.5, 2026-07-23): [42 nil]
[(load-string "(def lsx 41) (+ lsx 1)")
 (load-string "")]
;; expect: [42 nil]
