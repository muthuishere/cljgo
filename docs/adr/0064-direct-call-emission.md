# ADR 0064 — Direct-call emission for statically-known local fns

Date: 2026-07-23 · Status: accepted
Builds on: ADR 0004 (fixed-arity calling convention, `lang.ApplyN` / `FnFuncN`),
ADR 0045 (native hot-core builtins). Perf gates: `pkg/emit/perf_test.go`
(`TestFactorialPerfBudget`, `TestCoreReducePerfBudget`), design/00 §1.4.

## Context

A pprof decomposition of the ~35× emitted-vs-handwritten-Go factorial gap
(the `TestFactorialPerfBudget` workload, design/00 §1.4) flagged the invoke
path. The emitter lowered *every* call — including a fn's own self-recursive
call — through `lang.ApplyN(fnval, args…)`, the dynamic dispatcher in
`pkg/lang/apply.go`. Emitted factorial's self-call read:

```go
var fact1 any                            // the self-name, boxed as any
tmp2 := lang.FnFunc1(func(n3 any) any {
    …
    tmp7 := lang.Apply1(fact1, tmp6)     // dynamic dispatch on every call
    …
})
fact1 = tmp2
```

`recur`/`loop` was already lowered to a Go `for` loop (genLoop /
genMethodBody) and is *not* the issue. The issue is genuine self-calls and
calls to `let`-bound fns: each pays `Apply1`'s type-switch
(`switch f := fn.(type) { case FnFunc1: … }`) on every invocation, and the
`any`-typed callee blocks the Go compiler from seeing the concrete closure
type at the call site. The self-name (`fn.Local`) and immutable `let`
bindings hold one statically-known closure of known fixed arity — that fact
was thrown away at the boundary.

## Decision

**Emit a direct Go invocation of the closure value for calls whose callee is
a statically-known local fn of matching fixed arity**, bypassing
`lang.ApplyN`.

1. **Typed handle.** When an fn* is a single fixed-arity method (≤ 4 params,
   non-variadic — the `singleFixedMethod` shape), the emitter keeps a
   `lang.FnFuncN`-typed handle on the closure in addition to the existing
   `any`-typed value:
   - **self-name** (`fn.Local`): a pre-declared `var fact1d lang.FnFuncN`
     the closure captures, assigned alongside the `any` self-name right after
     construction. Direct calls can only fire once it is set, exactly as the
     existing self-name binding.
   - **`let`-bound fn**: the `lang.FnFuncN`-typed temp `genFn` already
     returns is registered directly; `let` bindings are immutable, so it
     holds that closure for the whole block (and any nested closure that
     captures it).
2. **Registry.** A `directFns map[*ast.BindingNode]directFn` keys these by
   binding *pointer identity*, so shadowing and name reuse can never
   mis-resolve. At an `OpInvoke` whose `Fn` is an `OpLocal` resolving to a
   registered binding **and whose arity matches**, the emitter writes
   `tmp := fact1d(args…)` — a direct call of the closure value, no
   type-switch. The fn position of such a call is a side-effect-free local
   read, so evaluating the args first is order-preserving.
3. **Conservative fallback — semantics are non-negotiable.** Anything not
   provably a fixed match falls through to the unchanged `lang.ApplyN` path:
   multi-arity / variadic fns, arity mismatches (so the real
   `lang.NewArityError` still fires, byte-identical), the callee used as a
   *value* (passed, returned — still the `any` binding), `loop` carriers
   (reassigned by `recur`, never registered), `letfn` (its names bind to
   variadic shims, not fixed-arity fns — core.clj), and any non-local callee
   (vars still deref per call via `.Get()`, ADR 0004). `letfn`/loop/`set!`
   locals are simply never entered into `directFns`.

