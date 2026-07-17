# 10 — Debt & blind-spot audit (2026-07-17)

Draft v1. A systematic sweep for the *class* of defect that S22/S24 exposed —
not one bug, but four recurring patterns. Each finding cites evidence; perf
numbers are hyperfine means on Apple M5 Pro, go1.26.3, against let-go v1.11.1
built from source on the same machine.

The four patterns:

- **P1 — hot fn on the wrong side of the native line** (the `reduce` mistake)
- **P2 — temporary decision nobody revisited** (the `v0; AOT core is M5` mistake)
- **P3 — gate pointed at the working path** (the `perf_test.go` mistake)
- **P4 — published number nobody reconciles** (the `<10 ms startup` mistake)

---

## P1 — Hot core fns still interpreted (measured, ranked)

With S24's native `reduce` applied, the remaining idiom gaps isolate the next
offenders. "Residual" = startup-subtracted gap vs let-go after native reduce.
Every named fn is interpreted `core.clj`; none is a builtin today.

| idiom | HEAD | + native `reduce` | let-go | residual | blames (core.clj line) |
|---|---|---|---|---|---|
| `frequencies` (1e6) | 2438.8 ms | 1877.6 ms | 189.6 ms | **10.0×** | `frequencies` :943 — also blocked on missing HAMT transients (P2-3) |
| `map`+sum (1e6) | 1481.7 ms | 892.9 ms | 95.0 ms | **9.6×** | `map` :493 |
| `mapv`+count (1e6) | 915.7 ms | 930.8 ms | 100.1 ms | **9.5×** | `mapv` :673 — did NOT improve with native reduce; its own loop |
| `comp` per element (1e6) | 2027.3 ms | 1145.2 ms | 134.5 ms | **8.6×** | `comp` :623 |
| `filter`+sum (1e6) | 1773.5 ms | 1294.3 ms | 200.2 ms | **6.5×** | `filter` :520 |
| `group-by` (1e5) | 200.8 ms | 147.3 ms | 36.5 ms | 3.7× | `group-by` :947 |
| `into []` (1e6) | 681.2 ms | 185.7 ms | 50.0 ms | 3.5× | mostly fixed by native reduce; rest is transients |
| `update` loop (1e5) | 144.1 ms | — | 96.2 ms | 1.3× | fine |
| `concat`+count (1e6) | 376.3 ms | — | 337.8 ms | 1.0× | fine (but see P2-5: eager) |
| `apply str` (1e5) | 35.8 ms | — | 11.8 ms | 0.9× | fine |

**ADR 0039 candidate order confirmed by measurement**: `map`, `filter`,
`mapv`, `comp` (and the transducer arities), then `frequencies`/`group-by` —
which likely collapse once `map`/`reduce`/transients land, so re-measure
before moving them (the 0038 discipline: measurement names the fn, one per
PR, JVM-oracle-gated).

## P2 — Temporary decisions whose revisit condition has arrived

1. **HAMT transients missing** (`pkg/lang/TODO.md` S4 defect #2; also
   `set.go:197`, `map.go:14`). Deferred since M0, "3–5 days, no design risk,
   ~4–6× on into-style builds." With `reduce` native, this is now the named
   blocker under `frequencies`/`into`/`group-by`. **Next after the P1 list.**
