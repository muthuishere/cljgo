;; Associative destructuring: {:keys [..]} binds locals from keyword lookups,
;; :or supplies defaults for absent keys, :as binds the whole map. Here :b is
;; absent so it takes its :or default of 9.
;; oracle: (let [{:keys [a b] :or {b 9} :as m} {:a 1}] [a b m]) => [1 9 {:a 1}]
;;   (JVM Clojure 1.12.5, clojure CLI)
(let [{:keys [a b] :or {b 9} :as m} {:a 1}]
  [a b m])
;; expect: [1 9 {:a 1}]
