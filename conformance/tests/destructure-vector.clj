;; Sequential destructuring (clojure.core/destructure via core.clj): positional
;; binds through nth, `& rest` collects the tail seq through nthnext/next, and
;; `:as` binds the whole collection. A pure macro expansion into let* over
;; simple symbols, so eval and AOT are byte-identical.
;; oracle: (let [[a b & r :as all] [1 2 3 4]] [a b r all]) => [1 2 (3 4) [1 2 3 4]]
;;   (JVM Clojure 1.12.5, clojure CLI)
(let [[a b & r :as all] [1 2 3 4]]
  [a b r all])
;; expect: [1 2 (3 4) [1 2 3 4]]
