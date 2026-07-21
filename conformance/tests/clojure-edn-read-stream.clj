;; clojure.edn/read (core/edn.cljg + the -edn-read seam): successive reads
;; from ONE stream return successive forms, continuing exactly where the
;; previous read stopped; {:eof v} returns v at end-of-stream. The stream is
;; a Go io.Reader (strings/NewReader here) where the JVM takes a
;; java.io.PushbackReader — same contract, host-appropriate type.
;; oracle: skip — Go interop constructs the stream; the JVM equivalent
;;   (java.io.PushbackReader over a StringReader, clojure 1.12.5,
;;   2026-07-21) returns the same four values: {:a 1}, [2 3], foo, :done.
(require-go '[strings])
(require '[clojure.edn :as edn])
(def r (strings/NewReader "{:a 1} [2 3] foo"))
[(edn/read r)
 (edn/read r)
 (edn/read r)
 (edn/read {:eof :done} r)]
;; expect: [{:a 1} [2 3] foo :done]
