// Package version is the single source of truth for cljgo's version, the
// Go toolchain hosting it, and the Clojure language level it targets.
//
// Version is plain SemVer and is overridable at link time, so a release
// build carries its git tag without a source edit:
//
//	go build -ldflags "-X github.com/muthuishere/cljgo/pkg/version.Version=0.1.0"
//
// The CLI (cmd/cljgo) and the language-level vars (*cljgo-version*,
// *clojure-version* — pkg/eval/version_builtins.go) both read from here, so
// `cljgo --version` and (cljgo-version) can never disagree.
package version

import (
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// Version is cljgo's own version: plain SemVer, "major.minor.patch" with an
// optional "-qualifier". Kept clean (no host/language suffix) because Go
// module tags must be valid SemVer for `go install …@latest` to resolve —
// the host and language levels are reported alongside it, not baked into it.
//
// Set via -ldflags on release builds; the in-source default carries the
// "-dev" qualifier so a source-built binary is distinguishable from a
// release binary (ADR 0028 — SynthGoMod trusts IsRelease() to decide
// whether the generated go.mod may pin the published module at Version).
var Version = "0.1.0-dev"

// ReleaseVersion is the published module tag this binary is PROVABLY built
// from, as plain SemVer without the "v" ("0.8.4"), or "" when there is no
// such proof (ADR 0116).
//
// Two build paths produce a genuine release, and only one of them was ever
// recognized:
//
//   - goreleaser stamps Version via -ldflags. That is the tagged-archive
//     path, and it is what IsRelease() originally meant.
//   - `go install github.com/muthuishere/cljgo/cmd/cljgo@v0.8.4` compiles
//     from the module proxy with NO ldflags, so Version keeps its in-source
//     "0.1.0-dev" default. The requested tag is still authoritative — the Go
//     toolchain resolved and checksum-verified it — and it is recorded in
//     the binary as BuildInfo.Main.Version.
//
// Missing the second path is what made a `go install`ed cljgo refuse to
// build anything: SynthGoMod saw IsRelease() == false, fell through to the
// walk-up runtime search, and failed with "needs a local runtime tree" on a
// binary whose whole promise is that it needs no source checkout. That path
// is not a corner case — templates/web/Dockerfile installs cljgo exactly
// that way, so the shipped web template could not build in Docker.
//
// Only a real requested tag counts. "(devel)" (a plain local build) and
// Go-synthesized pseudo-versions are rejected, as is any tag carrying a
// prerelease qualifier (a goreleaser snapshot), so nothing here can invent
// a module version that was never published.
func ReleaseVersion() string {
	if isPlainSemVer(Version) {
		return Version // ldflags-stamped: the tagged-archive path
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return releaseVersionFromBuildInfo(bi)
}

// releaseVersionFromBuildInfo is ReleaseVersion's pure core over the
// module version Go recorded, split out for the same reason
// devSuffixFromBuildInfo is: the test binary's own build info carries no
// requested tag, so the mapping is untestable through the real thing.
func releaseVersionFromBuildInfo(bi *debug.BuildInfo) string {
	v := strings.TrimPrefix(bi.Main.Version, "v")
	if v == "" || bi.Main.Version == "(devel)" || isPseudoVersion(bi.Main.Version) {
		return ""
	}
	if !isPlainSemVer(v) {
		return ""
	}
	return v
}

// isPlainSemVer reports whether s is exactly "major.minor.patch" with no
// prerelease qualifier — the shape a published module tag has.
func isPlainSemVer(s string) bool {
	in := Parse(s)
	return in.Qualifier == "" && in.String() == s
}

// Self is the version this binary should REPORT: the published tag when
// there is one, else the in-source Version. Every reporter goes through
// here — `cljgo version` (Full), (cljgo-version) / *cljgo-version*, and the
// nREPL handshake — so they cannot disagree, which is the promise this
// package's doc comment makes and which a `go install …@v0.8.4` binary would
// otherwise break: Full() would say 0.8.4 while (cljgo-version) still said
// 0.1.0-dev (ADR 0116).
func Self() string {
	if rv := ReleaseVersion(); rv != "" {
		return rv
	}
	return Version
}

// IsRelease reports whether this binary may pin the published module: true
// exactly when ReleaseVersion() found a tag, so "v"+ReleaseVersion() names a
// module the generated go.mod can require without a local replace
// (ADR 0028, widened by ADR 0116).
func IsRelease() bool {
	return ReleaseVersion() != ""
}

// ClojureVersion is the Clojure language level cljgo targets — the version
// of real JVM Clojure the conformance suite is verified against (the
// semantic oracle, per CLAUDE.md). This is what (clojure-version) reports:
// a program asking "what Clojure am I?" is asking about the language, not
// about our implementation. (cljgo-version) answers the latter.
const ClojureVersion = "1.12.5"

// CoreAsyncVersion is the version of JVM org.clojure/core.async that cljgo's
// clojure.core.async is verified against — the semantic oracle for channels,
// alts!/alt!, and the mult/pub/mix distribution surface (ADR 0040). core.async
// is a separate library from Clojure with its own release cadence, so its
// oracle version is tracked separately from ClojureVersion. The conformance
// suite's chan-*.clj expectations are frozen from `clojure -Sdeps
// '{:deps {org.clojure/core.async {:mvn/version "1.6.681"}}}'`.
const CoreAsyncVersion = "1.6.681"

// GoVersion is the Go toolchain hosting this binary, without the "go"
// prefix ("1.26.3"). Read from the runtime rather than hardcoded or
// injected, so it is always the toolchain that actually built this binary
// and cannot drift from reality.
func GoVersion() string {
	return strings.TrimPrefix(runtime.Version(), "go")
}

// Full is the human-readable version line: cljgo's SemVer plus the host Go
// toolchain and the targeted Clojure level, e.g.
//
//	0.1.0 (Go 1.26.3, Clojure 1.12.5)
//
// This is the REPL banner and the `cljgo version` body. All three numbers
// matter to someone reporting a bug: ours, the host's, the language's.
//
// A release binary (IsRelease(), stamped by goreleaser's -ldflags) reports
// its clean tag and nothing more — that value already identifies the build.
// Every OTHER build path (`go build`, `go install`, a plain dev binary) never
// gets that stamp, so Version stays the in-source "0.1.0-dev" default and,
// before this, told a bug reporter nothing (issue #170). Those builds get a
// DevSuffix appended instead, built from the VCS data Go's own toolchain
// already embeds (runtime/debug.ReadBuildInfo) — never a source edit, never
// a bump of Version.
func Full() string {
	base := Self() + " (Go " + GoVersion() + ", Clojure " + ClojureVersion + ")"
	if IsRelease() {
		return base
	}
	if suffix := DevSuffix(); suffix != "" {
		return base + " " + suffix
	}
	return base
}

// DevSuffix describes a non-release build's provenance for the version
// line, e.g. "[dev build, module v0.8.2, commit 0b230f2]" or "[dev build,
// commit 0b230f2, dirty]". Empty when no build info is available at all
// (e.g. -buildvcs=false), in which case Full() falls back to just the
// in-source dev version with no further claim.
//
// Sourced from runtime/debug.ReadBuildInfo(), which Go's toolchain populates
// on every module-aware build without any ldflags:
//   - BuildInfo.Main.Version is the requested module version for
//     `go install github.com/muthuishere/cljgo/cmd/cljgo@v0.8.2` — the exact
//     tag a user asked for, even though Version itself was never stamped.
//   - the "vcs.revision" / "vcs.modified" build settings are the commit (and
//     dirty-worktree flag) for a local `go build`/`go install` from a git
//     checkout.
func DevSuffix() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return devSuffixFromBuildInfo(bi)
}

// devSuffixFromBuildInfo is DevSuffix's pure core, split out so the mapping
// from a *debug.BuildInfo to the rendered suffix is testable without relying
// on the test binary's own (VCS-stamping-free, per Go's toolchain) build
// info — see version_test.go.
func devSuffixFromBuildInfo(bi *debug.BuildInfo) string {
	var revision string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
			if len(revision) > 12 {
				revision = revision[:12]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	// bi.Main.Version is "(devel)" for a plain local build/install (not
	// `@<version>`) and "" is never returned by ReadBuildInfo. A plain
	// `go build`/`go install` run inside cljgo's own module (any git
	// checkout, not just a fresh clone) also gets a Go-computed PSEUDO-
	// version instead — "v0.8.3-0.20260731112918-0b230f271bc2[+dirty]" —
	// built from the very same commit and dirty bit already surfaced below
	// as "commit"/"dirty", so showing both would just say the same thing
	// twice in two shapes. Only a real requested tag (`go install
	// module@v0.8.2`) is worth surfacing as "module".
	moduleVersion := bi.Main.Version
	if moduleVersion == "" || moduleVersion == "(devel)" || isPseudoVersion(moduleVersion) {
		moduleVersion = ""
	}

	var parts []string
	parts = append(parts, "dev build")
	if moduleVersion != "" {
		parts = append(parts, "module "+moduleVersion)
	}
	if revision != "" {
		parts = append(parts, "commit "+revision)
	}
	if dirty {
		parts = append(parts, "dirty")
	}
	if len(parts) == 1 {
		// Nothing but the plain "dev build" marker — no VCS data at all
		// (e.g. built with -buildvcs=false, or outside any git checkout).
		return "[dev build, commit unknown]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// pseudoVersionRE matches Go's module pseudo-version suffix — a 14-digit
// timestamp and a 12-hex-char commit (optionally preceded by "0." or
// "<pre>.0." when a prior tag exists, and followed by an optional
// "+dirty"/"+incompatible" build tag — golang.org/x/mod/module.
// IsPseudoVersion's documented shape). A synthesized version, not a tag
// anyone requested.
var pseudoVersionRE = regexp.MustCompile(`[0-9]{14}-[0-9a-f]{12}(\+[0-9A-Za-z.]+)?$`)

// isPseudoVersion reports whether v is a Go-synthesized pseudo-version
// rather than a real requested tag.
func isPseudoVersion(v string) bool {
	return pseudoVersionRE.MatchString(v)
}

// Info is the parsed shape of a version string, mirroring Clojure's
// *clojure-version* map: {:major 1 :minor 12 :incremental 5 :qualifier nil}.
type Info struct {
	Major       int
	Minor       int
	Incremental int
	Qualifier   string // "" when absent (=> nil in the Clojure map)
}

// Parse splits "1.12.5-alpha1" into its components. Missing numeric parts
// are 0; a trailing "-qualifier" is split off first. Parse is total:
// unparseable segments yield 0 rather than an error, since a version string
// is not user input we need to diagnose.
func Parse(s string) Info {
	var in Info
	if i := strings.IndexByte(s, '-'); i >= 0 {
		in.Qualifier = s[i+1:]
		s = s[:i]
	}
	dst := []*int{&in.Major, &in.Minor, &in.Incremental}
	for i, p := range strings.Split(s, ".") {
		if i >= len(dst) {
			break
		}
		*dst[i], _ = strconv.Atoi(p)
	}
	return in
}

// String renders an Info back to "major.minor.incremental[-qualifier]" —
// the inverse of Parse.
func (in Info) String() string {
	s := strconv.Itoa(in.Major) + "." + strconv.Itoa(in.Minor) + "." + strconv.Itoa(in.Incremental)
	if in.Qualifier != "" {
		s += "-" + in.Qualifier
	}
	return s
}
