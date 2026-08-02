# cycle-v0.8.5-0.8.9-consumer-defects — the two defect fixes that changed a contract

> **Backfill.** Written 2026-08-02, after the fixes shipped. Neither fix
> carried an ADR (none was needed — both restore behaviour existing ADRs
> already require), and neither went through OpenSpec at the time. This entry
> is a record, not a plan; every task is checked because every task is done.
>
> It deliberately covers **two** of the cycle's issue fixes, not all of them.
> The rest are listed at the bottom with the reason each was judged below the
> bar, so that judgement is part of the record rather than an omission someone
> has to re-derive.

## Why

The v0.8.5–v0.8.9 cycle was driven by two external consumers (koine,
toolnexus) exercising released archives. Most of what they found were defects
that a fix simply reverses. Two were different: they were **silent
cross-host divergences** whose correct behaviour no spec states, only a
comment or a single test — and in both cases the comment was confidently
wrong.

### #182 — `cljgo test --compiled` could not run a `.cljc` suite

`nsNameFor` derives a namespace symbol by dropping a file's extension, and
carried its OWN hand-written list:

```go
stem := strings.TrimSuffix(strings.TrimSuffix(rel, ".cljg"), ".clj")
```

The resolver's list is `{.cljgo, .cljg, .clj, .cljc}` (ADR 0068). Two copies of
one convention, and this copy was never updated — so `demo/core_test.cljc`
became the namespace `demo.core-test.cljc`, the extension turning into a name
segment, and the compiled leg emitted a require for a name that cannot exist.
`.cljgo` was broken identically; the regression test found that, nobody
reported it.

The severity is not the broken flag. `.cljc` is the dual-host extension, so
this hits exactly the projects targeting JVM Clojure AND cljgo, and it fails in
the direction that looks fine: interpreted green, compiled unable to build.
**Anyone who believes `--compiled` is their AOT test path silently has no AOT
coverage** — the REPL-vs-binary divergence ADR 0007 calls unforgivable,
occurring in the harness meant to detect it.

### #187 — Maven repository order disagreed with tools.deps

`DefaultMvnRepos` was Clojars-then-Central, with a comment claiming that WAS
"the tools.deps default set". It is not: tools.deps returns central, then
clojars, then other repos, unconditionally (0.22.1492,
`clojure/tools/deps/util/maven.clj:161-165`, read from the jar in `~/.m2`).

cljgo fetches from the first repository that answers, so a coordinate published
to BOTH resolved to a **different artifact** on cljgo than on the JVM —
silently: no conflict, no diagnostic, nothing in the project files to hint at
it. That is the one failure a dual-host `.cljc` project cannot tolerate; you
would be debugging a behavioural difference in your own code while the cause
was which jar got fetched. Found by spike **s79** while surveying what
`deps.edn` `:deps` translation would cost; the bug is independent of that
feature and was live on main.

## What Changed

- **#182:** `pkg/eval.SourceExts` is now the single exported list
  (`{.cljgo, .cljg, .clj, .cljc}`), read by both the resolver and `nsNameFor`.
  Not a tidy-up — two subsystems must agree on what a file's namespace is
  called, and the bug is the cost of disagreeing.
- **#187:** `DefaultMvnRepos` is Central-then-Clojars, matching tools.deps,
  with the source citation at the declaration. `(mvn-repo …)` still prepends,
  so a project can override.

## Impact

- **Affected specs:** `source-resolution`, `dependency-resolution`
- **Affected code:** `pkg/eval/libload.go`, `cmd/cljgo/bri.go`,
  `pkg/deps/mvncoord.go`
- **Relies on:** ADR 0068 (the extension set), ADR 0007 (execution-leg
  parity), ADR 0052/0095 (dependency resolution). No new ADR was needed: both
  fixes restore what those decisions already require.

## Non-goals

- Reading `:deps` from `deps.edn` (s79 surveyed it; see ADR 0119's non-goals).
- Any change to how `(mvn-repo …)` overrides the default set.

## What this cycle shipped that did NOT warrant an entry, and why

`CLAUDE.md` is explicit that trivial fixes skip OpenSpec, and a backfill that
invents ceremony is worse than the gap. Recorded here so the omission is a
judgement, not an oversight:

- **#176** (a JVM `build.clj` broke every `cljgo run` in the project) — has an
  ADR and an entry: `adr-0117-build-clj-is-tools-build`.
- **#177** (a released binary could not build without a source checkout) and
  **#189** (a dev build claimed to be a release and silently compiled against
  the published runtime) — both are ADR 0116, covered by
  `adr-0116-release-build-detection`. #189 is the widening recorded there as
  task 4.
- **#185** (`cljgo repl`/`nrepl` resolved nothing from the project) — covered
  in two halves by `adr-0118-one-project-resolution` and
  `adr-0119-deps-edn-paths`.
- **#178 / G5025** (a build file with no `build` fn reported a column into a
  synthesized file that does not exist) and **G5026** (an error naming the
  wrong condition — the file WAS found and loaded, it just defined no
  namespace) — **diagnostics, deliberately no entry.** The repo already tracks
  every code in three places that are checked against each other:
  `pkg/diag/registry.go`, `docs/diagnostics/<CODE>.md` +
  `docs/diagnostics/registry.lock`, and the site's diagnostics table (with a
  test that stops the three drifting). An OpenSpec requirement restating the
  message would be a fourth copy of the same fact, would be the copy nothing
  verifies, and would go stale first. The general contract that governs them —
  how a cljgo error must read — is doctrine in `CLAUDE.md` and ADR 0015, not a
  per-code spec.
- **The `:exit-code` documentation fix** shipped alongside G5026 — the
  semantics were never wrong, only the recommended read (`nil` means "not yet
  reaped", not "still running"). A comment/docstring correction plus
  `TestWaitIsTheReliableReadAfterDraining`, which pins the SAFE contract rather
  than the race. No contract changed.
- **The `parsePOM` `CharsetReader` fix** — a non-UTF-8 POM failed with a Go
  decoder error dressed up as "not a Maven POM". A defect fix restoring
  intended behaviour, with no contract to state beyond "cljgo parses the POMs
  that exist".
