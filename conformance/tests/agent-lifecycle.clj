;; agent lifecycle completions (fundamentals audit 2026-07):
;;   await-for            — bounded await; true if all agents drain in time
;;                          (or there are none), false on timeout/failed.
;;   agent-errors (dep.)  — a seq of the agent's error(s), or nil when ready.
;;   clear-agent-errors   — clears a failed agent and returns its value
;;                          (restart-agent to the value it already holds).
;;   release-pending-sends — 0 (cljgo dispatches each send immediately, so
;;                          there is never a batch to release).
;;   shutdown-agents      — nil (a documented no-op; cljgo's per-agent
;;                          goroutines are never pooled/torn down).
;; oracle (clojure 1.12.5, 2026-07-21):
;;   [true 1 true 1 10 nil nil 0 nil]
;; The failing action fn ends in a dead-code `nil` after the throw (the
;; AOT throw-only-body gap, see agent-error-restart.clj).
(let [a (agent 0)
      _ (send a inc)
      drained (await-for 2000 a)
      no-agents (await-for 100)
      p (promise)
      b (agent 10 :error-handler (fn [_ _] (deliver p :x)) :error-mode :fail)
      _ (send b (fn [_] (throw (ex-info "boom" {})) nil))
      _ @p
      errs (agent-errors b)
      errs-count (count errs)
      cleared (clear-agent-errors b)
      err-after (agent-error b)
      no-err (agent-errors (agent 0))]
  [drained @a no-agents errs-count cleared err-after no-err
   (release-pending-sends) (shutdown-agents)])
;; expect: [true 1 true 1 10 nil nil 0 nil]
