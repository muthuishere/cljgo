;; clojure.string predicates + search, plus clojure.core/subs. index-of /
;; last-index-of return nil when not found; blank? is true for nil, "", and
;; all-whitespace strings.
;; oracle (Clojure 1.12.5, `clojure -M`, 2026-07-15):
;;   (clojure.string/blank? "  ")             => true
;;   (clojure.string/blank? nil)              => true
;;   (clojure.string/blank? "x")              => false
;;   (clojure.string/starts-with? "hello" "he") => true
;;   (clojure.string/ends-with? "hello" "lo")   => true
;;   (clojure.string/includes? "hello" "ell")   => true
;;   (clojure.string/index-of "hello" "l")      => 2
;;   (clojure.string/index-of "hello" "z")      => nil
;;   (clojure.string/last-index-of "hello" "l") => 3
;;   (subs "hello" 1 3)                          => "el"
(require 'clojure.string)
[(clojure.string/blank? "  ")
 (clojure.string/blank? nil)
 (clojure.string/blank? "x")
 (clojure.string/starts-with? "hello" "he")
 (clojure.string/ends-with? "hello" "lo")
 (clojure.string/includes? "hello" "ell")
 (clojure.string/index-of "hello" "l")
 (clojure.string/index-of "hello" "z")
 (clojure.string/last-index-of "hello" "l")
 (subs "hello" 1 3)]
;; expect: [true true false true true true 2 nil 3 "el"]
