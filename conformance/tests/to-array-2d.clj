;; to-array-2d — a collection of collections into an array of Object[]
;; rows (tail wave, 2026-07-23; a cljgo array is a Go slice, ADR 0025).
;; A non-collection row throws with the RT.toArray message, class-named
;; like the JVM's.
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o2.clj):
;;   (vec (map vec (to-array-2d [[1 2] [3]]))) => [[1 2] [3]]
;;   (alength (to-array-2d [[1] [2] [3]])) => 3
;;   (alength (to-array-2d [])) => 0
;;   (to-array-2d [1 2]) throws
;;     "Unable to convert: class java.lang.Long to Object[]"
[(vec (map vec (to-array-2d [[1 2] [3]])))
 (alength (to-array-2d [[1] [2] [3]]))
 (alength (to-array-2d []))
 (try (to-array-2d [1 2]) (catch Exception e (ex-message e)))]
;; expect: [[[1 2] [3]] 3 0 "Unable to convert: class java.lang.Long to Object[]"]
