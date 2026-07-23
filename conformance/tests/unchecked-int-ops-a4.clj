;; unchecked-inc-int / unchecked-dec-int / unchecked-negate-int — the
;; -int arithmetic names complete. cljgo stance (documented, matches the
;; other -int ops): they operate on int64 (no boxed Integer type), so
;; wrap-around happens at 64 bits, not the JVM's 32 — values inside int32
;; range agree exactly, and only those are frozen here.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): [(unchecked-dec-int 0)
;; (unchecked-negate-int 5) (unchecked-inc-int 5)] => [-1 -5 6]
[(unchecked-dec-int 0) (unchecked-negate-int 5) (unchecked-inc-int 5)]
;; expect: [-1 -5 6]
