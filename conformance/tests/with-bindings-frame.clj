;; with-bindings / with-bindings*: push a Var->value map as thread
;; bindings around a body/thunk; the frame pops afterwards (root value
;; visible again).
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (def ^:dynamic *dv* 1)
;; [(with-bindings {#'*dv* 42} *dv*) (with-bindings* {#'*dv* 7}
;; (fn [] *dv*)) *dv*] => [42 7 1]
(def ^:dynamic *dv* 1)
[(with-bindings {#'*dv* 42} *dv*) (with-bindings* {#'*dv* 7} (fn [] *dv*)) *dv*]
;; expect: [42 7 1]
