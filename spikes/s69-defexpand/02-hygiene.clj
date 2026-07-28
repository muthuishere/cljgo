;; s69 Q2 — HYGIENE. (prototype.clj is prepended by run.sh)
(println)

;; === failure 1: the BODY's local captures the caller's expression ========
;; body binds `a`; the caller passes an expression that mentions its own `a`.
(defexpand-naive scale-naive [x] (let [a 100] (* a x)))
(println "A. naive, body local `a` shadows the caller's `a`:")
(println "   (let [a 2] (scale-naive (+ a 1)))")
(println "   expansion:" (macroexpand-1 '(scale-naive (+ a 1))))
(println "   =>" (let [a 2] (scale-naive (+ a 1))) "   ; WRONG: wanted 300 (100*(2+1)), got 100*101")

(defexpand scale [x] (let [a 100] (* a x)))
(println)
(println "B. defexpand renames the body local AND binds the arg first:")
(println "   expansion:" (macroexpand-1 '(scale (+ a 1))))
(println "   =>" (let [a 2] (scale (+ a 1))) "   ; correct")
(defn scale-fn [x] (let [a 100] (* a x)))
(println "   defn says:" (let [a 2] (scale-fn (+ a 1))))

;; === failure 2: the PARAMETER name collides with a caller local ==========
;; parameter is named `a`; the body also refers to a *global* `a`.
(def a :global-a)
(defexpand-naive who-naive [a] [a (str a)])
(println)
(println "C. naive, parameter name `a` colliding with a caller local `a`:")
(println "   (let [a :caller] (who-naive 1)) =>" (let [a :caller] (who-naive 1)))
(defexpand who [a] [a (str a)])
(println "   defexpand                        =>" (let [a :caller] (who 1)))

;; === failure 3: free reference in the body must NOT be captured =========
;; body refers to a var `limit`; the caller happens to have a local `limit`.
(def limit 10)
(defexpand-naive cap-naive [x] (min x limit))
(defexpand-unqualified cap-unq [x] (min x limit))
(defexpand cap [x] (min x limit))
(defn cap-fn [x] (min x limit))
(println)
(println "D. body free reference `limit` (a var = 10), caller has (let [limit 0] ...):")
(println "   naive                  =>" (let [limit 0] (cap-naive 5)) "  ; captured the caller's 0")
(println "   defexpand, no qualify  =>" (let [limit 0] (cap-unq 5)) "  ; renaming ALONE is not enough")
(println "   defexpand + qualify    =>" (let [limit 0] (cap 5)) "  ; correct")
(println "   defn                   =>" (let [limit 0] (cap-fn 5)) "  ; the reference semantics")
(println "   qualified expansion:" (macroexpand-1 '(cap 5)))

;; === how far renaming reaches ===========================================
(defexpand nested [x]
  (let [acc 0]
    (loop [i 0 acc acc]
      (if (< i 3) (recur (inc i) (+ acc x)) acc))))
(println)
(println "E. renaming reaches let + loop bindings inside the body:")
(println "   expansion:" (macroexpand-1 '(nested q)))
(println "   (let [acc 99 i 99 x 7] (nested x)) =>" (let [acc 99 i 99 x 7] (nested x)))

(defexpand with-fn [x] ((fn [y] (+ y x)) 10))
(println "   fn params renamed too:" (macroexpand-1 '(with-fn y)))
(println "   (let [y 5] (with-fn y)) =>" (let [y 5] (with-fn y)) "  ; want 15")

;; === is R2 (body-local renaming) load-bearing once R1+R3 hold? ==========
(defexpand-no-rename scale-nr [x] (let [a 100] (* a x)))
(defexpand-no-rename nested-nr [x] (let [acc 0] (loop [i 0 acc acc]
                                                 (if (< i 3) (recur (inc i) (+ acc x)) acc))))
(println)
(println "F. R1+R3 only (no body-local renaming):")
(println "   expansion:" (macroexpand-1 '(scale-nr (+ a 1))))
(println "   (let [a 2] (scale-nr (+ a 1)))     =>" (let [a 2] (scale-nr (+ a 1))) " (want 300)")
(println "   (let [acc 99 i 99 x 7] (nested-nr x)) =>" (let [acc 99 i 99 x 7] (nested-nr x)) " (want 21)")

;; === the case R3 alone cannot cover: a FORWARD reference ================
(defexpand call-later [x] (helper x))
(defn helper [n] (* n 1000))
(println)
(println "G. body calls `helper`, defined AFTER the defexpand:")
(println "   expansion:" (macroexpand-1 '(call-later 2)))
(println "   (call-later 2)                =>" (call-later 2))
(println "   (let [helper (fn [_] :hijacked)] (call-later 2)) =>"
         (let [helper (fn [_] :hijacked)] (call-later 2)))
(println "   ; expansion-time resolution in the DEFINING ns saves the forward ref;")
(println "   ; had we qualified at DEFINITION time, `helper` would still be bare.")

;; === once R1 is ELIDED (see 08), R2 becomes load-bearing again ==========
(defexpand-no-rename-elide scale-nre [x] (let [a 100] (* a x)))
(defexpand                 scale-ok  [x] (let [a 100] (* a x)))
(println)
(println "H. R1 elided + no body-local renaming = capture is BACK:")
(println "   expansion:" (macroexpand-1 '(scale-nre a)))
(println "   (let [a 2] (scale-nre a)) =>" (let [a 2] (scale-nre a)) " (want 200)")
(println "   (let [a 2] (scale-ok  a)) =>" (let [a 2] (scale-ok a))  " (want 200)")
(println "   ; conclusion: R2 is not optional in the shipping design.")
