# tasks — adr-0116-release-build-detection

**Backfilled 2026-08-02 from shipped code.** Every box is checked because the
work is done and released (v0.8.6, widened in v0.8.9); the list is
reconstructed from `873a344` / `a1946df` and the current tree, not from a plan
that preceded them.

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`

## 1. Prove the tag
- [x] 1.1 `version.ReleaseVersion() string` — ldflags-stamped `Version` first, else `debug.ReadBuildInfo().Main.Version` with `v` stripped, else `""`.
- [x] 1.2 Reject `(devel)`, pseudo-versions (`pseudoVersionRE`) and any prerelease qualifier (`isPlainSemVer`).
- [x] 1.3 `IsRelease()` = `ReleaseVersion() != ""`.

## 2. Pin the right tag
- [x] 2.1 `SynthGoMod` writes `"v" + version.ReleaseVersion()`, not `"v" + Version` (`pkg/emit/program.go:509,522-526`).
- [x] 2.2 `pkg/emit/gomod_test.go` covers the emitted go.mod for both branches.

## 3. One reporter
- [x] 3.1 `version.Self()` — published tag when there is one, in-source `Version` otherwise.
- [x] 3.2 `cljgo version` (`Full`), `(cljgo-version)`, `*cljgo-version*` and the nREPL handshake all read it, so the numbers cannot disagree.

## 4. A checkout is never a release (the widening, #189)
- [x] 4.1 `releaseVersionFromBuildInfo` returns `""` when any `vcs.*` build setting is present — Go stamps `Main.Version` from the nearest tag for a local `go build`, which made every post-v0.8.5 dev build claim to be a release.
- [x] 4.2 `pkg/version/version_test.go` — `TestBuildFromCheckoutIsNeverARelease`.

## 5. Tests
- [x] 5.1 `TestReleaseVersionFromBuildInfo` pins the mapping (real tag → release; `(devel)`, pseudo-version, prerelease qualifier, empty → not a release).
- [x] 5.2 `TestReleaseVersionPrefersLdflags` pins that a stamped `Version` wins outright and that the dev default never becomes a release just because the test binary carries build info of its own.

## 6. End-to-end acceptance (release-gated, not CI-runnable)
- [x] 6.1 `go install …/cmd/cljgo@vX.Y.Z` + `cljgo new -template cli` + `cljgo build` is the reproduction and the acceptance check. It cannot run in CI against an unreleased fix — the first binary able to pass it is the one built from the tag containing the fix. Confirmed against the v0.8.6 release.
