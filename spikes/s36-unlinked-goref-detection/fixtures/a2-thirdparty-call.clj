;; CASE A2 — third-party unlinked CALL (OpHostCall). Must HARD-ERROR post-fix.
(require-go '["github.com/gorilla/websocket" :as ws])
(println "call result:" (ws/FormatCloseMessage 1000 "bye"))
