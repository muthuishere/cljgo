# ADR 0116 — `go install …@vX.Y.Z` is a release build

- Status: accepted
- Date: 2026-08-01
- Supersedes: none (widens ADR 0028's release-pin rule)

## Context

ADR 0028 gave `SynthGoMod` a precedence order for locating the cljgo runtime:
explicit `-runtime` > `$CLJGO_SRC` > **release-pin** > walk-up repo detection.
The release-pin branch is the one that makes a downloaded binary self-
sufficient: a release writes a bare `require github.com/muthuishere/cljgo
v<Version>` with no `replace`, so **binary + Go toolchain is the whole `cljgo
build` story** — no source checkout anywhere.

That branch was gated on `version.IsRelease()`, which tested only whether the
package-level `Version` variable held a plain SemVer tag. `Version` is stamped
by exactly one mechanism: goreleaser's `-ldflags` on the tagged archives.

But that is not the only way a user obtains a release. The other is:

```
go install github.com/muthuishere/cljgo/cmd/cljgo@v0.8.4
```

Go compiles that from the module proxy, having **resolved and checksum-
verified the tag**, and passes no ldflags. So `Version` kept its in-source
`"0.1.0-dev"` default, `IsRelease()` returned false, `SynthGoMod` fell through
to walk-up detection, found no repo, and failed:

```
error: this is a dev cljgo binary (version 0.1.0-dev), so the generated
go.mod needs a local runtime tree: cannot locate the
github.com/muthuishere/cljgo source tree (set CLJGO_SRC or run inside the repo)
```

A binary installed at an explicit release tag **could not build a single
project**. Reproduced against the published v0.8.4.

This was not a corner case:

- `templates/web/Dockerfile` installs cljgo with exactly this command, so the
  **shipped web template could not build in Docker** — day-one impact for a
  new user following the deploy guide.
- Two independent consumers (koine, toolnexus) hit it separately, and both
  reported the same downstream symptom before the cause was known.

The requested tag was never actually missing. Go records it in the binary as
`debug.ReadBuildInfo().Main.Version`, and `pkg/version` was **already reading
and validating that exact field** — `devSuffixFromBuildInfo` used it to render
`[dev build, module v0.8.4]`, rejecting `(devel)` and pseudo-versions. The
information sat one function away from the decision that needed it. The
version *line* was right; only the *gate* was wrong. A version-string-only
fix would therefore have been cosmetic and left the build broken.

## Decision

**A binary is a release when it can prove which published tag it was built
from — by either mechanism.**

`version.ReleaseVersion()` returns that tag as plain SemVer, or `""`:

1. `Version` is a plain `major.minor.patch` (goreleaser ldflags) → that value.
2. Otherwise `BuildInfo.Main.Version` is a real requested tag → that value,
   with the `v` stripped.
3. Otherwise `""`.

Rule 2 rejects `(devel)` (a plain local build), Go-synthesized pseudo-versions,
and any tag carrying a prerelease qualifier (goreleaser snapshots). Nothing
here can invent a module version that was never published — the pin's
correctness rests on the toolchain's own checksum-verified resolution, not on
a string we assembled.

`IsRelease()` becomes `ReleaseVersion() != ""`, and `SynthGoMod` pins
`"v" + ReleaseVersion()` rather than `"v" + Version` — which would otherwise
emit `v0.1.0-dev`, a tag that does not exist.

**One reporter, so the numbers cannot disagree.** `version.Self()` returns the
published tag when there is one and the in-source `Version` otherwise, and
`cljgo version`, `(cljgo-version)`, `*cljgo-version*` and the nREPL handshake
all read it. Without this, the fix would have created a fresh contradiction —
`cljgo version` saying `0.8.4` while `(cljgo-version)` still said `0.1.0-dev`,
breaking the promise `pkg/version`'s doc comment makes explicitly.

## Consequences

- `go install …@vX.Y.Z` yields a fully working cljgo: it builds projects with
  no source tree, exactly as the tagged archives do.
- The web template's Docker build works at every release rather than none.
- `cljgo version` on such a binary reports `0.8.4 (Go 1.26.3, Clojure 1.12.5)`
  instead of `0.1.0-dev … [dev build, module v0.8.4]`. This is a deliberate
  *narrowing* of ADR 0113's #170 dev-provenance line: it still fires for every
  genuinely-dev build (commit + dirty), but a resolved release tag is no longer
  described as a dev build, because it is not one.
- `pkg/version.Version` still stays `"0.1.0-dev"` in source forever. Nothing
  here reintroduces a version constant to bump.
- The pin is only as good as the tag: `go install`ing a tag whose module was
  never published would produce a `require` Go cannot resolve. That is already
  true of the goreleaser path and is caught at `go build` with a clear
  toolchain error, not silently.

## Verification

- `pkg/version/version_test.go` — `TestReleaseVersionFromBuildInfo` pins the
  mapping (real tag → release; `(devel)`, pseudo-version, prerelease qualifier,
  empty → not a release), and `TestReleaseVersionPrefersLdflags` pins that a
  stamped `Version` wins outright and that the dev default never becomes a
  release just because the test binary carries build info of its own.
- `pkg/emit/gomod_test.go` covers the emitted go.mod for both branches.
- End-to-end: `go install …/cmd/cljgo@v0.8.4` + `cljgo new -template cli` +
  `cljgo build` is the reproduction, and is the acceptance check for the next
  release. It cannot run in CI against an unreleased fix — the first binary
  able to pass it is the one built from the tag that contains this ADR.
