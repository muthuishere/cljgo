# ADR 0007 — JVM Clojure is the semantic oracle; Go paths are the enforced goldens
Date: 2026-07-12 · Status: accepted

## Context
"Faithful Clojure" needs a ground truth; users must not need Java.

## Decision
Three layers: (1) real JVM Clojure 1.12.5 verifies every expectation at
AUTHORING time only (58 syntax-quote goldens; every conformance ;; expect:
cited); (2) the evaluator runs all conformance files on every test run;
(3) from M2 the emitted binary runs the SAME files — byte-identical output
required. An ORACLE=1 mode re-audits all frozen expectations against the
clojure CLI on demand (weekly/pre-release), keeping Java out of the default
loop entirely.

## Consequences
Java is an authoring/audit tool, never a runtime or CI-default dependency;
divergence between our two Go paths is a release blocker.
