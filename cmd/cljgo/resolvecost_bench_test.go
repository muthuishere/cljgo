package main

// Spike s72 harness — what does hoisting project resolution into run()
// COST every subcommand? Benchmarks resolveRunDeps, the production function
// resolveProjectForCommand calls, across four project shapes and four sizes.
//
// Not a correctness test: nothing here asserts. See
// spikes/s72-project-resolution-cost/RESULTS.md for the numbers and verdict.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/build"
	"github.com/muthuishere/cljgo/pkg/deps"
	"github.com/muthuishere/cljgo/pkg/eval"
)

// mkLib writes a minimal local library: one source root, one namespace.
func mkLib(tb testing.TB, root, name string) {
	tb.Helper()
	dir := filepath.Join(root, name)
	src := filepath.Join(dir, "src", name)
	if err := os.MkdirAll(src, 0o755); err != nil {
		tb.Fatal(err)
	}
	body := fmt.Sprintf("(ns %s.core)\n(defn hello [] \"hi from %s\")\n", name, name)
	if err := os.WriteFile(filepath.Join(src, "core.cljg"), []byte(body), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// mkProject writes an app project declaring n local :path dependencies (and
// creates those libraries as siblings). n == 0 writes a dep-free build file.
// Returns the project directory.
func mkProject(tb testing.TB, root string, n int) string {
	tb.Helper()
	app := filepath.Join(root, "app")
	if err := os.MkdirAll(filepath.Join(app, "src", "app"), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "src", "app", "core.cljg"),
		[]byte("(ns app.core)\n(defn -main [] (println \"hi\"))\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("(defn build [b]\n")
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("lib%03d", i)
		mkLib(tb, root, name)
		fmt.Fprintf(&b, "  (dep b %q {:path \"../%s\"})\n", name, name)
	}
	b.WriteString("  (exe b {:name \"app\" :main \"src/app/core.cljg\"}))\n")
	if err := os.WriteFile(filepath.Join(app, "build.cljgo"), []byte(b.String()), 0o644); err != nil {
		tb.Fatal(err)
	}
	return app
}

// benchResolve runs resolveRunDeps("") from inside dir — exactly what
// resolveProjectForCommand does for a subcommand with no script anchor.
func benchResolve(b *testing.B, dir string) {
	b.Helper()
	b.Chdir(dir)
	saved := deps.ResolvedRoots()
	b.Cleanup(func() { deps.SetResolvedRoots(saved) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolveRunDeps("")
	}
}

// --- shape 1: no build file anywhere (the floor) ----------------------------

func BenchmarkResolveNoBuildFile(b *testing.B) {
	benchResolve(b, b.TempDir())
}

// --- shape 2: build.cljgo, no deps, no lock ---------------------------------
//
// Still pays build.LoadPlan, which boots a fresh interpreter.

func BenchmarkResolveBuildFileNoDeps(b *testing.B) {
	root := b.TempDir()
	benchResolve(b, mkProject(b, root, 0))
}

// --- shape 3: build.cljgo declaring N deps, NO lock (the #168 path) ---------
//
// Returns build.ErrNoLock. Costs one LoadPlan for the source roots and a
// second inside the no-lock branch.

func BenchmarkResolveDepsNoLock(b *testing.B) {
	for _, n := range []int{1, 5, 20, 50} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root := b.TempDir()
			benchResolve(b, mkProject(b, root, n))
		})
	}
}

// --- shape 4: build.cljgo + build.lock.edn with N locked deps ---------------
//
// The lock is produced by a real first resolve, so the benchmarked loop is
// the warm steady state every later invocation pays.

func BenchmarkResolveLocked(b *testing.B) {
	for _, n := range []int{1, 5, 20, 50, 200} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root := b.TempDir()
			app := mkProject(b, root, n)
			cwd, err := os.Getwd()
			if err != nil {
				b.Fatal(err)
			}
			if err := os.Chdir(app); err != nil {
				b.Fatal(err)
			}
			if err := build.ResolveProjectDeps(filepath.Join(app, "build.cljgo")); err != nil {
				b.Fatal(err)
			}
			if err := os.Chdir(cwd); err != nil {
				b.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(app, "build.lock.edn")); err != nil {
				b.Fatal("no lock written: ", err)
			}
			benchResolve(b, app)
		})
	}
}

// --- the pieces, so the total can be attributed ----------------------------

func BenchmarkFindBuildFileMiss(b *testing.B) {
	dir := b.TempDir()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = build.FindBuildFile(dir)
	}
}

// BenchmarkEvalBoot isolates the fresh-interpreter boot LoadPlan pays. It is
// the term everything else in this file is dominated by.
func BenchmarkEvalBoot(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eval.New()
	}
}

func BenchmarkLoadPlan(b *testing.B) {
	for _, n := range []int{0, 1, 5, 20, 50, 200} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root := b.TempDir()
			app := mkProject(b, root, n)
			bf := filepath.Join(app, "build.cljgo")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := build.LoadPlan(bf); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
