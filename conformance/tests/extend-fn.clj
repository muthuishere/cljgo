;; extend — the FUNCTIONAL core of the protocol system (extend-type /
;; extend-protocol are macros over it on the JVM), plus extends? /
;; find-protocol-impl / find-protocol-method, all over the SAME registry
;; the macros feed (tail wave, 2026-07-23; pkg/corelib/protocols.go).
;; The type argument is a class VALUE: a deftype/defrecord marker or a
;; well-known class ref (String, Long, ...).
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o1.clj):
;;   (defprotocol PP (pm [x]) (pn [x])) (deftype TT [])
;;   (extend TT PP {:pm (fn [x] :pm-tt) :pn (fn [x] :pn-tt)})
;;   (pm (TT.)) => :pm-tt
;;   (extends? PP TT) => true; (extends? PP Long) => false
;;   (satisfies? PP (TT.)) => true
;;   ((find-protocol-method PP :pm (TT.)) (TT.)) => :pm-tt
;;   (sort (keys (find-protocol-impl PP (TT.)))) => (:pm :pn)
;;   (extend String PP {:pm (fn [x] (str "pm-str-" x))}) (pm "q") => "pm-str-q"
;;   (find-protocol-method PP :pm 5) => nil
(defprotocol PP (pm [x]) (pn [x]))
(deftype TT [])
(extend TT PP {:pm (fn [x] :pm-tt) :pn (fn [x] :pn-tt)})
(extend String PP {:pm (fn [x] (str "pm-str-" x))})
[(pm (TT.))
 (extends? PP TT)
 (extends? PP Long)
 (satisfies? PP (TT.))
 ((find-protocol-method PP :pm (TT.)) (TT.))
 (vec (sort (keys (find-protocol-impl PP (TT.)))))
 (pm "q")
 (find-protocol-method PP :pm 5)]
;; expect: [:pm-tt true false true :pm-tt [:pm :pn] "pm-str-q" nil]
