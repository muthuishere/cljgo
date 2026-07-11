VERDICT: reflect.Select is fine as the dynamic alts! runtime — ~95ns/op at 2 ports (2.5x a static select's 37ns), linear in port count, all core.async semantics reproducible; keep the architecture's bet (static `alt!` → real `select` fast path, `reflect.Select` only for runtime port vectors). No alternative mechanism is needed.

# S10 — Dynamic alts! on reflect.Select

Machine: darwin/arm64 (Apple M5 Pro), go1.26.3. Code: `alts.go` (runtime),
`alts_test.go` (semantics, all under `testing/synctest`), `bench_test.go`.
All tests pass under `-race -count=2`.

## API proved

```go
Alts(ops []AltOp, opts AltOpts) (val any, ch any, ok bool)
// AltOp{Chan any, Value any, IsWrite bool}
// AltOpts{HasDefault, Default, Priority, HasTimeout, Timeout}
```

Result contract (core.async parity per doc 05 §4):
read ready `(v, ch, true)` · read closed `(nil, ch, false)` · write ready
`(true, ch, true)` · write closed `(false, ch, false)` · default
`(Default, DefaultPort, false)` · timeout `(nil, TimeoutPort, false)`.
Nil puts panic (core.async throws). `Chan` is `any`, so typed interop
channels (`chan int`) work directly as ports — reflect handles element-type
assignability, and `recvOK=false` gives us closed→nil normalization even
when the element zero value isn't nil (verified: closed `chan int` returns
`nil`, not `0`).

## Semantics findings

1. **Fairness — verified pseudo-random.** 2000 rounds over 2 always-ready
   channels: both sides chosen ~50% (test asserts ≥25% each; observed
   near-even). `reflect.Select` documents "uniform pseudo-random" and it
   holds — core.async's default RANDOM fairness is free.
2. **`:priority`** = one non-blocking single-case pass in listed order
   (each case + `SelectDefault`), then fall through to a blocking select
   over all cases if nothing was ready. Deterministic when >1 port is ready
   (500/500 rounds picked the first), still blocks correctly when none is.
   Order-sensitivity only exists in the multiple-ready case, so the
   blocking fallback waking on "whichever first" matches core.async.
3. **`:default`** = one extra `SelectDefault` case (or, under `:priority`,
   taken after the ordered pass misses). Verified taken *only* when nothing
   is ready, on both code paths.
4. **Timeout** composes cleanly as one extra recv case on `time.After(d)`.
   Loses to a ready op, fires alone otherwise.
5. **The one real wart: send on a closed channel.** Go panics, and
   `reflect.Select` does not report *which* case panicked. Fix: recover,
   then re-probe ops in listed order with non-blocking single-case selects.
   This is semantically sound — the panicked select completed *nothing*, so
   any op the probe completes is a legitimate alts! outcome, and the closed
   send deterministically registers as `(false, ch, false)` when reached.
   Verified with a closed write mixed among live ops. Note for M-async: if
   the runtime later grows a channel wrapper (needed anyway for
   dropping/sliding buffer policy side-tables, let-go's `chanPolicy`), it
   can track closed state and skip the recover dance; until then the shim
   is correct and cheap on the happy path (the `defer` costs ~1-2ns).

## synctest experience (validates the §3.1 claim)

`testing/synctest` fits our async suite exactly as doc 00 §3.1 bets:

- **`reflect.Select` participates fully** — a goroutine blocked in
  `reflect.Select` on bubble channels counts as durably blocked, so
  `synctest.Wait()` sequencing works (it's the same runtime `selectgo`).
  This was the open question; answered yes.
- **Virtual time is exact**: the timeout test asserts
  `time.Since(start) == 250ms` — equality, not tolerance — and the whole
  14-test suite (including 2500 fairness/priority rounds and multi-ms
  sleeps) runs in ~0.03s wall time.
- **Leak detection is free**: the bubble fails the test if any goroutine it
  started is still blocked at exit — no runtime.NumGoroutine bookkeeping.
- Caveat to carry into the conformance suite: channels/timers must be
  created *inside* the bubble; mixing bubbled and unbubbled channels
  panics. Runtime helpers must not lazily cache global channels/timers.

## Benchmarks

go1.26.3, darwin/arm64, Apple M5 Pro, `-benchtime 2s -count 3` (medians).
Pattern: n buffered(1) chans, one made ready per iteration, one alts-read
over all n — measures per-op cost *including per-call case construction*.

| mechanism | n=2 | n=8 | n=32 | allocs (n=2 / 8 / 32) |
|---|---|---|---|---|
| static Go `select` (the `alt!` fast path) | **37.5 ns** | — | — | 0 allocs, 8 B |
| raw `reflect.Select`, prebuilt cases | 66.4 ns | — | — | 2 allocs, 40 B |
| `Alts` (full: build cases + select + normalize) | 94.7 ns | 359 ns | 1531 ns | 3/13/38 allocs · 152/1032/4616 B |
| goroutine-per-chan fan-in (steady state) | 294 ns | 305 ns | 338 ns | 0 allocs (but n standing goroutines) |
| `Alts` `:default` miss, n=8 (polling worst case) | — | 474 ns | — | 13 allocs, 2112 B |

Reading the numbers:

- **Dynamic vs static at n=2: 2.5x (57 ns absolute).** Irrelevant next to
  any real work an alts! arm does; and static `alt!` avoids it entirely.
- **Cost is linear**: ~48 ns + ~140 B per additional case (case struct +
  reflect's internal runtimeSelect + recv boxing). No cliff.
- **Crossover pain**: fan-in overtakes `Alts` at n≥8 on raw ns/op (305 vs
  359) and is ~4.5x faster at n=32 — but it is **not an alts! substitute**:
  the pump goroutines *consume* values from source channels even when no
  alts! caller is waiting (breaks channel semantics for other takers),
  can't express write ops or per-op port identity without extra plumbing,
  and holds n goroutines alive per call site (the leak-tracking burden doc
  05 tells us to avoid). It's only viable as a user-level `merge`/`mult`
  pattern, which core.async already exposes separately.
- Allocation churn only matters for tight `:default` polling loops over
  large port vectors (2 KB/op at n=8). If that ever shows up in a profile,
  the emitter can cache the `[]reflect.SelectCase` when the port vector is
  loop-invariant — noted as an optional performance-ladder rung, not needed
  now.

## Recommendation

Adopt exactly the architecture's split (doc 00 §4.7 / doc 05 §4 table):

1. **Static `alt!` → emit a real `select`** — 0 allocs, 37 ns, and it's the
   common case in compiled code.
2. **Dynamic `alts!` → this `Alts` on `reflect.Select`** — semantics match
   core.async (random fairness verified, `:priority`, `:default`, timeout,
   closed-channel normalization), cost is linear and small. let-go's
   "carries over nearly verbatim" claim holds.
3. Fold the closed-send recover shim into the future channel-wrapper work
   (buffer-policy side-table) rather than building anything new for it.
4. Standardize the M4 async conformance suite on `testing/synctest` — it
   handled every case here deterministically, including `reflect.Select`
   blocking, at ~zero wall-clock cost.

Nothing in the numbers forces a different mechanism.
