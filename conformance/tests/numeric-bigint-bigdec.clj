;; bigint (clojure.lang.BigInt, prints N), biginteger (BigInteger, no N),
;; bigdec, and parse-* — all must render byte-identically in both modes.
[(bigint "123456789012345678901234567890")
 (biginteger 7)
 (bigdec "3.14")
 (bigint 5/2)
 (parse-long "42")
 (parse-long "nope")
 (parse-double "2.5")
 (parse-boolean "false")]
;; expect: [123456789012345678901234567890N 7 3.14M 2N 42 nil 2.5 false]
