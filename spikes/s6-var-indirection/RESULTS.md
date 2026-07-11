VERDICT: VALIDATED — REPL-liveness-by-default survives the numbers: per-call atomic Var deref costs ~0–2% and stays the default. The expensive part is NOT the var — it's the variadic `Fn`/Apply1 calling convention (×6.8, one heap alloc per call). M2's default emission must pair per-call deref with **fixed-arity closure calls**, which lands at 3.5× raw (fib) / 7.8× raw (factorial) — inside the §1-priority-4 "~10× of handwritten Go" M2 budget — with zero semantic compromise.

# S6 — Var-indirection cost in emitted code

- Machine: Apple M5 Pro, darwin/arm64, go1.26.3
- Method: hand-written Go approximating emitter output per variant; `testing.B` with `b.Loop()` + `b.ReportAllocs()`, `-count=5`, medians reported. Raw output in `bench-raw.txt`.
- Workloads: naive `fib(30)` (~2.69M calls/op) and recursive `fact(20)` (20 calls/op).
- Note: every benchmark stores its result into an `any` sink, so *all* variants (including Raw) carry one 8 B result-boxing alloc; the Raw baselines are therefore slightly pessimistic, i.e. the multipliers below are slightly *flattering* to the slow variants. Direction of error is safe.

## Benchmark table (medians of 5)

### fib(30)

| # | Variant | ns/op | × Raw | × prev step | B/op | allocs/op |
|---|---------|------:|------:|------------:|-----:|----------:|
| 1 | Raw Go (int64, direct) | 1,688,023 | 1.00 | — | 8 | 1 |
| 2 | Boxed (`any`), direct call | 4,986,478 | 2.95 | ×2.95 | 33,440 | 4,180 |
| 3 | Boxed + `lang.Fn` via Apply1 | 33,895,338 | 20.1 | ×6.80 | 43.1 MB | 2,696,720 |
| 4 | 3 + per-call Var deref (atomic.Value) | 34,598,827 | 20.5 | ×1.02 | 43.1 MB | 2,696,720 |
| 4p | 4 with atomic.Pointer | 34,362,925 | 20.4 | ≈4 | same | same |
| 4m | 4 with sync.Mutex | 36,392,036 | 21.6 | ×1.05 vs 4 | same | same |
| 4r | 4 with sync.RWMutex | 35,126,035 | 20.8 | ×1.02 vs 4 | same | same |
| 5 | Var deref hoisted (once per top-level call) | 32,834,865 | 19.5 | ×0.95 vs 4 (noise-level) | 43.1 MB | 2,696,719 |
| 6 | **Fixed-arity `func(any) any` + per-call Var deref** | **5,999,432** | **3.55** | **×5.8 faster than 4** | 33,440 | 4,180 |

### fact(20)

| # | Variant | ns/op | × Raw | B/op | allocs/op |
|---|---------|------:|------:|-----:|----------:|
| 1 | Raw Go | 14.65 | 1.00 | 8 | 1 |
| 2 | Boxed, direct | 96.80 | 6.6 | 120 | 15 |
| 3 | Boxed + Apply1 | 415.5 † | 28 † | 440 | 35 |
| 4 | 3 + per-call Var deref | 328.8 | 22.4 | 440 | 35 |
| 5 | Var deref hoisted | 336.9 | 23.0 | 440 | 35 |
| 6 | **Fixed-arity + per-call deref** | **114.4** | **7.8** | 120 | 15 |

† variant 3's runs were noisy (324–575 ns); its stable runs sit at ~324 ns, i.e. statistically identical to variant 4 — consistent with the deref being free.

### Deref-only microbench (settles the Var representation)

| Mechanism | single-thread ns/op | parallel (18 threads) ns/op |
|---|---:|---:|
| atomic.Value load | 1.68 | 0.0355 |
| atomic.Pointer load | 1.62 | — |
| sync.RWMutex RLock/RUnlock | 3.33 | — |
| sync.Mutex Lock/Unlock | 4.37 | 59.1 (**~1,665× worse than atomic**) |

