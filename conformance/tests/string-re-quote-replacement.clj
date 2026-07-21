;; clojure.string/re-quote-replacement (core/string.cljg): escapes a
;; replacement so replace/replace-first treat it literally. The escaped
;; VALUE is host-specific (cljgo rides Go regexp expansion where only $ is
;; special; the JVM's Matcher.quoteReplacement backslash-escapes \ and $) —
;; what is frozen here is the CONTRACT, whose result is oracle-identical.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (replace "day money day" #"(d)ay" (re-quote-replacement "$1x"))
;;     => "$1x money $1x"
;;   (replace-first "day money day" #"(d)ay" (re-quote-replacement "$1x"))
;;     => "$1x money day"
(require '[clojure.string :as str])
[(str/replace "day money day" #"(d)ay" (str/re-quote-replacement "$1x"))
 (str/replace-first "day money day" #"(d)ay" (str/re-quote-replacement "$1x"))]
;; expect: ["$1x money $1x" "$1x money day"]
