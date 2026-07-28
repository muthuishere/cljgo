;; s69 Q4 — RECURSION: the terminating half (the divergent cases live in
;; 04a-recursion-hang.clj / 04b-recursion-mutual.clj, run under `timeout`).
(println)

;; --- a DEFINITION-TIME rejection is trivial and precise -----------------
(defn dx-self-referential? [nm body]
  (boolean (some #{nm} (dx-symbols body))))
(println "A. definition-time self-reference check:")
(println "   (fact ...) self-referential? =>"
         (dx-self-referential? 'fact '((if (<= n 1) 1 (* n (fact (- n 1)))))))
(println "   (add! ...) self-referential? =>"
         (dx-self-referential? 'add! '((swap! a conj x))))
(println "   ; the LOCAL check catches direct self-reference. Mutual recursion")
(println "   ; (od names evn, evn names od) is NOT caught locally: at od's")
(println "   ; definition time evn does not exist yet.")

;; --- what DOES catch mutual recursion: an expansion-depth budget --------
(def dx-depth (atom 0))
(defn dx-guard! [nm]
  (swap! dx-depth inc)
  (when (> @dx-depth 64)
    (reset! dx-depth 0)
    (throw (ex-info (str "defexpand: expansion of " nm " did not terminate "
                         "(depth > 64) — a defexpand may not expand itself")
                    {:name nm}))))
(println)
(println "B. an expansion-depth budget catches BOTH shapes:")
(try (dotimes [_ 100] (dx-guard! 'fact))
     (catch Throwable e (println "   caught =>" (ex-message e))))

;; --- the third option: route the self-call to the FN --------------------
(println)
(defn fact-fn [n] (if (<= n 1) 1 (* n (fact-fn (- n 1)))))
(defexpand call-fact [n] (fact-fn n))
(println "C. 'inline the call site, keep the recursion in the fn':")
(println "   (call-fact 5) =>" (call-fact 5))
(println "   expansion:" (macroexpand-1 '(call-fact 5)))
(println "   ; a defexpand whose body names ITSELF could expand exactly once and")
(println "   ; resolve the inner occurrence to the fn fallback. This terminates,")
(println "   ; but it REQUIRES the fn fallback to exist (see 06).")
