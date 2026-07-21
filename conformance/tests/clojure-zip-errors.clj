;; clojure.zip error + edge paths (fundamentals audit 2026-07): embedded
;; core/zip.cljg. children on a leaf, insert-left/insert-right at the top,
;; remove at the top all throw; the messages are frozen via ex-message so
;; the same file oracles on the JVM (where zip throws java.lang.Exception —
;; ex-message reads any Throwable) and on cljgo (ex-info, the established
;; Exception. substitution, core.clj:1391). down on an empty branch => nil.
;; oracle (clojure 1.12.5, `clojure` CLI): this exact vector prints
;; byte-identically on the JVM (verified 2026-07-21).
(require '[clojure.zip :as z])
[(try (z/children (-> [1] z/vector-zip z/down)) (catch Exception e (ex-message e)))
 (try (z/insert-left (z/vector-zip [1]) 0) (catch Exception e (ex-message e)))
 (try (z/insert-right (z/vector-zip [1]) 0) (catch Exception e (ex-message e)))
 (try (z/remove (z/vector-zip [1])) (catch Exception e (ex-message e)))
 (z/down (z/vector-zip []))]
;; expect: ["called children on a leaf node" "Insert at top" "Insert at top" "Remove at top" nil]
