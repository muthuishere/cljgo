;; ADR 0040 #1: a transducer REQUIRES a buffered channel — (chan nil
;; xform) throws exactly as the JVM asserts. ((chan nil) alone stays a
;; legal unbuffered constructor, oracled: chan-nil-is-unbuffered => :v.)
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript.txt):
;;   xform-unbuffered-chan-throws => (:throws "java.lang.AssertionError"
;;     "Assert failed: buffer must be supplied when transducer is\nbuf-or-n")
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(chan nil (map inc))
;; expect-error: buffer must be supplied when transducer is
