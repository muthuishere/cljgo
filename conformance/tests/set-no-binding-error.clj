;; set! without a thread binding fails at runtime, even on a dynamic var.
;; Oracle (Clojure 1.12, 2026-07-12): IllegalStateException
;;   "Can't change/establish root binding of: *s2* with set".
(def ^:dynamic *s2* 1)
(set! *s2* 5)
;; expect-error: change/establish root binding
