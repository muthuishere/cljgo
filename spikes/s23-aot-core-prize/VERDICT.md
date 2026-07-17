# Spike S23 verdict — AOT-core buys ~86% of the `reduce` gap; the rest is the runtime

Closed 2026-07-16. Feeds **ADR 0037** (reserved) with S22.

**Exit criterion Part A: MET**, on the "prize is real and large" branch —
compiled `my-reduce` is **5.83×** faster than interpreted `clojure.core/reduce`,
against a 3× threshold. **Part B: MET** — inventory in §3.

**But the honest headline is narrower than S22 implied:** AOT-core does not win
`reduce`, it closes most of it. A residual ~1.8× belongs to `pkg/lang`, and
S22's framing did not see that. This spike exists because extrapolating the
prize from S22's 9.74× `fib` number would have been wrong — `fib` rides `rt`'s
arithmetic intrinsics; `reduce` is seq traversal and megamorphic dispatch.

## 1. The prize, measured

Identical input, same machine. Each row moves one more thing from interpreted
to compiled. Totals include cljgo's 29.8 ms / let-go's 4.9 ms startup:

| variant | total | compute | vs let-go |
|---|---|---|---|
| `clojure.core/reduce` — today | 674.2 ms ± 10.9 | 644.4 ms | **15.79×** |
| compiled `my-reduce`, interpreted `core/range` | 118.6 ms ± 4.2 | 88.8 ms | 2.78× |
| compiled reduce **and** range (no interpreted core in the hot loop) | 96.3 ms ± 2.5 | 66.5 ms | 2.26× |
| **let-go** | **42.7 ms ± 5.1** | 37.8 ms | 1.00× |

Decomposition of the 15.79× gap:

| cause | cost | share | fix |
|---|---|---|---|
| `clojure.core` interpreted | 578 ms | **~86%** | AOT-core (ADR 0037) |
| boot of `core.clj` | 29.8 ms | ~5% | same edge |
| `pkg/lang` runtime — boxing, `IFn` dispatch, seq machinery | 28.7 ms | ~4% | doc 04 §5 ladder |

**AOT-core takes `reduce` from 15.79× to ~2.26× of let-go — a 7× improvement
that still loses.** Anyone selling AOT-core as "we beat let-go" is overselling
it; it converts a catastrophic loss into a respectable one, and it is still by
a wide margin the best change available.

The residual is real and should not be silently attributed to AOT-core: even
with zero interpreted code in the hot loop, we are 1.76× slower than a bytecode
VM on compute. That is the `pkg/lang`/emitter axis, and it is the §1.4 "~10×
handwritten Go" ladder that `pkg/emit/perf_test.go` already tracks at ~35×.

## 2. Why the prize is smaller than S22's 9.74×

S22's `fib` number is the best case: arithmetic through `rt.Add2`/`Sub2`'s
guarded intrinsics (ADR 0004), monomorphic, no allocation. `reduce` is seq
traversal — `first`/`next` allocate, `f` is a megamorphic `IFn`, values box.
Compiling removes the tree-walk overhead but not the runtime's per-element
cost. **5.83×, not 9.74×, is the number ADR 0037 should be costed against.**

## 3. Part B — feasibility inventory

What must be severed for the linker to drop `pkg/eval` from an emitted binary.
Op coverage is **not** among them: all 27 `pkg/ast` ops already have emit cases
(`pkg/emit/emit.go:709-711`'s `default:` is the fence).

| # | edge | location | kind |
|---|---|---|---|
| 1 | ~292 Go builtins live in `pkg/eval`, not `pkg/lang`; every core leaf bottoms out on them | `pkg/eval/builtins.go` + 20 `*_builtins.go` | mechanical, large — **this is ADR 0023's "rt → eval decoupling"** |
| 2 | `pkg/emit/rt` imports `pkg/eval` for interop + exceptions, deliberately, for byte-identical semantics | `rt.go:26`, `:243-273` (`CallMethod`/`FieldGet`/`FieldSet`/`MakeStruct`/`NewStruct`), `:284-293` (`Throw`/`Recover`/`CatchMatches`) | **design question** — moving these must not fork REPL/AOT semantics (design/00 §2) |
| 3 | Multi-namespace emission does not exist — `EmitMain` emits one `package main` | `pkg/emit/program.go:59`, `:164-182` | **design question, hard prerequisite** — core spans 7+ namespaces; per-package `Load()` chaining is designed at design/04:39-79, scheduled "v0.5", unbuilt |
| 4 | `installDefmacro` is a hand-built Go seed with no source form to emit | `pkg/eval/macro.go:143-147` | design question — needs an equivalent seed in the emitted package or `pkg/lang` |
| 5 | The 12 `.cljg` files must come along; a binary with AOT `core.clj` but interpreted `predicates.cljg` still links reader+analyzer → **DCE win is zero** | `pkg/eval/eval.go:51-62` | mechanical, all-or-nothing |
| 6 | `protocols.cljg` dispatches through `pkg/eval/protocols.go` | `core/protocols.cljg` | design question — design/00 M5's `deftype`→struct / `defprotocol`→interface |
| 7 | 45 `defmacro`s in `core.clj` emit fine, but macro *values* need an analyzer to be useful | `core/core.clj:22-1317` | **not a blocker** — the ClojureScript split: binaries don't expand macros at runtime; the REPL keeps `eval.New()` |

Two findings that shrink the job:

- **`binding`/dynamic vars are already fine.** `OpDynBind` emits flat
  `lang.PushThreadBindings`/`PopThreadBindings` (`emit.go:663-681`) and
  `hoistVar` re-marks `:dynamic` (`emit.go:147-150`). design/04 §7's
  "no `binding`" non-goal is **stale** and ADR 0037 should retire it.
- **`core.clj` calls no `eval`/`resolve`/`require`/`load`/`intern`** — grepped.
  Those are builtins it defines around, not uses. No runtime-eval dependency.

**Sequencing consequence:** #3 (multi-namespace emission) gates everything, and
#5 means there is no incremental payoff — a half-migrated core links the
interpreter anyway and measures as zero. ADR 0037 cannot schedule this as
"AOT `core.clj` first, `.cljg` later"; the DCE win is all-or-nothing.

## 4. What ADR 0037 must decide

1. **Approach for #1** — move builtins to `pkg/lang`, or a `pkg/lang`-side
   intern path? Determines whether #2 is mechanical.
2. **Multi-namespace emission (#3) is its own milestone** and should probably
   be its own ADR + spec, landed before AOT-core.
3. **Cost against 5.83×, not 9.74×**, and state plainly that the endpoint is
   ~2.26× of let-go on `reduce`, not parity. Parity needs the §5 ladder too.
4. **Add a `clojure.core`-mediated perf gate.** `pkg/emit/perf_test.go`
   measures emitted-vs-handwritten factorial — user-code-only, the one path
   that already works. It is structurally blind to the entire S22/S23 finding.
   A suite-derived gate would have caught this in CI.

## Verdict: **proceed, with corrected expectations.**

AOT-core is the highest-value change available (86% of the largest gap, plus
startup + RSS + size). It is **not** a let-go-parity play, and #3/#5 mean it is
a milestone, not a patch. Recommend **ADR 0037** ratify the reframing and
schedule multi-namespace emission first.

## Files

- `README.md` — question + exit criterion, written before any code.
- `results/prize.json` — compiled vs interpreted `reduce` (5.83×).
- `results/upper-bound.json` — the 4-way ladder to let-go.

No `go.mod` — measurement only; this spike wrote no `pkg/` code, per ADR 0027
("spike code NEVER merges into pkg/; it only informs").
