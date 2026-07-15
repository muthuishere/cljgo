;; NaN?/array-map/sorted-map/sorted-map-by/subseq/rsubseq (ADR 0022,
;; design/08 §5 — the single biggest jank clojure-test-suite blockers).
;; Oracle (clojure 1.12.5): (NaN? ##NaN) => true, (NaN? 1) => false;
;; (array-map :a 1 :b 2 :a 3) => {:a 3, :b 2} (dup key keeps first
;; position, last value); (sorted-map :b 2 :a 1) => {:a 1, :b 2};
;; (sorted-map-by > 1 :a 3 :b 2 :c) => {3 :b, 2 :c, 1 :a}; subseq/rsubseq
;; dispatch on which of </<=/>/>= is passed (works on non-numeric sorted
;; keys, e.g. keywords) rather than invoking it on the keys.
[(NaN? ##NaN) (NaN? 1) (NaN? 1.0) (NaN? -1.5) (NaN? ##Inf)
 (seq (array-map :b 2 :a 1)) (array-map :a 1 :b 2 :a 3) (count (array-map))
 (sorted-map :b 2 :a 1) (sorted-map) (into (sorted-map) {:a 1 :b 2})
 (assoc (sorted-map :b 2) :a 1)
 (sorted-map-by > 1 :a 3 :b 2 :c)
 (subseq (sorted-map :a 1 :b 2 :c 3) > :a)
 (subseq (sorted-set 1 2 3 4 5) >= 2 < 5)
 (rsubseq (sorted-set 1 2 3 4 5) >= 2 <= 4)
 (subseq (sorted-set 1 2 3 4 5) < 10)
 (rsubseq (sorted-map :a 1 :b 2 :c 3) <= :b)]
;; expect: [true false false false false ([:b 2] [:a 1]) {:a 3, :b 2} 0 {:a 1, :b 2} {} {:a 1, :b 2} {:a 1, :b 2} {3 :b, 2 :c, 1 :a} ([:b 2] [:c 3]) (2 3 4) (4 3 2) (1 2 3 4 5) ([:b 2] [:a 1])]
