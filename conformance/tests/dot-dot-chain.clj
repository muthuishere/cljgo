;; .. (fundamentals batch A1): chained member access — (.. x f (g a) h)
;; threads x through each member form left to right. The JVM macro
;; expands to the bare `.` special form; cljgo has no bare `.`, so each
;; step expands to the dot-method form the analyzer implements
;; ((.f x), ADR 0010) — same threading semantics, cljgo's interop
;; spelling (core/core.clj).
;; oracle: skip — host methods differ (no java.lang.String methods on a
;; Go string); the threading semantics are frozen from clojure 1.12.5,
;; 2026-07-23: (.. "abc" toUpperCase (substring 1) length) => 2;
;; (.. "  x  " trim length) => 1.
[(.. (atom (atom 5)) Deref Deref)
 (.. (atom 1) (Reset 5))]
;; expect: [5 5]
