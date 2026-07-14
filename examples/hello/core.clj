;; M2 exit demo (design/00 §6): `cljgo build examples/hello/core.clj &&
;; ./hello` prints from a static native binary, startup < 50 ms.
;; Factorial exercises the whole v0 emitter: def, named fn*, if, recur-free
;; self-call through the Var, loop*/recur, and core.clj macros (defn, when)
;; expanded at compile time.

(defn fact [n]
  (if (< n 2)
    1
    (* n (fact (- n 1)))))

(defn sum-to [n]
  (loop [i 1 acc 0]
    (if (> i n)
      acc
      (recur (+ i 1) (+ acc i)))))

(println "hello from a cljgo binary")
(println "(fact 10) =" (fact 10))
(println "(sum-to 100000) =" (sum-to 100000))
(when true
  (println "macros expanded at compile time"))
