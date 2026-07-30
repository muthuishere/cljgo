;; NESTED reader conditionals in ordinary (non-Maven) project code keep the
;; JVM's semantics EXACTLY: a conditional with no selectable branch reads as
;; nothing, at any depth, even when that changes the enclosing form's arity.
;; This file is the standing guarantee that the Maven-origin starved check
;; (R1012, WithStarvedCondError — extended to nested conditionals after
;; medley 1.4.0 core.cljc:181) never leaks into project reading.
;;
;; oracle: JVM Clojure 1.12.5 (`clojure` CLI 1.12.5.1645), 2026-07-30, via
;; (load-file "nestoracle.cljc") on the same forms with the standing feature
;; substitution :cljgo -> :clj (each platform's own feature) and :clj ->
;; :cljs (a foreign one), since the JVM cannot be starved of :clj:
;;   (prn [(list :a #?(:cljs :x :cljr :y) :b)
;;         (let [a 1 #?@(:cljs [b 2]) c 3] [a c])
;;         (list :a #?(:cljs :x :default :d) :b)
;;         (list :a #?(:clj :g :cljs :x) :b)
;;         ["(f #?(:cljs x))" #_(f #?(:cljs z)) :end]
;;         (vector 1 #?(:cljs 2) 3)
;;         {:k #?(:clj 1 :default 9)}])
;;   => [(:a :b) [1 3] (:a :d :b) (:a :g :b) ["(f #?(:cljs x))" :end] [1 3] {:k 1}]
[;; starved, nested in ARGUMENT position: the arg simply vanishes.
 (list :a #?(:clj :x :cljs :y) :b)
 ;; starved SPLICE in a let binding vector: contributes zero elements.
 (let [a 1 #?@(:clj [b 2]) c 3] [a c])
 ;; :default present at the nested site => not starved, never an error.
 (list :a #?(:clj :x :default :d) :b)
 ;; :cljgo present at the nested site => not starved either.
 (list :a #?(:cljgo :g :clj :x) :b)
 ;; a `#?(` inside a STRING is text, and one inside a #_ discard is dropped
 ;; with the discarded form — neither is a conditional the reader selects on.
 ["(f #?(:clj x))" #_(f #?(:clj z)) :end]
 ;; starved inside a vector and a map, default (project) semantics: elided.
 (vector 1 #?(:clj 2) 3)
 {:k #?(:cljgo 1 :default 9)}]
;; expect: [(:a :b) [1 3] (:a :d :b) (:a :g :b) ["(f #?(:clj x))" :end] [1 3] {:k 1}]
