;; agent-error / restart-agent (ADR 0038 follow-on): a failing action puts
;; the agent in a :failed state — agent-error returns the stored
;; throwable, subsequent send/await throw "Agent is failed, needs
;; restart", and restart-agent installs a new state + clears the error
;; (throwing "Agent does not need a restart" if called on a non-failed
;; agent). Unblocks clojure-test-suite's add_watch.cljc (:cljgo-patched
;; catch class, see docs/suite-upstream.md) past its `agent-error` gap.
;; oracle (clojure 1.12.5, 2026-07-17): [true {:x 1} 1 "Agent is failed,
;; needs restart" 99 nil 100 "Agent does not need a restart"]
;;
;; The action fn's body ends in a dead-code `nil` after the `throw` —
;; a throw-only fn body currently trips an unrelated AOT-emitter gap
;; ("bodiless fn method": genMethodBody has no non-recur value to return
;; when the body is solely a throw node — reproducible standalone,
;; nothing to do with agents; out of scope for this batch). The `nil`
;; is never reached (throw always escapes) — purely a workaround so this
;; file exercises the dual harness like every other conformance test.
(let [a (agent 1)
      _ (send a (fn [_] (throw (ex-info "boom" {:x 1})) nil))
      _ (try (await a) (catch Exception _ nil))
      err1 (agent-error a)
      data1 (ex-data err1)
      deref1 @a
      send-msg (try (send a inc) :no-throw (catch Exception e (ex-message e)))
      restart-ret (restart-agent a 99)
      err2 (agent-error a)
      _ (send a inc)
      _ (await a)
      restart-again-msg (try (restart-agent a 1) :no-throw (catch Exception e (ex-message e)))]
  [(some? err1) data1 deref1 send-msg restart-ret err2 @a restart-again-msg])
;; expect: [true {:x 1} 1 "Agent is failed, needs restart" 99 nil 100 "Agent does not need a restart"]