Deliberately **not** done: hoisting the per-call var `.Get()` deref out of
emitted loops. ADR 0004 mandates per-call deref for redefinition liveness
(REPL/compile parity); lifting it out of a loop body would let a
redefinition made mid-loop go unseen — a semantics change, not a safe
micro-opt — so it stays per-call.

## Consequences

- **Dual-harness parity unaffected.** Only the *shape* of emitted Go changes;
  results and error behavior are identical, and the closure invoked is the
  same object `ApplyN` would have dispatched to. The AOT-core generated
  packages (`pkg/coreaot/*`) were regenerated (`go generate ./pkg/coreaot`)
  and the full uncached conformance dual-harness stays byte-identical green —
  the release-blocking bar. `TestGeneratedCoreIsUpToDate` guards the
  regenerated files.
- **Enables Go inlining / escape analysis.** With the concrete `FnFuncN`
  type visible at the call site the compiler can devirtualize and reason
  about escapes it could not through `any`+type-switch. Measured net-of-
  startup (darwin/arm64, 2026-07-23, AOT-compiled): `fib(37)` self-recursion
  ~1.10× faster, `(fact 15)`×2M ~1.07× faster. A modest, safe, consistent
  win on the self-recursion hot path — it removes the type-switch dispatch,
  not the boxing (args/results stay `any`; primitive hints are the separate
  design/04 §5 rung, post-M2).
- **Perf gate untouched.** `TestFactorialPerfBudget` is a ratio that divides
  two independently-measured net times and swings run-to-run on its sub-ms
  denominator (its own doc comment); the win is inside that noise band, so
  the 60× local ceiling is left as-is rather than tightened on a noisy
  reading. The gate keeps biting the ~168× naive-emission regression it
  exists to catch.
</content>

## Addendum — cross-var direct calls (2026-07-28)

Status: **accepted**, extends this ADR's decision from *local* fn bindings
to *top-level defns of the same compilation unit*. Builds on ADR 0066's
dirty-flag seal, which is the mechanism that makes it non-breaking. No
part of the original decision is reversed.

### What was still slow

