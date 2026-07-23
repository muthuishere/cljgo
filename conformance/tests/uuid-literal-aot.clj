;; #uuid literals as COMPILED constants (tail wave, 2026-07-23): the
;; emitter reconstructs a #uuid constant via reader.MustUUID (pkg/emit
;; constExpr — the MustInst pattern from the #inst fix), closing the
;; known gap where a bare #uuid in source failed AOT with "unsupported
;; constant type". This file exists to pin the COMPILED leg: literals at
;; top level, nested in collections, compared and printed.
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o3.clj):
;;   #uuid "550e8400-e29b-41d4-a716-446655440000" prints as itself
;;   (= #uuid "550e8400-..." #uuid "550E8400-...") => true (case-folded)
;; (Not frozen: (str u) — the JVM yields the bare "550e8400-..." while
;; cljgo's UUID stringer yields the tagged literal form; a pre-existing
;; divergence outside this change's scope.)
(def u #uuid "550e8400-e29b-41d4-a716-446655440000")
(def coll {:id #uuid "f81d4fae-7dec-11d0-a765-00a0c91e6bf6" :xs [#uuid "550e8400-e29b-41d4-a716-446655440000"]})
[u
 (= u #uuid "550E8400-E29B-41D4-A716-446655440000")
 (uuid? u)
 (:id coll)
 (= u (first (:xs coll)))]
;; expect: [#uuid "550e8400-e29b-41d4-a716-446655440000" true true #uuid "f81d4fae-7dec-11d0-a765-00a0c91e6bf6" true]
