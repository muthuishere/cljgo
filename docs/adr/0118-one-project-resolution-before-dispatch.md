# ADR 0118 — one project resolution, before dispatch

- Status: accepted
- Date: 2026-08-01
- Relates to: ADR 0052 (dependency resolution), ADR 0007 (execution-leg parity)
- Evidence: spike s72 (cost by shape), spike s78 (cross-runtime comparison)

## Context

A project's load path is described entirely by its build file. `build.cljgo`
declares source roots and dependencies; `build.lock.edn` pins them. One
function turns that into something the interpreter can search:

```
build.cljgo
  → build.LoadPlan            evaluate it
  → plan.SourceRoots()        → src/, test/
  → build.lock.edn            → dependency roots
  → deps.SetResolvedRoots()   → what eval.ResolveLibPath searches
```

`resolveRunDeps` is the only caller of that pipeline, and it lived at each
entry point that needed it. **Two of the eight evaluator bootstraps in
`cmd/cljgo` never called it** — `cljgo repl` and `cljgo nrepl` (issue #185).
Neither could require the project's own namespaces or its declared
dependencies, while `cljgo run` and `cljgo build` resolved both from the same
directory. A freshly generated `cljgo new` project could not require its own
namespace in its own REPL.

`require` was never broken. It was searching an empty list, and the error said
so for anyone reading closely:

```
could not locate namespace demo.core
(no registered provider, and no demo/core.clj/.cljg/.cljc
 relative to the requiring file)
```

"relative to the requiring file" is the fallback root — the one that remains
when `SetResolvedRoots` was never called. The failure was invisible because
the two things that kept working looked like most of the language:
Go interop (compiled in, no load path) and `clojure.core` / `cljg.*` / `bri.*`
(served by registered providers, which bypass the file search). Exactly one
category failed — anything that must be found as a file — and that is exactly
what the build file describes.

So the defect was not in the REPL. **It was that establishing the load path
is a step a caller has to remember**, and callers forget.

### What the three reference implementations do

Read from source, not documentation:

- **JVM Clojure — the bootstrap does not exist in-process.** `require` → `load`
  → `RT.load` (`RT.java:447-484`) resolves via `ClassLoader#getResource`
  (`RT.java:2210-2216`). The classpath is fixed by an external launcher
  (tools.deps — not even in the Clojure repo) before the JVM starts.
  `clojure.main/repl` touches the classloader exactly once (`main.clj:412-413`),
  wrapping it in a `DynamicClassLoader` so `RT/addURL` has somewhere to add —
  it **widens** what is reachable and never establishes the baseline.
  `main-opt`/`script-opt`/`null-opt` touch it not at all. A REPL-vs-script
  divergence is not merely avoided here; it is inexpressible.
- **let-go — one bootstrap before dispatch.** `buildSearchPaths()` →
  `resolver.NewNSResolver` → `rt.SetNSLoader`, at `lg.go:440-442`, ahead of
  every branch (compile, bundle, wasm, script, `-e`, REPL).
- **Glojure — one bootstrap before dispatch.** `GLJ_CLASSPATH` drained into a
  global `loadPath` as the first statement of `gljmain.Main`
  (`gljmain.go:58-62`), ahead of REPL / `-e` / `--nrepl` / file.

Ranked by robustness: Clojure (resolution precedes the process) > let-go and
Glojure (one in-process bootstrap before dispatch) > cljgo before this ADR
(per-call-site, and two of eight were wrong).

## Decision

**Resolve the project exactly once, in `run()`, before the subcommand
switch.** The five scattered call sites are deleted, not supplemented.

Three rules follow, and each was forced by a measurement rather than taste:

1. **The command list is a DENY-list, not an allow-list.** A subcommand added
   tomorrow resolves by default. Under an allow-list, forgetting means a
   silently broken command — which is precisely what happened twice. Under a
   deny-list, forgetting costs milliseconds.
2. **Failure is fatal, except for `repl` and `nrepl`.** For `run`/`test`/`dev`
   an unresolved project means the program cannot work, and continuing would
   resurface it later as a bare "could not locate namespace" — the unnamed
   failure G5023 exists to replace (#168). A REPL keeps its prompt, because
   the prompt is the tool you would use to investigate the failure being
   reported. Two lists is worse than one and is still the honest encoding:
   the policy genuinely differs, and collapsing it would be wrong for one of
   the two groups.
3. **A REPL-specific step may only widen what is reachable, never establish
   it.** This is Clojure's shape (`main.clj:412-413`) and it is the invariant
   that would have prevented #185: had the REPL only ever been able to *add*
   to a load path someone else established, forgetting would have been
   impossible.

### The deny-list, and why each entry is on it

- `new`, `version`, `help`, `explain` — no evaluation, and `explain` must stay
  usable inside a broken project; it is what you reach for when the build
  fails.
- `build`, `dist`, `publish` — they **create** the lock this resolution reads.
  Resolving first made `cljgo build` on a fresh clone print "no
  build.lock.edn … run `cljgo build` once" and then succeed, having just
  written it: an error contradicted two lines later by the same command.
- `cache`, `config`, `generate`/`g`, `routes`, `migrate` — never evaluate
  project code. Measured cost of resolving for them: **+78 ms (locked project)
  to +153 ms (dep-free), against a ~10 ms baseline** — 8.7× to 16.6× startup
  for work whose result they never read, plus a spurious G5023 line before
  their unrelated output.

## The measurements that changed the design

Both spikes contradicted an assumption in the first draft, which is the reason
they were run.

**Cost does not scale with dependency count — it does not scale at all.**
Locked project, warm: N=1 → 76.7 ms, N=50 → 78.3 ms, N=200 → 82.0 ms. That is
~27 µs and 29 KB per declared dep against a fixed ~76.7 ms / 111 MB — the
marginal term is 0.03% of the constant. **A dep-free project is the WORST
case at 156 ms, twice a 200-dep project.** The intuition is inverted, and any
future "make resolution scale" work would target the wrong term.

**The constant is an interpreter boot.** `eval.New()` costs 38.2 ms, 54.2 MB,
875k allocs, and `build.LoadPlan` is 98% that boot — evaluating the user's
actual build form is under 2 ms. Every figure above is a count of boots.

**Cross-runtime (s78, one session, within-table ratios only):** let-go and
Glojure add **zero** for their bootstrap — but they are doing a smaller job
(one `os.ReadFile` + EDN parse; `os.DirFS` does no I/O), and neither resolves
third-party dependencies at all. JVM Clojure's absolute numbers are JVM
startup (279 of 285 ms) and say nothing about load-path design; only its slope
(~1.8 ms/root) is informative. **No competitor's placement buys better startup
for the job cljgo actually does.** What is worth copying is the placement, and
that is what this ADR takes.

Two pieces of pure waste were found and deleted, both independent of the
hoist:

- `resolveRunDeps` looped `[filepath.Dir(file), "."]`, and `filepath.Dir("")`
  and `filepath.Dir("rel.clj")` are **both `"."`** — so the common shapes
  resolved the same directory twice. Measured at 74 ms on the layout
  `cljgo new` emits.
- `build.LoadPlan` ran twice per pass — once in `addProjectSourceRoots`, once
  in the no-lock branch — for a plan that was thrown away. One boot is 39.1 ms.

Both are simplifications that correctness wanted anyway; they are not
performance work buying complexity, and they stand whether or not the hoist
does.

## Consequences

- #185 cannot recur by forgetting: there is one call site, and new subcommands
  resolve by default.
- Five call sites become one. This is fewer moving parts, not more — the
  change would be worth making with no bug attached.
- Denied commands are unchanged. Measured warm and interleaved against the
  pre-hoist binary, `cljgo cache help` in a project is ~10 ms both before and
  after: the deny-list restores the floor, it does not make anything faster.
- `cljgo dev` no longer anchors resolution on its entry source file. It
  resolved the same directory anyway — a build file is at the project root,
  which `"."` reaches.
- **Not addressed here, deliberately:** project resolution boots an entire
  tree-walking interpreter (38 ms / 54 MB / 875k allocs) to read a declarative
  build file. That is the whole constant, and shrinking it is a real
  opportunity — but it is a different decision with its own risks (the build
  file is code, and ADR 0021 decision 4 gives it the full language on
  purpose). It deserves its own ADR rather than being smuggled in here.

## Verification

- `cmd/cljgo/repl_resolution_test.go` — the REPL and nREPL cases, each
  asserting on the call's own result rather than a trailing marker (the REPL
  continues past a failed require, so a marker proves nothing), plus
  `TestREPLAndRunAgreeOnResolution`, which pins the invariant rather than the
  symptom: whatever `cljgo run` can require from a directory, `cljgo repl`
  can too.
- `spikes/s72-project-resolution-cost/RESULTS.md` — cost by project shape and
  size, with allocation.
- `spikes/s78-project-resolution-crossruntime/RESULTS.md` — cljgo, let-go,
  Glojure and JVM Clojure in one table, with the not-apples-to-apples
  asymmetries stated.
