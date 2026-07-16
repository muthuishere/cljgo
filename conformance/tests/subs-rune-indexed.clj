;; subs indexes by UTF-16 code unit on the JVM, which for any BMP character
;; (everything but astral-plane/surrogate-pair codepoints) coincides with
;; the RUNE count — but NOT the UTF-8 byte count. A byte-offset
;; implementation (bare `s[start:end]` on the Go string) cuts a multi-byte
;; rune in half whenever start/end fall after one, corrupting the result.
;; Regression: clojure-test-suite core_test/subs.cljc (jank suite, ADR
;; 0022) — U+058E (2 UTF-8 bytes, 1 rune) in "ab֎de".
;; Oracle (clojure 1.12.5): ["ab֎de" "֎bcde" true]
[(subs "ab֎de" 0 5)
 (subs "֎bcde" 0)
 (= "" (subs "abcd֎" 5))]
;; expect: ["ab֎de" "֎bcde" true]
