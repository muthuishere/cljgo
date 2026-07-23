# ADR 0067 — Emitter numeric type inference (unboxed int64)

Date: 2026-07-23 · Status: **proposed** (spike s42 prototype; owner review gate)

Prototype: `spikes/s42-emitter-type-inference/VERDICT.md`; working code on the
spike branch (`pkg/emit/numtype.go`, `pkg/emit/rt/inttypes.go`, the emission
changes in `pkg/emit/emit.go` / `program.go`). Complements design/04 §5
(the primitive-hints / open-coded-intrinsics rung) and ADR 0045 (hot core
fns are native Go — the same "stop boxing everything" campaign, one rung up).

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
(primitive signatures + open-coded intrinsics), deferred past M2. This ADR
prototypes it: an emitter pass that carries provable `int64` through locals
and emits **raw Go arithmetic**, boxing only at boundaries.

A load-bearing fact makes this tower-safe: cljgo's pristine `+ - *` on two
int64 **throw** on overflow (`pkg/lang/numberops.go` `int64Ops.Add/Sub/
Multiply`) — they do NOT promote to BigInt (that is `+' -' *'`). So a checked
int64 result is ALWAYS an int64 or the very same `ArithmeticException`;
there is no third "the result became a BigInt" outcome that an int64 Go local
could not hold. That is exactly what lets the value stay unboxed.

## Decision (what the prototype does)

A conservative, bottom-up inference (`inferNumeric`) proves a node/binding is
`int64` and the emitter then emits unboxed Go. **When unsure it stays boxed —
correctness first.**

**Inference rules (int64 only in v1).** A node is `int64` when it is:
- an integer literal;
- a reference to a binding proven `int64`;
- `(+ a b)` / `(- a b)` / `(* a b)` / `(inc a)` / `(dec a)` on `int64`
  operands, where the op is the pristine `clojure.core` var (`/` never —
  ratio lives in the tower);
- a self-recursive call, with all-int64 args, of the fn currently being
  specialized (greatest-fixpoint: assume int64 return, validate the body).

A binding is `int64` when it is a `let`/`loop`/method-recur **carrier** whose
init AND every `recur` value are `int64` (a monotone fixpoint that demotes on
the first non-int64 flow), or a **specialized fn parameter** (below). Captured
carriers (closed over by a nested fn) and variadic params stay boxed.

**Three emission tiers.**
1. **Loop/let carriers** → `var i int64`, raw `rt.IAdd/ISub/IMul/IInc/IDec`
   (checked; panic-identical to the tower) and raw `< > ==` in `if` tests.
   The accumulator never leaves a register across iterations.
2. **Parameter specialization** — a single-fixed-arity fn whose body is
   int64-provable with every param assumed int64 gets a **dual body**: an
   entry guard `if pI, ok := p.(int64); ok { …raw int64… }` and, falling
   through for a non-int64 (float / BigInt / redefined-op) caller, the
   original boxed body. The external `any → any` calling convention is
   unchanged, so every existing/dynamic/higher-order caller still works.
3. **Rung 3 — typed-func lift** for a self-recursive, capture-free
   specialized fn: lift it to a package-level `func factL(n int64) int64`
   with **direct int64 recursion**. This is what makes fib/fact's recursive
   returns alloc-free — without it, body specialization alone still boxes at
   every recursion level (the return crosses the `any` FnFunc boundary).

**Tower preservation.** The unboxed ops reproduce `int64Ops`' overflow tests
byte-for-byte and panic the same `ArithmeticException("integer overflow")`;
`/`, ratios, int/float contagion and BigInt all reach the boxed path (a float
or BigInt operand makes the node non-int64; a runtime float/BigInt arg fails
the entry guard). The conformance overflow/promotion tests
(`numeric-overflow-throws`, `numeric-promotion`) now run THROUGH `rt.IAdd`
and stay green.

**Conservative line (what v1 does NOT infer).** float64 (all floats stay
boxed); multi-arity / variadic / >4-arity fn specialization; cross-fn return
typing (only *self* calls type); capturing closures for the rung-3 lift; any
op beyond `+ - * inc dec < > =`. All of these fall back to the existing boxed
emission, unchanged.

**Redefinition — the one semantic decision for the owner.** The unboxed ops
do NOT deref the operator var per call, so they do not observe a runtime
`(with-redefs [+ …])` of a core arithmetic op — matching JVM Clojure's
`MaybePrimitiveExpr` intrinsics, which also ignore redefinition, and
design/04 §5 rung 4. This is STRICTER than cljgo's existing boxed intrinsic,
which keeps the ADR-0004 liveness guard. No conformance test redefines core
arithmetic, so the dual harness stays byte-identical — but this is a real,
narrow divergence from the boxed path and must be **ratified here** before
apply. If the owner wants full liveness, the fallback is a once-per-call
`rt.ArithPristine()` entry guard (cheap; keeps the boxing win, costs the
redefinition-ignoring speed on the arithmetic).

## Consequences

**Measured win** (darwin/arm64; `TestS42Measure`, boxed-baseline vs unboxed,
best-of-3 wall + total process mallocs):

| kernel            | wall off→on | speedup | mallocs off→on |
|-------------------|-------------|---------|----------------|
| loop/recur sum ×10M | 190→14 ms | **13.9×** | 20.2M → 0.16M |
| `(fact 15)` ×2M   | 472→69 ms   | **6.8×**  | 24.2M → 4.2M  |
| `fib 35`          | 281→30 ms   | **9.3×**  | (already low)  |

The canonical `TestFactorialPerfBudget` fell from **~35× to 5.1×**
handwritten Go. The `convT64` allocations fall exactly as the pprof
decomposition predicted (loop: −20M; fact: −20M).

**Regenerated AOT core.** The pass also fires inside `core.clj`, so
`pkg/coreaot/*` was regenerated (the generated-up-to-date gate enforces it);
core numeric fns unbox too. This surfaced — and the prototype fixed — one
real miscompile: the inference's zero value had to be `ntUnknown`, so an
untyped param read never masquerades as the meet-identity and wrongly types
a loop carrier seeded from it.

**Ceiling honesty.** The honest floor of the whole CLJS/`any`-calling-
convention model on such kernels is ~2–3×; these numbers reach it because the
kernels are monomorphic and either loop-local (accumulator in a register) or
lifted to a typed Go func (direct recursion). A kernel that stays inside the
boxed calling convention (e.g. a specialized-but-capturing recursion) lands
at ~2× — real, but not the headline.

**What's NOT yet proven / risks.** (1) The redefinition divergence above.
(2) float64 is unhandled — a large class of numeric code sees nothing.
(3) `rt.MustInt64` on the inline-Tier-2 self-call is an inference invariant
(unreachable if inference is sound) that panics through the normal error
channel rather than a bare crash if ever violated. (4) The rung-3 lift is
gated to capture-free, nested-fn-free bodies; broadening it needs closure
handling.

**Apply scope recommendation (v1):** ship tiers 1–3 for `int64` behind this
ADR, with the redefinition semantics ratified (or the `ArithPristine` guard
added if the owner wants strict liveness). Add float64 and multi-arity
specialization as follow-ups. Keep the `CLJGO_NUMINFER_OFF` kill switch.
