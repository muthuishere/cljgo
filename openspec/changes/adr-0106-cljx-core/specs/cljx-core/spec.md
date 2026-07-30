## ADDED Requirements

### Requirement: cljx.core provides opt-in ergonomic aliases that never shadow clojure.core

`cljx.core` MUST provide `add!`, `del!`, `bump!`, `upd!`, `put-in!`, `upd-in!`,
`clear!`, `toggle!` and `dbg` as a lazy, opt-in, pure-Clojure namespace with no
Go shim and no dependency. Every helper MUST be exactly equivalent to the
`swap!`/`reset!` form it replaces, and its docstring MUST state that form.
No name in `cljx.core` may shadow any name in `clojure.core` — in particular
the transient family `assoc!` `conj!` `dissoc!` `disj!` `pop!` is off-limits —
and names MUST avoid colliding with `clojure.core.async` (`put!`, `take!`).
Behaviour MUST be identical interpreted and compiled.

#### Scenario: add! is exactly swap!+conj

- GIVEN an atom holding a vector
- WHEN `(add! a x)` is called
- THEN the atom holds the same value as if `(swap! a conj x)` had been called

#### Scenario: bump! counts by key from absent

- GIVEN an atom holding an empty map
- WHEN `(bump! a "harsh")` is called twice and `(bump! a "shaama")` once
- THEN the atom holds `{"harsh" 2, "shaama" 1}`

### Requirement: dbg prints and returns its value

`(dbg x)` MUST print a labelled representation of `x` and RETURN `x` unchanged,
so it can be inserted anywhere in a threading pipeline without altering the
result. `(dbg label x)` MUST use the caller's label.

#### Scenario: dbg is transparent in a pipeline

- GIVEN a `->>` pipeline that computes a value
- WHEN `dbg` is inserted at any stage
- THEN the pipeline's result is unchanged and the intermediate value is printed
