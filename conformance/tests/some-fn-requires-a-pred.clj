;; some-fn has no 0-arg arity on the JVM ([p] is the minimum) — (some-fn)
;; must throw an arity exception, not silently build a function that
;; always returns false. A cljgo implementation using `[& preds]` (all
;; preds optional) accepts zero preds with no error, which is wrong.
;; Regression: clojure-test-suite core_test/some_fn.cljc (jank suite, ADR
;; 0022).
;; Oracle (clojure 1.12.5): :threw
(try
  (some-fn)
  (catch Exception e :threw))
;; expect: :threw
