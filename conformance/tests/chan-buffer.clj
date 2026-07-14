;; M4+ buffer policies (design/05 §4): (chan (dropping-buffer n)) rejects
;; over-capacity puts; (chan (sliding-buffer n)) evicts the oldest to admit the
;; newest. Both never block the producer (Go channels have no native
;; drop/slide — a policy layer over a non-blocking send, let-go's chanPolicy).
;; Deterministic: single-producer puts into a size-2 buffer, then close+drain.
;;   dropping 2 <- 1 2 3  => buffer holds [1 2], 3 dropped => drain [1 2 nil]
;;   sliding  2 <- 1 2 3  => 1 evicted, buffer holds [2 3]  => drain [2 3 nil]
;; oracle: skip — cljgo concurrency (JVM core.async differs)
(def cd (chan (dropping-buffer 2)))
(>! cd 1)
(>! cd 2)
(>! cd 3)
(close! cd)
(def cs (chan (sliding-buffer 2)))
(>! cs 1)
(>! cs 2)
(>! cs 3)
(close! cs)
[(<! cd) (<! cd) (<! cd) (<! cs) (<! cs) (<! cs)]
;; expect: [1 2 nil 2 3 nil]
