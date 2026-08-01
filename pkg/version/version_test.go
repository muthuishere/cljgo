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
			// A real `go install module@vX.Y.Z` binary carries NO vcs.*
			// settings — it is resolved through the module proxy, not built
			// from a checkout. Verified with `go version -m` on an actual
			// `go install …/cmd/cljgo@v0.8.6` binary.
			//
			// This fixture used to pair a module version WITH vcs.revision, a
			// combination that never occurs, and that fiction is what let the
			// release-detection bug through: Go also stamps Main.Version from
			// the nearest tag for a plain local build, so the two cases are
			// told apart by exactly the VCS data this fixture wrongly gave to
			// both.
			name: "go install module@version",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "v0.8.2"}},
			want: "[dev build, module v0.8.2]",
		},
		{
			// The local build Go stamps with the nearest tag: report the
			// COMMIT, never "module", because nobody requested that version.
			name: "local build in a tagged repo",
			bi: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.8.5"},
				Settings: []debug.BuildSetting{
					{Key: "vcs", Value: "git"},
					{Key: "vcs.revision", Value: "0b230f271bc21ad8e66740cba2488e58c78cfc8"},
					{Key: "vcs.modified", Value: "false"},
				},
			},
			want: "[dev build, commit 0b230f271bc2]",
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

// TestReleaseVersionFromBuildInfo is the regression test for the defect
// koine and toolnexus both hit independently: `go install
// github.com/muthuishere/cljgo/cmd/cljgo@v0.8.4` produced a binary that
// REFUSED TO BUILD ANY PROJECT — "this is a dev cljgo binary (version
// 0.1.0-dev), so the generated go.mod needs a local runtime tree" — because
// IsRelease() consulted only the ldflags-stamped Version, which `go install`
// never sets. The requested tag was sitting in the binary's own build info
// the whole time. templates/web/Dockerfile installs cljgo exactly that way,
// so this broke the shipped web template in Docker (ADR 0116).
func TestReleaseVersionFromBuildInfo(t *testing.T) {
	cases := []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{
			// THE bug: a real requested tag is authoritative — the Go
			// toolchain resolved and checksum-verified it against the proxy.
			name: "go install module@v0.8.4 is a release",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "v0.8.4"}},
			want: "0.8.4",
		},
		{
			name: "local build is not a release",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want: "",
		},
		{
			// A pseudo-version is Go-synthesized from a commit, never a
			// published tag — pinning it in a generated go.mod would name a
			// module version nobody released.
			name: "pseudo-version is not a release",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "v0.8.3-0.20260731112918-0b230f271bc2"}},
			want: "",
		},
		{
			// goreleaser snapshot builds carry a qualifier; they are not
			// published tags either.
			name: "prerelease qualifier is not a release",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: "v0.9.0-rc1"}},
			want: "",
		},
		{
			name: "no build info version at all",
			bi:   &debug.BuildInfo{Main: debug.Module{Version: ""}},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := releaseVersionFromBuildInfo(c.bi); got != c.want {
				t.Errorf("releaseVersionFromBuildInfo() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestReleaseVersionPrefersLdflags pins the goreleaser path: when Version IS
// stamped, that value wins outright and no build info is consulted.
func TestReleaseVersionPrefersLdflags(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })

	Version = "0.8.4"
	if got := ReleaseVersion(); got != "0.8.4" {
		t.Errorf("ldflags-stamped ReleaseVersion() = %q, want 0.8.4", got)
	}
	if !IsRelease() {
		t.Error("ldflags-stamped IsRelease() = false, want true")
	}
	if got := Full(); !strings.HasPrefix(got, "0.8.4 (Go ") {
		t.Errorf("Full() = %q, want it to lead with the stamped 0.8.4", got)
	}

	// The in-source default is never a release, and must not become one just
	// because the test binary has build info of its own.
	Version = "0.1.0-dev"
	if got := ReleaseVersion(); got != "" {
		t.Errorf("dev-default ReleaseVersion() = %q, want \"\"", got)
	}
}

// TestBuildFromCheckoutIsNeverARelease is the regression test for a defect
// ADR 0116 shipped: Go stamps BuildInfo.Main.Version from the nearest VCS tag
// even for a plain local `go build` inside the repo, so once v0.8.5 was
// tagged every dev build reported v0.8.5 and IsRelease() went true.
//
// The visible half was a version line that lied. The serious half was silent:
// SynthGoMod took the release-pin branch and wrote `require …/cljgo v0.8.5`
// with NO replace, so a developer building a project with their own cljgo
// compiled against the PUBLISHED runtime instead of their working tree —
// local changes ignored, with nothing to indicate it.
//
// The discriminator is the presence of VCS settings: `go install
// module@vX.Y.Z` resolves through the module proxy and records none, while
// any build from a checkout records vcs=git plus vcs.revision.
func TestBuildFromCheckoutIsNeverARelease(t *testing.T) {
	fromCheckout := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.8.5"}, // Go's nearest-tag stamp
		Settings: []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: "f05506f33a3a012e2cf859588c8f919a69727bf6"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	if got := releaseVersionFromBuildInfo(fromCheckout); got != "" {
		t.Errorf("a build from a git checkout reported release %q; it must never be a release", got)
	}

	// The go-install path is unchanged: a proxy-resolved module, no VCS data.
	fromProxy := &debug.BuildInfo{Main: debug.Module{Version: "v0.8.5"}}
	if got := releaseVersionFromBuildInfo(fromProxy); got != "0.8.5" {
		t.Errorf("go install …@v0.8.5 = %q, want 0.8.5", got)
	}
}
