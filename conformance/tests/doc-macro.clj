;; clojure.repl/doc (ADR 0031 ride-along): embedded core/repl.cljg. The
;; probes use plain `def` vars because cljgo's defn does not stamp
;; :arglists yet (defn attr-map DEVIATION, core/core.clj) — a defn'd var
;; would print an arglists line on the JVM that cljgo omits; def vars
;; print identically on both.
;; harness: eval — clojure.repl is REPL tooling (doc prints to *out*); no emitter surface for it in v0
;; oracle (clojure 1.12.5, verified 2026-07-16):
;;   (require 'clojure.repl)
;;   (def answer "The answer to everything." 42) (def bare 7)
;;   [(with-out-str (clojure.repl/doc answer))
;;    (with-out-str (clojure.repl/doc bare))
;;    (with-out-str (clojure.repl/doc nosuchsym))]
;;   => ["-------------------------\nuser/answer\n  The answer to everything.\n"
;;       "-------------------------\nuser/bare\n" ""]
(require 'clojure.repl)
(def answer "The answer to everything." 42)
(def bare 7)
[(with-out-str (clojure.repl/doc answer))
 (with-out-str (clojure.repl/doc bare))
 (with-out-str (clojure.repl/doc nosuchsym))]
;; expect: ["-------------------------\nuser/answer\n  The answer to everything.\n" "-------------------------\nuser/bare\n" ""]
