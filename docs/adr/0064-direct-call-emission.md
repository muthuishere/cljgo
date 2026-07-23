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
