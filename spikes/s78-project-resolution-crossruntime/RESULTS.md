# s78 — where project resolution belongs: cljgo vs let-go vs Glojure vs JVM Clojure

**Question.** Issue #185: `cljgo repl` / `cljgo nrepl` booted evaluators with no
project source roots, because resolution lives at each CALL SITE and only some
sites called it. The proposed fix (applied, uncommitted, `cmd/cljgo/main.go`)
hoists ONE resolution into `run()` before the subcommand switch, behind a
deny-list. Three other Clojure runtimes already hoist. **What does hoisting
COST, and does anyone else's shape buy measurably better startup?**

**Measured 2026-08-01**, darwin/arm64, Apple M-series, go1.26.3, cljgo 0.8.5.
Harness: `hyperfine 1.20.0`, `--input /dev/null -i`, 20 runs + 3 warmups for
the Go binaries, 10 runs + 2 warmups for the JVM. All fixtures are LOCAL
paths; nothing touched the network.

Machine state: no `go test`, `go build`, `java` or benchmark process was
running during any round (checked before round 1 and between rounds). Load
average sat at 1.9–2.4 from idle background daemons (editor, agent listeners,
WindowServer at ~23% of one core); this is not a compute-idle machine, but it
is stable across all rows, which is what within-table ratios need. **Absolute
ms here are comparable only within this table.**

## Entrants, and exactly what was built

| runtime | binary | provenance |
|---|---|---|
| cljgo (patched) | `go build ./cmd/cljgo` | this worktree, `main.go` patch applied |
| cljgo (HEAD) | `git archive HEAD \| go build` | same tree at HEAD, patch absent — the A/B control |
| let-go | `go build .` (root `lg.go`) | `references/let-go` @ `0e56abd6` |
| Glojure | `go build ./cmd/glj` | `references/glojure` @ `c74bc07d`, reports v0.6.8 |
| JVM Clojure | `clojure` CLI 1.12.5.1645 | Homebrew, Temurin via jenv |

Competitor binaries were rebuilt from those trees rather than reused from
`benchmark/.build/aotcmp/` — that directory is **empty** in this worktree, so
there was nothing to reuse. Neither Go binary was built with goreleaser flags,
so **no size claim is made or implied here.**

## The workload

"Boot the evaluator, evaluate nothing, exit" — the floor every invocation pays:

