;; CASE A — third-party unlinked require-go member. Must HARD-ERROR (post-fix),
;; naming the module path and member. Today it silently returns nil.
(require-go '["github.com/gorilla/websocket" :as ws])
(println "gorilla/websocket close-normal code:" ws/CloseNormalClosure)
