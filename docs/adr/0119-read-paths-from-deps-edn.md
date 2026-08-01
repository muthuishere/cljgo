# ADR 0119 — read `:paths` from `deps.edn`

- Status: accepted
- Date: 2026-08-01
- Extends: ADR 0118 (one project resolution, before dispatch)
- Relates to: ADR 0052 (dependency resolution), the precedence principle

## Context

ADR 0118 gave cljgo one project resolution before dispatch, so no entry point
can forget it. It resolves from `build.cljgo` — and a whole class of project
does not have one.

**`.cljc` is the dual-host mechanism.** A library that targets JVM Clojure and
cljgo from one source tree is written in `.cljc`, published to Clojars with
tools.build, and consumed as a library. On the JVM its source roots come from
`deps.edn`:

```clojure
{:paths ["src"]
 :deps  {org.clojure/clojure {:mvn/version "1.12.5"}}}
```

Such a project has no reason to carry a second project file describing the
same roots. koine is exactly this shape: `deps.edn` with `:paths ["src"]`, a
root `build.clj` for tools.build, and no `build.cljgo` at all.

So cljgo had **no declared source roots for it**, and the failure was
invisible in exactly the way that matters:

- `cljgo run src/foo.cljc` works — resolution falls back to the directory of
  the file being run.
- koine's whole conformance suite passes on the released v0.8.6: 13 checks,
  271 assertions, JVM and cljgo identical on every one.
- `cljgo repl` at the project root resolves **nothing**, because there is no
  requiring file to be relative to.

A green suite and a REPL that cannot see the project. This is the remaining
half of issue #185, and ADR 0118 did not fix it — the fix published roots that
this class of project never declared to us.

## Decision

**When a directory has no cljgo build file, read `:paths` from its `deps.edn`
and publish those as source roots.**

Deliberately the narrowest possible reading:

- **`:paths` only.** Not `:deps`, not `:aliases`, not `:extra-paths`, not
  `:replace-paths`. Those belong to tools.deps, which resolves them with alias
  semantics cljgo does not implement. Reading them halfway would produce a
  load path that agrees with neither host — worse than reading nothing.
- **Only a vector of strings.** tools.deps also permits a map form; that is
  skipped rather than guessed at.
- **Only when there is no `build.cljgo`/`build.cljg` ANYWHERE in the search.**
  A cljgo build file is cljgo's own declaration and wins absolutely: where one
  exists, `deps.edn` is **not consulted at all** — not merged, not consulted
  for keys the build file omits, not used as a fallback for anything. If you
  run cljgo and you want cljgo to read your project, you declare it to cljgo.

  This is ADR 0055's most-specific-first applied to the project description
  rather than the file extension, and it means a project can always opt out of
  a stale or JVM-only `:paths` by declaring its own.

  "Anywhere in the search" is load-bearing and was got wrong in the first cut.
  Resolution looks in the directory of the file being run and then in the
  working directory; a per-directory fallback let
  `cljgo run /elsewhere/foo.cljc` take `/elsewhere`'s `deps.edn` and return
  before ever reaching a `build.cljgo` in the working directory — reading
  `deps.edn` for a project that HAS a cljgo build file. Resolution therefore
  makes TWO passes: every directory is checked for a build file first, and
  only if none exists anywhere is `deps.edn` considered.
- **Errors are swallowed to nil.** A malformed or exotic `deps.edn` belongs to
  the JVM toolchain; cljgo refusing to start over a form it was never going to
  use would be worse than cljgo not learning anything from it. Nothing here
  can make resolution *wrong* — it can only add roots the project itself
  declared.

Parsing uses the existing EDN reader (`pkg/deps/edn_read.go`), not the
evaluator. `deps.edn` is data, and reading it must not pay the ~39 ms
interpreter boot that `build.LoadPlan` costs (measured in spike s72).

### Why this is the right shape, not just a convenient one

**The precedence principle.** `deps.edn` is Clojure's file, not ours. The
right move is to honour the part we can honour *exactly* and stay quiet about
the rest, rather than inventing a cljgo dialect of a format someone else owns.

**Precedent.** let-go already does this: `PathsFromDepsEdn` folds `deps.edn`
`:paths` into its search path as a fallback when no explicit flag or env var
is given (`references/let-go/pkg/resolver/resolver.go:65-99`), and explicitly
leaves everything beyond `:paths` to external tools.

**It is a fallback, never an override.** Adding a source root a project
already declared cannot break a project that declared it. The failure mode of
being wrong here is "cljgo learns nothing", not "cljgo resolves the wrong
file".

## Consequences

- A `deps.edn`-only dual-host project gets a working REPL and nREPL at its
  project root. Verified against koine: `(require 'koine.json)` from the
  project root failed on v0.8.6 and on ADR 0118's branch, and succeeds now.
- No regression for koine's suite — all 13 checks, 271 assertions, still
  identical across JVM and cljgo.
- cljgo now reads two project descriptions. That is a real cost in moving
  parts, and it is bounded on purpose: one key, one shape, one precedence
  rule, no alias semantics. The alternative — telling every dual-host library
  to add a `build.cljgo` describing roots it has already described — pushes
  the cost onto every consumer instead, to serve a file cljgo could simply
  read.
- **Not addressed:** `:deps` from `deps.edn`. cljgo resolves dependencies
  through `build.cljgo` + `build.lock.edn` (ADR 0052), and Maven coordinates
  there are already classified for Java interop. Reading `:deps` would mean
  either duplicating tools.deps' resolution or pretending to. A project that
  wants cljgo to resolve dependencies declares them to cljgo.

## Verification

- `cmd/cljgo/repl_resolution_test.go` — `TestREPLResolvesADepsEdnOnlyProject`
  (koine's shape: `deps.edn` with `:paths`, no build file) and
  `TestBuildCljgoWinsOverDepsEdn`, which points `deps.edn` at a decoy
  directory and asserts the cljgo build file wins, so the precedence cannot
  silently invert; and `TestBuildCljgoWinsFromAnyDirectoryInTheSearch`, which
  puts the decoy `deps.edn` beside the script and the `build.cljgo` in the
  working directory — the exact shape the per-directory fallback got wrong.
  Confirmed to fail (reading the decoy) when the two-pass order is reverted.
- Manual, against the real koine at 0.9.0: `run-conformance.sh` green on both
  hosts before and after, and `cljgo repl` at the project root now resolves
  `koine.json`.
