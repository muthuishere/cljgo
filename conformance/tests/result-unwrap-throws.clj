;; unwrap is the bridge to the exception world: on err (and none) it
;; THROWS an ex-info carrying the failure payload, so a railway value can
;; escape into try/catch. Here it surfaces as a runtime error.
;; oracle: skip — cljgo Result/Option primitive (no JVM ok/err/unwrap)
;; harness: eval — the throw is a runtime panic; v0 has no compiled error-output contract
(unwrap (err :boom))
;; expect-error: unwrap called on
