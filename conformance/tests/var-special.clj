;; (var sym) evaluates to the Var object itself, not its value.
;; Oracle (Clojure 1.12, 2026-07-12): (var vv) → #'user/vv (this
;; codebase prints vars in the M0 "#=(var ns/name)" reader form).
(def vv 3)
(var vv)
;; expect: #=(var user/vv)
