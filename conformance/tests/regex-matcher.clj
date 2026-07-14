;; re-matcher + re-find/1 + re-groups: a stateful matcher walks successive
;; matches; re-groups reads the groups of the last match. Mirrors
;; java.util.regex.Matcher (what JVM clojure.core wraps).
;; harness: eval — pkg/emit has no reader.Regex constant emission yet, so
;;   #"..." literals can't compile; the regex fns are eval-verified here.
;; oracle (Clojure 1.12.5, `clojure -M`, 2026-07-15):
;;   (let [m (re-matcher #"\d+" "a1b22")] [(re-find m) (re-find m) (re-find m)])
;;     => ["1" "22" nil]
;;   (let [m (re-matcher #"(\d+)-(\d+)" "12-34")] (re-find m) (re-groups m))
;;     => ["12-34" "12" "34"]
[(let [m (re-matcher #"\d+" "a1b22")]
   [(re-find m) (re-find m) (re-find m)])
 (let [m (re-matcher #"(\d+)-(\d+)" "12-34")]
   (re-find m)
   (re-groups m))]
;; expect: [["1" "22" nil] ["12-34" "12" "34"]]
