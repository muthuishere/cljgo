;; s69 Q7 — port cljx.core helpers onto the defexpand prototype and prove they
;; behave IDENTICALLY to the swap!/reset! forms they document. Same equivalence
;; style as conformance/tests/cljx-core-atom-verbs.clj: run the sugar on one
;; atom, the raw form on another, compare.
(println)

;; --- the ported helpers (single-arity shapes) ---------------------------
(defexpand add!    "(swap! a conj x)"        [a x] (swap! a conj x))
(defexpand add-kv! "(swap! a assoc k v)"     [a k v] (swap! a assoc k v))
(defexpand bump!   "(swap! a inc)"           [a] (swap! a inc))
(defexpand bump-k! "(swap! a update k (fnil inc 0))" [a k] (swap! a update k (fnil inc 0)))
(defexpand toggle! "(swap! a not)"           [a] (swap! a not))
(defexpand del!    "dissoc for a map, disj for a set"
  [a k] (swap! a (fn [c] (if (set? c) (disj c k) (dissoc c k)))))
(defexpand clear!  "(reset! a (empty @a))"   [a] (reset! a (empty @a)))

(println "A. expansions (what the sugar compiles to):")
(println "   (add! todo \"x\")  =>" (macroexpand-1 '(add! todo "x")))
(println "   (bump! n)        =>" (macroexpand-1 '(bump! n)))
(println "   (toggle! f)      =>" (macroexpand-1 '(toggle! f)))
(println "   (del! m :k)      =>" (macroexpand-1 '(del! m :k)))

;; --- the equivalence table ---------------------------------------------
(def result
  (let [a1 (atom []) b1 (atom [])
        _ (do (add! a1 "one") (add! a1 "two")
              (swap! b1 conj "one") (swap! b1 conj "two"))
        a2 (atom {}) b2 (atom {})
        _ (do (add-kv! a2 :k "v") (swap! b2 assoc :k "v"))
        a3 (atom 0) b3 (atom 0)
        _ (do (bump! a3) (bump! a3) (swap! b3 inc) (swap! b3 inc))
        a4 (atom {}) b4 (atom {})
        _ (do (bump-k! a4 "harsh") (bump-k! a4 "harsh") (bump-k! a4 "shaama")
              (swap! b4 update "harsh" (fnil inc 0))
              (swap! b4 update "harsh" (fnil inc 0))
              (swap! b4 update "shaama" (fnil inc 0)))
        a8 (atom {:x 1 :y 2}) b8 (atom {:x 1 :y 2})
        _ (do (del! a8 :x) (swap! b8 dissoc :x))
        a9 (atom #{:p :q}) b9 (atom #{:p :q})
        _ (do (del! a9 :p) (swap! b9 disj :p))
        a10 (atom false) b10 (atom false)
        _ (do (toggle! a10) (swap! b10 not))
        a11 (atom [1 2 3]) _ (clear! a11)
        a12 (atom {:k 1})  _ (clear! a12)]
    [(= @a1 @b1) (= @a2 @b2) (= @a3 @b3) (= @a4 @b4)
     (= @a8 @b8) (= @a9 @b9) (= @a10 @b10)
     @a4
     (and (empty? @a11) (vector? @a11))
     (and (empty? @a12) (map? @a12))]))
(println)
(println "B. equivalence to the raw swap!/reset! forms:")
(println "  " result)
(println "   all true? =>" (every? true? (remove map? result)))

;; --- the hygiene/once-only properties survive the port ------------------
(def hits (atom []))
(defn pick [k a] (swap! hits conj k) a)
(def box (atom []))
(reset! hits [])
(add! (pick :atom box) (do (swap! hits conj :val) 1))
(println)
(println "C. once-only + order inside the ported sugar:")
(println "   evaluation log =>" @hits "  ; each argument exactly once, left to right")
(println "   atom           =>" @box)

;; a caller local named `a` (the parameter name) must not matter
(println "   under (let [a :caller] ...) =>"
         (let [a :caller c (atom [])] (add! c a) @c))

;; --- the interpreted cost, measured -------------------------------------
(defn add-fn! [a x] (swap! a conj x))
(println)
(println "D. interpreted cost, 300k iterations (indicative only; the AOT numbers")
(println "   are the other track's job). `time` is cljgo's only clock here.")
(def n 300000)
(print "   defn add-fn!   ") (flush)
(time (let [a (atom [])] (dotimes [_ n] (add-fn! a 1))))
(print "   defexpand add! ") (flush)
(time (let [a (atom [])] (dotimes [_ n] (add! a 1))))
(print "   raw swap!      ") (flush)
(time (let [a (atom [])] (dotimes [_ n] (swap! a conj 1))))
