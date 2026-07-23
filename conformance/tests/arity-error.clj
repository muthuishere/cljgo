;; Calling a fn with an unsupported argument count is an arity error, and
;; it names the Var the fn was def'd into (def name > self-name > "fn" —
;; the ADR 0048 rule both legs share). oracle (clojure 1.12.5):
;;   (def f (fn* f [x y] (+ x y))) (f 1)
;;   => "Wrong number of args (1) passed to: user/f--142" — the JVM name
;;   is the munged fn class; cljgo's convention is the clean var name
;;   (user/f), lowercase "wrong".
(def f (fn* f [x y] (+ x y)))
(f 1)
;; harness: eval — expects an error: cljgo build fails at compile/eval time; v0 has no compiled error-output contract
;; expect-error: wrong number of args (1) passed to: user/f
