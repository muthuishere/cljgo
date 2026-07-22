;; ADR 0050 decision 4 / ADR 0049: a static Java surface hard-errors at analysis
;; with file:line — never a silent nil. cljgo does not do Java; the Class/member
;; static call resolves no namespace, and (per ADR 0049) the failure is LOUD, as
;; loud as an unlinked Go module. Oracle: on the JVM this returns a long; on
;; cljgo's Go host it must ERROR, not return nil/"".
(System/currentTimeMillis)
;; harness: eval — ADR 0050 dec 4: Java statics error at compile/eval; there is no compiled nil to compare, the point is the error is raised not swallowed
;; expect-error: no such namespace: System