**Atomic confirmed.** atomic.Value and atomic.Pointer are equivalent (~1.6–1.7 ns, wait-free, zero-alloc); either works — pick atomic.Pointer if the root must hold arbitrary types without atomic.Value's consistent-concrete-type restriction. Mutex is 2.6× slower uncontended and catastrophic under contention — exactly the wrong shape for read-mostly vars in a goroutine-heavy language. Rejected.

## Where the cost actually is (step multipliers, fib)

- **1→2 boxing: ×2.95.** ~4,180 allocs/op — only results ≥ 256 heap-box (Go's `staticuint64s` caches 0–255, so small int args box for free). This is the price of the `any` value model; recoverable later via the primitive-hints/unboxed-locals ladder rungs.
- **2→3 calling convention: ×6.80 — THE dominant cost.** `lang.Fn func(...any) any` forces a heap-allocated 1-element `[]any` at every call (2,692,537 calls → 2,692,540 slice allocs → 43 MB/op of GC pressure). "Apply1 fast path" cannot help: the slice materializes at the `fn(a)` call itself, inside Apply1.
- **3→4 var indirection: ×1.02.** Two atomic loads per fib call ≈ 3.4 ns against a ~12 ns call body — noise. **REPL-liveness is essentially free.**
- **4→5 hoisting: ×0.95, i.e. nothing.** Saving an already-free load saves nothing.
- **4→6 fixed-arity convention: ×5.8 faster.** `Var1.Deref1()(x)` with `Fn1 func(any) any` removes the per-call slice; allocs collapse from 2,696,720 to 4,180/op (boxing only). Per-call deref retained.

## Is variant 5 (hoisted deref) semantically acceptable?

Under hoisting, a re-`def` lands on the **next top-level call**; a deep in-flight recursion keeps running the old fn. JVM Clojure comparison:

- Ordinary (non-direct-linked) JVM Clojure derefs the var **per invocation** — a mid-recursion re-def is picked up by the very next recursive call. Variant 4 is the faithful one.
- JVM Clojure **with direct linking** is strictly *worse* than variant 5: re-defs are never seen by existing callers at all. So variant 5 sits between the two and would be defensible…

…but it's **moot**: variant 5 buys 0% performance. There is no reason to spend any semantic budget on it. **Rejected — keep per-call deref.**

## Recommendation for M2 default emission

1. **Var = atomic load, deref at every call site, on by default.** Confirmed at ~1.7 ns / 0 allocs; the §4.2 rule "direct linking default-off" costs nothing measurable. No mutex anywhere on the deref path.
2. **Pull the "fixed-arity fn types" rung of the doc 04 §5 ladder INTO M2's default emission** — it is not an opt-in optimization, it's the difference between missing and meeting the M2 budget. Concretely: for a call site `(f x)` where arity is known statically (the overwhelmingly common case), emit `f.Deref1()(x)`-shaped code against a fixed-arity closure field instead of `Apply1(f.Deref(), x)` over variadic `Fn`. Emitted defns should carry both representations — fixed-arity fields `invoke1..invoke4` for direct-shaped call sites, plus the variadic `Fn` for `apply`, HOF and IFn interop. (Exact struct shape is doc 04's call; this spike proves the win: ×5.8, minus one alloc per call.)
3. **Keep Apply1..4 as the fallback** for unknown callables (keywords, colls, evaluator fns) — just don't make it the emitted-code common path.
4. **Don't bother with hoisting** (see above).
5. The remaining gap to raw (3.55× fib / 7.8× fact) is boxing — attack post-M2 with primitive hints/unboxed locals as already planned; no design change needed now.

## Perf-budget check (00-architecture §1 priority 4)

> "M2: emitted factorial within ~10x of handwritten Go"

| Emission plan | fact(20) vs raw | fib(30) vs raw | Budget (~10×) |
|---|---:|---:|---|
| Variadic Fn + Apply1 + per-call deref (naive plan) | 22.4× | 20.5× | **FAIL** |
| **Fixed-arity + per-call deref (recommended)** | **7.8×** | **3.55×** | **PASS** |

REPL-liveness and "performance is a feature, no compromise" are not in tension — the numbers say we can have both, provided the calling convention is fixed-arity from M2 day one.
