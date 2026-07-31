;; ISSUE #171: ok/err/just/none/etc. moved OUT of clojure.core into
;; cljx.meta (ADR 0115, precedence principle) -- required and referred
;; here explicitly, exactly as real user code must.
(require (quote cljx.meta))
(clojure.core/refer (quote cljx.meta))

;; unwrap is the bridge to the exception world: on err (and none) it
;; THROWS an ex-info carrying the failure payload, so a railway value can
;; escape into try/catch. Here it surfaces as a runtime error.
;; oracle: skip — cljgo Result/Option primitive (no JVM ok/err/unwrap)
;; harness: eval — the throw is a runtime panic; v0 has no compiled error-output contract
(unwrap (err :boom))
;; expect-error: unwrap called on
