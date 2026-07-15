;; Checked + throws on int64 overflow (Clojure semantics); the emit
;; intrinsic falls through to the same tower path, so both modes throw.
(+ 9223372036854775807 1)
;; expect-error: integer overflow
