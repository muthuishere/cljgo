;; char-escape-string / char-name-string — the printer's plain data maps
;; (fundamentals batch A2).
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (char-escape-string \newline) => "\n" (the 2-char escape)
;;   (char-escape-string \tab) => "\t"; \return => "\r"; \" => "\""
;;   (char-escape-string \\) => "\\"; \formfeed => "\f"; \backspace => "\b"
;;   (char-escape-string \a) => nil; (count char-escape-string) => 7
;;   (char-name-string \newline) => "newline"; \space => "space";
;;   \tab => "tab"; \backspace => "backspace"; \formfeed => "formfeed";
;;   \return => "return"; (char-name-string \a) => nil;
;;   (count char-name-string) => 6
[(char-escape-string \newline)
 (char-escape-string \tab)
 (char-escape-string \return)
 (char-escape-string \")
 (char-escape-string \\)
 (char-escape-string \formfeed)
 (char-escape-string \backspace)
 (char-escape-string \a)
 (count char-escape-string)
 (char-name-string \newline)
 (char-name-string \space)
 (char-name-string \tab)
 (char-name-string \backspace)
 (char-name-string \formfeed)
 (char-name-string \return)
 (char-name-string \a)
 (count char-name-string)]
;; expect: ["\\n" "\\t" "\\r" "\\\"" "\\\\" "\\f" "\\b" nil 7 "newline" "space" "tab" "backspace" "formfeed" "return" nil 6]
