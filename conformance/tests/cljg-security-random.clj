;; cljg.security randomness (ADR 0103): uuid is a well-formed v4 (version
;; nibble 4, variant nibble 8-b), random n returns n bytes as 2n hex chars,
;; and token returns a non-empty string — SHAPE-ONLY assertions (the values
;; are cryptographically random; only their form is deterministic).
;;
;; oracle: skip — cljg.security is a cljgo host namespace with no JVM package
;; (java.util.UUID exists but this file requires cljg.security, which the
;; `clojure` CLI cannot load); the v4 shape matches java.util.UUID/randomUUID.
;; Frozen against cljgo; REPL-vs-binary parity via the dual harness.
(require '[cljg.security :as sec])
[(boolean (re-matches #"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}" (sec/uuid)))
 (not= (sec/uuid) (sec/uuid))
 (count (sec/random 16))
 (boolean (re-matches #"[0-9a-f]{32}" (sec/random 16)))
 (string? (sec/token))
 (pos? (count (sec/token)))]
;; expect: [true true 32 true true true]
