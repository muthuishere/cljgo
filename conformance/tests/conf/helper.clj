;; Dependency namespace for the multi-ns conformance tests (ADR 0042).
;; Lives under tests/conf/ — outside the harness glob (tests/*.clj) —
;; and is loaded only via require from the entry files.
(ns conf.helper)
(defn exclaim [s] (str s "!"))
(defmacro square [x] (list '* x x))
