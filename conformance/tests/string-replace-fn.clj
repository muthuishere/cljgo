;; clojure.string/replace + replace-first with a FUNCTION replacement
;; (ADR 0110 ask 3 — cljgo used to throw "replace expects a string").
;; The JVM dispatches on the replacement: a CharSequence is a $-group
;; template, anything else is called per match (clojure.string/replace-by).
;; What the fn receives is re-groups of the match — the whole matched string
;; when the pattern has NO capturing groups, else [whole g1 g2 …] with nil for
;; a group that did not participate. The fn's result is spliced in LITERALLY
;; (Matcher/quoteReplacement), so a returned "$1" is the characters $1.
;; oracle (Clojure 1.12.5, `clojure -M`, 2026-07-30):
;;   (str/replace "a1" #"\d" (fn [m] "X"))                  => "aX"
;;   (str/replace "a1b2" #"\d" #(str "<" % ">"))            => "a<1>b<2>"
;;   (str/replace "a1b2" #"([a-z])(\d)" #(pr-str %))
;;     => "[\"a1\" \"a\" \"1\"][\"b2\" \"b\" \"2\"]"
;;   (str/replace "a1" #"((a)(1))" #(pr-str %))  => "[\"a1\" \"a1\" \"a\" \"1\"]"
;;   (str/replace "aXb" #"(x)?b" #(pr-str %))    => "aX[\"b\" nil]"
;;   (str/replace "abc" #"\d" (fn [m] "X"))      => "abc"      ; no match
;;   (str/replace "a1" #"(\d)" (fn [m] "$1"))    => "a$1"      ; literal
;;   (str/replace "aaa" #"" (fn [m] "-"))        => "-a-a-a-"  ; zero-width
;;   (str/replace-first "a1b2" #"\d" #(str "<" % ">"))       => "a<1>b2"
;;   (str/replace-first "a1b2" #"([a-z])(\d)" #(pr-str %))   => "[\"a1\" \"a\" \"1\"]b2"
;;   (str/replace-first "abc" #"\d" (fn [m] "X"))            => "abc"
;;   (str/replace "a1b2" #"\d" "X")              => "aXbX"    ; string still works
;;   (str/replace "a1b2" #"(\d)" "[$1]")         => "a[1]b[2]"
;;   (str/replace "a1" #"\d" (fn [m] 42))        => throws (ClassCastException
;;     Long -> String on the JVM; cljgo raises the named G5010 instead)
(require '[clojure.string :as str])
[(str/replace "a1" #"\d" (fn [m] "X"))
 (str/replace "a1b2" #"\d" (fn [m] (str "<" m ">")))
 (str/replace "a1b2" #"([a-z])(\d)" (fn [m] (pr-str m)))
 (str/replace "a1" #"((a)(1))" (fn [m] (pr-str m)))
 (str/replace "aXb" #"(x)?b" (fn [m] (pr-str m)))
 (str/replace "abc" #"\d" (fn [m] "X"))
 (str/replace "a1" #"(\d)" (fn [m] "$1"))
 (str/replace "aaa" #"" (fn [m] "-"))
 (str/replace-first "a1b2" #"\d" (fn [m] (str "<" m ">")))
 (str/replace-first "a1b2" #"([a-z])(\d)" (fn [m] (pr-str m)))
 (str/replace-first "abc" #"\d" (fn [m] "X"))
 (str/replace "a1b2" #"\d" "X")
 (str/replace "a1b2" #"(\d)" "[$1]")
 (try (str/replace "a1" #"\d" (fn [m] 42)) (catch Throwable _ :threw))
 (try (str/replace-first "a1" #"\d" (fn [m] 42)) (catch Throwable _ :threw))]
;; expect: ["aX" "a<1>b<2>" "[\"a1\" \"a\" \"1\"][\"b2\" \"b\" \"2\"]" "[\"a1\" \"a1\" \"a\" \"1\"]" "aX[\"b\" nil]" "abc" "a$1" "-a-a-a-" "a<1>b2" "[\"a1\" \"a\" \"1\"]b2" "abc" "aXbX" "a[1]b[2]" :threw :threw]
