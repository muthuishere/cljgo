;; A .cljc namespace loads via require (ADR 0068), with reader
;; conditionals selecting the :cljgo/:default branches (ADR 0036).
;; The selection is host-specific BY DESIGN: the same file on the JVM
;; oracle picks the :clj branches — verified against Clojure 1.12.5,
;; 2026-07-23, clojure -Sdeps '{:paths ["."]}' from conformance/tests:
;; ["jvm" "jvm-only" "x@jvm"]. Frozen here is cljgo's selection —
;; :cljgo where present, :default as fallback; the other hosts'
;; :clj/:cljs/:cljr branches (hostdemo.cljc carries all three) never
;; match (ADR 0036's ratified feature set).
(require 'conf.hostdemo)
[conf.hostdemo/platform conf.hostdemo/fallback (conf.hostdemo/tag "x")]
;; expect: ["cljgo" "fallback" "x@cljgo"]
