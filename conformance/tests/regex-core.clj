;; clojure.core regex fns backed by Go's regexp (RE2): re-find returns the
;; whole-match string when the pattern has no groups, else [whole g1 g2 …];
;; re-matches must match the ENTIRE string (nil otherwise); re-seq is a seq
;; of successive matches; re-pattern builds a #"..." value.
;; harness: eval — pkg/emit has no reader.Regex constant emission yet, so
;;   #"..." literals can't compile; the regex fns are eval-verified here.
;; oracle (Clojure 1.12.5, `clojure -M`, 2026-07-15):
;;   (re-find #"(\d+)-(\d+)" "12-34") => ["12-34" "12" "34"]
;;   (re-find #"\d+" "ab12")          => "12"
;;   (re-matches #"\d+" "ab12")       => nil
;;   (re-matches #"(\d+)-(\d+)" "12-34") => ["12-34" "12" "34"]
;;   (re-seq #"\d+" "a1b22c333")      => ("1" "22" "333")
;;   (re-pattern "\\d+")              => #"\d+"
[(re-find #"(\d+)-(\d+)" "12-34")
 (re-find #"\d+" "ab12")
 (re-matches #"\d+" "ab12")
 (re-matches #"(\d+)-(\d+)" "12-34")
 (re-seq #"\d+" "a1b22c333")
 (re-pattern "\\d+")]
;; expect: [["12-34" "12" "34"] "12" nil ["12-34" "12" "34"] ("1" "22" "333") #"\d+"]
