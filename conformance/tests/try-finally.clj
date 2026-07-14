;; finally always runs for side effect (its value discarded); the try
;; still yields the body value. The atom proves the finally executed.
;; Oracle (clojure 1.12.5): (let [a (atom 0) r (try 42 (finally (reset! a 99)))] [r @a]) => [42 99].
(let [a (atom 0)
      r (try 42 (finally (reset! a 99)))]
  [r @a])
;; expect: [42 99]
