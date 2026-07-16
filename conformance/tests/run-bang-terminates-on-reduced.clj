;; run! must terminate as soon as proc returns a (reduced x) — its reducing
;; fn is `#(proc %2)`, which returns proc's result directly, NOT an
;; unconditional nil. An implementation that discards proc's return value
;; (e.g. `(fn [_ x] (proc x) nil)`) hides the Reduced from `reduce`, so it
;; never short-circuits and proc runs on the WHOLE collection instead of
;; stopping after the first (reduced ...).
;; Regression: clojure-test-suite core_test/run_bang.cljc (jank suite, ADR
;; 0022).
;; Oracle (clojure 1.12.5): 1
(let [calls (atom 0)]
  (run! (fn [_] (swap! calls inc) (reduced :done)) (range 5))
  @calls)
;; expect: 1
