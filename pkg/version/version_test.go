package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

// TestIsRelease covers both binary shapes ADR 0028 distinguishes: the
// in-source dev default and the release-ldflags-stamped plain tag
// (.goreleaser.yaml stamps `{{ .Version }}` = the git tag minus "v",
// e.g. "0.1.0" — the exact shape the release case simulates here).
func TestIsRelease(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"0.1.0", true},       // release ldflags stamp
		{"1.12.5", true},      // any plain tag
		{"0.1.0-dev", false},  // in-source default shape
		{"0.1.0-rc1", false},  // prerelease
		{"0.1.1-next", false}, // goreleaser snapshot shape
		{"0.1", false},        // not a full tag
		{"garbage", false},    // unparseable
	}
	restore := Version
	defer func() { Version = restore }()
	for _, c := range cases {
		Version = c.v
		if got := IsRelease(); got != c.want {
			t.Errorf("IsRelease() with Version=%q = %v, want %v", c.v, got, c.want)
		}
	}
}

// TestDefaultIsDev pins the ADR 0028 dev marker: the in-source default must
// never look like a release, or a source-built binary would emit go.mods
// pinning a module version it wasn't built from.
func TestDefaultIsDev(t *testing.T) {
	if IsRelease() {
		t.Fatalf("in-source default Version=%q must not be a release", Version)
	}
	if Parse(Version).Qualifier == "" {
		t.Fatalf("in-source default Version=%q must carry a dev qualifier", Version)
	}
}

// TestFullReleaseCarriesNoDevSuffix pins the release shape: once Version is
// a plain-tag stamp (goreleaser's -ldflags, IsRelease() == true), Full()
// must report exactly that tag and never append a "[dev build ...]" suffix
// — a release binary's own tag already identifies the build (issue #170).
func TestFullReleaseCarriesNoDevSuffix(t *testing.T) {
	restore := Version
	defer func() { Version = restore }()
	Version = "0.8.2"
	full := Full()
	if !strings.HasPrefix(full, "0.8.2 (Go ") {
		t.Fatalf("Full() = %q, want it to start with the clean release tag", full)
	}
	if strings.Contains(full, "dev build") {
		t.Fatalf("Full() = %q, a release build must not carry a dev-build suffix", full)
	}
}

// TestDevSuffixFromBuildInfo is the regression test for issue #170: every
// build cljgo's own -ldflags never stamps (`go build`, `go install`, a
// plain dev binary) must still identify itself — a commit when the
// toolchain embedded VCS data, the requested module version for
// `go install module@version`, and a dirty marker for an uncommitted
// worktree — instead of the bare, undifferentiated "0.1.0-dev" every such
// build reported before this change.
func TestDevSuffixFromBuildInfo(t *testing.T) {
	cases := []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{
			name: "go install module@version, clean",
			bi: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.8.2"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "0b230f271bc21ad8e66740cba2488e58c78cfc8"},
					{Key: "vcs.modified", Value: "false"},
				},
			},
			want: "[dev build, module v0.8.2, commit 0b230f271bc2]",
		},
		{
			name: "local go build from a dirty git checkout",
			bi: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "0b230f271bc21ad8e66740cba2488e58c78cfc8"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			want: "[dev build, commit 0b230f271bc2, dirty]",
		},
		{
			name: "no VCS data at all (-buildvcs=false)",
			bi: &debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: nil,
			},
			want: "[dev build, commit unknown]",
		},
		{
			// `go build`/`go install` run inside cljgo's own module (any git
			// checkout — not just `go install module@version`) gets a
			// Go-synthesized pseudo-version here, built from the exact same
			// commit already reported as "commit" below — showing it too
			// would just say the same thing twice in two shapes.
			name: "pseudo-version main module is not surfaced twice",
			bi: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.8.3-0.20260731112918-0b230f271bc2+dirty"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "0b230f271bc21ad8e66740cba2488e58c78cfc8"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			want: "[dev build, commit 0b230f271bc2, dirty]",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := devSuffixFromBuildInfo(c.bi); got != c.want {
				t.Errorf("devSuffixFromBuildInfo() = %q, want %q", got, c.want)
			}
		})
	}
}
