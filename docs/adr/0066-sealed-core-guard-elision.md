# ADR 0066 — Sealed core arithmetic: dirty-flag guard elision

Date: 2026-07-23 · Status: **accepted — dirty-flag variant** (owner-directed 2026-07-23: "highest performance, non-breaking"; refines/conditions ADR 0004; the hard-seal variant below remains a separate owner-gated question) · Spike: s43

## Context

Every 2-argument core arithmetic call (`+ - * / < <= > >= =`) emits as a guarded
intrinsic in `pkg/emit/rt` (`rt.Add2(v, x, y)` etc). Per ADR 0004 each helper,
on **every call**, does two things purely to honor redefinition liveness:

1. `v.Get()` — an `atomic.Value` load + `Box` unwrap on the operator var
   (`(*Var).Get`/`getRoot`), and
2. `f != origAdd` — a `runtime.efaceeq` interface-compare of the derefed value
   against the boot-time pristine builtin, to answer "has `+` been redefined?"

A pprof decomposition of cljgo's ~35× (factorial) / competitor gap attributed
roughly **~10%** of arithmetic CPU to the var deref and **~8%** to the `efaceeq`.
Both costs are paid on the overwhelmingly common path where the answer is always
"no, `+` is still `+`". ADR 0004 accepted this ("per-call deref costs ~2%,
free") but the intrinsic's *compare* was not in S6's model, and the two together
are the single largest sink after boxing.

### What JVM Clojure actually does here (measured, 2026-07-23, clojure 1.12.5)

`+` carries `:inline` metadata, so at a direct 2-arg call site the JVM compiler
emits `clojure.lang.Numbers.add(3,4)` **at compile time** and the var is never
consulted at runtime. Consequences, all verified against the `clojure` CLI:

- `(with-redefs [+ (fn [a b] (* a b))] (+ 3 4))` ⇒ **7**, not 12.
- `(alter-var-root #'clojure.core/+ …)` then `(+ 3 4)` ⇒ **7** (the inline is permanent).
- Only when `+` is passed as a *value* — `(apply + [3 4])`, `(reduce + …)` — is
  a redefinition seen.

cljgo, by contrast, derefs at runtime and **does** see such a redefinition (it
returns 12). cljgo is therefore *strictly more live than the JVM* at inlined
arithmetic sites — a pre-existing divergence, not one this ADR introduces. This
is the crux: the very liveness ADR 0004 pays for on every call is liveness JVM
Clojure **does not even provide**.

## Decision (proposed)

Add a process-global monotonic dirty flag, `lang.CoreArithDirty` (`atomic.Bool`),
and **seal** the seven core arithmetic vars. Mechanism:

- `(*Var).Seal()` marks a var. `rt.Boot` seals `+ - * / < > =` **after** the
  pristine snapshot and after compiled-core load — so no boot-time `BindRoot`
  trips the flag.
- `(*Var).BindRoot` and `(*Var).AlterRoot` call `tripIfSealed()`: if the var is
  sealed, they set `CoreArithDirty = true`. This covers every runtime
  root-mutation path — emitted `def` (`gv.BindRoot`), `alter-var-root`, and
  `with-redefs` (which rides on `alter-var-root`).
- Each intrinsic checks `CoreArithDirty` **once** (one relaxed `atomic.Bool` load,
  branch-predicted false):
  - **false** (common): skip the deref and the compare entirely, open-code the
    int64 op.
  - **true**: fall back to the original ADR 0004 guarded path (deref +
    `efaceeq`; route a redefined value through `lang.Apply2`).

The flag is deliberately **never reset**. Correctness only requires that a
*currently*-redefined var take the guarded path; monotonicity avoids any
cross-goroutine reset race. Cost: after the first-ever redefinition of any core
arithmetic op in a process, all seven permanently pay the old guard again (still
correct, just no longer elided).

### How this conditions ADR 0004

ADR 0004 mandates per-call deref "everywhere" so a live redefinition is seen
immediately. This ADR **narrows** that mandate for the seven sealed arithmetic
vars: the deref happens **once the operator has ever been redefined**, not on
every call. The observable liveness is **unchanged** — `with-redefs`/`set!`/`def`
of a core arithmetic op is still seen (see the escape-hatch test) — because the
mutation itself trips the flag *before* the next intrinsic call reads it. What
changes is only *when the machinery runs*: never, until the first redefinition.
This is a refinement, not a reversal: ADR 0004's guarantee (redefinition is
seen) holds; ADR 0004's *implementation detail* (deref on literally every call)
is relaxed.

