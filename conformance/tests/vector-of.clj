;; vector-of — the typed persistent vector's observable contract (tail
;; wave, 2026-07-23): ctor elements coerce through the checked scalar
;; casts, equality/lookup/subvec match the JVM.
;; DEVIATIONS (documented, not frozen): the JVM builds a
;; clojure.core.Vec over a primitive array and re-coerces on EVERY conj
;; ((conj (vector-of :int) 2.5) => [2] there, plain-conj 2.5 here) and
;; on nil throws NullPointerException; cljgo coerces at the ctor only,
;; over an ordinary persistent vector (class diverges). The :byte
;; range error text also differs in case ("value out of range for
;; byte: 200" here vs "Value ..." there), so only its presence is
;; relied on elsewhere, not frozen here.
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o2.clj+o3.clj):
;;   (vector-of :int 1 2 3) => [1 2 3]
;;   (= (vector-of :int 1 2) [1 2]) => true
;;   (conj (vector-of :int 1) 9) => [1 9]
;;   (vector-of :int 1.5) => [1] (RT.intCast truncates)
;;   (vector-of :double 1 2) => [1.0 2.0]
;;   (vector-of :float 1.5) => [1.5]
;;   (vector-of :boolean true) => [true]
;;   (vector-of :char \a) => [\a]
;;   [(get (vector-of :int 4 5) 1) (nth (vector-of :int 4 5) 0)] => [5 4]
;;   (subvec (vector-of :int 1 2 3) 1) => [2 3]
[(vector-of :int 1 2 3)
 (= (vector-of :int 1 2) [1 2])
 (conj (vector-of :int 1) 9)
 (vector-of :int 1.5)
 (vector-of :double 1 2)
 (vector-of :float 1.5)
 (vector-of :boolean true)
 (vector-of :char \a)
 [(get (vector-of :int 4 5) 1) (nth (vector-of :int 4 5) 0)]
 (subvec (vector-of :int 1 2 3) 1)]
;; expect: [[1 2 3] true [1 9] [1] [1.0 2.0] [1.5] [true] [\a] [5 4] [2 3]]
