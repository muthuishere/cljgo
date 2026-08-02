# adr-0118-one-project-resolution — one project resolution, before dispatch

> **Backfill.** Written 2026-08-02, after the change shipped (`237f0b1`,
> `8f3fd4f`, `92fb2f5`; v0.8.7). ADR 0118 was written first; the proposal,
> spec delta and tasks were skipped. This is a record of what shipped, not a
> plan. It also closes the first half of issue **#185** — the REPL and nREPL
> resolving nothing from the project — so no separate entry exists for that
> issue. The remaining half is `adr-0119-deps-edn-paths`.

## Why

A project's load path is described entirely by its build file. One pipeline
turns that into something the interpreter can search:

```
build.cljgo → build.LoadPlan → plan.SourceRoots() → src/, test/
            → build.lock.edn → dependency roots
            → deps.SetResolvedRoots() → what eval.ResolveLibPath searches
```

`resolveRunDeps` is the only caller, and it lived at each entry point that
needed it. **Two of the eight evaluator bootstraps in `cmd/cljgo` never called
it** — `cljgo repl` and `cljgo nrepl` (#185). Neither could require the
project's own namespaces or its declared dependencies, while `cljgo run` and
`cljgo build` resolved both from the same directory. A freshly generated
`cljgo new` project could not require its own namespace in its own REPL.

`require` was never broken; it was searching an empty list, and the error said
so — "no `demo/core.clj/.cljg/.cljc` **relative to the requiring file**" is the
fallback root, the one that remains when `SetResolvedRoots` was never called.
The failure was invisible because the two things that kept working looked like
most of the language: Go interop (compiled in, no load path) and
`clojure.core` / `cljg.*` / `bri.*` (registered providers, which bypass the
file search). Exactly one category failed — anything that must be found as a
file — and that is exactly what the build file describes.

So the defect was not in the REPL. **It was that establishing the load path is
a step a caller has to remember**, and callers forget.

Read from source, the three reference implementations agree on placement: JVM
Clojure fixes the classpath in an external launcher before the process starts
(`clojure.main/repl` only *widens* it, `main.clj:412-413`); let-go builds one
NSResolver and installs it before every mode branch (`lg.go:440-442`); Glojure
drains `GLJ_CLASSPATH` into one global load path as the first statement of
`gljmain.Main` (`gljmain.go:58-62`). cljgo's per-call-site scheme ranked below
all three.

## What Changed

- **One resolution, in `run()`, before the subcommand switch.** The five
  scattered call sites are deleted, not supplemented.
- **The command list is a DENY-list.** A subcommand added tomorrow resolves by
  default; forgetting the list costs milliseconds instead of silently breaking
  a command — which is precisely what happened twice.
- **Failure is fatal, except for `repl` and `nrepl`.** For `run`/`test`/`dev`
  an unresolved project means the program cannot work, and continuing
  resurfaces it later as a bare "could not locate namespace" — the unnamed
  failure G5023 exists to replace (#168). A REPL keeps its prompt, because the
  prompt is the tool you would use to investigate the failure being reported.
  Two lists is worse than one and is still the honest encoding: the policy
  genuinely differs.
- Two pieces of pure waste deleted, both independent of the hoist:
  `resolveRunDeps` looped `[filepath.Dir(file), "."]` and `filepath.Dir("")`
  and `filepath.Dir("rel.clj")` are **both `"."`**, so common shapes resolved
  the same directory twice (74 ms on the layout `cljgo new` emits); and
  `build.LoadPlan` ran twice per pass for a plan that was thrown away (39.1 ms
  per boot). Both are simplifications correctness wanted anyway.

## The measurements

Spikes **s72** (cost by shape) and **s78** (cross-runtime) were run because
both contradicted an assumption in the first draft.

- **Cost does not scale with dependency count — it does not scale at all.**
  Locked project, warm: N=1 → 76.7 ms, N=50 → 78.3 ms, N=200 → 82.0 ms —
  ~27 µs and 29 KB per declared dep against a fixed ~76.7 ms / 111 MB, a
  marginal term 0.03% of the constant. **A dep-free project is the WORST case
  at 156 ms, twice a 200-dep project.** The intuition is inverted, and "make
  resolution scale" work would target the wrong term.
- **The constant is an interpreter boot.** `eval.New()` costs 38.2 ms, 54.2 MB,
  875k allocs, and `build.LoadPlan` is 98% that boot — evaluating the user's
  build form is under 2 ms. Every figure above is a count of boots.
- **Denied commands measured:** resolving for them costs +78 ms (locked) to
  +153 ms (dep-free) against a ~10 ms baseline — 8.7× to 16.6× startup for work
  whose result they never read, plus a spurious G5023 line.
- **Cross-runtime (s78, one session, within-table ratios only):** let-go and
  Glojure add zero for their bootstrap, but do a smaller job (one
  `os.ReadFile` + EDN parse) and resolve no third-party dependencies at all.
  JVM Clojure's absolute numbers are JVM startup (279 of 285 ms); only its
  slope (~1.8 ms/root) is informative. No competitor's placement buys better
  startup for the job cljgo actually does — what is worth copying is the
  placement, and that is what shipped.

## Impact

- **Affected specs:** `project-resolution`
- **Affected code:** `cmd/cljgo/main.go` (`resolveProjectForCommand`,
  `resolveRunDeps`, `resolveFromBuildFile`, `addSourceRoots`, the two command
  lists), and the five deleted call sites.
- Denied commands are unchanged: `cljgo cache help` in a project is ~10 ms
  before and after. The deny-list restores the floor; it does not make anything
  faster.

## Non-goals

- **Shrinking the constant.** Project resolution boots a whole tree-walking
  interpreter (38 ms / 54 MB / 875k allocs) to read a declarative build file.
  That is the entire cost and a real opportunity, but it is a different
  decision with its own risks — ADR 0021 decision 4 gives the build file the
  full language on purpose. It deserves its own ADR rather than being smuggled
  in here.
- **Concurrency or caching of resolution.** Nothing measured asks for either.
- Changing what `cljgo dev` resolves in practice: it no longer anchors on its
  entry source file, but it resolved the same directory anyway — a build file
  is at the project root, which `"."` reaches.
