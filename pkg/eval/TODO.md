# pkg/eval TODO

- Char args in arithmetic coerce numerically ((- 23 56 \\)) => -74); JVM Clojure throws ClassCastException. Align in eval v3 + conformance file. (Found by REPL-robustness fix, 2026-07-12.)
