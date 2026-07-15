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
	"runtime"
	"strconv"
	"strings"
)

// Version is cljgo's own version: plain SemVer, "major.minor.patch" with an
// optional "-qualifier". Kept clean (no host/language suffix) because Go
// module tags must be valid SemVer for `go install …@latest` to resolve —
// the host and language levels are reported alongside it, not baked into it.
//
// Set via -ldflags on release builds; the default is the in-development value.
var Version = "0.1.0"

// ClojureVersion is the Clojure language level cljgo targets — the version
// of real JVM Clojure the conformance suite is verified against (the
// semantic oracle, per CLAUDE.md). This is what (clojure-version) reports:
// a program asking "what Clojure am I?" is asking about the language, not
// about our implementation. (cljgo-version) answers the latter.
const ClojureVersion = "1.12.5"

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
func Full() string {
	return Version + " (Go " + GoVersion() + ", Clojure " + ClojureVersion + ")"
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
