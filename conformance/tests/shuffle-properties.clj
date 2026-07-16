;; shuffle (design/08 batch E, ADR 0022): a NEW shuffled vector, same
;; count and same elements as the source; throws on nil/string/map/
;; non-collection (matching the JVM's `new ArrayList(coll)` requiring a
;; java.util.Collection). Output order is non-deterministic, so this
;; checks PROPERTIES (vector?/count/set-equality), not exact contents.
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(let [x [1 2 3 4 5]
       s (shuffle x)]
   [(vector? s) (= (count x) (count s)) (= (set x) (set s))])
 (try (shuffle nil) :nothrow (catch Exception _e :threw))
 (try (shuffle "abc") :nothrow (catch Exception _e :threw))
 (try (shuffle {}) :nothrow (catch Exception _e :threw))
 (try (shuffle 1) :nothrow (catch Exception _e :threw))]
;; expect: [[true true true] :threw :threw :threw :threw]