## Consequences

### Measured (darwin/arm64, 2026-07-23)

- **Intrinsic microbench** (`pkg/emit/rt/guard_bench_test.go`, guarded-vs-elided
  in one binary):
  - `Add2`: 7.92 ns/op → **6.27 ns/op** (~21% faster).
  - `LTBool`: 6.76 ns/op → **5.18 ns/op** (~23% faster).
- **pprof** of the two regimes: in the guarded profile `(*Var).Get` (4.0% cum),
  `(*Var).getRoot` (2.8%), and `runtime.efaceeq` (1.8%) are all present; in the
  elided profile **all three frames vanish** — only `Add2` remains.
- **End-to-end factorial** (`(fact 15)` × 2M, `TestFactorialPerfBudget`):
  net emitted work **443.7 ms → 325.2 ms (~27% faster)**; ratio **38.0× → 31.6×**.
  (Arithmetic-dense: 4 intrinsic ops per fact call, so it beats the ~15–18%
  estimate.)
- **`(reduce + (range 2e6))`: unchanged** (~77 ms). `reduce` invokes `+` as a
  *value* through IFn dispatch, never via `Add2` — this optimization does not
  touch that path. Honest scope: the win is at **direct 2-arg call sites** only.

### Liveness caveat + escape hatch

- The escape hatch works: `(with-redefs [+ (fn [a b] (* a b))] (+ 3 4))` still
  returns **12**, and the value is restored to 7 after the form, identically in
  REPL and compiled binary (`TestSealedGuardWithRedefsEscapeHatch`). Dual-harness
  and full conformance stayed green.
- Caveat: once tripped, the flag never clears, so a program that redefines a core
  arithmetic op pays the guard for the rest of its life. This is the correct,
  conservative trade — such programs are rare and already off the fast path.

### Risks

- A **new** core-var root-mutation path that bypasses `BindRoot`/`AlterRoot`
  would silently miss the trip. Today those are the only two root writers (grep
  confirms); a `tripIfSealed()` must accompany any future third.
- `atomic.Bool.Load` is one instruction but not *zero*; the hard-seal alternative
  below removes even that.

## Alternatives

1. **Hard seal (no flag, no fallback).** Emit the int64 op with no var reference
   at all; a redefinition of `+` is simply never seen at inlined sites. This is
   **faster still** (no atomic load) and **more JVM-conformant** (matches JVM's
   `:inline` exactly — `[7 7 7]`). It is rejected *as the default* only because it
   changes cljgo's current observable behavior (`[7 12 7] → [7 7 7]`) and
   contradicts ADR 0004's letter. **Recommendation: this is the owner-gated
   question.** Given that JVM itself does not provide inlined-site liveness, a
   `--seal-core` release/build flag (or making hard-seal the default with an
   `^:redefinable`-style opt-out) is defensible and would be the bigger win. This
   ADR ships the safe dirty-flag now and surfaces hard-seal for a ruling.
2. **Per-op dirty flags** (one bool per var) instead of one global. Marginally
   less pessimistic after a redefinition (only the touched op slows), at the cost
   of 7 flags and more trip-site bookkeeping. Not worth it — redefining core
   arithmetic is a cold, rare event.
3. **Build-mode gate only** (elide under `-release`, keep the guard in dev). More
   machinery than the dirty flag for the same steady-state win; the dirty flag
   already gives dev the fast path *and* full liveness, so a mode split buys
   nothing here.
4. **Do nothing.** Leaves ~8%+10% of arithmetic CPU on a guard that answers "no"
   every time, for liveness the JVM does not even offer at these sites.

## Startup cost note (2026-07-23)

The benchmark re-run's AOT-startup regression (6.5 → 9.5 ms) initially
pointed here; measurement acquitted the seal — Boot's snapshot + seven
`Seal()` calls are sub-microsecond, and a bisected build at the pre-campaign
commit showed the identical 9.0 ms. The real costs (boot-time whole-namespace
refers + GC cycling through the boot burst) and the clawback (bulk refer +
boot GC deferral, landing startup at 4.4 ms) are documented in ADR 0067's
"Startup cost + clawback" addendum.
