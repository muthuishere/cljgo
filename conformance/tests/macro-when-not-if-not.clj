;; when-not / if-not. Oracle (clojure 1.12.5): [2 nil :a :b nil].
[(when-not false 1 2) (when-not true 1) (if-not false :a :b) (if-not true :a :b) (if-not true :a)]
;; expect: [2 nil :a :b nil]
