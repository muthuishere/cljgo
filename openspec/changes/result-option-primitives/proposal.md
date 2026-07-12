## Why

ADR 0014 (docs/adr/0014-result-option-primitives.md, status proposed) mandates
Rust/Elm-style Result/Option as **core primitives** — not a library
afterthought — on top of Go's errors-as-values substrate, while Clojure
exceptions stay 100% untouched. Target M3, alongside interop: Go's `(T, error)`
returns (design/05 §2, the owning contract) should lift into Results so Go
calls compose railway-style end-to-end.

## What Changes

- Core constructors and tagged values: `(ok v)` / `(err e)` for Result;
  Option constructors settled in design (ADR 0014 names `some`/`none` collide
  with clojure.core — design D2 resolves this, fidelity priority 3 governs).
- Core predicates & combinators: `result?`, `ok?`, `err?`, `unwrap` (throws
  on err — the bridge to exceptions), `unwrap-or`, `map-ok`, `map-err`,
  `and-then`.
- `let?` railway binding form: any binding evaluating to an err/none
  short-circuits the whole form to that value (Rust's `?` as a binding macro).
- Interop lift: a per-call-site variant that lifts Go `(T, error)` → Result,
  completing the three-layer story raw `[v err]` (ADR 0005, unchanged) →
  Result lift → `!` throw. Auto-lift vs explicit settled in design.
- Readable printing for all four tagged values, round-tripping through the
  reader in both modes.
- Representation perf spike (tagged struct vs 2-elem vector vs keyword-tagged)
  benchmarked against ADR 0004 budgets before the representation freezes.
- Opt-in strictness lint: analyzer warning for discarding a Result unchecked
  (Elm discipline as a lint, never a hard break).

## Non-goals

- No change to try/catch/finally/throw/ex-info/ex-data semantics (ADR 0014
  decision 1; eval v3 lands them as designed).
- No change to the raw `[v err]` interop shaping of ADR 0005 / design/00 §4.3.
- No match/exhaustiveness checking — noted as a relates-to for ADR 0009
  comptime, deferred to its own change.
- No `-r` Result-returning variants across core APIs yet — only the
  primitives, combinators, and interop lift; the core-API sweep is follow-up.
- No hard errors from the strictness lint.

## Capabilities

### New Capabilities
- `result-option`: Result/Option tagged values, predicates, combinators,
  `let?` short-circuiting, printing/reading, and the interop Result lift.

### Modified Capabilities
(none — openspec/specs/ has no existing capability this touches)

## Impact

- ADRs relied on: **0014** (implemented here), 0005 (error mapping — layered
  on, unchanged), 0004 (calling convention + perf budgets govern the
  representation spike), 0002 (dual-mode). Design authority: design/05 §2
  (the `(value, error)` problem — owning section), design/00 §4.3 (error
  mapping contract), design/02 (value model for the new tagged types).
- Code: pkg/lang (tagged types, printer/reader support), core/ (combinators,
  `let?`), pkg/analyzer (discard lint), interop layer (lift variant),
  pkg/emit (emitted-Go shape for the tagged values).
- Conformance: new conformance/tests/*.clj (cljgo extension — no JVM oracle
  for the primitives; exception-bridge behavior verified against real
  Clojure), dual-harness from M2.
