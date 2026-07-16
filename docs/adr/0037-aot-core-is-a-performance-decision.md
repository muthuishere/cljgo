# ADR 0037 — AOT-`core.clj` is a performance decision, and multi-namespace emission gates it
Date: 2026-07-16 · Status: proposed
Supersedes: **ADR 0023 decision #2** (framing only; #1 "strip by default" stands)
Evidence: spikes **S19** (`spikes/s19-aot-core-perf/VERDICT.md`), **S20**
(`spikes/s20-aot-core-prize/VERDICT.md`)

## Context

ADR 0023 #2 named AOT-compiling `core.clj` "the structural fix" for **binary
size** (6.6MB → ~2MB), with startup (ADR 0019) as a bundled benefit. That
framing set the priority: an M5 item, "the single biggest lever" for size.

S19 and S20 measured it. The framing was wrong in a way that mattered — it
undersold the change and pointed the perf gates at the wrong path.

**S19.** `pkg/emit/program.go:172` emits `main` as `rt.Boot(); Load()`.
`rt.Boot()` calls `eval.New()`, which tree-walks the embedded `core.clj` + 12
`.cljg` files (2980 lines) on **every startup**. Emitted code reaches
`clojure.core` only via `lang.InternVarName(...).Get()`, and `InternVarName`
creates an *unbound* var — the value is bound at runtime by the interpreter.
So an emitted binary is a native Go program whose standard library is a set of
interpreted closures rebuilt from source on each run. Measured, same binary,
same machine:

| | AOT binary | interpreted | speedup from compiling |
|---|---|---|---|
| `fib` — work in **user** code | 993.6 ms | 9683 ms | **9.74×** |
| `reduce` — work in **`clojure.core`** | 701.5 ms | 700.0 ms | **1.00×** |

`cljgo build` compiles the user's forms and does **nothing** for `clojure.core`.
On let-go's own benchmark suite (7 files, unmodified), every runtime installed
and measured on one machine, the consequence splits cleanly:

| Benchmark | cljgo | let-go | babashka | joker | clojure JVM |
|---|---|---|---|---|---|
| `tak` | 921.9 ms | 1.26 s | 1.14 s | 12.40 s | **492.0 ms** |
| `fib` | 961.6 ms | 1.15 s | 1.17 s | 13.16 s | **442.9 ms** |
| `transducers` | 171.8 ms | 27.9 ms | **15.7 ms** | — | 355.2 ms |
| `reduce` | 719.3 ms | 45.6 ms | **22.6 ms** | 1.48 s | 308.6 ms |

We win `tak`/`fib` — fastest here bar the JVM, ahead of a bytecode VM and a
GraalVM native image — and lose every `clojure.core`-routed benchmark, `reduce`
by 15.8× to let-go and **31.8× to babashka**.

**joker is the control.** It is the other Go tree-walk interpreter. On `fib` we
are 13.7× *ahead* of it (we are a compiler); on `reduce` we are 2.1× ahead of
it and 15.8× behind let-go (we are an interpreter). Same binary, same run. A
third-party implementation confirms the §1 A/B and rules out "`pkg/lang` is
just slow" — if it were, `fib` would be slow too.

**S20.** Quantified the prize on `reduce`'s real shape rather than
extrapolating from `fib` (which rides `rt.Add2`/`Sub2` intrinsics, ADR 0004):

| cause of the 15.79× `reduce` gap | cost | share | fix |
|---|---|---|---|
| `clojure.core` interpreted | 578 ms | **~86%** | this ADR |
| `core.clj` boot | 29.8 ms | ~5% | same edge |
| `pkg/lang` — boxing, `IFn` dispatch, seqs | 28.7 ms | ~4% | doc 04 §5 ladder |

Compiled `my-reduce` vs interpreted `clojure.core/reduce`: **5.83×**.

## Decision

1. **Reframe: AOT-`core.clj` is a performance decision.** The priority order is
   **performance** (5.83× on every `clojure.core` call path) → startup
   (~2 ms floor vs 29.8 ms) → RSS → size (~2 MB). One edge, four wins. ADR
   0023 #2's size framing is superseded; its measurements stand.

2. **It is not a parity play, and we will not market it as one.** The endpoint
   is ~2.26× of let-go on `reduce`, not 1.00×. The residual ~1.76× is
   `pkg/lang` + emitter and belongs to the doc 04 §5 ladder — which
   `pkg/emit/perf_test.go` already tracks at ~35× against the §1.4 ~10× budget.
   Cost this work against **5.83×, not 9.74×**.

