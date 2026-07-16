;; A panicking WATCH function (not the action itself) also fails the
;; agent, but the state install already happened (the JVM writes the new
;; state, THEN notifies watches, ADR 0038 follow-on) — so @g reflects the
;; update even though agent-error is now set. Earlier watches (added
;; before the failing one) still ran and logged normally. Unblocks
;; clojure-test-suite's add_watch.cljc "watch agent" testing block.
;; oracle (clojure 1.12.5, 2026-07-17): [[[:w 20 21]] 21 true]
;;
;; The watcher fn's body ends in a dead-code `nil` after the `throw` —
;; a throw-only fn body currently trips an unrelated AOT-emitter gap
;; ("bodiless fn method": genMethodBody has no non-recur value to return
;; when the body is solely a throw node — reproducible standalone,
;; nothing to do with agents; out of scope for this batch). The `nil`
;; is never reached (throw always escapes) — purely a workaround so this
;; file exercises the dual harness like every other conformance test.
(let [g (agent 20)
      log (atom [])
      _ (add-watch g :w (fn [k _r o n] (swap! log conj [k o n])))
      _ (add-watch g :e (fn [_k _r _o _n] (throw (ex-info "watcherr" {})) nil))
      _ (send g inc)
      _ (try (await g) (catch Exception _ nil))]
  [@log @g (some? (agent-error g))])
;; expect: [[[:w 20 21]] 21 true]