- cljgo — `cljgo repl` with stdin at `/dev/null` (the #185 command itself)
- let-go — `lg -e nil`
- Glojure — `glj -e nil`, `GLJ_CLASSPATH` set to the N roots
- JVM Clojure — `clojure -M -e nil`

Plus `cljgo version`, which the patch's deny-list excludes — the control that
shows resolution is genuinely skipped.

`N` is **source roots the runtime is told about**. That is the only axis all
four share; §"Not apples-to-apples" says what each one does with it.

## The table

Mean ± σ, milliseconds. `bare` = a directory with no project file of any kind.

| N | cljgo `version` (deny-listed) | cljgo `repl` HEAD | cljgo `repl` PATCHED | let-go | Glojure | Clojure (warm cp) | Clojure (cold cp) |
|---|---|---|---|---|---|---|---|
| bare | 7.6 ± 0.5 | 47.8 ± 1.8 | 48.1 ± 1.2 | 5.3 ± 0.6 | 54.5 ± 1.5 | 279.2 ± 3.2 | — |
| 0 | 7.3 ± 0.3 | 200.1 ± 2.6 | **354.7 ± 3.3** | 5.1 ± 0.4 | 54.4 ± 1.9 | 284.8 ± 2.4 | 706.2 ± 3.5 |
| 1 | 7.9 ± 0.9 | 125.5 ± 1.8 | 202.2 ± 3.0 | 4.4 ± 0.3 | 54.8 ± 1.6 | 297.6 ± 11.0 | 710.8 ± 9.4 |
| 5 | 7.7 ± 0.4 | 126.4 ± 1.8 | 202.3 ± 2.5 | 4.8 ± 0.4 | 54.7 ± 1.2 | 294.3 ± 1.9 | 717.9 ± 7.8 |
| 20 | 7.5 ± 0.5 | 125.6 ± 1.9 | 204.1 ± 2.0 | 4.7 ± 0.3 | 53.9 ± 0.8 | 319.3 ± 4.7 | 743.1 ± 3.5 |
| 50 | 7.5 ± 0.3 | 127.7 ± 2.0 | 204.3 ± 3.4 | 4.6 ± 0.3 | 54.3 ± 1.0 | 375.6 ± 18.6 | 787.0 ± 7.0 |

### Resolution cost = row minus that runtime's OWN bare baseline

| runtime | N=1 | N=5 | N=20 | N=50 | as % of own baseline (N=50) |
|---|---|---|---|---|---|
| cljgo HEAD | +77.7 | +78.6 | +77.8 | +79.9 | **+167%** |
| cljgo PATCHED | +154.1 | +154.2 | +156.0 | +156.2 | **+325%** |
| let-go | −0.9 | −0.5 | −0.6 | −0.7 | **0%** (below noise) |
| Glojure | +0.3 | +0.2 | −0.6 | −0.2 | **0%** (below noise) |
| Clojure warm | +18.4 | +15.1 | +40.1 | +96.4 | **+35%** |
| Clojure cold | +431.6 | +438.7 | +463.9 | +507.8 | **+182%** |

## Growth as N rises

- **cljgo — FLAT.** 77.7 ms at N=1 → 79.9 ms at N=50: **0.045 ms per extra
  dependency** across 49 more. The cost is a near-pure constant, and that
  constant is `build.LoadPlan` **booting a fresh interpreter to evaluate
  `build.cljgo`**. Peak RSS confirms it: 47.4 MB bare → 57.3 MB at N=5 →
  58.3 MB at N=50 — ~10 MB for the first resolution, ~0 for the next 45 deps.
- **let-go — ZERO, not merely flat.** `buildSearchPaths()` →
  `resolver.PathsFromDepsEdn(".")` (`pkg/resolver/resolver.go:67`) is one
  `os.ReadFile` and one EDN parse producing a `[]string`. No stat, no scan, no
  evaluation per root. Roots are consulted lazily at `require` time.
- **Glojure — ZERO.** `gljmain.Main` splits `GLJ_CLASSPATH` and calls
  `runtime.AddLoadPath(os.DirFS(path))` per entry. `os.DirFS` constructs a
  struct; it performs no I/O. 54.5 ms bare vs 54.3 ms at 50 roots.
- **JVM Clojure warm — LINEAR, ~1.8 ms per source root** ((375.6−284.8)/50).
  This is the JVM opening and indexing 50 more classpath entries, not
  Clojure's doing.
- **JVM Clojure cold — LINEAR, ~1.6 ms per root on top of a ~425 ms fixed
  cost** for the external tools.deps classpath computation, which spawns its
  own JVM.

## Two findings that matter more than the table

### 1. The patch DOUBLES resolution; it does not replace it

The patch's own comment says it "removes four scattered calls in favour of
one". **It removes none.** `git diff --stat` is `59 insertions(+)`, zero
deletions. `resolveRunDeps` still runs inside `runREPL` (`main.go:223`),
`runFile` (`:265`), `nrepl.go:43`, and `bri.go:254,364` — so every command the
hoist covers that already resolved now resolves **twice**, and each call pays
a full fresh interpreter boot.

That is the entire patched-vs-HEAD delta, and it is exact: +76.7 ms at N=1,
+78.1 at N=50 — one more resolution, to the millisecond.

`cljgo repl` in a real project therefore goes **125.5 ms → 202.2 ms (+61%)**
for no behavioral change whatsoever. HEAD already resolved in the REPL
(`main.go:164` at HEAD): **issue #185 was already fixed at the call site.**
What the hoist actually buys is coverage of the commands that never called it.

### 2. `resolveRunDeps("")` visits the same directory twice — pre-existing

`resolveRunDeps` loops over `[]string{filepath.Dir(file), "."}`. When `file`
is `""` (every `repl` / `nrepl` / most `bri` call) or a bare relative filename,
`filepath.Dir` yields `"."` — **so the loop does identical work twice**, unless
an early `return` fires first. It does not fire for a project with **no
declared dependencies**, which `continue`s.

Measured discriminator, HEAD binary, dep-free project:

| invocation | `filepath.Dir(file)` | mean |
|---|---|---|
| `cljgo run tiny.clj` | `"."` — same dir twice | 196.9 ms |
| `cljgo run /tmp/s78/tiny2.clj` | elsewhere — one pass | 123.0 ms |

**74 ms of pure duplicated work**, on the shape `cljgo new` generates. It is
why the N=0 column is *slower* than N=1..50: with deps and no lock the loop
returns `ErrNoLock` on the first pass; with zero deps it goes around again.
Patched, N=0 pays this twice over: **354.7 ms to open a REPL in an empty
project.**

### 3. `cljgo build` now prints an error it then disproves

`build` is not on the deny-list, so on a project with deps and no lock the
hoisted call emits G5023 to stderr — and `cljgo build` then creates the lock
and exits 0. Reproduced on every fixture at N=1,5,20,50:

```
error: build.cljgo declares 1 dependency but has no build.lock.edn
help: run `cljgo build` once to resolve and pin them, then `cljgo run` works
cljgo build: installed .../app
EXIT=0
```

Telling a user to run the command they are running, then succeeding, is a
correctness defect the measurement turned up as a by-product.

## What the hoist actually buys, priced

For a command that did NOT resolve at HEAD, the hoist is one resolution:

| `cljgo check tiny.clj` (N=1 project) | mean |
|---|---|
| HEAD | 48.2 ± 1.3 ms |
| PATCHED | 125.2 ± 1.9 ms |

**+77.0 ms, +160%**, on a command whose whole job took 48 ms. That is the real
price list for `check`, `config`, `routes`, `test`, `dev`, `migrate`, `suite`,
`generate`, `publish`, `cache`, `dist`.

The deny-list works where it is applied: `cljgo version` is 7.5 ms inside a
50-dep project, identical to bare.

## What this EXCLUDES

- **The network, entirely.** Every dependency is a local `:path`. Maven/Clojars
  fetches, DNS, TLS: absent. These are floors, not costs.
- **Warm caches throughout** — OS page cache, Go build cache, and for the
  "warm" Clojure column a populated `.cpcache`. The cold Clojure column is the
  only one that pays a first-run penalty.
- **JVM warmup and JIT.** Every JVM number is interpreted-bytecode cold-start.
  A long-lived JVM amortises all of it; these numbers say nothing about that.
- **The external classpath computation for the three others.** cljgo, let-go
  and Glojure have no analogue of tools.deps — nothing is precomputed by a
  separate process, so there is no separate process to measure.
- **`require` itself.** Every runtime here was told about N roots and then
  asked to load nothing from them. Lazy designs (let-go, Glojure) push their
  cost to first `require`; that cost is real and is NOT in this table.
- **Allocation is peak RSS of a process, not B/op.** Go-level allocation
  attribution for `resolveRunDeps` belongs to spike **s72
  (`s72-project-resolution-cost`, in flight in this same worktree —
  `cmd/cljgo/resolvecost_bench_test.go`)**; its `BenchmarkLoadPlan` /
  `BenchmarkResolveLocked` numbers were deliberately not duplicated here, and
  it was not run so as not to contend with this session.

## Where this is NOT apples-to-apples — read this before the numbers

This is the honest core of the comparison. **The four runtimes are not doing
the same amount of work, and the table cannot be read as a ranking.**

1. **cljgo EVALUATES its manifest; nobody else does.** `build.cljgo` is
   Clojure code, so `LoadPlan` boots an interpreter to run it. let-go and
   Clojure parse EDN data; Glojure splits an env var. cljgo's ~78 ms is
   almost entirely that boot — a cost the others structurally cannot have,
   because their manifest is not a program. This is a real capability
   difference (a build file with logic in it), not a slower version of the
   same thing.
2. **cljgo RESOLVES dependencies; let-go and Glojure do not.** let-go has no
   third-party dependency resolution at all, and Glojure's `GLJ_CLASSPATH` is
   handed to it fully computed. Their 0 ms is 0 ms *for a smaller job*.
   Quoting it against cljgo's 78 ms as a speed comparison would be false.
3. **Clojure's cost is paid by a different process.** tools.deps computes the
   classpath before the JVM that runs your code starts, and caches it in
   `.cpcache`. The "warm" column is what a developer normally feels; the
   "cold" column (~+425 ms fixed) is what actually happens the first time and
   after any `deps.edn` edit. **cljgo has no cache layer here, so its column
   is always the cold one.** Comparing cljgo-cold to Clojure-warm flatters
   Clojure; comparing to Clojure-cold flatters cljgo. Both are printed.
4. **JVM Clojure's numbers are dominated by JVM startup and say almost
   nothing about load-path design.** 279 ms of the 285 ms N=0 figure is the
   JVM getting out of bed. Its *slope* (1.8 ms/root) is informative; its
   *level* is not, and it must not be used to rank designs.
5. **"Roots" mean different things.** For cljgo, N is N local `:path`
   dependencies, each contributing a source root *and* a lock entry. For the
   others, N is N plain source-root strings. cljgo's N is strictly more work
   per unit — which makes its flatness a stronger result, not a weaker one.
6. **Eager vs lazy.** let-go and Glojure register roots and stop; the search
   happens at `require`. cljgo resolves and validates up front. Measuring only
   startup structurally favours the lazy designs. The compensating cost lands
   outside this table.

## VERDICT

**Hoisting resolution before dispatch is the right shape, and the deny-list is
the right mechanism — but the patch as applied is not affordable, for reasons
that are all fixable without adding anything.**

**Is hoisting affordable?** In principle yes. One resolution is ~78 ms, flat
from 1 to 50 dependencies, +10 MB RSS once. On a REPL, an nREPL server, a
`dev` server or a `build`, 78 ms is invisible. It is *not* invisible on the
short commands — `cljgo check` more than triples, 48 → 125 ms — and that is a
genuine regression a user will feel in an editor loop.

**But the patch as applied is not.** It adds a resolution instead of moving
one, so every already-covered command (`repl`, `nrepl`, `run`, `dev`, bri)
pays **twice**: `cljgo repl` 125.5 → 202.2 ms, N=0 200.1 → 354.7 ms, for zero
behavioral gain. The fix is deletion, not mechanism: **remove the five
surviving call sites** (`main.go:223`, `main.go:265`, `nrepl.go:43`,
`bri.go:254`, `bri.go:364`) so the hoist is what its own comment claims — one
call replacing many, fewer moving parts than before. Done that way the change
is independently justified by correctness and costs the covered commands
nothing they were not already paying.

Two further deletions are worth more than any optimisation available here:

- **`resolveRunDeps`'s `[filepath.Dir(file), "."]` loop double-visits `"."`.**
  Deduplicating that list is a two-line change worth **74 ms** on every
  dep-free project — more than the entire hoist costs. It is a pre-existing
  bug, independently justified, and it is the single largest number in this
  spike.
- **`build.LoadPlan` is called twice inside one `resolveRunDeps` pass** (once
  by `addProjectSourceRoots`, once by the no-lock branch) for the same build
  file. `addProjectSourceRoots` already dedups the *roots*; it does not dedup
  the *plan load*. Threading the plan the caller already has into the branch
  below removes a whole interpreter boot with no new state.

**Is the deny-list the right size?** The direction is right — deny-list, not
allow-list, so a new subcommand is slow rather than silently broken. Two
changes:

- **Add `build`.** It is the command that CREATES the lock, so resolving
  before it runs produces a G5023 error the command then disproves by
  succeeding. `build` does its own resolution afterwards regardless; the
  hoisted call is both wrong and redundant. (`dist` and `publish` deserve the
  same look for the same reason.)
- **Consider `check`.** `cljgo check` is a diagnostic run from an editor on
  save. +160% is the worst relative hit measured, and — like `explain` — it is
  a command you reach for *when the project is broken*. If it turns out to
  need project roots to analyze correctly, keep it and accept the cost; if not,
  deny it. That is a correctness question the measurement cannot settle, and
  it should be settled on correctness, not on the 77 ms.

**Does any competitor's design buy measurably better startup?** For the job
cljgo actually does, **no — the difference is structural, not a design win to
copy.** let-go's and Glojure's 0 ms are 0 ms because they parse data or split
an env var and defer everything else; neither resolves dependencies at all,
and Glojure requires an external step to have already produced the classpath.
JVM Clojure is the one that does comparable work, and it does it in a
*separate cached process* costing ~425 ms cold and ~1.8 ms per root warm —
worse than cljgo's 78 ms flat, and only tolerable because of a cache with an
invalidation story cljgo should not want.

**What cljgo could take from them is the placement, not the mechanism** — and
placement is exactly what the patch already does. All three hoist to one
site before mode dispatch, which is why none of them can have a REPL that
forgets what its run path does. That argument stands on its own and would be
worth making at the same speed.

**No cache, no lazy layer, no strategy object is proposed, and none is
warranted.** Every number above is either a constant interpreter boot that
belongs to a real capability, or duplicated work that should simply stop
happening. The remedies are five deletions and one deny-list entry.

## Surprises / things that contradict the proposed design

1. **Issue #185 was already fixed at HEAD.** `runREPL` calls `resolveRunDeps`
   at `main.go:164` in HEAD, and the measurement proves the REPL resolves
   there (125.5 ms in-project vs 47.8 ms bare). The hoist is a
   *generalisation*, not the #185 fix — the proposal's framing overstates what
   it repairs, and its cost/benefit should be argued on the ~11 uncovered
   subcommands instead.
2. **N=0 is the worst case, not the best.** An empty project is 1.8× a 50-dep
   project. Every intuition about "cost grows with the project" is inverted
   here, and the shape that suffers most is the one `cljgo new` emits.
3. **Resolution is flat in N and always will be**, because cljgo's manifest is
   a program: the interpreter boot dwarfs anything the manifest declares. Any
   future work aimed at "making resolution scale" is aimed at the wrong term.
   The term that matters is the boot.
4. **`cljgo build` emits a false diagnostic under the patch** and still exits
   0 — found by fixture generation, not by looking for it.
5. **cljgo's bare REPL startup (47.8 ms) is already faster than Glojure's
   bare `-e nil` (54.5 ms)** in this table — but Glojure is booting to
   evaluate an expression and cljgo to a prompt, so this is a note, not a
   claim. It is not a competitive result and should not be quoted as one.
