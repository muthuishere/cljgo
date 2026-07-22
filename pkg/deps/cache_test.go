package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheRootPrecedence(t *testing.T) {
	t.Setenv("CLJGO_CACHE", "/x/y")
	if r, _ := CacheRoot(); r != "/x/y" {
		t.Fatalf("CLJGO_CACHE not honored: %s", r)
	}
	os.Unsetenv("CLJGO_CACHE")
	t.Setenv("XDG_CACHE_HOME", "/xdg")
	if r, _ := CacheRoot(); r != filepath.Join("/xdg", "cljgo") {
		t.Fatalf("XDG_CACHE_HOME not honored: %s", r)
	}
}

func TestIdentityKeyStableAndDistinct(t *testing.T) {
	a := IdentityKey("u", "s", "sub")
	if a != IdentityKey("u", "s", "sub") {
		t.Fatal("IdentityKey not stable")
	}
	if a == IdentityKey("u", "s", "other") {
		t.Fatal("IdentityKey collides across subdir")
	}
	if len(a) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(a))
	}
}

func TestTreeHashDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "hello", "sub/b.txt": "world"})
	h1, err := TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 13-byte tamper.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello, tamper"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2, err := TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("TreeHash did not change after tamper")
	}
	if !strings.HasPrefix(h1, "sha256:") {
		t.Fatalf("tree hash should be sha256-prefixed: %s", h1)
	}
}

func TestCacheCleanRemovesReadOnlyTree(t *testing.T) {
	cache := newCache(t)
	// Simulate a published immutable entry.
	entry := filepath.Join(cache, "src", "deadbeef")
	writeFiles(t, entry, map[string]string{"a.clj": "(ns a)"})
	if err := markReadOnly(entry); err != nil {
		t.Fatal(err)
	}
	// A plain RemoveAll on a 0555 tree fails on this platform's dir perms.
	if err := CacheClean(); err != nil {
		t.Fatalf("CacheClean failed: %v", err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("cache root should be gone, stat err=%v", err)
	}
}
