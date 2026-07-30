package build

// The lock-writing contract at the BUILD level (ADR 0052 decision 3 / ADR
// 0095): a first project-mode resolve writes build.lock.edn next to
// build.cljgo with the integrity fields filled in, and a second resolve reuses
// it — no re-shopping, no re-download, byte-identical file. The defect this
// covers was reported against a real Clojars build, where the committed tests
// only ever exercised pkg/deps in isolation and nothing asserted the file
// actually lands beside build.cljgo.
//
// NO NETWORK: the Maven repository is an httptest.Server serving a
// jar+pom tree built in memory, prepended with (mvn-repo …) so it answers
// before the defaults; the git dep is a local file:// repository; the cache is
// a throwaway CLJGO_CACHE.

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lockMvnRepo is a minimal Maven repository double: pom + jar + .sha1
// sidecars, counting hits so a reused lock can be proven to fetch nothing.
type lockMvnRepo struct {
	files map[string][]byte
	hits  int
	srv   *httptest.Server
}

func newLockMvnRepo(t *testing.T) *lockMvnRepo {
	t.Helper()
	r := &lockMvnRepo{files: map[string][]byte{}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p := strings.TrimPrefix(req.URL.Path, "/")
		b, ok := r.files[p]
		if !ok {
			http.NotFound(w, req)
			return
		}
		r.hits++
		w.Write(b)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *lockMvnRepo) put(path string, body []byte) {
	r.files[path] = body
	sum := sha1.Sum(body)
	r.files[path+".sha1"] = []byte(hex.EncodeToString(sum[:]) + "\n")
}

// publish adds one standalone (no <parent>, no <dependencies>) coordinate.
func (r *lockMvnRepo) publish(t *testing.T, group, artifact, version string, srcs map[string]string) {
	t.Helper()
	base := strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/" + version + "/" + artifact + "-" + version
	pom := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>%s</groupId>
  <artifactId>%s</artifactId>
  <version>%s</version>
  <packaging>jar</packaging>
</project>
`, group, artifact, version)
	r.put(base+".pom", []byte(pom))

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range srcs {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	r.put(base+".jar", buf.Bytes())
}

func lockGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// restoreWritable puts the write bit back on the cache's 0555 immutable trees
// so the test harness can remove the temp dir.
func restoreWritable(dir string) {
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
}

func writeLockFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLockWrittenEvenWhenGoRequireMergeFails pins the ORDERING: the lock is
// written as soon as the graph resolves, not at the end of a successful
// resolveDeps. A go-require conflict (ADR 0052 decision 4) aborts after the
// artifacts are already fetched and pinned; leaving the cache populated with
// no manifest is exactly the "nothing is pinned" failure this defect is about.
func TestLockWrittenEvenWhenGoRequireMergeFails(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CLJGO_CACHE", cache)
	t.Cleanup(func() { restoreWritable(cache) })

	repo := newLockMvnRepo(t)
	repo.publish(t, "fixt", "fixt", "1.0.0", map[string]string{
		"fixt/core.clj": "(ns fixt.core)\n",
	})

	proj := t.TempDir()
	writeLockFile(t, filepath.Join(proj, "src", "p.clj"), "(ns p)\n")
	writeLockFile(t, filepath.Join(proj, BuildFileName), fmt.Sprintf(`(defn build [b]
  (mvn-repo b %q)
  (dep b "fixt/fixt" {:mvn/version "1.0.0"})
  (let [app (exe b {:name "probe" :main "src/p.clj"})]
    (go-require app "github.com/google/uuid" "v1.6.0")
    (go-require app "github.com/google/uuid" "v1.5.0")
    (install b app)))
`, repo.srv.URL))

	plan, err := LoadPlan(filepath.Join(proj, BuildFileName))
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	if _, err := plan.resolveDeps(); err == nil {
		t.Fatal("expected a go-require version conflict")
	}
	if _, err := os.Stat(filepath.Join(proj, "build.lock.edn")); err != nil {
		t.Fatalf("the graph resolved (artifacts are cached) but no build.lock.edn was written: %v", err)
	}
}

// TestProjectResolveWritesThenReusesLock is the regression test for the
// reported defect: after a project-mode resolve, build.lock.edn must exist
// beside build.cljgo, carrying the integrity fields; a second resolve must
// reuse it without touching the repository again.
func TestProjectResolveWritesThenReusesLock(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("CLJGO_CACHE", cache)
	t.Cleanup(func() { restoreWritable(cache) })

	repo := newLockMvnRepo(t)
	repo.publish(t, "fixt", "fixt", "1.0.0", map[string]string{
		"fixt/core.clj": "(ns fixt.core)\n(defn f [] 1)\n",
	})

	// A git dep and a path dep alongside the Maven one — ADR 0052 must not
	// regress: three kinds, one lock.
	gitDir := t.TempDir()
	lockGit(t, gitDir, "init", "-q", "-b", "main")
	writeLockFile(t, filepath.Join(gitDir, "cljgo.manifest.edn"), `{:paths ["src"]}`)
	writeLockFile(t, filepath.Join(gitDir, "src", "g.clj"), "(ns g)\n")
	lockGit(t, gitDir, "add", "-A")
	lockGit(t, gitDir, "commit", "-q", "-m", "init")
	gitSHA := lockGit(t, gitDir, "rev-parse", "HEAD")

	localDir := t.TempDir()
	writeLockFile(t, filepath.Join(localDir, "cljgo.manifest.edn"), `{:paths ["src"]}`)
	writeLockFile(t, filepath.Join(localDir, "src", "l.clj"), "(ns l)\n")

	proj := t.TempDir()
	writeLockFile(t, filepath.Join(proj, "src", "p.clj"), "(ns p)\n(defn -main [& _] (println 1))\n")
	writeLockFile(t, filepath.Join(proj, BuildFileName), fmt.Sprintf(`(defn build [b]
  (mvn-repo b %q)
  (dep b "fixt/fixt" {:mvn/version "1.0.0"})
  (dep b "g" {:git %q :ref "main"})
  (dep b "l" {:path %q})
  (let [app (exe b {:name "probe" :main "src/p.clj"})]
    (install b app)))
`, repo.srv.URL, "file://"+gitDir, localDir))

	plan, err := LoadPlan(filepath.Join(proj, BuildFileName))
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	if _, err := plan.resolveDeps(); err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	lockPath := filepath.Join(proj, "build.lock.edn")
	first, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("first resolve did not write build.lock.edn beside build.cljgo: %v", err)
	}
	got := string(first)
	for _, want := range []string{
		`:name "fixt/fixt"`, `:mvn/group "fixt"`, `:mvn/artifact "fixt"`,
		`:mvn/version "1.0.0"`, `:mvn/repo "` + repo.srv.URL + `"`,
		":mvn/sha256 \"sha256:", ":mvn/pom-sha256 \"sha256:", ":tree/hash \"sha256:",
		`:mvn/namespaces {:pure ["fixt.core"]}`,
		`:name "g"`, `:git/sha "` + gitSHA + `"`,
		`:name "l"`, `:local/unlocked? true`,
		":paths", ":lock/version 1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("build.lock.edn missing %q\n--- lock ---\n%s", want, got)
		}
	}

	// Second resolve: the lock is load-bearing — no repository traffic, and
	// the file is unchanged.
	hits := repo.hits
	if hits == 0 {
		t.Fatal("first resolve fetched nothing from the repository double")
	}
	plan2, err := LoadPlan(filepath.Join(proj, BuildFileName))
	if err != nil {
		t.Fatalf("LoadPlan (2): %v", err)
	}
	if _, err := plan2.resolveDeps(); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if repo.hits != hits {
		t.Errorf("second resolve re-fetched from the repository: %d hits before, %d after", hits, repo.hits)
	}
	second, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("lock changed on the second resolve:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}
