# adr-0117-build-clj-is-tools-build — `build.clj` belongs to tools.build, not to cljgo

> **Backfill.** Written 2026-08-02, after the change shipped (`a186f25`,
> v0.8.6). ADR 0117 was written first; the proposal, spec delta and tasks were
> skipped. This entry is a record of what shipped, not a plan — every task is
> checked because every task is done. It also closes issue **#176**; no
> separate entry exists for that issue.

## Why

ADR 0055 made the build file "probed, not fixed": `cljgo build` looked for
`build.cljgo`, then `build.cljg`, then `build.clj`. It recorded that as
**additive** — every file that resolved before still resolved, and one more
name now did too. That reasoning held only while nothing read the build file
outside `cljgo build`.

v0.8.3 changed it. `cljgo run` began consulting the project build file for
dependency resolution (#168) and source roots, so the probe list landed on the
path of *every run in the project* — and the third entry stopped being
additive.

`build.clj` is not a generic name. It is **the tools.build convention**, and
it is how a library publishes to Clojars, so a dual-host project targeting JVM
Clojure and cljgo necessarily has one. cljgo was claiming another tool's build
script and evaluating it. koine's `build.clj` opens with
`(:require [clojure.tools.build.api :as b])`, which is JVM-only, so **every
`cljgo run` in the project** failed with `could not locate namespace
clojure.tools.build.api`. koine bisected it against clean clones: v0.8.2 works,
v0.8.3 and v0.8.4 fail, renaming `build.clj` away is the whole delta, and all
13 of their conformance checks failed. "Rename your `build.clj`" was never an
available answer — the name is the ecosystem's, not ours.

The reported *mechanism* was wrong, and that mattered: koine attributed it to
v0.8.3's source-roots work sweeping in the project root, but
`DefaultSourceRoots` is `["src","test"]` and the root is never added. A fix
aimed at the source roots would have passed the minimal repro and left the real
path live. cljgo also already held the correct view elsewhere —
`pkg/deps/mvnclassify.go` lists `build.clj` among the markers that identify a
**JVM** project, so one part of the codebase read the file as evidence of the
JVM while another claimed it as cljgo's own.

## What Changed

- `build.BuildFileNames` is `["build.cljgo", "build.cljg"]`,
  most-specific-first as before. A lone `build.clj` now reads as "no cljgo
  build file", which is what it is.
- ADR 0055's decision 1 — `.clj` acceptance for **source** resolution — is
  untouched. The narrowing applies only to the build-description probe, where
  the name is owned by another tool.

Two alternatives were rejected in the ADR and are recorded here so the spec is
not re-litigated: sniffing the file for tools.build shape (a second code path
plus a heuristic wrong in both directions), and special-casing the `run` path
(leaves `cljgo build` still evaluating tools.build scripts, and splits "the
project's build file" in two). Removing a list entry removes a moving part;
both alternatives add one.

## Impact

- **Affected specs:** `project-resolution` (new capability — how cljgo decides
  what describes the surrounding project)
- **Affected code:** `pkg/build/build.go` (`BuildFileNames`, `FindBuildFile`)
- **Breaking** for anyone who named their cljgo build file `build.clj`. The fix
  is a rename to `build.cljgo`, and `cljgo build` already reports `no
  build.cljgo in the current directory`, which names the file to create. The
  trade is not close: keeping the name breaks every dual-host project, dropping
  it breaks a project that chose a name colliding with tools.build.

## Non-goals

- Reading `build.clj` in any reduced or sniffed form. It is another tool's
  file; cljgo does not read it at all.
- Touching `.clj` **source** resolution (ADR 0055 decision 1). That is the
  ecosystem-bridge story and remains correct.
