package main

// Spike s79 harness — what would it COST to read a project description from
// Clojure's `deps.edn` instead of cljgo's `build.cljgo`? Benchmarks the
// production path (resolveRunDeps, which calls deps.DepsEDNPaths) against the
// build.cljgo shapes s72 measured, at four dependency counts.
//
// The structural claim under test: build.cljgo is EVALUATED by a fresh
// tree-walking interpreter (39 ms boot, s72 §3), while deps.edn is only PARSED
// as data. If that holds, the deps.edn path is not merely faster, it is in a
// different cost class — and the growth term is the EDN parse, which the
// whole-file read pays even when only `:paths` is consumed.
//
// Not a correctness test: nothing here asserts. See
// spikes/s79-deps-edn-deps-translation/RESULTS.md.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/deps"
)

// mkDepsEDNProject writes an app project described ONLY by a deps.edn
// declaring n `:mvn/version` dependencies — the shape a pure .cljc Clojure
// library on Clojars actually has. No build.cljgo, so resolveRunDeps takes the
// ADR 0119 branch.
func mkDepsEDNProject(tb testing.TB, root string, n int) string {
	tb.Helper()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(filepath.Join(app, "src", "app"), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "src", "app", "core.cljc"),
		[]byte("(ns app.core)\n(defn -main [] (println \"hi\"))\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("{:paths [\"src\"]\n :deps {org.clojure/clojure {:mvn/version \"1.12.5\"}\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "        net.clojars.example/lib%03d {:mvn/version \"1.%d.0\"}\n", i, i)
	}
	b.WriteString("       }}\n")
	if err := os.WriteFile(filepath.Join(app, "deps.edn"), []byte(b.String()), 0o644); err != nil {
		tb.Fatal(err)
	}
	return app
}

// BenchmarkResolveDepsEDN runs resolveRunDeps("") from inside a deps.edn-only
// project — the production ADR 0119 path, same entry point s72 benchmarked.
func BenchmarkResolveDepsEDN(b *testing.B) {
	for _, n := range []int{0, 1, 5, 20} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root := b.TempDir()
			benchResolve(b, mkDepsEDNProject(b, root, n))
		})
	}
}

// BenchmarkDepsEDNPaths isolates read+EDN-parse+stat, with no resolveRunDeps
// scaffolding, so the growth term is attributable to the parse alone.
func BenchmarkDepsEDNPaths(b *testing.B) {
	for _, n := range []int{0, 1, 5, 20, 50, 200} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root := b.TempDir()
			app := mkDepsEDNProject(b, root, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if r := deps.DepsEDNPaths(app); len(r) == 0 {
					b.Fatal("no roots")
				}
			}
		})
	}
}
