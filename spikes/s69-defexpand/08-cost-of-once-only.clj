;; s69 Q7b — the COST of the once-only rule, interpreted. 07 showed defexpand
;; SLOWER than defn; this file repeats it to separate signal from noise, and
;; isolates the cause (the extra `let` frame the once-only rule introduces).
(println)

(defn add-fn! [a x] (swap! a conj x))
(defexpand           add!    [a x] (swap! a conj x))  ; R1 + R1' (elision on)
(defexpand-no-elide  add-ne! [a x] (swap! a conj x))  ; R1 always binds
(defexpand-naive     add-nv! [a x] (swap! a conj x))  ; no binding at all
(def n 300000)

(defn run-round [i]
  (println (str "  --- round " i " ---"))
  (print "  raw swap!             ") (flush)
  (time (let [a (atom [])] (dotimes [_ n] (swap! a conj 1))))
  (print "  defn add-fn!          ") (flush)
  (time (let [a (atom [])] (dotimes [_ n] (add-fn! a 1))))
  (print "  defexpand, no elision ") (flush)
  (time (let [a (atom [])] (dotimes [_ n] (add-ne! a 1))))
  (print "  defexpand, elision    ") (flush)
  (time (let [a (atom [])] (dotimes [_ n] (add! a 1))))
  (print "  naive (unsafe)        ") (flush)
  (time (let [a (atom [])] (dotimes [_ n] (add-nv! a 1)))))

(println "A. interpreted, n =" n)
(dotimes [i 3] (run-round (inc i)))

(println)
(println "B. the expansions being compared:")
(println "   no elision:" (macroexpand-1 '(add-ne! a 1)))
(println "   elision   :" (macroexpand-1 '(add! a 1)))
(println "   naive     :" (macroexpand-1 '(add-nv! a 1)))
(println "   elision keeps R1 when it MATTERS:")
(println "             " (macroexpand-1 '(add! a (do (side!) 1))))

(println)
(println "C. elision must not break once-only. Re-checking:")
(def log (atom []))
(defn t [k v] (swap! log conj k) v)
(def box (atom []))
(reset! log [])
(add! box (t :once 1))
(println "   (add! box (t :once 1)) log =>" @log)
(reset! log [])
(defexpand twice [x] (+ x x))
(println "   (twice (t :v 3))       =>" (twice (t :v 3)) " log =>" @log)
