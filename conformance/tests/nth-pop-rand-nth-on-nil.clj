;; nth / nthnext / rand-nth / pop on a nil collection (ADR-less batch,
;; batch/error-files): a nil coll is nil at ANY index — RT.nthFrom checks
;; coll==null before ever inspecting n — so nth/nthnext/rand-nth/pop never
;; throw for a nil coll, no matter the index. (nth nil idx) still requires
;; idx itself to be an integer (n is a primitive int in Clojure's nth
;; signature, so the unboxing NPE happens before the null check); nthnext's
;; n is only inspected once the seq is non-nil, so (nthnext nil nil) is
;; fine.
;; oracle (clojure 1.12.5): [(nth nil 10) (nth nil -5) (nth nil 10 :nf)
;; (nthnext nil nil) (rand-nth nil) (pop nil)] => [nil nil :nf nil nil nil];
;; (nth nil nil) throws.
[[(nth nil 10) (nth nil -5) (nth nil 10 :nf) (nthnext nil nil) (rand-nth nil) (pop nil)]
 (try (nth nil nil) :nothrow (catch Exception _e :threw))]
;; expect: [[nil nil :nf nil nil nil] :threw]
