;; uuid? must recognize the reader.UUID value produced by #uuid literals,
;; random-uuid, and parse-uuid. It previously always returned false — a
;; leftover from before cljgo had a UUID value type at all.
;; oracle (clojure 1.12.5):
;; [(uuid? (random-uuid)) (uuid? #uuid "f81d4fae-7dec-11d0-a765-00a0c91e6bf6")
;;  (uuid? :not-a-uuid)]
;; => [true true false]
[(uuid? (random-uuid)) (uuid? #uuid "f81d4fae-7dec-11d0-a765-00a0c91e6bf6")
 (uuid? :not-a-uuid)]
;; expect: [true true false]
