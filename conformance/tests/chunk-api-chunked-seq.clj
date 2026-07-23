;; The chunk API over real chunked seqs: ranges are chunked (chunk size
;; 32), plain lists are not; chunk-first/-next/-rest expose the chunk
;; boundary. Documented cljgo DEVIATION (not asserted here): a VECTOR's
;; seq is chunked on the JVM but not in cljgo yet.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23):
;; [(chunked-seq? (seq (range 100))) (chunked-seq? (seq '(1 2 3)))
;; (count (chunk-first (seq (range 100))))
;; (first (chunk-next (seq (range 100))))
;; (first (chunk-rest (seq (range 100))))] => [true false 32 32 32]
[(chunked-seq? (seq (range 100)))
 (chunked-seq? (seq '(1 2 3)))
 (count (chunk-first (seq (range 100))))
 (first (chunk-next (seq (range 100))))
 (first (chunk-rest (seq (range 100))))]
;; expect: [true false 32 32 32]
