;; A DESCENDING range must be the same seq down every access path.
;;
;; LongRange.Next terminated on `start+step >= end`, which is only correct for
;; a positive step: for (range 6 1 -1) the second element 5 is already >= end
;; 1, so first/rest saw a ONE-element seq while Count() said 5. Nothing threw.
;; The chunked path (map, vec, reduce) and count walked all five and looked
;; right, so only a first/rest consumer saw the truncation — which is why a
;; backwards scan like (some f (range (dec n) 0 -1)) returned nil instead of a
;; hit, silently.
;;
;; Reported by the toolnexus Clojure port (2026-08-02): its JVM legs passed and
;; its cljgo legs failed on exactly this branch, leaving a compacted transcript
;; two messages short with no error anywhere.
;;
;; Both spellings of the traversal are pinned deliberately — the seq path and
;; the chunked path disagreed, so testing one alone proves nothing about the
;; other. `some` is included because it is the shape that actually bit.
;;
;; oracle (clojure 1.12.5), each form verified individually:
;;   (range 6 1 -1)                    => (6 5 4 3 2)
;;   (range 10 0 -2)                   => (10 8 6 4 2)
;;   (map inc (range 6 1 -1))          => (7 6 5 4 3)
;;   (some #(when (< % 4) %) (range 6 1 -1)) => 3
;;   (take 3 (range 6 1 -1))           => (6 5 4)
;;   (drop 2 (range 6 1 -1))           => (4 3 2)
;;   (range 5 5 -1)                    => ()
;;   (last (range 6 1 -1))             => 2
;;   (into [] (range 3 -3 -1))         => [3 2 1 0 -1 -2]
;;   (count (range 6 1 -1))            => 5
;;   (range 0 5 1)                     => (0 1 2 3 4)
[(range 6 1 -1)
 (range 10 0 -2)
 (map inc (range 6 1 -1))
 (some #(when (< % 4) %) (range 6 1 -1))
 (take 3 (range 6 1 -1))
 (drop 2 (range 6 1 -1))
 (range 5 5 -1)
 (last (range 6 1 -1))
 (into [] (range 3 -3 -1))
 (count (range 6 1 -1))
 (range 0 5 1)]
;; expect: [(6 5 4 3 2) (10 8 6 4 2) (7 6 5 4 3) 3 (6 5 4) (4 3 2) () 2 [3 2 1 0 -1 -2] 5 (0 1 2 3 4)]
