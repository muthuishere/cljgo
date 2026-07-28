;; cljx.core/dbg (ADR 0106): dbg PRINTS its value and RETURNS it unchanged, so
;; it can be dropped anywhere in a threading pipeline without changing the
;; result — the whole point of the helper. This freezes both halves: the
;; pipeline's result is identical with and without dbg, and the printed text is
;; what the docstring promises (captured with with-out-str so the assertion is
;; deterministic).
;;
;; oracle: n/a — cljx.core is a cljgo namespace with no JVM package. The
;; behaviour frozen here is self-checking (dbg-vs-no-dbg equality), and
;; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljx.core :as x])
(let [plain  (->> [120 45 80] (filter #(> % 50)) (reduce +))
      out    (atom nil)
      dbgged (with-out-str
               (reset! out (->> [120 45 80]
                                (x/dbg "prices")
                                (filter #(> % 50))
                                (x/dbg "over 50")
                                (reduce +))))
      one-arg (with-out-str (x/dbg {:a 1}))]
  [(= plain @out)
   plain
   dbgged
   one-arg
   (= 42 (x/dbg 42))])
;; expect: [true 200 "prices: [120 45 80]\nover 50: (120 80)\n" "dbg: {:a 1}\n" true]
