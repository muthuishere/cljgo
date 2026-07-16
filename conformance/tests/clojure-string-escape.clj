;; clojure.string/escape (ADR 0022 batch/harness-misc): replaces each
;; character ch of s per cmap — (cmap ch) nil/missing => ch passes through
;; unchanged, non-nil => (str (cmap ch)) is appended instead. Bogus/unrelated
;; keys in cmap (an int, a symbol, a keyword) are simply never looked up by
;; a char key and do not affect the result. oracle (clojure 1.12.5):
;;   (clojure.string/escape "a<b" {\< "&lt;"}) => "a&lt;b"
;;   (clojure.string/escape "abc" {\a "A_A" \c "C_C" (int \a) 1 nil 'junk :garbage 42.42}) => "A_AbC_C"
;;   (clojure.string/escape "" {}) => ""
(require '[clojure.string :as str])
[(str/escape "a<b" {\< "&lt;"})
 (str/escape "abc" {\a "A_A" \c "C_C" (int \a) 1 nil 'junk :garbage 42.42})
 (str/escape "" {})
 (try (str/escape nil {\a "A_A"}) :no-throw (catch Throwable e :threw))
 (try (str/escape 1 {\a "A_A"}) :no-throw (catch Throwable e :threw))]
;; expect: ["a&lt;b" "A_AbC_C" "" :threw :threw]
