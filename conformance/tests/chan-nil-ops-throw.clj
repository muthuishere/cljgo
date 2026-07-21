;; ADR 0040 #8: ops on a nil channel THROW (IllegalArgumentException-
;; shaped) — the "nil channel blocks forever" lore is refuted by the
;; oracle. The JVM's message is protocol-dispatch noise ("No
;; implementation of method: :take! … found for class: nil"); the error
;; TYPE is aligned (lang.IllegalArgumentError), the text kept readable.
;; oracle (JVM core.async 1.6.681, spikes/s19-core-async/oracle/transcript2.txt):
;;   nil-chan-take => [:threw "java.lang.IllegalArgumentException"]
;;   nil-chan-put  => [:threw "java.lang.IllegalArgumentException"]
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; oracle: skip — needs the core.async dep; frozen from the S19 transcript
(<! nil)
;; expect-error: <! expects a channel, got nil
