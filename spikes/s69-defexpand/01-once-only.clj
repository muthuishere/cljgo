;; s69 Q1 — ONCE-ONLY EVALUATION + left-to-right ORDER.
;; (prototype.clj is prepended by run.sh)
(println)

;; --- the failure: a naive macro duplicates its argument -----------------
(defmacro twice-naive [x] (list '+ x x))
(println "A. naive macro, (twice-naive (do (println \"  side!\") 1)):")
(println "   =>" (twice-naive (do (println "  side!") 1)))
(println "   expansion:" (macroexpand-1 '(twice-naive (do (println "side!") 1))))

;; --- the fix: defexpand binds each argument once ------------------------
(println)
(defexpand twice [x] (+ x x))
(println "B. defexpand, (twice (do (println \"  side!\") 1)):")
(println "   =>" (twice (do (println "  side!") 1)))
(println "   expansion:" (macroexpand-1 '(twice (do (println "side!") 1))))

;; --- a function is the reference semantics ------------------------------
(println)
(defn twice-fn [x] (+ x x))
(println "C. plain defn, (twice-fn (do (println \"  side!\") 1)):")
(println "   =>" (twice-fn (do (println "  side!") 1)))

;; --- evaluation ORDER: left to right ------------------------------------
(println)
(def log (atom []))
(defn t [k v] (swap! log conj k) v)
(defexpand three [a b c] (- (+ a b) c))
(reset! log [])
(println "D. order, (three (t :a 10) (t :b 20) (t :c 5)):")
(println "   result  =>" (three (t :a 10) (t :b 20) (t :c 5)))
(println "   log     =>" @log)
(reset! log [])
(defn three-fn [a b c] (- (+ a b) c))
(println "   fn log  =>" (do (three-fn (t :a 10) (t :b 20) (t :c 5)) @log))

;; --- an UNUSED parameter still evaluates (function semantics) -----------
(println)
(defexpand ignore-second [a b] a)
(reset! log [])
(println "E. unused param, (ignore-second (t :a 1) (t :b 2)):")
(println "   result =>" (ignore-second (t :a 1) (t :b 2)))
(println "   log    =>" @log "  ; :b MUST appear — a function would evaluate it")

;; --- machine-checkable equivalence table --------------------------------
(println)
(def counter (atom 0))
(defn tick [] (swap! counter inc))
(reset! counter 0) (def r-macro (twice-naive (tick))) (def n-macro @counter)
(reset! counter 0) (def r-exp   (twice (tick)))       (def n-exp   @counter)
(reset! counter 0) (def r-fn    (twice-fn (tick)))    (def n-fn    @counter)
(println "F. evaluations of the argument:")
(println "   naive macro:" n-macro "result" r-macro)
(println "   defexpand  :" n-exp   "result" r-exp)
(println "   defn       :" n-fn    "result" r-fn)
(println "   defexpand matches defn?" (and (= n-exp n-fn) (= r-exp r-fn)))
