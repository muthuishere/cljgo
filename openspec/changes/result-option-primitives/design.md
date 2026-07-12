## Context

ADR 0014 makes Result/Option primitive while exceptions stay Clojure. Go
interop already shapes multi-returns as `[v err]` (ADR 0005, design/05 §2);
this change adds the middle layer. ADR 0014 left four things to this round:
representation (perf-budgeted per ADR 0004), names (`let?` etc.), auto-lift
vs explicit at interop, and printing/reading.

## Goals / Non-Goals

**Goals:**
- Settle representation (via spike), names, lift policy, print/read syntax.
- Railway composition over Go calls with zero per-call ceremony beyond one
  keyword.
- Identical semantics interpreted and compiled (design/03 §7d).

**Non-Goals:**
- Exception-system changes; `[v err]`/`!` changes; match/exhaustiveness;
  core-API `-r` sweep; hard lint failures.

## Decisions

### D1 — Representation: tagged Go structs, ratified by spike (proposed)
Hypothesis: small pkg/lang structs — `Result{ok bool; val any; e any}` and
`Option{has bool; val any}` — with `None` and `Ok(nil)`-style singletons
pre-allocated. Alternatives: 2-elem vector (allocates a persistent vector +
loses type identity — `result?` becomes structural guessing), keyword-tagged
map (worst allocation, best REPL introspection). **Spike task S1 benchmarks
all three** (construct / predicate / unwrap / let? chain of 5) against the
ADR 0004 budget (non-allocating fast path expected for construct+test);
representation freezes only after the spike lands numbers in the change.
Emitted-Go shape: the same struct — value identity across modes, no boxing
change to the ADR 0004 calling convention.

### D2 — Names (settled; owner ratifies the one deviation from ADR 0014)
- Result: `ok`, `err`, `result?`, `ok?`, `err?`, `unwrap`, `unwrap-or`,
  `map-ok`, `map-err`, `and-then` — as in ADR 0014.
- Option: ADR 0014's working names `(some v)`/`none` **collide with
  clojure.core/some (the pred-scanning fn) and some? (non-nil pred)** —
  shadowing them breaks fidelity priority 3 and real ported code. Settled:
  constructors **`just`** / **`none`** (Elm lineage), predicates `option?`,
  `just?`, `none?`; `unwrap`/`unwrap-or`/`and-then` are polymorphic over both
  types. Deviation from the ADR's sketch is flagged for owner sign-off in
  review; ADR 0014 already delegated naming to this design round.
- Binding form: **`let?`** as in ADR 0014. `try!` (expression-position
  early-return) is NOT included — without a function-level return convention
  it can't be a macro; revisit with match support.

### D3 — Interop lift: explicit per call site, no auto-lift (settled)
`(net.http/Get url :result)` — a call-variant keyword exactly parallel to
ADR 0005's `!`, producing `(ok v)` / `(err e)` instead of `[v err]` / throw.
Auto-lift (all Go calls return Result by default) rejected: it changes the
meaning of every existing interop call, breaks ADR 0005's documented default,
and hides an allocation. Three layers, picked per call site, exactly as
ADR 0014 §5 states. Consumers touched by this cross-package contract:
interop registry/reflect path (interpreted), pkg/emit interop emission (AOT),
design/05 §2 documentation.

### D4 — Printing/reading (settled; verify printer conventions against real `clojure` CLI before freezing)
Tagged literals, namespaced with the implementation tag: `#cljgo/ok 5`,
`#cljgo/err {:code 7}`, `#cljgo/just 5`, `#cljgo/none nil` (tagged literal
grammar requires a following form; `nil` by convention, ignored). Reader
support built into both modes; `pr-str`/`read-string` round-trips. Rejected:
`(ok 5)` print form (reads back as a call, not a value — violates print-read
fidelity), bare `#ok` (unnamespaced tags are reserved for users per Clojure
convention).

### D5 — `let?` semantics (settled)
`(let? [a (f) b (g a)] body)` — evaluates bindings left to right; if any
binding value satisfies `err?` or `none?`, the whole form returns THAT value
immediately (not nil); otherwise Result/Option bindings bind the unwrapped
value and plain values bind unchanged. Destructuring supported after unwrap.
Implemented as a core macro over the primitives — no analyzer special form,
so both modes get it identically for free.

### D6 — Discard lint (settled)
Opt-in flag (`--warn-unused-result`): analyzer warns when an expression whose
static head is a known Result-returning form (`ok`, `err`, `:result` calls,
`map-ok`, `and-then`) appears in statement position discarded. Warning, never
error; wired as a structured diagnostic (W-band) per ADR 0015.

## Risks / Trade-offs

- [Struct representation leaks Go identity semantics into `=`] → define `=`/hash for the tagged types in pkg/lang value tables (design/00 §4.2) before exposure.
- [`just` deviates from ADR sketch] → flagged for owner; documented in ADR 0014 status note at archive time.
- [let? on plain nil-returning fns surprises] → docs: nil is NOT none; only tagged values short-circuit.

## Open Questions

- Spike S1 outcome could overturn D1's struct hypothesis (then D4/D5 stand unchanged; only pkg/lang internals move).

> RATIFIED (owner, 2026-07-12): constructors are `just`/`none` — clojure.core/some is untouchable per the precedence principle in CLAUDE.md.
