;; (vec array) on a small array ALIASES it — the array becomes the vector's
;; storage, so in-place mutation is visible through the vector (JVM
;; LazilyPersistentVector.createOwning; suite vec.cljc). Also: vec accepts
;; any Seqable (sorted-set previously threw "Unable to convert").
;; oracle (clojure 1.12.5): [[-1 2 3] [1 2 3]]
(let [a (to-array [1 2 3])
      v (vec a)]
  (aset a 0 -1)
  [v (vec (sorted-set 3 1 2))])
;; expect: [[-1 2 3] [1 2 3]]