The decision above covers only callees reachable through a lexical Go
handle (a fn's self-name, a `let`-bound fn). The far more common shape —
one top-level `defn` calling another — was untouched: `(add2 3 4)` emitted

```go
tmp17 := v_f_add2.Get()                       // atomic.Value load + Box unwrap
tmp18 := lang.Apply2(tmp17, int64(3), int64(4)) // type-switch dispatch on `any`
```

That is a var deref plus `lang.ApplyN`'s type switch on *every* call, paid
to honor a redefinition that in practice never happens.

### Decision

A top-level `def` whose init is a single-fixed-arity fn* **publishes a
package-level typed handle** (`var fnD_ns_name lang.FnFuncN`) and then
**arms the var's per-var seal bit** (`Var.SealDirect()`). A matching-arity
call site emits

```go
tmp17 := v_f_add2.Direct()          // one relaxed atomic.Bool load
var tmp18 any
if !tmp17 { tmp18 = v_f_add2.Get() } // fn position still read BEFORE args
var tmp19 any
if tmp17 { tmp19 = fnD_f_add2(int64(3), int64(4)) } // direct, no ApplyN
else     { tmp19 = lang.Apply2(tmp18, int64(3), int64(4)) }
```

1. **One seal, one trip site.** `Var.tripIfSealed` — already the single
   root-mutation hook ADR 0066 installed on `BindRoot`/`AlterRoot` — now
   also disarms the per-var `direct` bit. Every root writer (emitted
   `def`, `alter-var-root`, and therefore `with-redefs`) goes through it,
   so a redefinition is observed exactly as before, in the same instruction
   it was observed before. No second seal was invented.
2. **Per var, not global.** Unlike `CoreArithDirty` the bit is per var —
   ADR 0066 "Alternatives" §2, which is the right trade here precisely
   because *every* emitted `def` mutates a root; a global flag would be
   tripped by the program's own definitions.
3. **Publish-then-arm is the memory edge.** The handle write is ordered
   before the `SealDirect` store, and a reader loads the bit before it
   uses the handle; `atomic.Bool` Store/Load are sequentially consistent,
   so no reader can see an armed bit with a stale handle. `BindRoot`
   disarms first, so a second `def` can never leave one armed either.
4. **Conservative registration.** A var qualifies only when exactly one
   init-bearing `def` in the unit gives it a value, that `def` is a
   top-level form (or a statement of a top-level `do`), its init is a
   non-variadic single-fixed-arity fn*, and the var is **not** `:dynamic`
   (a thread binding would be invisible to a direct call;
   `PushThreadBindings` refuses a non-dynamic var, so the hazard is ruled
   out statically). `declare` — an init-less `(def g)` — emits no
   `BindRoot` and so does not disqualify, which is what makes forward
   references and mutual recursion eligible. Everything else (multi-arity,
   variadic, `apply`, arity mismatch, value position, a redefined var, a
   callee in another namespace) emits exactly as before, which is also
   what keeps `lang.NewArityError` byte-identical.
5. **Order preserved.** The fn position is read before the args in *both*
   arms (the bit on the fast path, the root on the slow one), so an arg
   expression that redefines the callee still does not affect this call.

### Measured (darwin/arm64, 2026-07-28, base c51fb41 vs perf/direct-call)

Wall-clock totals, best of 9 interleaved runs per binary.

| program (AOT) | base | branch | ratio |
|---|---|---|---|
| `benchmark/programs/calls.clj` (3 cross-fn calls × 1e7) | 344 ms | **300 ms** | **1.15×** |
| `fib.clj` (fib 35) | 43 ms | 41 ms | 1.05× (noise — already ADR 0067 lifted) |
| `tak.clj` | 54 ms | 56 ms | 0.96× (noise) |
| `reduce.clj` | 45 ms | 46 ms | — |
| `map-filter.clj` | 23 ms | 23 ms | — |
| `transducers.clj` | 35 ms | 34 ms | — |
| `persistent-map.clj` | 28 ms | 28 ms | — |
| `loop-recur.clj` | 22 ms | 22 ms | — |
| hello-world (startup floor) | 22 ms | 22 ms | — |

Interpreted leg unchanged (this is an emitter-only change): scaled-down
`calls` 402 → 405 ms, `fib` 116 → 117 ms, `tak` 90 → 92 ms — all noise.

`fib`/`tak` are self-recursive and already run as ADR 0067 rung-3 lifted
`int64` funcs, so they never touched the var path; the win is exactly where
the ADR says it is — **cross-fn calls between top-level defns**, ~15%.

Binary size: hello-world 6,684,722 → 6,783,810 B (**+99 KB, +1.5%**);
`calls` 6,684,738 → 6,800,338 B (+116 KB, +1.7%). The cost is the published
handles and the two-arm call sites, mostly in the regenerated
`pkg/coreaot` (core.clj alone publishes 265 handles).

### Consequences

- **Dual-harness parity unaffected**; the tree-walk evaluator is untouched.
  Frozen as `conformance/tests/direct-call-var-redefs.clj` (forward
  reference, plain call, `with-redefs` + restore, value position, mutual
  recursion, `alter-var-root` — oracle: clojure 1.12.5 CLI), passing in
  both harnesses, plus `pkg/emit/direct_call_var_test.go` for the emitted
  shape and the compiled escape hatch.
- **Non-breaking by default, so no flag.** Observable liveness is
  identical, exactly as ADR 0066's dirty-flag variant, so this ships on by
  default; the owner-gated *hard*-seal question ADR 0066 raised stays open
  and is unaffected.
- **Residual:** cross-*namespace* calls still deref (the handle is package
  private), the fallback arm costs one atomic load and a predictable
  branch, and lifting the callee body to a static (inlinable) package func
  instead of a func-valued global is a further rung not taken here.
