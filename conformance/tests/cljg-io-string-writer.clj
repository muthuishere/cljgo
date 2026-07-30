;; cljg.io/string-writer + writer-str — the public in-memory writer
;; `(binding [*out* w] …)` needs (known-issues 2026-07-28 #11). Holding the
;; writer across several forms is what with-out-str (whole body) and cljx.test's
;; `capturing` (a test body) cannot express; all three ride the SAME clojure.core
;; -string-writer substrate. Reading is non-destructive: writer-str twice returns
;; the same text, and writing more appends.
;;
;; oracle: skip — cljg.io is a cljgo host namespace with no JVM package (cljgo
;; has no java.io.StringWriter), so this file cannot run through the `clojure`
;; CLI verbatim. The contract was verified against java.io.StringWriter at
;; authoring time (clojure 1.12.5, 2026-07-28):
;;   (let [w (java.io.StringWriter.)]
;;     (binding [*out* w] (println "hi") (print "x")) (prn (str w)))
;;   => "hi\nx"
;; cljgo's reader is `writer-str` rather than `str` (a cljgo writer is a Go
;; io.Writer, not a JVM object whose toString is its buffer). Frozen against
;; cljgo's output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.io :as io])
(let [w (io/string-writer)]
  (binding [*out* w] (println "hi") (print "x"))
  (let [once (io/writer-str w)]
    (binding [*out* w] (print "!"))
    [once (io/writer-str w) (io/writer-str w)]))
;; expect: ["hi\nx" "hi\nx!" "hi\nx!"]
