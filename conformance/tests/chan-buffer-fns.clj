;; T1 (openspec core-async-first-class 1.2): (buffer n) is the explicit
;; fixed-buffer constructor and unblocking-buffer? distinguishes the
;; never-block policies. Both are clojure.core.async-only names.
;; oracle (fresh 2026-07-21 run, core.async 1.6.681 on Clojure 1.12.5):
;;   buffer-chan => [1 2 nil] · unblocking-buffer-fixed => false ·
;;   unblocking-buffer-dropping => true · unblocking-buffer-sliding => true
;; oracle: skip — needs the core.async dep; frozen from the fresh T1 oracle run
(require '[clojure.core.async :as async])
(def c (chan (async/buffer 2)))
(>! c 1)
(>! c 2)
(close! c)
[(<! c) (<! c) (<! c)
 (async/unblocking-buffer? (async/buffer 2))
 (async/unblocking-buffer? (dropping-buffer 2))
 (async/unblocking-buffer? (sliding-buffer 2))]
;; expect: [1 2 nil false true true]
