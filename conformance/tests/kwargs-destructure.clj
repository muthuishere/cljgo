;; The 1.11 kwargs bridge — seq-to-map-for-destructuring + the
;; destructure map branch's kv-seq wiring (tail wave, 2026-07-23): a fn
;; with [& {:keys [...]}] is callable with kv pairs, with one map, AND
;; with kv pairs plus a trailing map (whose entries update earlier keys
;; in place); let-destructuring a kv SEQ works the same way. This was
;; broken before (every shape returned nils — the -pb map branch never
;; converted a seq).
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o1.clj):
;;   (defn kw [& {:keys [a b]}] [a b])
;;   [(kw :a 1 :b 2) (kw {:a 1 :b 2}) (kw :a 1 {:b 2})] => [[1 2] [1 2] [1 2]]
;;   (let [{:keys [a]} (list :a 1)] a) => 1
;;   (seq-to-map-for-destructuring (seq [:a 1 :b 2])) => {:a 1, :b 2}
;;   (seq-to-map-for-destructuring (seq [{:a 1}])) => {:a 1}
;;   (seq-to-map-for-destructuring (seq [:a 1 {:b 2}])) => {:a 1, :b 2}
;;   (seq-to-map-for-destructuring (seq [:a 1 {:a 9 :b 2}])) => {:a 9, :b 2}
;;   (seq-to-map-for-destructuring nil) => {}
(defn kw [& {:keys [a b]}] [a b])
[(kw :a 1 :b 2)
 (kw {:a 1 :b 2})
 (kw :a 1 {:b 2})
 (let [{:keys [a]} (list :a 1)] a)
 (seq-to-map-for-destructuring (seq [:a 1 :b 2]))
 (seq-to-map-for-destructuring (seq [{:a 1}]))
 (seq-to-map-for-destructuring (seq [:a 1 {:b 2}]))
 (seq-to-map-for-destructuring (seq [:a 1 {:a 9 :b 2}]))
 (seq-to-map-for-destructuring nil)]
;; expect: [[1 2] [1 2] [1 2] 1 {:a 1, :b 2} {:a 1} {:a 1, :b 2} {:a 9, :b 2} {}]
