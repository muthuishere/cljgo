;; Munged class-name resolution (ADR 0039): the JVM spells the generated
;; class of a defprotocol/defrecord/deftype with the namespace's dashes
;; munged to underscores (ns cljgo-conf.mung-test => class prefix
;; cljgo_conf.mung_test). cljgo resolves such a dotted symbol — after
;; every normal lookup and the well-known-class table miss — back to the
;; defining var (demunging _ to -), so suite-style references like
;; clojure.core_test.ancestors.TestAncestorsProtocol work. Fail-closed:
;; the namespace must be loaded and the var must hold a protocol or a
;; deftype/defrecord type marker.
;;
;; harness: eval — class-name symbols resolve at the interpreter's
;; symbol-resolution level (ADR 0036/0039); AOT emission is deferred
;; (suite runs interpreted, ADR 0022 decision 4).
;; oracle: this EXACT file evaluated against clojure 1.12.5 (clojure CLI,
;; 2026-07-17) prints the same vector.
(ns cljgo-conf.mung-test)
(defprotocol MP)
(defrecord MR [] MP)
(deftype MT [] MP)
[(contains? (ancestors MR) cljgo_conf.mung_test.MP)
 (contains? (parents MT) cljgo_conf.mung_test.MP)
 (isa? MR cljgo_conf.mung_test.MP)
 (= MR cljgo_conf.mung_test.MR)
 (class? cljgo_conf.mung_test.MR)]
;; expect: [true true true true true]
