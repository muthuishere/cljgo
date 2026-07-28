;; s69 t1 — does defexpand inline, stay hygienic, evaluate once, and keep the HOF?
(defexpand add! [a x] (swap! a conj x))

(def todo (atom []))
(add! todo "buy milk")
(add! todo "ship spike")
(println "1. value:" @todo)

;; expansion is visible: macroexpand does NOT show it (it is not a macro),
;; but the analyzer rewrote it. Prove semantics instead.

;; 2. HOF fallback: the var is still a real fn.
(println "2. fn?:" (fn? add!))
(def a2 (atom []))
(doseq [x [1 2 3]] (add! a2 x))
(println "2b. doseq:" @a2)
(def a3 (atom []))
(apply add! [a3 :via-apply])
(println "2c. apply:" @a3)
(def a4 (atom #{}))
(println "2d. map over the var:" (map (fn [x] (add! a4 x)) [10 20]))

;; 3. once-only, left-to-right evaluation of arguments
(def log (atom []))
(defn side [tag v] (swap! log conj tag) v)
(def a5 (atom []))
(add! (side :first a5) (side :second 99))
(println "3. eval order/count:" @log "->" @a5)

;; 4. hygiene: caller has locals named exactly like the defexpand params
(defexpand twice! [a x] (let [c x] (swap! a conj c c)))
(let [a (atom []) x :caller-x c :caller-c]
  (twice! a x)
  (println "4. hygiene:" @a "x=" x "c=" c))

;; 5. local shadowing the name suppresses expansion (and the fn value works)
(let [add! (fn [a x] :shadowed)]
  (println "5. shadowed:" (add! nil nil)))

;; 6. definline (already in core, previously inert) now inlines too
(definline dsqr [x] `(* ~x ~x))
(println "6. definline:" (dsqr 5) (map dsqr [1 2 3]) (fn? dsqr))
