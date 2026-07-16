;; S16 with-precision probes — separate file because `with-precision` does
;; not resolve in cljgo (compile error aborts every form after it).
;; Oracle: clojure -M probes_wp.clj. cljgo's failure to load this file at
;; all IS the recorded baseline divergence.
;;
;; Rows: every assertion in the suite's with_precision.cljc + default
;; rounding + MathContext-on-add/div sweeps.

(defn safe [f]
  (try (pr-str (f)) (catch Exception e (str "THREW:" (ex-message e)))))

(defn p [label f] (println (str label " => " (safe f))))

;; suite with_precision.cljc rows (clojuredocs examples)
(p "wp1 UP 1.1*1"          (fn [] (with-precision 1 :rounding UP (* 1.1M 1M))))
(p "wp1 CEILING 1.1*1"     (fn [] (with-precision 1 :rounding CEILING (* 1.1M 1M))))
(p "wp1 UP -1.1*1"         (fn [] (with-precision 1 :rounding UP (* -1.1M 1M))))
(p "wp1 CEILING -1.1*1"    (fn [] (with-precision 1 :rounding CEILING (* -1.1M 1M))))
(p "wp1 DOWN 1.9*1"        (fn [] (with-precision 1 :rounding DOWN (* 1.9M 1M))))
(p "wp1 FLOOR 1.9*1"       (fn [] (with-precision 1 :rounding FLOOR (* 1.9M 1M))))
(p "wp1 DOWN -1.9*1"       (fn [] (with-precision 1 :rounding DOWN (* -1.9M 1M))))
(p "wp1 FLOOR -1.9*1"      (fn [] (with-precision 1 :rounding FLOOR (* -1.9M 1M))))
(p "wp1 HALF_EVEN 1.5*1"   (fn [] (with-precision 1 :rounding HALF_EVEN (* 1.5M 1M))))
(p "wp1 HALF_EVEN 2.5*1"   (fn [] (with-precision 1 :rounding HALF_EVEN (* 2.5M 1M))))
(p "wp1 HALF_EVEN -1.5*1"  (fn [] (with-precision 1 :rounding HALF_EVEN (* -1.5M 1M))))
(p "wp1 HALF_EVEN -2.5*1"  (fn [] (with-precision 1 :rounding HALF_EVEN (* -2.5M 1M))))
(p "wp1 HALF_UP 1.5*1"     (fn [] (with-precision 1 :rounding HALF_UP (* 1.5M 1M))))
(p "wp1 HALF_DOWN 1.5*1"   (fn [] (with-precision 1 :rounding HALF_DOWN (* 1.5M 1M))))
(p "wp1 HALF_UP -1.5*1"    (fn [] (with-precision 1 :rounding HALF_UP (* -1.5M 1M))))
(p "wp1 HALF_DOWN -1.5*1"  (fn [] (with-precision 1 :rounding HALF_DOWN (* -1.5M 1M))))
(p "wp1 UNNECESSARY 1.5*1" (fn [] (with-precision 1 :rounding UNNECESSARY (* 1.5M 1M))))
(p "wp1 UNNECESSARY 2*1"   (fn [] (with-precision 1 :rounding UNNECESSARY (* 2M 1M))))

;; default rounding mode (HALF_UP) + division under a MathContext
(p "wp2 div 1/3"           (fn [] (with-precision 2 (/ 1M 3M))))
(p "wp5 div 1/3"           (fn [] (with-precision 5 (/ 1M 3M))))
(p "wp2 div 2/3"           (fn [] (with-precision 2 (/ 2M 3M))))
(p "wp3 add"               (fn [] (with-precision 3 (+ 1.2345M 0M))))
(p "wp3 sub"               (fn [] (with-precision 3 (- 1.2345M 0M))))
(p "wp3 mul"               (fn [] (with-precision 3 (* 1.2345M 1M))))
(p "wp4 HALF_DOWN div"     (fn [] (with-precision 4 :rounding HALF_DOWN (/ 1M 3M))))
(p "wp2 big"               (fn [] (with-precision 2 (+ 123M 0M))))
