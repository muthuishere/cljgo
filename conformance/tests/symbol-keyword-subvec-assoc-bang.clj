;; symbol/keyword/subvec/assoc! (batch/error-files), four independent fixes:
;;  - (symbol kw) / (symbol a-var) now construct from a Keyword/Var, not
;;    just a string or existing Symbol.
;;  - (namespace (symbol "" "hi")) now returns "" (an explicitly empty ns is
;;    distinct from no ns at all — a pre-existing `hasNs` flag was being
;;    ignored by an overly narrow `!= ""` guard).
;;  - (keyword nil) => nil (previously an unhandled-type panic).
;;  - subvec truncates a Ratio/float start/end the way Java's int cast
;;    does (toward zero), instead of requiring a literal integer.
;;  - assoc! tolerates a dangling odd trailing key (assoc'd with nil),
;;    unlike assoc.
;; oracle (clojure 1.12.5): [(symbol :abc) (symbol (var +))
;; (namespace (symbol "" "hi")) (keyword nil)] => [abc clojure.core/+ "" nil];
;; (subvec [0 1 2] 1/2 4/3) => [0];
;; (persistent! (apply assoc! (transient [1]) [0 1 1])) => [1 nil].
[[(symbol :abc) (symbol (var +)) (namespace (symbol "" "hi")) (keyword nil)]
 (subvec [0 1 2] 1/2 4/3)
 (persistent! (apply assoc! (transient [1]) [0 1 1]))]
;; expect: [[abc clojure.core/+ "" nil] [0] [1 nil]]
