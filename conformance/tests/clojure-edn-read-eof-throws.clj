;; clojure.edn/read at bare end-of-stream with no :eof option: throws, as on
;; the JVM (oracle, clojure 1.12.5, 2026-07-21: (edn/read r) on an exhausted
;; PushbackReader throws RuntimeException "EOF while reading").
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; oracle: skip — Go interop constructs the stream; the JVM message
;;   ("EOF while reading") is cited above from a PushbackReader run.
(require-go '[strings])
(require '[clojure.edn :as edn])
(edn/read (strings/NewReader "  "))
;; expect-error: EOF while reading
