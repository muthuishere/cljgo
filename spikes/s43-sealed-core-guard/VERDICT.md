# Spike s43 — Sealed core arithmetic guard elision — VERDICT

Status: CLOSED · 2026-07-23 · darwin/arm64 · feeds **ADR 0066** (proposed)

## Question

Every core arithmetic intrinsic (`rt.Add2`/`LT2`/…) pays a per-call var deref
(`Var.Get`, ~10% CPU) + interface-compare (`efaceeq`, ~8%) for ADR 0004
redefinition liveness. Can we elide both under a sealing assumption while keeping
the liveness escape hatch correct?

## Exit criteria — answered

### 1. Prototype guard elision under a sealing assumption — DONE

Mechanism chosen: **process-global monotonic dirty flag** (`lang.CoreArithDirty
atomic.Bool`), not a build flag. `rt.Boot` seals `+ - * / < > =` after the
pristine snapshot; `(*Var).BindRoot`/`AlterRoot` trip the flag when a sealed var
moves. Each intrinsic checks the flag once:
- flag false → open-code int64 directly (no deref, no compare);
- flag true → original guarded path (deref + `efaceeq` + `lang.Apply2`).

Files: `pkg/lang/var.go` (flag, `sealed` field, `Seal()`, `tripIfSealed()` in
BindRoot/AlterRoot), `pkg/emit/rt/rt.go` (all 10 intrinsics gated, Boot seals).

### 2. Measured — DONE

| Workload | Guarded (before) | Elided (after) | Δ |
|---|---|---|---|
| `Add2` microbench | 7.92 ns/op | 6.27 ns/op | **-21%** |
| `LTBool` microbench | 6.76 ns/op | 5.18 ns/op | **-23%** |
| factorial `(fact 15)`×2M, net emitted work | 443.7 ms | 325.2 ms | **-27%** |
| factorial ratio vs raw Go | 38.0× | 31.6× | -16.8% |
| `(reduce + (range 2e6))` | ~77 ms | ~77 ms | **unchanged** |

`reduce` is unchanged because it invokes `+` as an IFn *value*, never through
`Add2` — the elision only touches direct 2-arg call sites. Honest scope note.

**pprof** (`guard_bench_test.go`, `-cpuprofile`): guarded profile top frames
include `(*Var).Get` (4.0% cum), `(*Var).getRoot` (2.8% flat), `runtime.efaceeq`
(1.8% flat). Elided profile: **all three gone**, only `rt.Add2` remains. The
guard frames shrink to zero exactly as predicted.

### 3. Liveness escape hatch proven — DONE (make-or-break)

`TestSealedGuardWithRedefsEscapeHatch` (pkg/emit): `(with-redefs [+ (fn [a b]
(* a b))] (+ 3 4))` returns **12** (redefinition seen via the fallback), restores
to 7 after the form, **REPL == compiled binary** byte-for-byte. Dual-harness and
full `go test ./...` + conformance stayed **green**.

## The tension, surfaced honestly

ADR 0004 mandates per-call deref so redefinition is seen immediately. The
dirty-flag **refines** this (deref only after the first redefinition trips the
flag) without changing observable liveness — the mutation trips the flag before
the next call reads it.

**Key finding (measured, not assumed):** JVM Clojure 1.12.5 does **not** see a
`with-redefs`/`alter-var-root` of `+` at a direct 2-arg call site at all —
`:inline` emits `Numbers.add` at compile time. `(with-redefs [+ …] (+ 3 4))` ⇒ 7
on the JVM; cljgo returns 12. So cljgo is *more live than the JVM*, and the
liveness ADR 0004 pays for on every call is liveness the JVM does not provide.
This is pre-existing; the dirty flag preserves it.

## Recommendation

**Ship the dirty flag now** (this prototype): a real ~21–27% win on
arithmetic-dense code, zero behavior change, full liveness retained, gates green.
It is the correct-and-safe default.

**Escalate to owner:** the **hard-seal** alternative (no flag, no fallback — emit
the bare int64 op, never see a runtime redefinition at inlined sites) is *faster
still* AND *more JVM-conformant* (`[7 7 7]`, matching `:inline`). It changes
cljgo's current `[7 12 7]` behavior and contradicts ADR 0004's letter, so it is
owner-gated — a `--seal-core` release flag or a default-with-opt-out. Given the
JVM evidence, hard-seal is defensible and the larger prize; the dirty flag is the
no-regret step that also keeps the escape hatch if the owner wants to retain
cljgo's extra liveness.

## Un-proven risks

- Only `BindRoot`/`AlterRoot` write a var root today (grep-confirmed). A future
  third root-writer must call `tripIfSealed()` or the trip is silently missed.
- `atomic.Bool.Load` is cheap but non-zero; hard-seal removes even it.
- Numbers are single-machine (darwin/arm64); CI runners will differ in absolute
  ms but the *ratio* of guarded:elided should hold (the elided path is strictly
  less work).
