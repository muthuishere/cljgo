package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleLock() *Lock {
	return &Lock{
		Version:   LockVersion,
		BuildHash: "sha256:abc123",
		Deps: []LockedDep{
			{
				Name:     "zeta",
				GitURL:   "file:///tmp/zeta",
				GitRef:   "v1.0.0",
				GitSHA:   "1111111111111111111111111111111111111111",
				TreeHash: "sha256:deadbeef",
				Paths:    []string{"src"},
				Requires: []string{"alpha"},
				Impure: &Impurity{
					GoRequire: []GoReq{{Path: "github.com/google/uuid", Version: "v1.6.0"}},
					FFI:       []string{"sqlite3"},
				},
			},
			{
				Name:     "alpha",
				GitURL:   "file:///tmp/alpha",
				GitRef:   "main",
				GitSHA:   "2222222222222222222222222222222222222222",
				TreeHash: "sha256:cafef00d",
				Paths:    []string{"src", "extra"},
				Pure:     true,
			},
			{
				Name:          "local-hole",
				Paths:         []string{"src"},
				Requires:      []string{"alpha"},
				LocalUnlocked: true,
				Pure:          true,
			},
		},
	}
}

func TestLockRoundTripByteIdentical(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.edn")
	p2 := filepath.Join(dir, "b.edn")
	if err := WriteLock(p1, sampleLock()); err != nil {
		t.Fatal(err)
	}
	if err := WriteLock(p2, sampleLock()); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(p1)
	b2, _ := os.ReadFile(p2)
	if string(b1) != string(b2) {
		t.Fatalf("two writes of the same graph differ:\n%s\n---\n%s", b1, b2)
	}

	// Read it back, write again — must be byte-identical (idempotent).
	l, err := LoadLock(p1)
	if err != nil {
		t.Fatal(err)
	}
	p3 := filepath.Join(dir, "c.edn")
	if err := WriteLock(p3, l); err != nil {
		t.Fatal(err)
	}
	b3, _ := os.ReadFile(p3)
	if string(b1) != string(b3) {
		t.Fatalf("load->write not byte-identical:\n%s\n---\n%s", b1, b3)
	}
}

func TestLockTwoIndependentWritesByteEqual(t *testing.T) {
	// Same content, deps supplied in different order -> name-sorted output is
	// byte-equal.
	dir := t.TempDir()
	a := sampleLock()
	b := sampleLock()
	b.Deps[0], b.Deps[2] = b.Deps[2], b.Deps[0] // shuffle
	p1 := filepath.Join(dir, "a.edn")
	p2 := filepath.Join(dir, "b.edn")
	if err := WriteLock(p1, a); err != nil {
		t.Fatal(err)
	}
	if err := WriteLock(p2, b); err != nil {
		t.Fatal(err)
	}
	x, _ := os.ReadFile(p1)
	y, _ := os.ReadFile(p2)
	if string(x) != string(y) {
		t.Fatalf("independent writes differ despite same graph")
	}
}

func TestLockLocalHolePreserved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "build.lock.edn")
	if err := WriteLock(p, sampleLock()); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), ":local/unlocked? true") {
		t.Fatalf("local hole marker missing:\n%s", raw)
	}
	l, err := LoadLock(p)
	if err != nil {
		t.Fatal(err)
	}
	hole := l.find("local-hole")
	if hole == nil {
		t.Fatal("local-hole not read back")
	}
	if !hole.LocalUnlocked {
		t.Fatal("LocalUnlocked lost on round-trip")
	}
	if hole.TreeHash != "" || hole.GitSHA != "" {
		t.Fatalf("local hole must be unhashed, got tree=%q sha=%q", hole.TreeHash, hole.GitSHA)
	}
	if len(hole.Requires) != 1 || hole.Requires[0] != "alpha" {
		t.Fatalf("local hole transitive deps not preserved: %v", hole.Requires)
	}
}

func TestLockImpurityRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "l.edn")
	if err := WriteLock(p, sampleLock()); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLock(p)
	if err != nil {
		t.Fatal(err)
	}
	z := l.find("zeta")
	if z == nil || z.Impure == nil {
		t.Fatal("zeta impurity lost")
	}
	if z.Pure {
		t.Fatal("impure dep marked pure")
	}
	if len(z.Impure.GoRequire) != 1 || z.Impure.GoRequire[0].Path != "github.com/google/uuid" {
		t.Fatalf("go-require lost: %+v", z.Impure.GoRequire)
	}
	if len(z.Impure.FFI) != 1 || z.Impure.FFI[0] != "sqlite3" {
		t.Fatalf("ffi lost: %v", z.Impure.FFI)
	}
	a := l.find("alpha")
	if a == nil || !a.Pure || a.Impure != nil {
		t.Fatal("pure dep not read as pure")
	}
}

func TestLoadLockAbsent(t *testing.T) {
	l, err := LoadLock(filepath.Join(t.TempDir(), "nope.edn"))
	if err != nil {
		t.Fatalf("absent lock should not error: %v", err)
	}
	if l != nil {
		t.Fatalf("absent lock should be nil, got %+v", l)
	}
}
