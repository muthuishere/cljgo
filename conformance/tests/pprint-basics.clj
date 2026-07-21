;; clojure.pprint (fundamentals audit 2026-07): the pretty printer's core
;; surface — pprint, print-table (both arities), write to a string, and
;; *print-right-margin* forcing one element per line. Output is
;; whitespace-exact, so these expectations are frozen byte-for-byte.
;; oracle (clojure 1.12.5, 2026-07-21): the same five forms under
;; `clojure -M -e` printed
;;   ["{:a 1, :b [1 2 3]}\n"
;;    "\n| :a | :b |\n|----+----|\n|  1 |  x |\n| 22 | yy |\n"
;;    "\n| :b | :a |\n|----+----|\n|  x |  1 |\n"
;;    "[1 2 3]"
;;    "[0\n 1\n 2\n 3\n 4\n 5\n 6\n 7\n 8\n 9\n 10\n 11]\n"]
;; Note print-table's leading newline and its `+`-jointed rule row: both
;; are upstream behavior, not a formatting slip.
(require '[clojure.pprint :as pp])
[(with-out-str (pp/pprint {:a 1 :b [1 2 3]}))
 (with-out-str (pp/print-table [{:a 1 :b "x"} {:a 22 :b "yy"}]))
 (with-out-str (pp/print-table [:b :a] [{:a 1 :b "x"}]))
 (pp/write [1 2 3] :stream nil)
 (with-out-str (binding [pp/*print-right-margin* 20] (pp/pprint (vec (range 12)))))]
;; expect: ["{:a 1, :b [1 2 3]}\n" "\n| :a | :b |\n|----+----|\n|  1 |  x |\n| 22 | yy |\n" "\n| :b | :a |\n|----+----|\n|  x |  1 |\n" "[1 2 3]" "[0\n 1\n 2\n 3\n 4\n 5\n 6\n 7\n 8\n 9\n 10\n 11]\n"]
