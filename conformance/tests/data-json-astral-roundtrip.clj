;; clojure.data.json astral-plane round-trip (ADR 0097) — THE case the pure
;; draft got wrong. A rune > U+FFFF (emoji U+1F600) must write as a \uXXXX
;; surrogate PAIR by default, read back from that pair to the SAME rune, and
;; write raw under :escape-unicode false — byte-identically to the JVM. The Go
;; codec uses utf16.EncodeRune / DecodeRune to close the BMP-only divergence.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/write-str "😀")                     => "😀"
;;   (json/read-str "\"\\ud83d\\ude00\"")       => "😀"
;;   (json/write-str "😀" :escape-unicode false) => "😀"
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
[(json/write-str "😀")
 (json/read-str "\"\\ud83d\\ude00\"")
 (json/write-str "😀" :escape-unicode false)]
;; expect: ["\"\\ud83d\\ude00\"" "😀" "\"😀\""]
