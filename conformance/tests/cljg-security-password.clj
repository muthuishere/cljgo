;; cljg.security passwords (ADR 0103): hash-password emits a self-describing
;; argon2id PHC string and check-password round-trips it — printing ONLY
;; booleans (the salted hash is random per call and must never be frozen or
;; printed). A wrong password and a non-hash string are both false, never an
;; error.
;;
;; oracle: skip — cljg.security is a cljgo host namespace with no JVM package
;; (argon2id hashing has no clojure.core analog); frozen against cljgo's
;; behavior. REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.security :as sec])
(require '[clojure.string :as str])
(def h (sec/hash-password "hunter2"))
[(string? h)
 (str/starts-with? h "$argon2id$")
 (sec/check-password "hunter2" h)
 (sec/check-password "wrong-password" h)
 (sec/check-password "hunter2" "not-a-hash")]
;; expect: [true true true false false]
