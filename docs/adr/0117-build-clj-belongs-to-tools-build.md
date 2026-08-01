# ADR 0117 — `build.clj` belongs to tools.build, not to cljgo

- Status: accepted
- Date: 2026-08-01
- Narrows: ADR 0055 (decision 2 — the build-file probe list)

## Context

ADR 0055 made the build file "probed, not fixed": `cljgo build` looks for
`build.cljgo`, then `build.cljg`, then `build.clj`, taking the first present.
It recorded this as **additive** — every file that resolved before still
resolved, and one more name now did too.

That reasoning held only while nothing read the build file outside
`cljgo build`. v0.8.3 changed it: `cljgo run` began consulting the project
build file for dependency resolution (issue #168) and source roots. The probe
list was suddenly on the path of *every run in the project*, and the third
entry stopped being additive.

`build.clj` is not a generic name in the Clojure ecosystem. It is **the
tools.build convention**, and it is how a library publishes to Clojars — so a
dual-host project targeting both JVM Clojure and cljgo necessarily has one.
cljgo was claiming another tool's build script and evaluating it. koine's
`build.clj` opens with:

```clojure
(ns build
  (:require [clojure.tools.build.api :as b]))
```

`clojure.tools.build.api` is JVM-only and cljgo cannot resolve it, so **every
`cljgo run` in the project** failed with:

```
error: could not locate namespace clojure.tools.build.api
```

koine bisected it against clean clones of each tag — v0.8.2 works, v0.8.3 and
v0.8.4 fail, and renaming `build.clj` away is the whole delta. All 13 of their
conformance checks failed on v0.8.3+. This was a total block on precisely the
dual-host projects cljgo exists to serve, and "rename your `build.clj`" was
never an available answer: the name is the ecosystem's, not ours.

The reported *mechanism* was wrong, and that matters. koine attributed it to
v0.8.3's source-roots work sweeping in the project root; `DefaultSourceRoots`
is `["src","test"]` and the root is never added, so that does not follow. A fix
aimed at the source roots would have passed the minimal repro and left the real
path live. The cause is the build-file **name collision**.

Notably, cljgo already held the correct view elsewhere: `pkg/deps/mvnclassify.go`
lists `build.clj` among the markers that identify a **JVM** project. One part of
the codebase read the file as evidence of the JVM while another claimed it as
cljgo's own.

## Decision

**Drop `build.clj` from the accepted build-file names.** `BuildFileNames` is
`["build.cljgo", "build.cljg"]`, most-specific-first as before. A lone
`build.clj` now reads as "no cljgo build file", which is what it is.

ADR 0055's decision 1 — `.clj` acceptance for **source** resolution — is
untouched and remains correct. That is the ecosystem-bridge story: the same
library's `.clj` files are what JVM Clojure reads. The narrowing applies only
to the build-description probe, where the name is owned by another tool.

Two alternatives were rejected:

- **Sniff the file and skip it if it looks like tools.build.** Adds a second
  code path and a heuristic that is wrong in both directions (a cljgo build
  file that requires something unresolvable would be silently skipped; a
  tools.build file that happens to parse would still be run). *Simplicity
  first*: removing a list entry removes a moving part, sniffing adds one.
- **Keep it and special-case the `run` path.** That leaves `cljgo build` still
  evaluating tools.build scripts, and splits the definition of "the project's
  build file" in two.

## Consequences

- Dual-host projects work again: `build.clj` for the JVM and `build.cljgo` for
  cljgo coexist, each read by its own tool.
- **Breaking for anyone who named their cljgo build file `build.clj`.** The fix
  is a rename to `build.cljgo`, and `cljgo build` already reports
  `no build.cljgo in the current directory` — which names the file to create.
  The trade is not close: keeping the name breaks every dual-host project,
  dropping it breaks a project that chose a name colliding with tools.build.
- One fewer probe on the `run` path.
- The latent lesson is the one worth keeping: ADR 0055 called the third entry
  "additive" because at the time nothing else read the build file. *Additive*
  was a statement about the callers of that moment, not a property of the
  decision — and it expired silently when a new caller appeared.

## Verification

- `pkg/build/findbuildfile_test.go` — a lone `build.clj` yields `""`, and it
  never shadows a real cljgo build file in the dual-host layout.
- `pkg/build/consumer_defects_test.go` —
  `TestJVMBuildCljDoesNotBreakTheProject` runs the **real cljgo binary** on a
  project laid out exactly as koine's (a tools.build `build.clj` at the root
  plus `src/demo/app.cljc`) and asserts `cljgo run` succeeds and never mentions
  `clojure.tools.build.api`. Confirmed to fail with the reported error when the
  entry is restored, so the test proves the fix rather than merely accompanying
  it.
