;; s69 Q3 — WITH-REDEFS, the one real semantic risk (ADR 0107).
(println)

;; --- the reference: a plain defn IS redefinable -------------------------
(defn greet-fn [n] (str "hello " n))
(println "A. defn under with-redefs:")
(println "   normal  =>" (greet-fn "ann"))
(println "   redefed =>" (with-redefs [greet-fn (fn [n] (str "STUB " n))] (greet-fn "ann")))
(println "   after   =>" (greet-fn "ann"))

;; --- the divergence: an inlined defexpand call site is FROZEN ----------
(defexpand greet [n] (str "hello " n))
(println)
(println "B. defexpand (unconditional inline) under with-redefs:")
(println "   normal  =>" (greet "ann"))
(println "   expansion:" (macroexpand-1 '(greet "ann")))
;; a macro var cannot be with-redefs'd to a fn value in a way the call site sees:
(println "   with-redefs on the name is not even expressible: the call site was")
(println "   already expanded at ANALYSIS time, before with-redefs ever ran.")

;; the closest thing a user would actually write in a test: stub the fn the
;; body calls. That still works, because the BODY is what got inlined.
(defn fetch [k] (str "real:" k))
(defexpand fetch-twice [k] [(fetch k) (fetch k)])
(println)
(println "C. stubbing a fn the defexpand BODY calls (this DOES work):")
(println "   normal  =>" (fetch-twice "a"))
(println "   redefed =>" (with-redefs [fetch (fn [k] (str "STUB:" k))] (fetch-twice "a")))

;; --- option (b): the ADR-0066-shaped guard ------------------------------
(defexpand-guarded gmul [a b] (* a b))
(println)
(println "D. defexpand-guarded (ADR 0066 shape: inline, but keep a liveness guard):")
(println "   expansion:" (macroexpand-1 '(gmul 3 4)))
(println "   normal  =>" (gmul 3 4))
(println "   redefed =>" (with-redefs [gmul-fn (fn [a b] (+ a b))] (gmul 3 4)))
(println "   after   =>" (gmul 3 4))
(println "   ; the guard costs ONE identical? compare per call site and the")
(println "   ; emitted fallback call — vs zero for the unconditional inline.")

;; --- alter-var-root is the other root writer ---------------------------
(println)
(println "E. alter-var-root (permanent redefinition):")
(alter-var-root #'gmul-fn (fn [_] (fn [a b] (- a b))))
(println "   guarded gmul after alter-var-root =>" (gmul 3 4) " ; sees it (want -1)")
(alter-var-root #'fetch (fn [_] (fn [k] (str "ALTERED:" k))))
(println "   unguarded fetch-twice             =>" (fetch-twice "a")
         " ; body call goes through the var, so it sees it too")

;; --- what a cljx.test stub/spy (ADR 0105) needs ------------------------
;; ADR 0105: (stub #'ns/f replacement) / (spy #'ns/f) are implemented with
;; with-redefs. Model them here.
(println)
(defn stub-call [v replacement thunk]
  (with-redefs-fn {v replacement} thunk))
(defn svc [x] (* x 2))
(defexpand svc-x [x] (* x 2))                    ; user "optimised" svc into a defexpand
(defexpand-guarded gsvc [x] (* x 2))
(println "F. cljx.test-style stub via with-redefs-fn:")
(println "   stub a defn        =>" (stub-call #'svc (fn [_] :stubbed) (fn [] (svc 5))))
(println "   stub a defexpand   => (no var to stub — the name is a macro)")
(println "   stub a guarded one =>" (stub-call #'gsvc-fn (fn [_] :stubbed) (fn [] (gsvc 5))))

;; --- a SPY (pass-through recorder) on the guarded form ------------------
(def calls (atom []))
(println)
(println "G. cljx.test-style spy on the guarded form:")
(def real-gsvc gsvc-fn)
(println "   result =>" (stub-call #'gsvc-fn
                                   (fn [x] (swap! calls conj x) (real-gsvc x))
                                   (fn [] [(gsvc 5) (gsvc 7)])))
(println "   calls  =>" @calls "  ; the spy SAW both inlined call sites")
