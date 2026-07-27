;; cljg.system/getenv (ADR 0101): reads ONE environment variable by name —
;; the value string when set, nil when unset, or a supplied default when unset.
;; Same contract as System/getenv (the 2-arity default is a cljgo convenience);
;; environ hands back the whole environment as a map. PATH is present in any
;; process the harness runs (eval + compiled), so getenv returns a non-nil
;; string for it and environ contains it; a nonsense name is unset.
;;
;; oracle: skip — cljg.system is a cljgo host namespace with no JVM package,
;; so this file cannot run through the `clojure` CLI verbatim. getenv's
;; semantics were verified against System/getenv at authoring time (clojure
;; 1.12.5, 2026-07-27): (System/getenv "PATH") => a non-nil string;
;; (System/getenv "CLJGO_NOPE_XYZ") => nil (Java's getenv has no default arg —
;; the 2-arity fallback is cljgo's own). Frozen against cljgo's output;
;; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.system :as sys])
[(string? (sys/getenv "PATH"))
 (sys/getenv "CLJGO_NOPE_XYZ")
 (sys/getenv "CLJGO_NOPE_XYZ" "fallback")
 (contains? (sys/environ) "PATH")]
;; expect: [true nil "fallback" true]
