;; slurp/spit (fundamentals audit 2026-07): file path in, string out. spit
;; coerces content via str (numbers, keywords, maps, nil => ""), truncates
;; by default, appends under :append true; UTF-8 in and out; a missing
;; file throws (message names the file). The scratch file is a RELATIVE
;; path: every harness (eval, compiled binary, JVM oracle) runs with cwd =
;; the conformance/ package dir, and the first spit truncates, so reruns
;; are deterministic; conformance/.gitignore covers the leftover.
;; oracle (clojure 1.12.5, 2026-07-21 — same sequence via clojure -M):
;;   ["hello\n" "42" "abc" ":kw" "" "unicode-héllo-世界" true "enc" nil "x" "{:a 1}"]
(def p "slurp-spit-scratch.tmp")
(spit p "hello\n")
(def r1 (slurp p))
(spit p 42)
(def r2 (slurp p))
(spit p "a")
(spit p "b" :append true)
(spit p "c" :append true)
(def r3 (slurp p))
(spit p :kw)
(def r4 (slurp p))
(spit p nil)
(def r5 (slurp p))
(spit p "unicode-héllo-世界")
(def r6 (slurp p))
(def missing-throws (try (slurp "no-such-file-xyz.txt") (catch Exception e (some? (ex-message e)))))
(spit p "enc" :encoding "UTF-8")
(def r7 (slurp p :encoding "UTF-8"))
(def spit-ret (spit p "x" :append false))
(def r8 (slurp p))
(spit p {:a 1})
(def r9 (slurp p))
[r1 r2 r3 r4 r5 r6 missing-throws r7 spit-ret r8 r9]
;; expect: ["hello\n" "42" "abc" ":kw" "" "unicode-héllo-世界" true "enc" nil "x" "{:a 1}"]
