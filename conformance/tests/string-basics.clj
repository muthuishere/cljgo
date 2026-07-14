;; clojure.string: join / casing / trim / reverse. The namespace is embedded
;; and loaded at boot; (require 'clojure.string) then finds it.
;; oracle (Clojure 1.12.5, `clojure -M`, 2026-07-15):
;;   (clojure.string/join [1 2 3])          => "123"
;;   (clojure.string/join ", " [1 2 3])     => "1, 2, 3"
;;   (clojure.string/upper-case "abc")      => "ABC"
;;   (clojure.string/lower-case "ABC")      => "abc"
;;   (clojure.string/capitalize "hELLO")    => "Hello"
;;   (clojure.string/reverse "abc")         => "cba"
;;   (clojure.string/trim "  hi  ")         => "hi"
;;   (clojure.string/triml "  hi  ")        => "hi  "
;;   (clojure.string/trimr "  hi  ")        => "  hi"
;;   (clojure.string/trim-newline "hi\n\n") => "hi"
(require 'clojure.string)
[(clojure.string/join [1 2 3])
 (clojure.string/join ", " [1 2 3])
 (clojure.string/upper-case "abc")
 (clojure.string/lower-case "ABC")
 (clojure.string/capitalize "hELLO")
 (clojure.string/reverse "abc")
 (clojure.string/trim "  hi  ")
 (clojure.string/triml "  hi  ")
 (clojure.string/trimr "  hi  ")
 (clojure.string/trim-newline "hi\n\n")]
;; expect: ["123" "1, 2, 3" "ABC" "abc" "Hello" "cba" "hi" "hi  " "  hi" "hi"]
