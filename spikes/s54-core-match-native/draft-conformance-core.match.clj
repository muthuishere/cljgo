;; clojure.core.match — SCOPED native port (spike s54, ADR 0097).
;; The `match` macro compiles pattern rows to nested test/bind expressions
;; and returns the first matching clause's action; a trailing :else (or a
;; row of wildcards) is the catch-all, otherwise a "No matching clause"
;; ex-info is thrown.
;;
;; oracle: captured from the REAL org.clojure/core.match 1.1.1 on Clojure
;; 1.12.5 via:
;;   clojure -Sdeps '{:deps {org.clojure/core.match {:mvn/version "1.1.1"}}}' \
;;           -M -e "(require '[clojure.core.match :refer [match match-let]]) ..."
;; Each ;; expect: below is that JVM run's exact (pr-str ...) output.
;; Frozen values (all 24 verified identical under cljgo in the s54 driver):
;;   lit :a · wild-else :default · bind 30 · vec-bind 6 ·
;;   vec-rest [1 [2 3 4]] · seq [1 2 3] · seq-rest [1 [2 3 4]] ·
;;   map [1 2] · map-only :no · or :in · guard :even · as [1 2 [1 2]] ·
;;   nested [1 2 3] · nomatch "ERR:No matching clause: 9" · kw :got-foo ·
;;   nil :was-nil · app :got · str :hi · multi :one-b · match-let 3 ·
;;   vec-len :two · bool :t · wild-in-vec :mid
;;
;; expect: [:a :default 30 6 [1 [2 3 4]] [1 2 3] [1 [2 3 4]] [1 2] :no :in :even [1 2 [1 2]] [1 2 3] "ERR:No matching clause: 9" :got-foo :was-nil :got :hi :one-b 3 :two :t :mid]
(require '[clojure.core.match :refer [match match-let]])

[;; 1. literal row vs wildcard row
 (match [1 2] [1 2] :a [_ _] :b)                         ;; => :a
 ;; 2. no literal row matches -> :else
 (match [5 6] [1 2] :a :else :default)                   ;; => :default
 ;; 3. binding symbol usable in the action
 (match [3] [x] (* x 10))                                ;; => 30
 ;; 4. vector pattern, positional binding
 (match [[1 2 3]] [[a b c]] (+ a b c))                   ;; => 6
 ;; 5. vector rest pattern `&`
 (match [[1 2 3 4]] [[a & r]] [a r])                     ;; => [1 [2 3 4]]
 ;; 6. seq pattern, fixed arity
 (match [(list 1 2 3)] [([a b c] :seq)] [a b c])         ;; => [1 2 3]
 ;; 7. seq pattern rest
 (match [(list 1 2 3 4)] [([a & r] :seq)] [a (vec r)])   ;; => [1 [2 3 4]]
 ;; 8. map pattern
 (match [{:a 1 :b 2}] [{:a a :b b}] [a b])               ;; => [1 2]
 ;; 9. map :only — extra key -> keyset mismatch -> :else
 (match [{:a 1 :b 2}] [({:a a} :only [:a])] a :else :no) ;; => :no
 ;; 10. :or alternatives
 (match [3] [(:or 1 2 3)] :in :else :out)                ;; => :in
 ;; 11. :guard predicate
 (match [4] [(n :guard even?)] :even :else :odd)         ;; => :even
 ;; 12. :as binding of the whole occurrence
 (match [[1 2]] [([a b] :as v)] [a b v])                 ;; => [1 2 [1 2]]
 ;; 13. nested vector patterns
 (match [[1 [2 3]]] [[a [b c]]] [a b c])                 ;; => [1 2 3]
 ;; 14. no clause + no :else -> thrown "No matching clause: <val>"
 (try (match [9] [1] :x) (catch Throwable e (str "ERR:" (ex-message e)))) ;; => "ERR:No matching clause: 9"
 ;; 15. keyword literal
 (match [:foo] [:foo] :got-foo :else :no)                ;; => :got-foo
 ;; 16. nil literal
 (match [nil] [nil] :was-nil :else :no)                  ;; => :was-nil
 ;; 17. app pattern `:<<` — (inc 4) = 5 matches literal 5
 (match [4] [(5 :<< inc)] :got :else :no)                ;; => :got
 ;; 18. string literal
 (match ["hi"] ["hi"] :hi :else :no)                     ;; => :hi
 ;; 19. multiple occurrences
 (match [1 :b] [1 :b] :one-b [_ _] :other)               ;; => :one-b
 ;; 20. match-let
 (match-let [x 1 y 2] [1 2] (+ x y) :else :no)           ;; => 3
 ;; 21. vector length discrimination
 (match [[1 2]] [[a b c]] :three [[a b]] :two :else :no) ;; => :two
 ;; 22. boolean literal
 (match [true] [true] :t [false] :f :else :no)           ;; => :t
 ;; 23. wildcard inside a vector pattern with a literal
 (match [[1 2 3]] [[_ 2 _]] :mid :else :no)]             ;; => :mid
