;; CASE C — stdlib require-go only. Must produce real values, no error.
(require-go '[strings])
(require-go '[strconv])
(require-go '[math])
(println "ToUpper:" (strings/ToUpper "hello"))
(println "Itoa:" (strconv/Itoa 42))
(println "Pi:" math/Pi)
