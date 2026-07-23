# ADR 0067 — Emitter numeric type inference (unboxed int64)

Date: 2026-07-23 · Status: **accepted** (owner-directed 2026-07-23: *"i will
take whatever makes high performance assuming its not breaking"* — the same
directive that ratified ADR 0066; the redefinition question §"Redefinition"
below is resolved by the ADR 0066 `CoreArithDirty` entry guard, so the
feature is non-breaking by construction) · Spike: s42
(`spikes/s42-emitter-type-inference/VERDICT.md`)

Complements design/04 §5 (the primitive-hints / open-coded-intrinsics rungs
3–4), ADR 0045 (native hot core fns — the same "stop boxing everything"
campaign one rung down), ADR 0064 (direct-call emission — the boxed local-fn
counterpart of this ADR's typed lift), and ADR 0066 (sealed-core dirty flag —
this ADR's redefinition-liveness mechanism).

## Context

Every emitted local is `any`. A pprof/allocation decomposition of the
~35× emitted-vs-handwritten-Go gap on `(fact 15)`×2M (design/00 §1.4;
`perf_test.go`) found the single dominant cost: the guarded arithmetic
intrinsics (`rt.Add2/Sub2/Mul2`) compute an `int64` and then **re-box it
into `any` via `runtime.convT64`** on the way back out — because the value
immediately flows into another `any` local. That reboxing was **~23% of CPU
and 100% of the 12M heap allocations** on the benchmark. Because values flow
as `any`, Go's inliner and escape analysis are blind to the arithmetic.

design/04 §5 already names the fix as the endgame ladder's rungs 3–4
(primitive signatures + open-coded intrinsics), deferred past M2. Spike s42
prototyped it; this ADR ships it.

A load-bearing fact makes this tower-safe: cljgo's pristine `+ - *` on two
int64 **throw** on overflow (`pkg/lang/numberops.go` `int64Ops.Add/Sub/
Multiply`) — they do NOT promote to BigInt (that is `+' -' *'`). So a checked
int64 result is ALWAYS an int64 or the very same `ArithmeticException`;
there is no third "the result became a BigInt" outcome that an int64 Go local
could not hold. That is exactly what lets the value stay unboxed.

## Decision

A conservative, bottom-up inference (`pkg/emit/numtype.go` `inferNumeric`)
proves a node/binding is `int64`; the emitter then emits unboxed Go inside
**dirty-guarded regions**. **When unsure it stays boxed — correctness first.**

### Inference rules (int64 only in v1)

A node is `int64` when it is:
- an integer literal;
- a reference to a binding proven `int64`;
- `(+ a b)` / `(- a b)` / `(* a b)` / `(inc a)` / `(dec a)` on `int64`
  operands, where the op is the pristine `clojure.core` var (`/` never —
  ratio lives in the tower);
- a self-recursive call, with all-int64 args, of the fn being lift-
  specialized (greatest-fixpoint: assume int64 return, validate the body).

A binding is `int64` when it is a `let`/`loop`/method-recur **carrier** whose
init AND every `recur` value are `int64` (a monotone fixpoint demoting on the
first non-int64 flow), or a specialized fn parameter. Captured carriers
(closed over by a nested fn) and variadic params stay boxed. The inference
zero value is `ntUnknown` so an untyped lookup can never masquerade as the
meet-identity (a real miscompile the spike caught regenerating core).

### Emission tiers — every one behind `if !rt.CoreDirty()`

1. **Typed loop/let carriers** → `var i int64`, checked `rt.IAdd/ISub/IMul/
   IInc/IDec` (panic-identical to the tower) and raw `< > ==` in tests.
2. **Parameter specialization** — a single-fixed-arity fn (0–4 params) whose
   body proves int64 with every param seeded int64 gets a **dual body**:
   `if !rt.CoreDirty() { if nI, ok := n.(int64); ok { …typed… } }` falling
   through to the original boxed body for non-int64 callers and redefined
   core arithmetic. The external `any → any` calling convention is
   unchanged.
3. **Rung-3 typed-func lift** — a specialized fn that is capture-free, has
   no nested fn, and whose EVERY self-reference is an int64-proven
   arity-matching call lifts to a package-level `func fnL_ns_name(n int64)
   int64` with **direct int64 recursion** (self-call typing applies only
   here; the inline tier-2 path re-infers without it so a boxed self-call
   result is never re-typed). This is what makes fact/fib recursion
   alloc-free — body specialization alone still boxes every recursive
   return across the `any` FnFunc boundary (~2× instead of ~5–9×).
4. **Dual-emitted loops** — a numeric loop met OUTSIDE any guarded region
   (top-level, or inside an unspecializable fn body) opens its own:
   `if !rt.CoreDirty() { typed loop } else { boxed loop }`.

Composition with ADR 0064: where both apply to a self-recursive fn, the
typed lift wins (`genSelfCallInt` runs before the direct-call registry);
the ADR 0064 typed handle keeps the boxed local-fn case fast. The lifted
package func never references the closure-scoped ADR 0064 handles
(`canLift` rule 2 + the registration is masked during lifted-body emission).

### Redefinition — resolved via ADR 0066 (owner-ratified)

The unboxed ops do not deref the operator var. Liveness is preserved by the
ADR 0066 sealed-core flag instead: redefining `+ - * / < > =` (with-redefs,
alter-var-root, def) trips `lang.CoreArithDirty`; every typed region checks
it at entry (one relaxed atomic.Bool load — the cost ADR 0066 measured as
near-free, and re-measured here: ≤2 ms across the 2M-iteration kernels) and
falls through to the boxed emission, whose `Add2/…` helpers re-check per
call and take the redefined value. So **with-redefs of core arithmetic is
honored through every unboxed path** — semantics identical to what ADR 0066
shipped for the boxed intrinsics. Conformance
`numeric-redefs-unboxed-paths.clj` proves it through the lifted, the
specialized, and the dual-loop shapes, REPL == compiled byte-identical.

Granularity note: the check is per region ENTRY (each fn call, each loop
start). A redefinition landing MID-flight inside one region activation
(another thread, or a call made from inside the loop body) is seen from the
next activation on. JVM 1.12.5 never sees such redefs at `:inline`
arithmetic sites at all (measured, ADR 0066 §context — the conformance file
documents JVM's diverging output), so cljgo remains strictly more live than
the JVM here.

## Consequences

**Measured** (darwin/arm64, post-integration with #90–#93 merged, guards on;
`TestS42Measure` boxed-baseline vs unboxed, best-of-3 wall + process
mallocs):

| kernel              | wall off→on | speedup | mallocs off→on |
|---------------------|-------------|---------|----------------|
| loop/recur sum ×10M | 144→16 ms   | 8.8×    | 20.2M → 0.16M  |
| `(fact 15)` ×2M     | 314→68 ms   | 4.6×    | 24.2M → 4.2M   |
| `fib 35`            | 148→32 ms   | 4.7×    | (already low)  |

The dirty guard costs ≤2 ms vs the unguarded spike numbers (68 vs 69 ms
fact, 16 vs 14 ms loop-sum — noise-level). `TestFactorialPerfBudget` fell
**~35× → 4.8×**, under the ~10× M2 budget for the first time; the local
gate tightened 60→15 and CI 120→60 to lock it in. The `convT64` allocations
fall exactly as the pprof decomposition predicted (−20M on loop and fact).

**AOT core**: the pass runs on `core.clj` too (regenerated `pkg/coreaot/*`,
drift-gated); core's few int64 loops (destructure's arity counter et al)
unbox behind the same guards.

**Ceiling honesty.** The honest floor of the whole CLJS/`any`-calling-
convention model on such kernels is ~2–3×; the kernels above reach ~5×
against raw Go because they are monomorphic and either loop-local or lifted
to a typed Go func. A kernel that stays inside the boxed calling convention
(a specialized-but-capturing recursion, tier 2 without the lift) lands at
~2× — real, but not the headline.

**Not yet inferred (follow-ups):** float64 (all floats stay boxed);
multi-arity / variadic / >4-arity specialization; cross-fn (non-self)
return typing; broadening the lift to capturing closures.
`CLJGO_NUMINFER_OFF=1` remains the kill switch and the A/B measurement
lever.

**Shipped follow-up (2026-07-23): `<=`/`>=` comparisons.** The
benchmark-corpus fib (`(if (<= n 1) …)`) DID lift to a typed func, but its
`<=` test still emitted a per-call var deref + boxing `lang.Apply2` — `<=`
and `>=` were missing from all three comparison tables. They now join
`intUnboxCmp` (raw Go compare on proven int64), `testIntrinsics`
(rt.LEBool/GEBool) and `intrinsic2` (rt.LE2/GE2), and — required for
with-redefs liveness through the raw compare — `<=`/`>=` join the sealed
core set (rt.Boot now seals NINE vars, was seven). Corpus-exact `fib 35`
AOT wall time (hyperfine, startup included): 739 → 31 ms; tak (40 ms) and
loop-recur (9.5 ms) held. Conformance: numeric-le-ge-compare.clj,
numeric-le-ge-overflow-boundary.clj, numeric-le-ge-redefs-unboxed.clj
(dual harness, oracle-verified).
