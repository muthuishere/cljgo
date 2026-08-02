# adr-0119-deps-edn-paths — read `:paths` from `deps.edn`

> **Backfill.** Written 2026-08-02, after the change shipped (`c2bfa04`,
> v0.8.7). ADR 0119 was written first; the proposal, spec delta and tasks were
> skipped. This is a record of what shipped, not a plan. It closes the
> remaining half of issue **#185**, which `adr-0118-one-project-resolution`
> did not fix; there is no separate entry for that issue.

## Why

ADR 0118 gave cljgo one project resolution before dispatch, so no entry point
can forget it. It resolves from `build.cljgo` — and a whole class of project
does not have one.

**`.cljc` is the dual-host mechanism.** A library targeting JVM Clojure and
cljgo from one source tree is written in `.cljc`, published to Clojars with
tools.build, and consumed as a library. On the JVM its source roots come from
`deps.edn` (`{:paths ["src"] …}`). Such a project has no reason to carry a
second project file describing the same roots. koine is exactly this shape:
`deps.edn` with `:paths ["src"]`, a root `build.clj` for tools.build, and no
`build.cljgo` at all.

cljgo therefore had **no declared source roots for it**, and the failure was
invisible in the way that matters most: `cljgo run src/foo.cljc` works
(resolution falls back to the directory of the file being run), koine's whole
conformance suite passed on v0.8.6 — 13 checks, 271 assertions, JVM and cljgo
identical — and `cljgo repl` at the project root resolved **nothing**, because
there is no requiring file to be relative to. A green suite and a REPL that
cannot see the project.

## What Changed

When a directory has no cljgo build file, cljgo reads `:paths` from its
`deps.edn` and publishes those as source roots. Deliberately the narrowest
possible reading:

- **`:paths` only.** Not `:deps`, not `:aliases`, not `:extra-paths`, not
  `:replace-paths`. Those belong to tools.deps, which resolves them with alias
  semantics cljgo does not implement; reading them halfway would produce a load
  path that agrees with neither host — worse than reading nothing.
- **Only a vector of strings.** tools.deps also permits a map form; that is
  skipped rather than guessed at.
- **Only when there is no `build.cljgo`/`build.cljg` ANYWHERE in the search.**
  A cljgo build file wins absolutely: where one exists, `deps.edn` is not
  consulted at all — not merged, not consulted for keys the build file omits,
  not a fallback for anything. "Anywhere in the search" is load-bearing and was
  got wrong in the first cut: resolution looks in the directory of the file
  being run and then in the working directory, and a per-directory fallback let
  `cljgo run /elsewhere/foo.cljc` take `/elsewhere`'s `deps.edn` and return
  before ever reaching a `build.cljgo` in the working directory. Resolution
  therefore makes **two passes** — every directory is checked for a build file
  first, and only if none exists anywhere is `deps.edn` considered.
- **Errors swallowed to nil.** A malformed or exotic `deps.edn` belongs to the
  JVM toolchain. Nothing here can make resolution *wrong* — it can only add
  roots the project itself declared, so the failure mode of being wrong is
  "cljgo learns nothing".
- **Parsed with the existing EDN reader** (`pkg/deps/edn_read.go`), not the
  evaluator. `deps.edn` is data, and reading it must not pay the ~39 ms
  interpreter boot `build.LoadPlan` costs (spike s72).

Why this shape rather than a convenient one: the **precedence principle** —
`deps.edn` is Clojure's file, not ours, so honour the part we can honour
exactly and stay quiet about the rest rather than inventing a cljgo dialect of
someone else's format. **Precedent** — let-go already folds `deps.edn`
`:paths` into its search path as a fallback and explicitly leaves everything
beyond `:paths` to external tools
(`references/let-go/pkg/resolver/resolver.go:65-99`). And it is a **fallback,
never an override**: adding a source root a project already declared cannot
break a project that declared it.

## Impact

- **Affected specs:** `project-resolution`
- **Affected code:** `pkg/deps/depsedn.go` (new: `DepsEDNFileName`,
  `DepsEDNPaths`), `cmd/cljgo/main.go` (`resolveRunDeps`'s two passes,
  `addSourceRoots` shared by both paths so they cannot drift).
- cljgo now reads two project descriptions. That is a real cost in moving
  parts, bounded on purpose: one key, one shape, one precedence rule, no alias
  semantics. The alternative — telling every dual-host library to add a
  `build.cljgo` describing roots it has already described — pushes the cost
  onto every consumer to serve a file cljgo could simply read.

## Non-goals

- **`:deps` from `deps.edn`.** cljgo resolves dependencies through
  `build.cljgo` + `build.lock.edn` (ADR 0052), and Maven coordinates there are
  already classified for Java interop. Reading `:deps` would mean either
  duplicating tools.deps' resolution or pretending to. A project that wants
  cljgo to resolve dependencies declares them to cljgo.
- Alias semantics, `:extra-paths`, `:replace-paths`, or the map form of
  `:paths`.
- Merging `deps.edn` into a project that HAS a cljgo build file.
