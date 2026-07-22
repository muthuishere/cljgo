;; agent error-mode / error-handler (fundamentals audit 2026-07). An agent's
;; error policy is :fail (default) or :continue; the default is :continue
;; when an :error-handler is supplied at construction. In :continue mode a
;; throwing action does NOT fail the agent — the handler (if any) is called
;; (agent, throwable) and the queue keeps draining, old state kept for the
;; failing action. In :fail mode the handler is STILL called, then the agent
;; enters :failed. set-error-mode!/set-error-handler! change the policy after
;; construction. Sync is via await (:continue agents never fail, so await
;; drains cleanly) and a promise (a :fail agent's handler delivers before it
;; fails — await on a failed agent would block on the JVM).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   [2 ["boom"] nil :continue 5 :fired true :continue true :continue :fail]
;; The failing action fn ends in a dead-code `nil` after the throw — a
;; throw-only fn body trips an unrelated AOT-emitter gap (see
;; agent-error-restart.clj); the nil is never reached.
(let [hits (atom [])
      a (agent 0 :error-mode :continue
               :error-handler (fn [_ e] (swap! hits conj (ex-message e))))
      _ (send a (fn [_] (throw (ex-info "boom" {})) nil))
      _ (send a inc)
      _ (send a inc)
      _ (await a)
      p (promise)
      b (agent 5 :error-mode :fail
               :error-handler (fn [_ _] (deliver p :fired)))
      _ (send b (fn [_] (throw (ex-info "x" {})) nil))
      handler-fired @p
      c (agent 1)
      _ (set-error-mode! c :continue)
      _ (set-error-handler! c (fn [_ _] nil))]
  [@a @hits (agent-error a) (error-mode a)
   @b handler-fired (some? (agent-error b))
   (error-mode c) (some? (error-handler c))
   (error-mode (agent 0 :error-handler (fn [_ _] nil)))
   (error-mode (agent 0))])
;; expect: [2 ["boom"] nil :continue 5 :fired true :continue true :continue :fail]
