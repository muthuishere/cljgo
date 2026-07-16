;; assert (ADR 0022 batch/harness-misc): truthy expr => nil; falsy expr
;; throws with "Assert failed: <pr-str form>" (2-arity prepends the message).
;; cljgo throws an ex-info where the JVM news an AssertionError (no JVM class
;; hierarchy, design/05) — message text and catchability by Throwable match.
;; oracle (clojure 1.12.5):
;;   (assert true) => nil
;;   (try (assert false) (catch Throwable e (ex-message e)))
;;     => "Assert failed: false"
;;   (try (assert false "msg") (catch Throwable e (ex-message e)))
;;     => "Assert failed: msg\nfalse"
[(assert true)
 (try (assert false) (catch Throwable e (ex-message e)))
 (try (assert false "msg") (catch Throwable e (ex-message e)))
 (try (assert (= 1 2)) (catch Throwable e (ex-message e)))]
;; expect: [nil "Assert failed: false" "Assert failed: msg\nfalse" "Assert failed: (= 1 2)"]
