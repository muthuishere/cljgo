;; Batch 1 type/number predicates (ADR 0022, design/08 §5). Fidelity oracle
;; (clojure 1.12): (coll? "x") => false; (int? 1.0) => false; (fn? :a) => false
;; but (ifn? :a) => true; (float? 1) => false; (double? 1.0) => true.
[(any? nil) (any? 5)
 (coll? []) (coll? {}) (coll? #{}) (coll? "x") (coll? nil)
 (ifn? :a) (ifn? {}) (ifn? +) (fn? +) (fn? :a) (fn? {})
 (seqable? "x") (seqable? nil) (seqable? [1]) (seqable? 5)
 (counted? [1]) (associative? {}) (associative? [1]) (associative? #{})
 (reversible? [1]) (sorted? (sorted-set 1)) (sorted? #{1})
 (set? #{1}) (list? '(1)) (list? [1]) (indexed? [1]) (indexed? '(1))
 (number? 1) (number? 1.0) (number? \a) (int? 1) (int? 1.0)
 (integer? 1) (float? 1.0) (float? 1) (double? 1.0) (rational? 1)
 (ratio? 1) (decimal? 1) (boolean? true) (boolean? 1) (char? \a) (uuid? 1)
 (pos-int? 3) (neg-int? -3) (nat-int? 0) (pos-int? -1)]
;; expect: [true true true true true false false true true true true false false true true true false true true true false true true false true true false true false true true false true false true true false true true false false true false true false true true true false]
