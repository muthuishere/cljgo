;; clojure.string split / split-lines / replace / replace-first. split uses
;; Java Pattern.split semantics (trailing empties dropped when no limit; a
;; positive limit caps the piece count). replace/replace-first accept
;; string, char, or #"..." (with $1 group refs) match forms.
;; harness: eval — these forms use #"..." regex-literal separators/matches,
;;   which pkg/emit cannot emit as constants yet; eval-verified here. (The
;;   non-regex clojure.string fns run dual in string-basics/string-search,
;;   proving the clojure.string namespace loads AOT.)
;; oracle (Clojure 1.12.5, `clojure -M`, 2026-07-15):
;;   (clojure.string/split "a,b,c" #",")        => ["a" "b" "c"]
;;   (clojure.string/split "a,b,c," #",")        => ["a" "b" "c"]
;;   (clojure.string/split "a,b,c" #"," 2)       => ["a" "b,c"]
;;   (clojure.string/split-lines "a\nb\r\nc")    => ["a" "b" "c"]
;;   (clojure.string/replace "hello" "l" "L")    => "heLLo"
;;   (clojure.string/replace "hello" \l \L)      => "heLLo"
;;   (clojure.string/replace "12-34" #"(\d+)" "<$1>") => "<12>-<34>"
;;   (clojure.string/replace-first "hello" "l" "L")   => "heLlo"
;;   (clojure.string/replace-first "12-34" #"\d+" "X") => "X-34"
(require 'clojure.string)
[(clojure.string/split "a,b,c" #",")
 (clojure.string/split "a,b,c," #",")
 (clojure.string/split "a,b,c" #"," 2)
 (clojure.string/split-lines "a\nb\r\nc")
 (clojure.string/replace "hello" "l" "L")
 (clojure.string/replace "hello" \l \L)
 (clojure.string/replace "12-34" #"(\d+)" "<$1>")
 (clojure.string/replace-first "hello" "l" "L")
 (clojure.string/replace-first "12-34" #"\d+" "X")]
;; expect: [["a" "b" "c"] ["a" "b" "c"] ["a" "b,c"] ["a" "b" "c"] "heLLo" "heLLo" "<12>-<34>" "heLlo" "X-34"]
