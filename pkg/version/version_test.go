package version

import "testing"

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
