;; ADR 0040 #7: (chan 0) throws — JVM parity, the ONE deliberate M4-v0
;; break (it used to mean "unbuffered"; JVM programs cannot contain it,
;; so nothing portable observes the change; (chan) stays the unbuffered
;; constructor).
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript.txt):
;;   chan-zero => (:throws "java.lang.AssertionError"
;;                 "Assert failed: fixed buffers must have size > 0\n(pos? n)")
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(chan 0)
;; expect-error: fixed buffers must have size > 0
