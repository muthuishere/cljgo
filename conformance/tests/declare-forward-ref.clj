;; declare (fundamentals batch 1): forward-declare vars so a fn can call
;; one defined later; a declared-but-never-defined name carries
;; :declared meta (a later def resets the var's meta, so it is checked
;; on `never-defined`); declare leaves NO root binding, so a later
;; defonce still initializes.
;; oracle (clojure 1.12.5): this exact file => [42 true 5]
;; (via (pr-str (load-file ...))).
(declare ff never-defined d2)
(defn hh [x] (ff x))
(defn ff [x] (* 2 x))
(defonce d2 5)
[(hh 21) (:declared (meta #'never-defined)) d2]
;; expect: [42 true 5]
