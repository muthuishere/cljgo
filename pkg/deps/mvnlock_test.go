package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMavenLockRoundTrips proves the lock's new Maven shape survives a
// write/read cycle intact, INCLUDING the per-namespace purity map — which is
// what makes the report available offline, before a byte is fetched.
func TestMavenLockRoundTrips(t *testing.T) {
	r := newMvnRepo(t)
	r.publish(Coord{"hiccup", "hiccup", "1.0.5"}, "", mixedHiccup())

	res := mustResolve(t, r, []Dep{{Name: "hiccup/hiccup", MvnVersion: "1.0.5"}}, nil)

	path := filepath.Join(t.TempDir(), "build.lock.edn")
	if err := WriteLock(path, res.Lock); err != nil {
		t.Fatal(err)
	}
	// Deterministic: two writes of the same graph are byte-identical.
	b1, _ := os.ReadFile(path)
	if err := WriteLock(path, res.Lock); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(path)
	if string(b1) != string(b2) {
		t.Fatal("the lock is not byte-deterministic")
	}
	if !strings.Contains(string(b1), ":mvn/namespaces") {
		t.Fatalf("the purity map is not in the lock:\n%s", b1)
	}

	back, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	got := back.find("hiccup/hiccup")
	want := res.Lock.find("hiccup/hiccup")
	if got == nil || !got.IsMvn() {
		t.Fatal("the maven entry did not round-trip")
	}
	for _, f := range []struct{ name, a, b string }{
		{"group", got.MvnGroup, want.MvnGroup},
		{"artifact", got.MvnArtifact, want.MvnArtifact},
		{"version", got.MvnVersion, want.MvnVersion},
		{"repo", got.MvnRepo, want.MvnRepo},
		{"sha256", got.MvnSHA256, want.MvnSHA256},
		{"pom-sha256", got.MvnPomSHA, want.MvnPomSHA},
		{"tree/hash", got.TreeHash, want.TreeHash},
	} {
		if f.a != f.b {
			t.Errorf("%s: got %q, want %q", f.name, f.a, f.b)
		}
	}
	if len(got.MvnPureNS) != 8 || len(got.MvnJavaNS) != 2 {
		t.Errorf("the purity map did not round-trip: %d pure / %d java", len(got.MvnPureNS), len(got.MvnJavaNS))
	}
	if got.MvnJavaNS["hiccup.compiler"] == "" {
		t.Error("the per-namespace reason did not round-trip")
	}
}

// TestGitOnlyLockIsUnchanged is the regression guard on the existing shape: a
// lock with no Maven deps must be byte-identical to what it was before this
// change — no stray :mvn/* keys, no reordering.
func TestGitOnlyLockIsUnchanged(t *testing.T) {
	l := &Lock{Version: LockVersion, BuildHash: "h", Deps: []LockedDep{
		{Name: "greetlib", GitURL: "https://example/g", GitRef: "v1", GitSHA: "abc", TreeHash: "sha256:x", Paths: []string{"src"}},
		{Name: "sibling", LocalUnlocked: true, Paths: []string{"src"}},
	}}
	path := filepath.Join(t.TempDir(), "build.lock.edn")
	if err := WriteLock(path, l); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), ":mvn/") {
		t.Fatalf("a git-only lock gained maven keys:\n%s", b)
	}
	for _, want := range []string{":git/url", ":git/sha", ":tree/hash", ":local/unlocked?", ":pure? true"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the existing lock shape lost %q:\n%s", want, b)
		}
	}
}
