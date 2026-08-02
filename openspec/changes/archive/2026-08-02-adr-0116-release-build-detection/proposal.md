# adr-0116-release-build-detection — a binary that can prove its tag is a release

> **Backfill.** This proposal was written on 2026-08-02, after the change had
> already shipped (`873a344`, v0.8.6, widened by `a1946df`, v0.8.9). ADR 0116
> was written first, as the process requires; steps 2–3 — proposal, spec
> deltas, tasks — were skipped. This entry exists to close that gap, so it is
> a **record of what shipped**, not a plan. Every task below is checked
> because every task below is done; nothing here is proposing new work.
>
> It closes issues **#177** (a released cljgo could not be consumed without a
> source checkout) and **#189** (a local dev build claimed to be a release and
> silently compiled against the published runtime). Neither has a separate
> entry.

## Why

ADR 0028 gave `SynthGoMod` a precedence order for locating the cljgo runtime:
`-runtime` > `$CLJGO_SRC` > **release-pin** > walk-up repo detection. The
release-pin branch is what makes a downloaded binary self-sufficient — it
writes a bare `require github.com/muthuishere/cljgo v<tag>` with no `replace`,
so binary + Go toolchain is the whole `cljgo build` story.

That branch was gated on `version.IsRelease()`, which tested only whether the
package-level `Version` variable held a plain SemVer tag. `Version` is stamped
by exactly one mechanism: goreleaser's `-ldflags` on the tagged archives. But
that is not the only way a user obtains a release:

```
go install github.com/muthuishere/cljgo/cmd/cljgo@v0.8.4
```

Go compiles that from the module proxy having **resolved and checksum-verified
the tag**, and passes no ldflags. `Version` kept its in-source `"0.1.0-dev"`,
`IsRelease()` said false, `SynthGoMod` fell through to walk-up detection, found
no repo, and failed with "this is a dev cljgo binary … cannot locate the
github.com/muthuishere/cljgo source tree". **A binary installed at an explicit
release tag could not build a single project.** `templates/web/Dockerfile`
installs cljgo with exactly that command, so the shipped web template could not
build in Docker; two independent consumers (koine, toolnexus) hit it
separately.

The tag was never missing. Go records it as
`debug.ReadBuildInfo().Main.Version`, and `pkg/version` was already reading and
validating that exact field for the `[dev build, module v0.8.4]` suffix. The
version *line* was right; only the *gate* was wrong.

## What Changed

- `version.ReleaseVersion() string` — the published tag, or `""`. `Version` as
  plain `major.minor.patch` (goreleaser ldflags) wins; otherwise
  `BuildInfo.Main.Version` when it is a real requested tag, `v` stripped.
  `(devel)`, Go-synthesized pseudo-versions and prerelease qualifiers
  (goreleaser snapshots) are rejected.
- `IsRelease()` becomes `ReleaseVersion() != ""`; `SynthGoMod` pins
  `"v" + ReleaseVersion()`, never `"v" + Version` (which would emit
  `v0.1.0-dev`, a tag that does not exist).
- `version.Self()` — **one reporter**, so `cljgo version`, `(cljgo-version)`,
  `*cljgo-version*` and the nREPL handshake cannot disagree. Without it the fix
  would have created a fresh contradiction: `cljgo version` saying `0.8.4`
  while `(cljgo-version)` still said `0.1.0-dev`.
- **The later widening (`a1946df`, #189, v0.8.9), which ADR 0116 does not yet
  record:** a build from a source CHECKOUT is never a release, whatever
  `Main.Version` says. Go stamps `Main.Version` from the nearest VCS tag for a
  plain local `go build` inside the repo, so after v0.8.5 was tagged every dev
  build reported `v0.8.5` and `IsRelease()` went TRUE — and `SynthGoMod` then
  wrote a release pin with no `replace`, silently compiling a developer's
  project against the *published* runtime instead of their working tree. The
  discriminator is exact: `go install module@vX.Y.Z` resolves through the proxy
  and records no `vcs.*` build settings; any build from a checkout records
  `vcs=git` + `vcs.revision`. Presence of VCS data means "built from source",
  which is precisely what a release is not.

## Impact

- **Affected specs:** `build-distribution`
- **Affected code:** `pkg/version/version.go`, `pkg/emit/program.go`
  (`SynthGoMod`), and every version reporter routed through `Self()`.
- **Not affected:** `pkg/version.Version` still stays `"0.1.0-dev"` in source
  forever. Nothing here reintroduces a constant to bump.

## Non-goals

- Changing how goreleaser stamps the tagged archives. That path is unchanged
  and still wins outright.
- Inventing a module version. The pin's correctness rests on the toolchain's
  own checksum-verified resolution; a tag whose module was never published
  fails at `go build` with a clear toolchain error, not silently.
- Retiring ADR 0113's dev-provenance line (#170). It still fires for every
  genuinely-dev build (commit + dirty); this only stops describing a resolved
  release tag as a dev build.
