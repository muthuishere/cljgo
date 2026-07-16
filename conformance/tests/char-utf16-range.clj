;; Clojure's char is a Java `char` — a single UTF-16 code unit, max 0xFFFF —
;; NOT a full Unicode code point (Go's rune, max 0x10FFFF). CharCast previously
;; bounded against utf8.MaxRune, so values above 0xFFFF (e.g. astral-plane
;; code points) wrongly succeeded instead of throwing.
;; oracle (clojure 1.12.5): (char 65535) => \￿ ; (char 65895) throws
;; "Value out of range for char: 65895"
[(char 65535)
 (try (char 65895) (catch Exception _e :threw))]
;; expect: [\￿ :threw]
