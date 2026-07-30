// cache_compiled_test.go — proves cljg.cache (ADR 0093) LINKS and works in an
// AOT-compiled binary. The behavior (singleflight, expiry, the Cache interface) is
// covered interpreted in pkg/bri/cache_test.go; here a compiled app exercises the
// pure fetch/put path plus a user `Cache` reify, proving the whole namespace —
// protocol, reify impl, and cljg.os/now clock — links + boots, CGO_ENABLED=0.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

const cacheApp = `(require '[cljg.cache :as cache])
(defn -main [& _]
  (let [c (cache/local {:ttl 60})]
    (cache/put c :a 1)
    ;; a user backend implementing the same protocol works via the same fns
    (let [hits (atom 0)
          u    (reify cache/Cache
                 (-fetch [_ k f] (swap! hits inc) (f))
                 (-put [_ k v] v) (-evict [_ k] nil) (-clear [_] nil))]
      (println "cache" (cache/fetch c :a (fn [] :nope))
               (cache/fetch c :b (fn [] 2))
               "user" (cache/fetch u :z (fn [] 42)) @hits))))
`

func TestBriCoreCacheCompiled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "cacheapp.clj")
	if err := os.WriteFile(src, []byte(cacheApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "cacheapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (cljg.cache app): %v\n%s", err, out)
	}
	out, err := exec.Command(app).CombinedOutput()
	if err != nil {
		t.Fatalf("running the compiled cljg.cache binary: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); !strings.Contains(got, "cache 1 2 user 42 1") {
		t.Fatalf("compiled cljg.cache output =\n%q\nwant it to contain %q", got, "cache 1 2 user 42 1")
	}
}