2. **`concat` is eager** (`pkg/eval/builtins.go:247` — "revisit with the seq
   library (M5)"). M5-era work has shipped; eager concat is a semantic
   divergence from Clojure, not just perf.
3. **`doseq`/`for` lack `:when`/`:let`/`:while`** (`core/core.clj:1197,1213`
   "v0"). Everyday Clojure; direct suite-coverage cost.
4. **STM `commute` is a fake** (`pkg/lang/ref.go:48` "not concurrency-safe.
   nor is it correct for transactions"). Silent wrong-answers under
   concurrency — worse than unimplemented. Should either land or loudly
   throw.
5. **`-illegal-argument` stopgap "until throw lands"** — `throw` landed
   (`pkg/eval/builtins.go:593` vs OpThrow in `pkg/ast`). Trivial removal.
6. **`int64Ops.AddP` bare TODO** (`pkg/lang/numberops.go:214`) — survived two
   numeric-tower ADRs (0029/0032).
7. **design/04 §7 non-goal fence stale on 3 of 4 items** ("no binding, no
   lazy seqs, no macros" — all exist). ADR 0037 retires one clause; the
   fence text still stands.
8. **design/09 still sells AOT-core as a size fix** (:77,121,187) —
   superseded-in-fact by ADR 0037/0038; never re-pointed.
9. Lower tier: multifn preference placeholder (`multifn.go:136`), chan panic
   policy v0 (`chan.go:129`), var CAS/validator TODOs (`var.go:81`), `#inst`
   passthrough (`reader/tagged.go:40`), missing character literals
   (TODO.md).

## P3 — Gates that cannot see the failure they exist to catch

1. **CI budget values are set to never fire**: boot budget local 250 ms → CI
   **5 s** (`ci.yml:73`); perf ratio local 60× (measured ~35×) → CI **120×**
   (`ci.yml:77`); even `boot-bench.yml` uses 10 s. A 16× boot regression or a
   3× emitted-code regression merges green. ADR 0024's own text warns that
   neutering the gate on CI is equivalent to deleting it.
2. **No `clojure.core`-mediated perf gate exists** — `perf_test.go` measures
   user-code factorial only; the entire S22 finding was invisible to it.
   (ADR 0037 decision #5; should land with the first ADR 0039 fn.)
3. **The suite ratchet is specified as build-failing and runs nowhere**
   (design/08:115, ADR 0022:19 vs zero workflow references to `cljgo suite`).
4. **`Benchmark*` funcs never run in CI** (no `-bench` outside the manual
   boot-bench workflow, which has no assertion) — despite design/00 §1.4
   "benchmarked in CI, not vibes" and ADR 0004 "benchmarks live in CI".
5. **`expect-error` conformance files are silently exempt from the compiled
   harness** (`compiled_test.go:57` skips instead of failing when no written
   waiver exists) — a trusted class the gate never checks.
6. **What actually holds** (verified, for fairness): the dual-harness
   eval-vs-compiled byte-identity gate is real and runs in CI
   (`conformance/compiled_test.go:30`); waivers are in-file with required
   reasons; design/09 is honest that the ratchet is "partial".

## P4 — Published numbers that disagree with each other right now

1. **The ratchet's own artifact says 34/242** (`compat/.../scoreboard.json`:
   34 pass, 118 skipped — 2026-07-15 Batch-0) while the README badge said
   217/242, PR #40 proposed 234/242 (96.7%), and this branch shipped 89.7%.
   Four numbers lived in the tree at once; a wired ratchet would pin ≥ 34 and
   be useless. **Regenerate the scoreboard in the same commit as any headline
   change — or better, make CI regenerate it (fixes P3-3 simultaneously).**
2. **Var-resolution is published three ways**: 99.2% (README), 51.2%
   (design/08:184, ADR 0022:65), 65.7% (site, pre-branch). Same metric.
3. **This branch's own headline (89.7%) was already stale** when written —
   fixed at merge (2026-07-17): README/site/badge now publish **238/242
   (98.3%)**, re-measured against the upstream suite @164a4b3, and PR #40's
   competing 96.7% was closed rather than merged.

   The pattern bit twice more on the way, both worth recording. **(a)** The
   96.7%/234 figure was itself never reproducible — nobody re-ran it before
   proposing to publish it. **(b)** A "242/242 = 100%" reading briefly reached
   a PR body because the shared `clojure-test-suite` checkout was sitting on a
   local `cljgo-dialect` branch that adds the missing `:cljgo` reader
   conditionals. That number is real and defensible — but only with the
   checkout named, and those branches are not upstreamed. **The suite checkout
   is shared mutable state and another agent can move it under you: name the
   commit you measured (`--dir`, and record the SHA) or the number means
   nothing.** This is P4 with a moving denominator, and it is the sharpest
   version of the pattern in this document.
4. ADR 0023 status line still says "proposed" for a decision that ADR 0037
   superseded in framing — the AOT-core decision is spread across three ADRs,
   two unratified.

---

## The meta-lesson (why one process, not four patches)

All four patterns are the same failure: **a fact was established once
(correctly), encoded somewhere static (a doc, a gate value, a `v0` comment,
a scoreboard), and nothing re-derives it.** The fix that generalizes:

1. Numbers that appear in public (README/site/badges) must be generated from
   the same artifact CI checks (scoreboard.json), never hand-edited.
2. Every `v0`/"for now" comment must name its revisit condition; grep-able
   (`REVISIT(condition):`) so an audit like this one is mechanical.
3. A gate's CI value may not exceed its local default by more than ~2× without
   an ADR saying why (ADR 0024 covers host variance; 20–40× is not variance).
4. Suite + scoreboard regeneration + the core-mediated perf benchmark run in
   CI on every merge — one workflow addition closes P3-1/2/3/4 and P4-1 at
   once.
