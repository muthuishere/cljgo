;; Precedence principle (issue #171, CLAUDE.md, ADR 0115): clojure.core
;; must not carry cljgo-only names the JVM lacks -- ok/err (ADR 0014
;; Result/Option constructors) used to live in clojure.core, so LEGAL
;; user code defining its own ok/err collided with a var the JVM does not
;; have. Both now resolve to nil, exactly like the JVM (they live in
;; cljx.meta instead, required and referred deliberately -- see
;; conformance/tests/result-*.clj).
;; oracle (clojure 1.12.5, 2026-07-31): [nil nil]
[(resolve (quote clojure.core/ok)) (resolve (quote clojure.core/err))]
;; expect: [nil nil]
