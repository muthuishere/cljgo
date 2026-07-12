## ADDED Requirements

### Requirement: Result and Option are core tagged values
The system SHALL provide core constructors `(ok v)`, `(err e)`, `(just v)`,
and `none` producing proper tagged values (not naked nil or vectors), with
predicates `result?`, `ok?`, `err?`, `option?`, `just?`, `none?` that
distinguish them from all other values, identically in interpreted and
compiled modes.

#### Scenario: tagged values are not plain data
- **WHEN** `(ok [1 2])` is evaluated
- **THEN** `(result? (ok [1 2]))` is true, `(vector? (ok [1 2]))` is false, and `(ok? (err 1))` is false

#### Scenario: nil payloads stay distinguishable
- **WHEN** `(just nil)` and `none` are compared
- **THEN** `(just? (just nil))` is true and `(none? (just nil))` is false

### Requirement: Combinators compose and unwrap bridges to exceptions
The system SHALL provide `unwrap` (returns the payload of ok/just; THROWS an
ex-info carrying the err payload for err/none), `unwrap-or` (payload or
default), `map-ok`, `map-err`, and `and-then` with railway semantics
(`and-then` passes ok/just payloads to the next fn, short-circuits err/none
unchanged).

#### Scenario: unwrap throws on err
- **WHEN** `(unwrap (err {:code 7}))` is evaluated
- **THEN** an exception is thrown whose ex-data contains the err payload `{:code 7}`

#### Scenario: and-then short-circuits
- **WHEN** `(and-then (err :boom) (fn [x] (ok (inc x))))` is evaluated
- **THEN** the result is `(err :boom)` and the fn is never called

### Requirement: let? short-circuits on err and none
The system SHALL provide a `let?` binding form in which bindings evaluate
left to right; a binding value satisfying err? or none? makes the whole form
return that value immediately, and ok/just binding values bind their
unwrapped payload.

#### Scenario: failure exits early with the failing value
- **WHEN** `(let? [a (ok 1) b (err :nope) c (ok (inc a))] c)` is evaluated
- **THEN** the form returns `(err :nope)` and the binding for `c` is never evaluated

#### Scenario: success binds unwrapped payloads
- **WHEN** `(let? [a (ok 1) b (just 2)] (+ a b))` is evaluated
- **THEN** the result is `3`

### Requirement: Go interop lifts (T, error) to Result on request
The system SHALL provide a per-call-site `:result` call variant that lifts a
Go `(T, error)` return into `(ok T)` when error is nil and `(err e)`
otherwise, coexisting unchanged with the raw `[v err]` default and the `!`
throwing variant of ADR 0005, in both execution modes.

#### Scenario: lifted call composes railway-style
- **WHEN** a Go function returning `(value, error)` is called with the `:result` variant inside `let?`
- **THEN** a nil Go error binds the value and a non-nil Go error short-circuits the `let?` with `(err e)` wrapping the Go error value

### Requirement: tagged values print and read round-trip
The system SHALL print Result/Option values as the tagged literals
`#cljgo/ok <v>`, `#cljgo/err <e>`, `#cljgo/just <v>`, `#cljgo/none nil`, and
the reader SHALL read those literals back to equal values in both modes.

#### Scenario: pr-str round-trip
- **WHEN** `(read-string (pr-str (ok {:a 1})))` is evaluated
- **THEN** the result equals `(ok {:a 1})`

### Requirement: unchecked Result discards warn under opt-in strictness
The system SHALL, only when the opt-in strictness flag is enabled, emit a
warning-severity diagnostic when a statically known Result-producing
expression is discarded in statement position; the program SHALL still
compile and run.

#### Scenario: lint warns without breaking
- **WHEN** `(do (ok 1) 2)` is compiled with the strictness flag enabled
- **THEN** a warning diagnostic is emitted for the discarded Result and evaluation still yields `2`
