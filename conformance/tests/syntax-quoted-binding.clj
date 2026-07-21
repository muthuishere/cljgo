;; A macro that syntax-quotes `binding` (regression, fundamentals batch 4).
;;
;; Syntax-quote namespace-qualifies its symbols, so ``(binding …)` reads as
;; `clojure.core/binding`. On the JVM that resolves to the clojure.core MACRO
;; and works. cljgo implements binding as a SPECIAL FORM, and specials were
;; matched on bare names only — so the qualified symbol fell through to the
;; invoke path and threw "binding: cannot call as a function (special form)".
;; Every user macro expanding to (binding …) was broken; clojure.test's own
;; with-test-out is what tripped over it.
;;
;; Only `binding` is treated this way: the rest of cljgo's specials (if, do,
;; let*, fn*, try, …) are special forms in Clojure too, where
;; `clojure.core/if` is equally unresolvable (verified, clojure 1.12.5).
;; oracle (clojure 1.12.5, 2026-07-21): [(with-flag *flag*) *flag*
;; (clojure.core/binding [*flag* :qualified] *flag*)] => [:bound :root
;; :qualified]
(def ^:dynamic *flag* :root)
(defmacro with-flag [& body] `(binding [*flag* :bound] ~@body))
[(with-flag *flag*)
 *flag*
 (clojure.core/binding [*flag* :qualified] *flag*)]
;; expect: [:bound :root :qualified]
