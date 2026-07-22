package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const pureManifest = `{:paths ["src"]}`

func TestResolveColdThenWarmOffline(t *testing.T) {
	repo, url := makeRepo(t, map[string]string{
		"src/a.clj":          "(ns a)",
		"cljgo.manifest.edn": pureManifest,
	})
	cache := newCache(t)
	proj := t.TempDir()

	res, err := Resolve([]Dep{{Name: "a", GitURL: url, GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Update: true})
	if err != nil {
		t.Fatalf("cold resolve: %v", err)
	}
	d := res.Lock.find("a")
	if d == nil || d.GitSHA != headSHA(t, repo) {
		t.Fatalf("cold resolve did not pin HEAD sha")
	}
	if d.TreeHash == "" {
		t.Fatal("no tree hash recorded")
	}
	if len(res.Roots) != 1 || !strings.HasPrefix(res.Roots[0], cache) {
		t.Fatalf("root not under cache: %v", res.Roots)
	}

	// Remove the remote entirely; a warm, locked, offline build must still
	// resolve from the cache.
	os.RemoveAll(repo)
	res2, err := Resolve([]Dep{{Name: "a", GitURL: url, GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Lock: res.Lock, Offline: true})
	if err != nil {
		t.Fatalf("warm offline resolve failed: %v", err)
	}
	if res2.Lock.find("a").TreeHash != d.TreeHash {
		t.Fatal("warm resolve produced a different tree hash")
	}
}

func TestResolveForceMovedTagUnchanged(t *testing.T) {
	repo, url := makeRepo(t, map[string]string{
		"src/a.clj":          "(ns a) ;; v1",
		"cljgo.manifest.edn": pureManifest,
	})
	gitRun(t, repo, "tag", "v1")
	newCache(t)
	proj := t.TempDir()

	res, err := Resolve([]Dep{{Name: "a", GitURL: url, GitRef: "v1"}},
		ResolveOptions{ProjectDir: proj, Update: true})
	if err != nil {
		t.Fatal(err)
	}
	sha1 := res.Lock.find("a").GitSHA
	tree1 := res.Lock.find("a").TreeHash

	// Force-move the tag to a different, content-changing commit.
	writeFiles(t, repo, map[string]string{"src/a.clj": "(ns a) ;; v2 DIFFERENT"})
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "v2")
	gitRun(t, repo, "tag", "-f", "v1")
	if headSHA(t, repo) == sha1 {
		t.Fatal("tag did not actually move")
	}

	// The locked build ignores the moved tag and resolves the pinned content.
	res2, err := Resolve([]Dep{{Name: "a", GitURL: url, GitRef: "v1"}},
		ResolveOptions{ProjectDir: proj, Lock: res.Lock, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := res2.Lock.find("a"); got.GitSHA != sha1 || got.TreeHash != tree1 {
		t.Fatalf("locked build changed under a moved tag: sha %s->%s tree %s->%s",
			sha1, got.GitSHA, tree1, got.TreeHash)
	}
}

func TestResolveTamperIntegrityFailure(t *testing.T) {
	repo, url := makeRepo(t, map[string]string{
		"src/a.clj":          "(ns a)",
		"cljgo.manifest.edn": pureManifest,
	})
	cache := newCache(t)
	proj := t.TempDir()

	res, err := Resolve([]Dep{{Name: "a", GitURL: url, GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Update: true})
	if err != nil {
		t.Fatal(err)
	}
	sha := res.Lock.find("a").GitSHA

	// Tamper 13 bytes in the immutable cache entry.
	entry := filepath.Join(cache, "src", IdentityKey(url, sha, ""))
	if err := makeWritable(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry, "src", "a.clj"), []byte("(ns a) ;tamper"), 0o644); err != nil {
		t.Fatal(err)
	}

	os.RemoveAll(repo)
	_, err = Resolve([]Dep{{Name: "a", GitURL: url, GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Lock: res.Lock, Offline: true})
	if err == nil {
		t.Fatal("expected an integrity failure")
	}
	for _, want := range []string{"integrity", "expected", "got", res.Lock.find("a").TreeHash} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("integrity error must contain %q; got: %s", want, err.Error())
		}
	}
}

func TestResolveShaDivergenceErrors(t *testing.T) {
	_, url := makeRepo(t, map[string]string{"src/a.clj": "(ns a)", "cljgo.manifest.edn": pureManifest})
	newCache(t)
	proj := t.TempDir()

	lock := &Lock{Version: LockVersion, Deps: []LockedDep{{
		Name: "a", GitURL: url, GitRef: "main",
		GitSHA: "1111111111111111111111111111111111111111", TreeHash: "sha256:x", Paths: []string{"src"},
	}}}
	other := "2222222222222222222222222222222222222222"
	_, err := Resolve([]Dep{{Name: "a", GitURL: url, GitRef: other}},
		ResolveOptions{ProjectDir: proj, Lock: lock, Offline: true})
	if err == nil {
		t.Fatal("expected a lock/build sha divergence error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "1111111111111111111111111111111111111111") || !strings.Contains(msg, other) {
		t.Fatalf("divergence error must name both SHAs; got: %s", msg)
	}
}

func TestResolveConcurrentColdResolvers(t *testing.T) {
	_, url := makeRepo(t, map[string]string{"src/a.clj": "(ns a)", "cljgo.manifest.edn": pureManifest})
	cache := newCache(t)
	proj := t.TempDir()

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = Resolve([]Dep{{Name: "a", GitURL: url, GitRef: "main"}},
				ResolveOptions{ProjectDir: proj, Update: true})
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("resolver %d failed: %v", i, e)
		}
	}
	// Exactly one immutable entry, no temp leftovers.
	var real, tmp int
	for _, name := range srcEntries(t, cache) {
		if strings.HasPrefix(name, ".") {
			tmp++
		} else {
			real++
		}
	}
	if real != 1 {
		t.Fatalf("expected exactly 1 cache entry, got %d", real)
	}
	if tmp != 0 {
		t.Fatalf("expected 0 temp leftovers, got %d", tmp)
	}
}

func TestResolveTransitiveFromData(t *testing.T) {
	_, leafURL := makeRepo(t, map[string]string{"src/leaf.clj": "(ns leaf)", "cljgo.manifest.edn": pureManifest})
	midManifest := fmt.Sprintf(`{:paths ["src"]
 :deps [{:name "leaf" :git %q :ref "HEAD"}]
 :go-requires [{:path "github.com/google/uuid" :version "v1.6.0"}]}`, leafURL)
	_, midURL := makeRepo(t, map[string]string{"src/mid.clj": "(ns mid)", "cljgo.manifest.edn": midManifest})

	newCache(t)
	proj := t.TempDir()

	res, err := Resolve([]Dep{{Name: "mid", GitURL: midURL, GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Update: true, AllowCaps: map[string]bool{"go-require": true}})
	if err != nil {
		t.Fatalf("transitive resolve: %v", err)
	}
	if res.Lock.find("leaf") == nil {
		t.Fatal("transitive dep 'leaf' not recovered from manifest data")
	}
	mid := res.Lock.find("mid")
	if mid == nil || len(mid.Requires) != 1 || mid.Requires[0] != "leaf" {
		t.Fatalf("mid provenance (:requires) wrong: %+v", mid)
	}
	if len(res.GoRequires) != 1 || res.GoRequires[0].Path != "github.com/google/uuid" {
		t.Fatalf("dep go-require did not flow through: %+v", res.GoRequires)
	}
	if !res.Lock.find("leaf").Pure {
		t.Fatal("leaf should be pure")
	}
}

func TestResolvePurityUnacknowledgedRefusedBeforeFetch(t *testing.T) {
	newCache(t)
	proj := t.TempDir()
	// Impurity is readable from the lock alone -> refuse before any fetch.
	// The URL is bogus; if the gate did not fire first we would get a fetch
	// error instead of a capability error.
	lock := &Lock{Version: LockVersion, Deps: []LockedDep{{
		Name: "x", GitURL: "file:///definitely/not/here", GitRef: "main",
		GitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", TreeHash: "sha256:z",
		Paths: []string{"src"}, Impure: &Impurity{FFI: []string{"sqlite3"}},
	}}}
	_, err := Resolve([]Dep{{Name: "x", GitURL: "file:///definitely/not/here", GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Lock: lock, Offline: true, AllowCaps: map[string]bool{}})
	if err == nil {
		t.Fatal("expected refusal of unacknowledged impurity")
	}
	if !strings.Contains(err.Error(), "ffi") {
		t.Fatalf("refusal must name the :ffi capability; got: %s", err.Error())
	}
	if strings.Contains(err.Error(), "offline") {
		t.Fatalf("gate should fire BEFORE fetch, not report an offline/fetch error: %s", err.Error())
	}
}

func TestResolveCgoUnderCrossRefusedFfiPermitted(t *testing.T) {
	proj := t.TempDir()

	// A cgo/c-link dep under declared cross-targets is refused.
	cgoDir := t.TempDir()
	writeFiles(t, cgoDir, map[string]string{
		"src/x.clj":          "(ns x)",
		"cljgo.manifest.edn": `{:paths ["src"] :c-link [{:pkg-config "sqlite3"}]}`,
	})
	_, err := Resolve([]Dep{{Name: "cgodep", Path: cgoDir}},
		ResolveOptions{ProjectDir: proj, CrossTargets: []string{"linux/amd64"}, AllowCaps: map[string]bool{"cgo": true}})
	if err == nil {
		t.Fatal("cgo under cross-targets must be refused even when allowed")
	}
	if !strings.Contains(err.Error(), "cgo") || !strings.Contains(strings.ToLower(err.Error()), "cross") {
		t.Fatalf("cgo refusal must mention cgo + cross: %s", err.Error())
	}

	// An ffi dep under the same cross-targets is permitted (separate switch).
	ffiDir := t.TempDir()
	writeFiles(t, ffiDir, map[string]string{
		"src/y.clj":          "(ns y)",
		"cljgo.manifest.edn": `{:paths ["src"] :ffi [{:lib "sqlite3" :soname {:darwin "libsqlite3.dylib"}}]}`,
	})
	res, err := Resolve([]Dep{{Name: "ffidep", Path: ffiDir}},
		ResolveOptions{ProjectDir: proj, CrossTargets: []string{"linux/amd64"}, AllowCaps: map[string]bool{"ffi": true}})
	if err != nil {
		t.Fatalf("ffi under cross-targets should be permitted: %v", err)
	}
	if len(res.Roots) != 1 {
		t.Fatalf("expected ffi dep root, got %v", res.Roots)
	}
}

func TestResolveDepFFIReachesGoRequires(t *testing.T) {
	// A pure consumer depends on an ffi dependency that carries a go-require;
	// that requirement must reach the consumer's merged GoRequires (ADR 0044
	// library-carries-FFI hole).
	proj := t.TempDir()
	depDir := t.TempDir()
	writeFiles(t, depDir, map[string]string{
		"src/y.clj": "(ns y)",
		"cljgo.manifest.edn": `{:paths ["src"]
 :ffi [{:lib "sqlite3" :soname {:darwin "libsqlite3.dylib"}}]
 :go-requires [{:path "github.com/ebitengine/purego" :version "v0.10.1"}]}`,
	})
	res, err := Resolve([]Dep{{Name: "d", Path: depDir}},
		ResolveOptions{ProjectDir: proj, AllowCaps: map[string]bool{"ffi": true, "go-require": true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GoRequires) != 1 || res.GoRequires[0].Path != "github.com/ebitengine/purego" {
		t.Fatalf("dep FFI go-require did not reach consumer: %+v", res.GoRequires)
	}
}

func TestResolveVendorOverride(t *testing.T) {
	proj := t.TempDir()
	vendorBase := t.TempDir()
	vdep := filepath.Join(vendorBase, "v")
	writeFiles(t, vdep, map[string]string{"src/x.clj": "(ns x)", "cljgo.manifest.edn": pureManifest})
	th, err := TreeHash(vdep)
	if err != nil {
		t.Fatal(err)
	}
	lock := &Lock{Version: LockVersion, Deps: []LockedDep{{
		Name: "v", GitURL: "file:///unused", GitRef: "main",
		GitSHA: "cafef00dcafef00dcafef00dcafef00dcafef00d", TreeHash: th, Paths: []string{"src"},
	}}}

	res, err := Resolve([]Dep{{Name: "v", GitURL: "file:///unused", GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Lock: lock, VendorDir: vendorBase, Offline: true})
	if err != nil {
		t.Fatalf("vendor override should resolve offline: %v", err)
	}
	if len(res.Roots) != 1 || res.Roots[0] != filepath.Join(vdep, "src") {
		t.Fatalf("vendor root not used: %v", res.Roots)
	}

	// A vendor tree not matching the lock hash is rejected.
	writeFiles(t, vdep, map[string]string{"src/x.clj": "(ns x) ;; edited"})
	_, err = Resolve([]Dep{{Name: "v", GitURL: "file:///unused", GitRef: "main"}},
		ResolveOptions{ProjectDir: proj, Lock: lock, VendorDir: vendorBase, Offline: true})
	if err == nil || !strings.Contains(err.Error(), "does not match the lock") {
		t.Fatalf("edited vendor tree must be rejected against the lock; got: %v", err)
	}
}

func TestResolvedRootsHandle(t *testing.T) {
	SetResolvedRoots([]string{"/a", "/b"})
	got := ResolvedRoots()
	if len(got) != 2 || got[0] != "/a" || got[1] != "/b" {
		t.Fatalf("roots handle round-trip failed: %v", got)
	}
	// Returned slice is a copy — mutating it must not affect the stored value.
	got[0] = "/mutated"
	if ResolvedRoots()[0] != "/a" {
		t.Fatal("ResolvedRoots leaked its backing array")
	}
}
