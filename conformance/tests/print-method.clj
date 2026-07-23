;; print-method / print-dup / print-simple — the printer's multimethod
;; extension points (fundamentals batch A2). A custom method fires for the
;; type at EVERY depth and for pr AND println alike; keyword :type metadata
;; wins the dispatch; print-dup methods apply only under *print-dup*.
;; DEVIATION (documented): under *print-dup* built-in types keep their
;; ordinary readable printing (the JVM emits #=(...) constructor forms for
;; e.g. maps; cljgo's reader has no #= at all) — only the vector case,
;; where the JVM agrees, is frozen. Overriding a BUILT-IN class's printing
;; (e.g. (defmethod print-method clojure.lang.Keyword ...) => "KW:zap",
;; oracle-verified) also works in cljgo but is not frozen here: on the JVM
;; remove-method cannot restore the core method afterwards, which would
;; poison the rest of the oracle run.
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (deftype Pt [x y])
;;   (defmethod print-method Pt [p w] (.write w (str "#pt[" (.-x p) " " (.-y p) "]")))
;;   (pr-str (Pt. 1 2)) => "#pt[1 2]"
;;   (pr-str [(Pt. 1 2) :k]) => "[#pt[1 2] :k]"
;;   (print-str (Pt. 1 2)) => "#pt[1 2]"
;;   (pr-str {:a (Pt. 3 4)}) => "{:a #pt[3 4]}"
;;   (defmethod print-method ::special [x w] (.write w "#special"))
;;   (pr-str (with-meta [1 2] {:type ::special})) => "#special"
;;   (defmethod print-dup Pt [p w] (.write w (str "#dup-pt[" (.-x p) "]")))
;;   (binding [*print-dup* true] (pr-str (Pt. 1 2))) => "#dup-pt[1]"
;;   (binding [*print-dup* true] (pr-str [1 2])) => "[1 2]"
;;   (defrecord R [a]) (pr-str (R. 1)) => "#user.R{:a 1}", then after
;;   (defmethod print-method R [r w] (.write w "#custom-r")) => "#custom-r"
;;   (with-out-str (print-simple [1 2] *out*)) => "[1 2]"
;;   (with-out-str (print-simple "str" *out*)) => "str"
(deftype Pt [x y])
(defmethod print-method Pt [p w] (.write w (str "#pt[" (.-x p) " " (.-y p) "]")))
(defmethod print-method ::special [x w] (.write w "#special"))
(defmethod print-dup Pt [p w] (.write w (str "#dup-pt[" (.-x p) "]")))
(defrecord R [a])
(def r-plain (pr-str (R. 1)))
(defmethod print-method R [r w] (.write w "#custom-r"))
[(pr-str (Pt. 1 2))
 (pr-str [(Pt. 1 2) :k])
 (print-str (Pt. 1 2))
 (pr-str {:a (Pt. 3 4)})
 (pr-str (with-meta [1 2] {:type ::special}))
 (binding [*print-dup* true] (pr-str (Pt. 1 2)))
 (binding [*print-dup* true] (pr-str [1 2]))
 *print-dup*
 r-plain
 (pr-str (R. 1))
 (with-out-str (print-simple [1 2] *out*))
 (with-out-str (print-simple "str" *out*))]
;; expect: ["#pt[1 2]" "[#pt[1 2] :k]" "#pt[1 2]" "{:a #pt[3 4]}" "#special" "#dup-pt[1]" "[1 2]" false "#user.R{:a 1}" "#custom-r" "[1 2]" "str"]
