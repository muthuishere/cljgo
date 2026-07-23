;; clojure.core/reader-conditional + reader-conditional? (ADR 0050).
;; A ReaderConditional carries the whole `#?(...)` / `#?@(...)` body as a
;; :form list plus a :splicing? flag; it prints as `#?(...)` or `#?@(...)`,
;; supports keyword lookup and =. (This is the DATA value; the reader
;; yields the same values via read-string {:read-cond :preserve} — ADR
;; 0050's follow-up, wired 2026-07-23, frozen in
;; read-string-read-cond.clj.)
;;
;; oracle: JVM Clojure 1.12.5 (clojure CLI), 2026-07-22:
;;   (prn [(pr-str (reader-conditional '(:clj 1 :cljs 2) false))
;;         (:form (reader-conditional '(:clj 1 :cljs 2) false))
;;         (:splicing? (reader-conditional '(:clj 1 :cljs 2) false))
;;         (get (reader-conditional '(:clj 1) false) :nope :DEF)
;;         (reader-conditional? (reader-conditional '(:clj 1) false))
;;         (reader-conditional? 42)
;;         (= (reader-conditional '(:clj 1) false) (reader-conditional '(:clj 1) false))
;;         (= (reader-conditional '(:clj 1) false) (reader-conditional '(:clj 1) true))
;;         (pr-str (reader-conditional '(:clj [1 2]) true))
;;         (:splicing? (reader-conditional '(:clj 1) true))])
;;   => ["#?(:clj 1 :cljs 2)" (:clj 1 :cljs 2) false :DEF true false true false "#?@(:clj [1 2])" true]
[(pr-str (reader-conditional '(:clj 1 :cljs 2) false))
 (:form (reader-conditional '(:clj 1 :cljs 2) false))
 (:splicing? (reader-conditional '(:clj 1 :cljs 2) false))
 (get (reader-conditional '(:clj 1) false) :nope :DEF)
 (reader-conditional? (reader-conditional '(:clj 1) false))
 (reader-conditional? 42)
 (= (reader-conditional '(:clj 1) false) (reader-conditional '(:clj 1) false))
 (= (reader-conditional '(:clj 1) false) (reader-conditional '(:clj 1) true))
 (pr-str (reader-conditional '(:clj [1 2]) true))
 (:splicing? (reader-conditional '(:clj 1) true))]
;; expect: ["#?(:clj 1 :cljs 2)" (:clj 1 :cljs 2) false :DEF true false true false "#?@(:clj [1 2])" true]
