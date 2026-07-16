;; repeat (batch/error-files): n coerces the way a Java numeric cast would,
;; including a bare bool (true=1, false=0).
;; NOTE on oracle provenance: the suite's own test file splits this exact
;; case by dialect — #?(:clj (thrown? (repeat false :a)) :default (= []
;; (repeat false :a))) — because JVM Clojure's (repeat false :a) throws a
;; ClassCastException (Boolean isn't a Number), while every non-JVM dialect
;; in the suite (cljs/bb/lpy/phel) takes the lenient :default branch. cljgo's
;; reader answers :cljgo (never :clj, ADR-ratified, pkg/reader/readcond.go),
;; so cljgo always takes :default too — this behavior's oracle is the
;; suite's own :default branch, not a directly-runnable `clojure` CLI
;; invocation (which can only ever produce the :clj branch).
[(repeat false :a) (repeat true :a)]
;; expect: [() (:a)]
