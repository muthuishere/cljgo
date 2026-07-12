# ADR 0014 — Error handling: Result/Option as primitives (Rust/Elm-style), exceptions stay Clojure
Date: 2026-07-12 · Status: proposed (owner-directed; design via OpenSpec, target M3 with interop)

## Context
Clojure's error story is JVM exceptions (try/catch/ex-info) — kept 100% for
fidelity. Go's is errors-as-values — already shaped as [v err] at the interop
boundary (ADR 0005). Rust and Elm proved a third thing: making Result/Option
PRIMITIVE, with composition operators (`?`, and_then, with-default) and
no-silent-failure discipline, produces dramatically more reliable code than
either exceptions or bare tuples. Owner mandate: that model must be primitive
in cljgo, not a library afterthought.

## Decision
1. **Clojure exceptions unchanged** (try/catch/finally/throw/ex-info/ex-data
   land in eval v3 as designed). Fidelity governs; nothing breaks.
2. **Result and Option are core primitives**: `(ok v)` / `(err e)` and
   `(some v)` / `none` — proper tagged values (not naked nil/vectors), with
   core predicates & combinators: result?/ok?/err?, unwrap (throws on err —
   bridges to exception world), unwrap-or, map-ok/map-err, and-then.
3. **`?`-ergonomics, railway style**: a `let?` binding form (name settled in
   design) that short-circuits: any binding evaluating to (err e)/none makes
   the whole form return it immediately — Rust's `?` operator as a Clojure
   binding macro. Works seamlessly with interop: a `(:result)` call variant
   (or auto-lift rule, settled in design) lifts Go's (T, error) → Result, so
   Go calls compose railway-style end-to-end.
4. **Elm discipline where it pays**: core APIs that can fail return Result/
   Option in their `-r` variants; analyzer warning (opt-in strictness flag)
   for discarding a Result unchecked — Elm's "no unhandled failure" as a
   lint, never a hard break with Clojure idiom.
5. Three interop layers, one story: raw [v err] (ADR 0005, unchanged) →
   Result lift (this ADR) → `!` throw (ADR 0005). Pick per call site.

## Consequences
cljgo gets Rust-grade error composition on Go's errors-are-values substrate
without touching Clojure fidelity — exceptions, tuples, and Result coexist
with explicit bridges (unwrap / ex->err). Result/Option representation must
be cheap (spike: tagged struct vs 2-elem vector vs keyword-tagged — perf
budget applies, ADR 0004) and must print/read cleanly. Design round settles:
names (let?/try!/…), auto-lift vs explicit lift at interop, emitted-Go shape,
and match/exhaustiveness support (relates to ADR 0009 comptime for
compile-time checks).
