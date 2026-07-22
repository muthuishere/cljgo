;; CASE B — genuinely-nil Clojure values. Must NOT error; nil prints as nil.
(println "literal nil:" nil)
(println "missing key:" (get {:a 1} :nope))
(defn returns-nil [] nil)
(println "fn returns nil:" (returns-nil))
(println "when-false:" (when false :x))
