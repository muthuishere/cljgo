;; Batch A3: find-keyword — returns the keyword ONLY if one with that
;; name was already interned, else nil; never interns. A keyword argument
;; returns itself (it exists, so it is interned); reading this file's
;; :cljgo-a3fk-interned literal interns it before evaluation. The
;; never-interned names appear only as STRINGS, so they stay uninterned
;; (and a repeat lookup still misses — find-keyword must not intern).
;; Oracle (clojure 1.12.5): verified 2026-07-23.
[(find-keyword "cljgo-a3fk-never-interned")
 (find-keyword "cljgo-a3fk-never-interned")
 (find-keyword :cljgo-a3fk-kwarg)
 (do :cljgo-a3fk-interned (find-keyword "cljgo-a3fk-interned"))
 (find-keyword "cljgo-a3fk" "no-such-ns-name")
 (do :cljgo-a3fk/qualified (find-keyword "cljgo-a3fk" "qualified"))]
;; expect: [nil nil :cljgo-a3fk-kwarg :cljgo-a3fk-interned nil :cljgo-a3fk/qualified]
