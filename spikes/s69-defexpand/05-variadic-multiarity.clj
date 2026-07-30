;; s69 Q5 — VARIADIC and MULTI-ARITY defexpand.
(println)

;; --- v1 rejects variadic at DEFINITION time ----------------------------
(println "A. variadic parameters, current prototype rule (reject):")
(try
  (eval '(defexpand add-many! [a x & xs] (apply swap! a conj x xs)))
  (println "   accepted (unexpected)")
  (catch Throwable e (println "   rejected =>" (ex-message e))))

;; --- WHY: & xs is a runtime seq, but expansion sees FORMS --------------
;; A macro CAN take & xs (it receives the argument forms), so variadic
;; expansion is mechanically possible — what breaks is the once-only rule
;; combined with the `apply` idiom the fn version uses.
(defmacro add-many-hand! [a x & xs]
  (let [ga (gensym "a__") gx (gensym "x__")
        gs (mapv (fn [_] (gensym "r__")) xs)]
    (concat (list 'clojure.core/let (vec (concat [ga a gx x] (interleave gs xs))))
            (list (concat (list 'clojure.core/swap! ga 'clojure.core/conj gx) gs)))))
(def log (atom []))
(defn t [k v] (swap! log conj k) v)
(def acc (atom []))
(reset! log [])
(println)
(println "B. a HAND-written variadic expansion works and keeps once-only:")
(println "   expansion:" (macroexpand-1 '(add-many-hand! acc 1 2 3)))
(println "   result =>" (add-many-hand! acc (t :1 1) (t :2 2) (t :3 3)))
(println "   order  =>" @log)

;; --- the cost: no single expansion serves `apply` ----------------------
(println)
(println "C. the variadic tax: `(apply add-many! a xs)` cannot be expanded —")
(println "   the argument COUNT is unknown until runtime. Only the fn fallback")
(println "   can answer it, so variadic defexpand is inherently 'inline when")
(println "   the call is literal, call the fn otherwise'.")

;; --- multi-arity -------------------------------------------------------
;; A macro dispatches on the count of the FORMS it received — which for a
;; direct call site is exactly the arity. So multi-arity IS expressible.
(defmacro bump-hand!
  ([a]     (let [g (gensym "a__")] (list 'clojure.core/let [g a]
                                         (list 'clojure.core/swap! g 'clojure.core/inc))))
  ([a k]   (let [g (gensym "a__") gk (gensym "k__")]
             (list 'clojure.core/let [g a gk k]
                   (list 'clojure.core/swap! g 'clojure.core/update gk
                         (list 'clojure.core/fnil 'clojure.core/inc 0)))))
  ([a k n] (let [g (gensym "a__") gk (gensym "k__") gn (gensym "n__")]
             (list 'clojure.core/let [g a gk k gn n]
                   (list 'clojure.core/swap! g 'clojure.core/update gk
                         (list 'clojure.core/fnil 'clojure.core/+ 0) gn)))))
(println)
(println "D. multi-arity by expansion (dispatch on the call site's arg count):")
(def c1 (atom 0)) (def c2 (atom {}))
(println "   (bump-hand! c1)          =>" (bump-hand! c1))
(println "   (bump-hand! c2 :a)       =>" (bump-hand! c2 :a))
(println "   (bump-hand! c2 :a 5)     =>" (bump-hand! c2 :a 5))
(println "   equals swap! forms?      =>"
         (let [b1 (atom 0) b2 (atom {})]
           (swap! b1 inc) (swap! b2 update :a (fnil inc 0)) (swap! b2 update :a (fnil + 0) 5)
           [(= @c1 @b1) (= @c2 @b2)]))

;; --- arity ERROR quality ------------------------------------------------
(println)
(println "E. wrong arity at an expanded call site:")
(try (eval '(bump-hand! c1 :a :b :c))
     (catch Throwable e (println "   =>" (ex-message e))))
(println "   ; cljgo already NAMES the macro (user/bump-hand!) and gives the count.")
(println "   ; What it does not give is `(expects 1: [a] or 2: [a k] ...)` — the")
(println "   ; defexpand implementation must add Expected/Found per the doctrine.")
