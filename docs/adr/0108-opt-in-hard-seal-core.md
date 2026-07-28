# ADR 0108 — `--seal-core`: the hard seal of core arithmetic, opt-in and NOT default

Date: 2026-07-27 · Status: **accepted — opt-in flag only** · Refines: ADR 0066
(dirty-flag guard elision), ADR 0004 (per-call var deref), ADR 0067 (int64
regions)

## Context

ADR 0066 shipped the **safe** half of sealed-core arithmetic — a process-global
dirty flag, so a direct 2-arg call to `+ - * / < > = <= >=` pays one relaxed
`atomic.Bool` load instead of a var deref plus an interface-compare, while
still observing a `with-redefs`/`def`/`alter-var-root` of the operator. It
explicitly parked the faster **hard seal** (alternative 1) as an owner-gated
question: emit the int64 op with no var reference and no flag check at all.

Hard seal is *more* JVM-conformant — JVM Clojure's `:inline` on `+` compiles a
direct 2-arg site to `Numbers.add` and never consults the var, so the canonical
program answers `[7 7 7]` on the JVM and `[7 12 7]` on cljgo — but a
redefinition genuinely stops being observed at those sites, which contradicts
ADR 0004's letter and changes cljgo's own observable behavior.

## Decision

Implement hard seal as **`cljgo build --seal-core`** (also
`emit.Options.SealCore` and the `CLJGO_SEAL_CORE=1` environment seam), **off by
default**, and keep it off.

- **Flag off** — the emitter is byte-for-byte what it was: `rt.Add2(v, x, y)`,
  `rt.LTBool(v, x, y)`, and `if !rt.CoreDirty() {` region entries. Verified by
  diffing the generated `main.go` of a base-commit build against a
  flag-off build of this branch: identical.
- **Flag on** — arithmetic call sites emit the new unguarded twins
  (`rt.Add2S(x, y)`, `rt.LTBoolS(x, y)`, … in `pkg/emit/rt/seal.go`), the
  operator vars are not hoisted at all, and the ADR 0067 typed regions drop
  their `!rt.CoreDirty()` condition (the dual-emitted loop drops its now
  unreachable boxed arm entirely). Semantics for every non-redefined input are
  identical, including the overflow throw and its frozen message.
- Scope is the program being emitted. The pre-compiled core (`pkg/coreaot`) and
  `pkg/briaot` are committed Go and are unaffected either way.

## Consequences — the measurement that decides the ruling

All numbers darwin/arm64, Apple M5 Pro, go1.26, branch `perf/seal-core-flag`
off base `c51fb41`. Benchmark sources are committed:
`pkg/emit/seal_bench_test.go` (end-to-end, `CLJGO_SEAL_BENCH=1`) and
`pkg/emit/rt/seal_bench_test.go` (intrinsic micro).

**Intrinsic micro** (`go test ./pkg/emit/rt -bench 'Guard|Sealed'`, best of 3):

| helper | guarded, flag dirty | guarded, flag clean (ships today) | hard-sealed |
|---|---|---|---|
| `Add2` | 7.37 ns/op | 5.89 ns/op | **5.85 ns/op** |
| `LTBool` | 6.82 ns/op | 5.06 ns/op | **4.96 ns/op** |

**End-to-end** (`hyperfine -N -w 3 -r 20`, whole binary, startup included):

| program | base `c51fb41` | branch, flag off | branch, `--seal-core` |
|---|---|---|---|
| `fact 15` × 2M | 66.0 ± 1.7 ms | 66.0 ± 1.3 ms | 64.8 ± 2.1 ms (1.02× ± 0.04) |
| float accumulate × 5M (boxed `rt.Add2` per op) | 71.2 ± 2.3 ms | 70.8 ± 0.9 ms | 71.8 ± 1.2 ms (0.99×) |

Emitted-vs-handwritten-Go ratio (`pkg/emit/seal_bench_test.go`, startup factored
out): `fact` 5.35× off → 5.63× on; float 20.1× off → 20.5× on — i.e. inside the
run-to-run noise of that harness, in both directions. Stripped binary size is
6,684,722 B in every mode (identical).

**Ruling data: hard seal buys essentially nothing over the shipped dirty flag —
~1% on `Add2`, ~2% on `LTBool`, and nothing distinguishable end-to-end.** ADR
0066 already captured the win; the remaining `atomic.Bool.Load` is a single
acquire load off a hot read-only cache line and is not a measurable cost. The
recommendation to the owner is therefore: **do not make hard seal the default**
— the observability loss is real and the speed it would buy is not. The flag
exists for someone who wants the JVM's inlining *semantics*, not for speed.

### Behavioral matrix (frozen by `pkg/emit/guard_seal_test.go`)

```
(def normal   (+ 3 4))
(def redefd   (with-redefs [+ (fn [a b] (* a b))] (+ 3 4)))
(def restored (+ 3 4))
```

| leg | result |
|---|---|
| interpreted (`cljgo run`) — always fully live | `[7 12 7]` |
| compiled, default | `[7 12 7]` |
| compiled, `--seal-core` | `[7 7 7]` |
| real Clojure 1.12.5 (oracle) | `[7 7 7]` |

Under `--seal-core` the compiled leg intentionally diverges from the REPL. The
dual-harness rule (ADR 0002/0007) is *not* relaxed: it binds the default build,
which stays identical in both legs and is what conformance runs. The flag's
divergence is opt-in, frozen in a test, and moves the binary **toward** the JVM
oracle. `--seal-core` must never be turned on for a conformance run.

## Risks

- A user who opts in and later relies on `with-redefs` of core arithmetic (a
  test double for `+`) gets silently different behavior at direct 2-arg sites.
  Mitigation: it is off by default, `cljgo build --help` states the cost in one
  line, and the flag name says what it does. It is also exactly the trap JVM
  Clojure users already live with.
- Two families of intrinsics now exist in `pkg/emit/rt` and must not drift. The
  bodies are duplicated deliberately (delegating would perturb the default
  path's inlining); `TestSealedMatchesGuarded` pins them to identical results
  on shared inputs.

## Alternatives

1. **Make hard seal the default** (with an `^:redefinable` opt-out). Rejected on
   the measurement above: it changes observable behavior for no measurable
   speed.
2. **Build-tag the `rt` package** instead of emitting different call sites.
   Rejected: a build tag on a shared runtime package would silently change the
   semantics of every binary built in that workspace, including the AOT core.
3. **Do nothing / leave hard seal unimplemented.** Rejected: the owner asked for
   the number, and the number only exists once the lever is built.
