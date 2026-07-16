# ADR 0026 — `instance?`: the class position is syntax, not a value

Date: 2026-07-16 · Status: accepted

## Context
The jank clojure-test-suite (ADR 0022) calls `instance?` with bare class
symbols cljgo cannot represent as first-class values: `clojure.lang.Atom`,
`java.util.UUID`, `String`, `clojure.lang.BigInt`, etc. cljgo has no
`java.lang.Class` object and no JVM class hierarchy (design/05: host interop
is Go-struct-based, ADR 0010), so evaluating `clojure.lang.Atom` as an
ordinary symbol reference has nothing to resolve to and would throw
"unresolve symbol" before `instance?` ever ran.

cljgo already has a working precedent for exactly this problem: a `catch`
clause's class symbol (`(catch SomeClass e ...)`) is never resolved as a
var — `pkg/analyzer` captures its NAME as a string onto `ast.CatchNode`, and
`CatchMatches` (`pkg/eval/ex_builtins.go`) matches by that string
(`"Throwable"`, `"ExceptionInfo"`, …) at catch time. `instance?` adopts the
same model.

## Decision
1. **`instance?` is a macro (`core/core.clj`), not a plain function.** When
   its first argument is a literal symbol, the macro captures the symbol's
   full printed name (`(str c)`, e.g. `"clojure.lang.Atom"`) as a STRING at
   macroexpansion time and never evaluates it — expanding to
   `(clojure.core/-instance-of-name? "clojure.lang.Atom" x)`. A non-symbol
   first argument (already a value in hand — most commonly a deftype/
   defrecord type-name var, which legitimately evaluates to a `*TypeMarker*`)
   is evaluated normally and checked via the existing `-instance?` builtin
   (`pkg/eval/protocols.go`), unchanged.
2. **`-instance-of-name?`** (`pkg/eval/misc_builtins.go`) resolves the name
   two ways, in order:
   - if the name resolves to a var bound to a `*TypeMarker*` (a user
     deftype/defrecord type), compare `dispatchKey(v)` to the marker's name
     — the same identity check `-instance?`/`satisfies?` already use;
   - else take the name's LAST dotted segment (`clojure.lang.Atom` →
     `Atom`, `java.util.UUID` → `UUID`) and match it against a fixed table
     covering both `dispatchKey`'s existing designators (`String`, `Long`,
     `Double`, `Boolean`, `Character`, `Keyword`, `Symbol`, the persistent
     collection designators, `IFn`, `ISeq`) and cljgo's host wrapper types
     that have no clojure.core designator of their own (`Atom`, `Delay`,
     `Var`, `Namespace`, `BigInt`, `BigDecimal`, `UUID`). An unrecognized
     name falls back to comparing the segment directly against
     `dispatchKey(v)` (covers a bare deftype-style name typed without going
     through a var, and fails closed — `false` — for anything genuinely
     unknown, never a crash).

## Consequences
- **Deviation, documented:** because the class position is syntax, a
  literal class symbol only works in DIRECT call position —
  `(instance? String x)` works; `(partial instance? String)` or
  `(map (partial instance? Long) xs)` do not, because `String`/`Long` are
  not values cljgo can hand around (there is nothing for them to evaluate
  to). This mirrors `catch`'s existing class-symbol limitation exactly, so
  it is consistent within cljgo, not a new kind of gap. The one suite file
  observed to lean on this (`atom.cljc`, `(every? (partial instance? …) …)`)
  still fails on that expression; its direct-call-form assertions
  (`(instance? clojure.lang.Atom the-atom)`) pass.
- **`instance?` cannot be `resolve`d/`var`'d as a function value** either,
  since it is a macro — consistent with `and`/`or`/`when`/every other
  cljgo control-flow macro, and irrelevant to the suite's usage (always
  direct calls except the one case above).
- A later `class`/host-interop layer (if cljgo ever grows real Class-like
  values, e.g. as part of a fuller ADR 0010 story) could replace the
  syntax-matching with genuine value resolution without changing
  `instance?`'s call-site behavior for the common case — only the
  `(partial instance? X)` composition would start working.
