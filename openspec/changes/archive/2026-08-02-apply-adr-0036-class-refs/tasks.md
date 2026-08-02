# Tasks — apply-adr-0036-class-refs

## 1. Class refs

- [x] 1.1 `pkg/eval/class_refs.go`: ClassRef + canonical name table
  (java.lang/* simple+qualified; java.math, java.util.UUID and
  clojure.lang/* qualified-only) + interning + `classRefVar`; extract
  `classNameMatchesValue` from `-instance-of-name?`.
- [x] 1.2 `resolveVar` last-resort fallback (eval.go); `class?` builtin
  (misc_builtins.go); `-instance?` ClassRef branch (protocols.go).

## 2. Hierarchies

- [x] 2.1 `core/hierarchies.cljg`: JVM-faithful derive asserts (class
  tags ok, global parent must be namespaced Named, 3-arity tag
  `(or class? named)` / parent named); descendants throws on class tags
  (both arities). Oracle citations updated in-file.

## 3. Conformance + verification

- [x] 3.1 `conformance/tests/class-refs-hierarchy.clj` (harness: eval —
  AOT class-ref emission deferred; oracle: skip — (parents String)
  divergence documented, per-form facts cited from the 1.12.5 run).
- [x] 3.2 `conformance/tests/reader-conditionals.clj`: no-match
  SELECTING elision case (`[1 #?(:cljs 2) 3]` ⇒ `[1 3]`),
  oracle-verified.
- [x] 3.3 Gates green; suite 217 → 219 with no regressions.
