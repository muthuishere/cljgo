;; s69 Q6 — THE FN FALLBACK. ADR 0107 rule 4 promises a real fn under the same
;; name so (map add! …) / (apply add! …) keep working. This file measures what
;; a MACRO-based mechanism can and cannot do about that.
(println)

(defexpand dbl [x] (* 2 x))
(println "A. defexpand'd name used as a VALUE:")
(println "   (macroexpand-1 '(dbl 4)) =>" (macroexpand-1 '(dbl 4)))
(println "   (dbl 4)                  =>" (dbl 4) "   ; direct call: fine")
(try (println "   (map dbl [1 2 3])        =>" (eval '(map dbl [1 2 3])))
     (catch Throwable e (println "   (map dbl [1 2 3])        => THREW:" (ex-message e))))
(try (println "   (apply dbl [4])          =>" (eval '(apply dbl [4])))
     (catch Throwable e (println "   (apply dbl [4])          => THREW:" (ex-message e))))
(try (println "   dbl                      =>" (eval 'dbl))
     (catch Throwable e (println "   dbl (as a value)         => THREW:" (ex-message e))))
(println "   (fn? (deref (var dbl)))  =>" (try (fn? @#'dbl) (catch Throwable e (ex-message e))))

;; --- why it cannot be fixed inside a macro ------------------------------
;; One namespace, one var per name. `defmacro` binds the var to a macro fn and
;; flags it :macro; a `defn` of the same name overwrites that. They cannot
;; coexist under one name — proof by construction:
(println)
(println "B. can a name be BOTH a macro and a fn value?")
(defmacro mname [x] (list 'inc x))
(println "   after (defmacro mname ...): (mname 1)   =>" (mname 1))
(defn mname [x] (dec x))
(println "   after (defn mname ...):     (mname 1)   =>" (mname 1) "  ; the macro is GONE")
(println "   (map mname [1 2 3])                     =>" (map mname [1 2 3]))
(println "   ; whichever def runs last wins. There is no 'both'.")

;; --- what the :inline-metadata approach gives ---------------------------
;; The var IS a real fn; only the ANALYZER, at a direct call site, chooses to
;; splice the body instead of emitting a call. cljgo's `definline` already
;; stores that metadata (core/core.clj:2462) — it just never acts on it.
(println)
(definline dsqr [x] `(* ~x ~x))
(println "C. `definline` today (metadata stored, no call-site inlining):")
(println "   (dsqr 5)          =>" (dsqr 5))
(println "   (fn? dsqr)        =>" (fn? dsqr) "   ; it IS a value")
(println "   (map dsqr [1 2 3])=>" (map dsqr [1 2 3]))
(println "   (:inline (meta (var dsqr))) present? =>" (some? (:inline (meta #'dsqr))))
(println "   ; the var carries both the fn and the inline template. THIS is the")
(println "   ; only shape that satisfies ADR 0107 rule 4.")

;; --- the guarded prototype's workaround, and its cost -------------------
(defexpand-guarded gdbl [x] (* 2 x))
(println)
(println "D. the guarded prototype keeps a fn — under a DIFFERENT name:")
(println "   (gdbl 4)              =>" (gdbl 4))
(println "   (map gdbl-fn [1 2 3]) =>" (map gdbl-fn [1 2 3]))
(try (println "   (map gdbl [1 2 3])    =>" (eval '(map gdbl [1 2 3])))
     (catch Throwable e (println "   (map gdbl [1 2 3])    => THREW:" (ex-message e))))
(println "   ; two names is not what ADR 0107 promises. The name split is an")
(println "   ; artefact of the macro mechanism, not of the semantics.")
