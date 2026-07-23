;; chunk-buffer/chunk-append/chunk build an IChunk; chunk-cons of a
;; non-empty chunk yields a chunked seq onto the rest, of an EMPTY chunk
;; yields the rest itself — exactly clojure.core/chunk-cons.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): building a 2-item chunk, then
;; [(chunk-cons c '(9 9)) (chunked-seq? (chunk-cons c '(9 9)))
;; (chunk-cons (chunk (chunk-buffer 4)) '(7 8))]
;; => [(1 2 9 9) true (7 8)]
(let [b (chunk-buffer 4)]
  (chunk-append b 1)
  (chunk-append b 2)
  (let [c (chunk b)]
    [(chunk-cons c '(9 9))
     (chunked-seq? (chunk-cons c '(9 9)))
     (chunk-cons (chunk (chunk-buffer 4)) '(7 8))]))
;; expect: [(1 2 9 9) true (7 8)]
