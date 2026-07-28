;; A Java static call is DIAGNOSED as what it is: `Integer` is a class, not a
;; namespace. cljgo hosts Clojure on Go, so java.lang.Integer's statics do not
;; exist; the old bare `no such namespace: Integer` pointed the user at a
;; namespace they were never going to find. The leading clause stays (frozen by
;; java-static-loud-error.clj / ADR 0054 dec 4); the diagnosis is appended, and
;; the render layer attaches I4001 + `help: did you mean parse-long?`.
;; Oracle: on the JVM (clojure CLI 1.12.5) `(Integer/parseInt "42")` returns 42;
;; on cljgo's Go host it must ERROR — the contract here is WHICH error.
(Integer/parseInt "42")
;; harness: eval — Java statics error at analysis; there is no compiled value to compare, the point is the diagnosis text
;; oracle: skip — the JVM resolves Integer/parseInt; cljgo has no JVM classes (documented deviation, docs/diagnostics/I4001.md)
;; expect-error: no such namespace: Integer (Integer is a Java class, not a namespace: cljgo hosts Clojure on Go, so the Java static Integer/parseInt is unavailable)
