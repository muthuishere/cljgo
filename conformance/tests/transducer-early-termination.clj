;; Batch 4 transducers (ADR 0022, design/08 §5): early termination via the
;; `reduced` box propagating correctly through COMPOSED xforms, in both
;; directions — take inside/outside cat (cat wraps its inner reduce with a
;; preserving-reduced rf so an inner `reduced` re-wraps and stops the outer
;; reduce too), infinite sources cut by take, partition-by's buffered run
;; flushed by the completion arity even when take truncates, and halt-when
;; (with and without retf) escaping with its value from transduce/into.
;; Oracle (clojure 1.12.5):
;; [[2 4] [1 2 3] [1 2 3 4] [0 2] [1 :x] [[2 2]] 5 [[2 4] 5] 5]
[(into [] (comp (map inc) (filter even?)) [1 2 3])
 (into [] (comp cat (take 3)) [[1 2] [3 4] [5 6]])
 (transduce (comp (take 2) cat) conj [[1 2] [3 4] [5 6]])
 (into [] (comp (filter even?) (take 2)) (range))
 (into [] (comp (interpose :x) (take 2)) [1 2 3])
 (into [] (comp (partition-by even?) (take 1)) [2 2 3 3 4])
 (transduce (halt-when odd?) conj [2 4 5 6])
 (transduce (halt-when odd? (fn [res in] [res in])) conj [2 4 5 6])
 (into [] (halt-when odd?) [2 4 5 6])]
;; expect: [[2 4] [1 2 3] [1 2 3 4] [0 2] [1 :x] [[2 2]] 5 [[2 4] 5] 5]