3. **Multi-namespace emission is a prerequisite and gets its own ADR + spec.**
   `EmitMain` emits one `package main` (`pkg/emit/program.go:59`); `core` spans
   7+ namespaces (`clojure.core`, `.string`, `.set`, `.edn`, `.test`, `.repl`,
   `cljgo.build`, `clojure.core-test.portability`). Per-package `Load()`
   chaining is designed (design/04:39-79, "v0.5") and unbuilt. **It lands
   first, as its own change.** This ADR does not schedule AOT-core behind an
   unbuilt prerequisite in the same spec.

4. **The migration is all-or-nothing; do not schedule it incrementally.** A
   binary with AOT `core.clj` but interpreted `predicates.cljg` still links the
   reader + analyzer, so the linker drops nothing and the change measures as
   **zero**. `core.clj` + all 12 `.cljg` files move together or not at all.

5. **Add a `clojure.core`-mediated perf gate, before the migration, not after.**
   `pkg/emit/perf_test.go` benchmarks emitted-vs-handwritten factorial —
   user-code-only, the one path that already worked. It is structurally blind
   to the entire S19/S20 finding: a 16.54× regression against a competitor was
   invisible to a green CI. A gate derived from a `clojure.core`-heavy workload
   (`reduce` over a large `range` is the obvious seed) must exist first, or we
   cannot prove the migration worked.

6. **Approach for the `pkg/eval` → `pkg/lang` builtin move is deferred to the
   spec**, with two candidates: (a) move the ~292 builtins into `pkg/lang`, or
   (b) a `pkg/lang`-side intern path that `pkg/eval` registers into. (a) is
   mechanical but large and settles #7 below; (b) is smaller but may fork
   REPL/AOT semantics — which design/00 §2 forbids.

7. **`pkg/emit/rt` must stop importing `pkg/eval`** (`rt.go:26`) — the interop
   (`CallMethod`/`FieldGet`/`FieldSet`/`MakeStruct`/`NewStruct`, `:243-273`) and
   exception (`Throw`/`Recover`/`CatchMatches`, `:284-293`) delegations exist
   deliberately, to keep interpreter and AOT byte-identical. Severing them must
   preserve that; one implementation, two callers, not two implementations.

8. **Retire design/04 §7's "no `binding`/dynamic vars" non-goal.** It is stale:
   `OpDynBind` already emits flat `lang.PushThreadBindings`/`PopThreadBindings`
   (`emit.go:663-681`) and `hoistVar` re-marks `:dynamic` (`emit.go:147-150`).

9. **Macros in AOT binaries follow the ClojureScript split** and this is not a
   blocker. `core.clj`'s 45 `defmacro`s emit fine; their *values* need an
   analyzer to be useful, and a binary does not expand macros at runtime. The
   REPL keeps `eval.New()`. `core.clj` calls no `eval`/`resolve`/`require`/
   `load`/`intern` (verified), so there is no runtime-eval dependency to break.

## Consequences

- **The headline number changes.** "AOT-core → 2MB binaries" becomes "AOT-core
  → 5.83× on every `clojure.core` path, plus ~2 ms startup, plus ~2 MB". Size
  is the third prize.
- **M5's ordering changes.** Multi-namespace emission is promoted out of
  "v0.5" into a prerequisite milestone with its own ADR.
- **We ship a known gap.** Until this lands, `cljgo build` is honestly
  described as "compiles your code; `clojure.core` stays interpreted". README
  and site now say so, including the benchmarks we lose.
- **CI gets a gate that can see the whole program**, not just emitted user code.
- **Op coverage is not at risk** — all 27 `pkg/ast` ops already emit; the
  blockers are structural (namespaces, builtin location, `rt`→`eval` edges).
- **Risk: the residual.** If the doc 04 §5 ladder never closes the remaining
  ~1.76×, cljgo ends up a compiler that beats a bytecode VM on user code and
  matches it on library code. That is a good outcome and should be stated as
  the target rather than discovered as a disappointment.

## Alternatives rejected

- **Tune GC at boot.** `GOGC=off` takes boot 21.8 → 17.4 ms (~20%). A
  palliative on ~5% of the problem; deletes no allocation. Rejected as a fix,
  fine as an unrelated micro-optimization.
- **Optimize the tree-walker.** ADR 0034 already took boot 8.9× (211 → 23.7 ms)
  and `reduce` is still 16.54× off. Interpreting `clojure.core` at all is the
  defect; making the interpreter faster does not address it.
- **Precompile `core.clj` to a serialized AST/bytecode** rather than Go source.
  Removes read+analyze (~94% of boot allocations) but keeps the tree-walk, so
  it collects the startup/size prize and forfeits the 5.83× — the largest one.
  Rejected: it optimizes for the framing this ADR supersedes.
