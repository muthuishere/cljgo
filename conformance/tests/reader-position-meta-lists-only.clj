;; Vector/map/set literals carry NO reader position metadata — user ^ meta
;; stays exactly what was written (ADR 0038; JVM annotates lists only;
;; suite group_by.cljc / edn read_string.cljc caught :file/:line pollution).
;; oracle (clojure 1.12.5): [{:foo true} nil nil nil]
[(meta ^:foo [1 2]) (meta [1 2]) (meta {:a 1}) (meta #{1})]
;; expect: [{:foo true} nil nil nil]
