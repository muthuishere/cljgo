;; M3-v0 Go-interop demo (ADR 0010, design/05 §1-§2). The SAME source runs
;; interpreted (`cljgo run examples/interop/core.clj`) and AOT-compiled
;; (`cljgo build examples/interop/core.clj && ./interop`) with byte-identical
;; output — the whole point of the dual-harness. Uses only the v0 stdlib seed
;; surface: package fns, a const, (T,error) [v err] shaping, and the ! throw.

(require-go '[strings])
(require-go '[strconv])
(require-go '[math])

;; single-return package fns + int64->int arg coercion
(println "ToUpper:" (strings/ToUpper "hello"))
(println "Repeat:" (strings/Repeat "ab" 3))
(println "Itoa:" (strconv/Itoa 42))

;; const in value position (OpHostRef)
(println "Pi:" math/Pi)
(println "Sqrt 16:" (math/Sqrt 16.0))

;; (T, error) plain call -> [v err] vector; happy path err slot is nil
(println "Atoi ok:" (strconv/Atoi "123"))

;; ! suffix unwraps or throws; happy path returns the widened int64
(println "Atoi! ok:" (strconv/Atoi! "456"))

;; error slot is a truthy value the program can branch on (errors-as-values).
;; (get v 1) is the err slot; destructuring lands in a later milestone, so we
;; index the [v err] vector directly here.
(def bad (strconv/Atoi "not-a-number"))
(println "Atoi err branch ->" (if (get bad 1) "parse failed" (first bad)))
