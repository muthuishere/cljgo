;; pr-str prints readably: strings quoted, keywords with the colon.
(pr-str "hi" :kw [1 "two"])
;; expect: "\"hi\" :kw [1 \"two\"]"
