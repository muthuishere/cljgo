;; extenders — the extended-type listing. DEVIATION (documented): the
;; JVM returns Class objects ((extenders PP) => (user.TT
;; java.lang.String), printed via each class's toString); cljgo's
;; registry keys are dispatch-key strings with no reverse map to a class
;; value, so extenders returns the dispatch-key SYMBOLS ('user.TT,
;; 'String). Order is unspecified in both worlds (a map underneath) —
;; sorted here for a stable expectation.
;; oracle: skip — extenders returns dispatch-key symbols, not Class
;; objects (documented divergence; JVM 1.12.5 oracle cited above,
;; scratch tailwave/o1.clj E3/E8)
(defprotocol PP (pm [x]))
(defprotocol QZ (qm [x]))
(deftype TT [])
(extend TT PP {:pm (fn [x] :t)})
(extend String PP {:pm (fn [x] :s)})
[(vec (sort (map str (extenders PP))))
 (extenders QZ)]
;; expect: [["String" "user.TT"] nil]
