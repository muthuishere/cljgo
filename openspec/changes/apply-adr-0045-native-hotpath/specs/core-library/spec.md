## ADDED Requirements

### Requirement: hot core fns execute natively in both modes
`clojure.core/reduce`, `map`, `filter`, `mapv`, and `comp` SHALL be
implemented as native Go builtins interned before `core.clj` loads, serving
the REPL and emitted binaries through the same vars — never as interpreted
`core.clj` closures. Their behavior SHALL be byte-identical to JVM Clojure
1.12.5 for every arity, including the transducer forms of `map`/`filter`
and the `reduced` short-circuit contract of `reduce`.

#### Scenario: reduce honors the reduced box natively
- **WHEN** `(reduce (fn [a x] (if (> a 20) (reduced :big) (+ a x))) 0 (range 100))`
  runs interpreted and AOT-compiled
- **THEN** both print `:big`, matching the JVM oracle

#### Scenario: transducer arities compose through native comp
- **WHEN** `(into [] (comp (map inc) (filter even?)) (range 10))` runs on
  both harnesses
- **THEN** both produce `[2 4 6 8 10]`, matching the JVM oracle

#### Scenario: laziness is preserved on infinite inputs
- **WHEN** `(take 3 (map inc (range)))` and `(take 2 (filter even? (range)))`
  run on both harnesses
- **THEN** they terminate and match the JVM oracle

#### Scenario: no core.clj shadowing
- **WHEN** the evaluator boots
- **THEN** the vars `reduce`/`map`/`filter`/`mapv`/`comp` hold the native
  fns after `loadCore` completes (no later `defn` rebinds them)
